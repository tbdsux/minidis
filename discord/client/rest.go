package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tbdsux/minidis/discord"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRateLimitRetries = 5
	maxServerRetries    = 3
)

var serverRetryDelay = 500 * time.Millisecond

// RESTError is a non-2xx response from the Discord API.
type RESTError struct {
	Method     string
	Path       string // with any interaction or webhook token removed
	StatusCode int
	Code       int             // Discord's JSON error code, 0 if none
	Message    string          // Discord's error message
	Errors     json.RawMessage // per-field validation errors, if any
	Body       []byte          // the raw response body
}

func (e *RESTError) Error() string {
	// Some messages already start with the status, as in "401: Unauthorized".
	msg := strings.TrimPrefix(e.Message, strconv.Itoa(e.StatusCode)+": ")
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}

	out := fmt.Sprintf("discord: %s %s: %d %s", e.Method, e.Path, e.StatusCode, msg)
	if e.Code != 0 {
		out += fmt.Sprintf(" (code %d)", e.Code)
	}
	if len(e.Errors) > 0 {
		out += ": " + string(e.Errors)
	}

	return out
}

type auditLogReasonKey struct{}

// WithAuditLogReason returns a context whose requests record reason in the
// guild's audit log, for actions such as bans, kicks, role changes and message
// deletions:
//
//	err := s.GuildBanCreate(client.WithAuditLogReason(ctx, "spam"), guildID, userID, 0)
func WithAuditLogReason(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, auditLogReasonKey{}, reason)
}

// invalidRequests counts the responses that Discord counts against a bot's
// IP: 401, 403, and 429 other than for shared limits. 10,000 of them in 10
// minutes get the IP banned for a while, so passing half and nine tenths of
// that is logged.
type invalidRequests struct {
	mu     sync.Mutex
	start  time.Time
	count  int
	warned int // the highest share already logged, in tenths
}

const (
	invalidRequestWindow = 10 * time.Minute
	invalidRequestLimit  = 10000
)

func (r *invalidRequests) record(status int, h http.Header) {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
	case status == http.StatusTooManyRequests && h.Get("X-RateLimit-Scope") != "shared":
	default:
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if now := time.Now(); now.Sub(r.start) > invalidRequestWindow {
		r.start, r.count, r.warned = now, 0, 0
	}
	r.count++

	for _, tenths := range []int{9, 5} {
		if r.warned < tenths && r.count >= invalidRequestLimit*tenths/10 {
			r.warned = tenths
			log.Printf("discord: %d invalid requests (401, 403 or 429) in the last %s; at %d Discord bans the IP for a while",
				r.count, invalidRequestWindow, invalidRequestLimit)
			break
		}
	}
}

// Request calls any Discord API endpoint, for example
// Request(ctx, "GET", "/guilds/123/channels", nil, &channels).
//
// path is relative to the versioned base URL. body and out are JSON encoded and
// decoded, and either may be nil. Rate limits are handled the same way as for
// the typed methods.
func (s *Session) Request(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	contentType := ""

	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("discord: encode request body: %w", err)
		}
		contentType = "application/json"
	}

	return s.send(ctx, method, path, contentType, payload, out)
}

// requestWithFiles sends a JSON request, as multipart/form-data when there are files.
func (s *Session) requestWithFiles(ctx context.Context, method, path string, body any, files []*discord.File, out any) error {
	if len(files) == 0 {
		return s.Request(ctx, method, path, body, out)
	}

	payload, contentType, err := encodeMultipart(body, files)
	if err != nil {
		return err
	}

	return s.send(ctx, method, path, contentType, payload, out)
}

var multipartEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", "", "\n", "")

// headerEscaper keeps a header value from ending its line.
var headerEscaper = strings.NewReplacer("\r", "", "\n", "")

func encodeMultipart(body any, files []*discord.File) ([]byte, string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("discord: encode request body: %w", err)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="payload_json"`)
	header.Set("Content-Type", "application/json")

	part, err := w.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if _, err = part.Write(payload); err != nil {
		return nil, "", err
	}

	for i, f := range files {
		if err := writeFile(w, fmt.Sprintf("files[%d]", i), f); err != nil {
			return nil, "", err
		}
	}

	if err = w.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), w.FormDataContentType(), nil
}

// writeFile adds f to w as the form field name.
func writeFile(w *multipart.Writer, name string, f *discord.File) error {
	if f == nil || f.Reader == nil {
		return fmt.Errorf("discord: file %s has no Reader", name)
	}

	contentType := f.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`, name, multipartEscaper.Replace(f.Name)))
	header.Set("Content-Type", headerEscaper.Replace(contentType))

	part, err := w.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, f.Reader); err != nil {
		return fmt.Errorf("discord: read file %q: %w", f.Name, err)
	}
	return nil
}

func (s *Session) send(ctx context.Context, method, path, contentType string, payload []byte, out any) error {
	route, major := routeKey(method, path)
	safePath := redactPath(path)

	// Interaction callbacks are not bound by the global limit and each one is
	// sent once, so they must not queue behind each other. Webhook routes with
	// a token, such as interaction followups, are not bound by it either.
	limited := !strings.HasPrefix(path, "/interactions/")
	tokenRoute := !limited || isWebhookTokenPath(path)

	// A rejected bot token stays rejected, and every further request would
	// count against Discord's limit on invalid requests.
	if !tokenRoute {
		if r := s.rejected.Load(); r != nil && r.token == s.Token {
			return &RESTError{Method: method, Path: safePath, StatusCode: http.StatusUnauthorized, Code: r.code,
				Message: "the token was rejected by an earlier request"}
		}
	}

	rateLimited, serverErrors := 0, 0
	for {
		status, header, data, err := s.attempt(ctx, limited, !tokenRoute, route, major, method, path, contentType, payload)
		if err != nil {
			if ctx.Err() != nil {
				err = context.Cause(ctx)
			}
			return fmt.Errorf("discord: %s %s: %w", method, safePath, err)
		}

		s.invalid.record(status, header)

		switch {
		case status >= 200 && status < 300:
			if out == nil || len(data) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("discord: %s %s: decode response: %w", method, safePath, err)
			}
			return nil

		case status == http.StatusTooManyRequests && rateLimited < maxRateLimitRetries:
			rateLimited++

			wait, global := parseRateLimit(header, data)
			if global {
				s.limiter.setGlobal(wait)
			}
			if err := sleep(ctx, wait); err != nil {
				return err
			}

		// A POST or PATCH may have been carried out before the gateway in front
		// of Discord failed, so only requests that are safe to repeat are.
		case (status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout) &&
			idempotent(method) && serverErrors < maxServerRetries:
			serverErrors++
			if err := sleep(ctx, serverRetryDelay<<(serverErrors-1)); err != nil {
				return err
			}

		default:
			e := newRESTError(method, safePath, status, data)
			if status == http.StatusUnauthorized && !tokenRoute {
				s.rejected.Store(&rejection{token: s.Token, code: e.Code})
			}
			return e
		}
	}
}

// rejection records a bot token Discord answered with 401 Unauthorized.
type rejection struct {
	token string
	code  int
}

// isWebhookTokenPath reports whether path is a webhook route that carries its
// token, which authenticates it instead of the bot token.
func isWebhookTokenPath(path string) bool {
	path, _, _ = strings.Cut(path, "?")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(segments) >= 3 && segments[0] == "webhooks"
}

// attempt sends one request, holding the route's rate limit bucket if limited.
// The bucket is released even if the round trip panics.
func (s *Session) attempt(ctx context.Context, limited, global bool, route, major, method, path, contentType string, payload []byte) (status int, header http.Header, data []byte, err error) {
	if limited {
		b, err := s.limiter.acquire(ctx, route, global)
		if err != nil {
			return 0, nil, nil, err
		}
		defer func() { s.limiter.release(b, route, major, header) }()
	}

	return s.roundTrip(ctx, method, path, contentType, payload)
}

// idempotent reports whether a request with method can be sent again safely.
// DELETE is left out: repeating one that went through gets a misleading 404.
func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodOptions:
		return true
	}
	return false
}

func (s *Session) roundTrip(ctx context.Context, method, path, contentType string, payload []byte) (int, http.Header, []byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.BaseURL+path, body)
	if err != nil {
		return 0, nil, nil, unwrapURLError(err)
	}

	req.Header.Set("Authorization", s.Token)
	req.Header.Set("User-Agent", s.UserAgent)
	if reason, ok := ctx.Value(auditLogReasonKey{}).(string); ok && reason != "" {
		req.Header.Set("X-Audit-Log-Reason", url.PathEscape(reason))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := s.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, nil, unwrapURLError(err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, nil, nil, err
	}

	return res.StatusCode, res.Header, data, nil
}

// unwrapURLError drops the URL from net/http errors, since it can contain an interaction token.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func parseRateLimit(h http.Header, data []byte) (time.Duration, bool) {
	var body struct {
		RetryAfter float64 `json:"retry_after"`
		Global     bool    `json:"global"`
	}
	notJSON := json.Unmarshal(data, &body) != nil

	// A 429 from Cloudflare rather than Discord, which bans the bot's IP when
	// it goes on, has no JSON body and no X-RateLimit-Scope. It is treated as
	// global, so that no other request goes out until it passes.
	global := body.Global || h.Get("X-RateLimit-Global") == "true" ||
		notJSON || h.Get("X-RateLimit-Scope") == ""

	if body.RetryAfter > 0 {
		return seconds(body.RetryAfter), global
	}
	if v, err := strconv.ParseFloat(h.Get("Retry-After"), 64); err == nil && v > 0 {
		return seconds(v), global
	}

	return time.Second, global
}

func newRESTError(method, path string, status int, data []byte) *RESTError {
	e := &RESTError{Method: method, Path: path, StatusCode: status, Body: data}

	var body struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(data, &body) == nil {
		e.Code, e.Message, e.Errors = body.Code, body.Message, body.Errors
	}

	return e
}

// esc escapes a path parameter, so that an ID or token taken from user input,
// such as "1/../../channels/2" or "1?x=y", stays one segment of the path.
func esc(param string) string { return url.PathEscape(param) }

func commandsPath(appID, guildID string) string {
	if guildID == "" {
		return "/applications/" + esc(appID) + "/commands"
	}
	return "/applications/" + esc(appID) + "/guilds/" + esc(guildID) + "/commands"
}

// ApplicationCommands lists an app's commands. An empty guildID means global commands.
func (s *Session) ApplicationCommands(ctx context.Context, appID, guildID string) ([]*discord.ApplicationCommand, error) {
	var out []*discord.ApplicationCommand
	err := s.Request(ctx, http.MethodGet, commandsPath(appID, guildID), nil, &out)
	return out, err
}

// ApplicationCommand fetches one command. An empty guildID means a global command.
func (s *Session) ApplicationCommand(ctx context.Context, appID, guildID, commandID string) (*discord.ApplicationCommand, error) {
	out := &discord.ApplicationCommand{}
	if err := s.Request(ctx, http.MethodGet, commandsPath(appID, guildID)+"/"+esc(commandID), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApplicationCommandCreate creates a command, or replaces the one with the same
// name and type. An empty guildID means a global command.
func (s *Session) ApplicationCommandCreate(ctx context.Context, appID, guildID string, cmd *discord.ApplicationCommand) (*discord.ApplicationCommand, error) {
	out := &discord.ApplicationCommand{}
	if err := s.Request(ctx, http.MethodPost, commandsPath(appID, guildID), cmd, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApplicationCommandEdit changes a command; fields left empty in cmd are kept.
// An empty guildID means a global command.
func (s *Session) ApplicationCommandEdit(ctx context.Context, appID, guildID, commandID string, cmd *discord.ApplicationCommand) (*discord.ApplicationCommand, error) {
	out := &discord.ApplicationCommand{}
	if err := s.Request(ctx, http.MethodPatch, commandsPath(appID, guildID)+"/"+esc(commandID), cmd, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuildApplicationCommandsPermissions lists the permission overwrites of an
// app's commands in a guild. Editing them needs a user's Bearer token with the
// applications.commands.permissions.update scope, which a bot token cannot
// provide, so it has no typed method; use Request with such a Session.
func (s *Session) GuildApplicationCommandsPermissions(ctx context.Context, appID, guildID string) ([]*discord.GuildApplicationCommandPermissions, error) {
	if guildID == "" {
		return nil, errNoPermissionGuild
	}

	var out []*discord.GuildApplicationCommandPermissions
	err := s.Request(ctx, http.MethodGet, commandsPath(appID, guildID)+"/permissions", nil, &out)
	return out, err
}

// ApplicationCommandPermissions returns the permission overwrites of one
// command in a guild. See GuildApplicationCommandsPermissions about editing.
func (s *Session) ApplicationCommandPermissions(ctx context.Context, appID, guildID, commandID string) (*discord.GuildApplicationCommandPermissions, error) {
	if guildID == "" {
		return nil, errNoPermissionGuild
	}

	out := &discord.GuildApplicationCommandPermissions{}
	if err := s.Request(ctx, http.MethodGet, commandsPath(appID, guildID)+"/"+esc(commandID)+"/permissions", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// errNoPermissionGuild is returned for permissions without a guild: they only
// exist per guild, and the global path would name a command "permissions".
var errNoPermissionGuild = errors.New("discord: command permissions need a guild ID")

// ApplicationCommandDelete deletes a command. An empty guildID means a global command.
func (s *Session) ApplicationCommandDelete(ctx context.Context, appID, guildID, commandID string) error {
	return s.Request(ctx, http.MethodDelete, commandsPath(appID, guildID)+"/"+esc(commandID), nil, nil)
}

// ApplicationCommandBulkOverwrite replaces all of an app's commands in one call.
// An empty guildID means global commands.
func (s *Session) ApplicationCommandBulkOverwrite(ctx context.Context, appID, guildID string, commands []*discord.ApplicationCommand) ([]*discord.ApplicationCommand, error) {
	if commands == nil {
		commands = []*discord.ApplicationCommand{}
	}

	var out []*discord.ApplicationCommand
	err := s.Request(ctx, http.MethodPut, commandsPath(appID, guildID), commands, &out)
	return out, err
}

// InteractionRespond sends the initial response to an interaction.
func (s *Session) InteractionRespond(ctx context.Context, i *discord.Interaction, res *discord.InteractionResponse) error {
	var files []*discord.File
	if res.Data != nil {
		files = res.Data.Files
	}

	return s.requestWithFiles(ctx, http.MethodPost, callbackPath(i), res, files, nil)
}

// InteractionRespondWithResponse is InteractionRespond, returning what the
// response created, such as the message sent.
func (s *Session) InteractionRespondWithResponse(ctx context.Context, i *discord.Interaction, res *discord.InteractionResponse) (*discord.InteractionCallbackResponse, error) {
	var files []*discord.File
	if res.Data != nil {
		files = res.Data.Files
	}

	out := &discord.InteractionCallbackResponse{}
	if err := s.requestWithFiles(ctx, http.MethodPost, callbackPath(i)+"?with_response=true", res, files, out); err != nil {
		return nil, err
	}
	return out, nil
}

func callbackPath(i *discord.Interaction) string {
	return "/interactions/" + esc(i.ID) + "/" + esc(i.Token) + "/callback"
}

// InteractionResponse fetches the original interaction response.
func (s *Session) InteractionResponse(ctx context.Context, i *discord.Interaction) (*discord.Message, error) {
	return s.FollowupMessage(ctx, i, "@original")
}

// FollowupMessage fetches a followup message.
func (s *Session) FollowupMessage(ctx context.Context, i *discord.Interaction, messageID string) (*discord.Message, error) {
	out := &discord.Message{}
	if err := s.Request(ctx, http.MethodGet, webhookMessagePath(i, messageID), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

func webhookMessagePath(i *discord.Interaction, messageID string) string {
	return "/webhooks/" + esc(i.AppID) + "/" + esc(i.Token) + "/messages/" + esc(messageID)
}

// InteractionResponseEdit edits the original interaction response.
func (s *Session) InteractionResponseEdit(ctx context.Context, i *discord.Interaction, edit *discord.WebhookEdit) (*discord.Message, error) {
	return s.FollowupMessageEdit(ctx, i, "@original", edit)
}

// InteractionResponseDelete deletes the original interaction response.
func (s *Session) InteractionResponseDelete(ctx context.Context, i *discord.Interaction) error {
	return s.FollowupMessageDelete(ctx, i, "@original")
}

// FollowupMessageCreate sends a followup message to an interaction.
func (s *Session) FollowupMessageCreate(ctx context.Context, i *discord.Interaction, params *discord.WebhookParams) (*discord.Message, error) {
	out := &discord.Message{}
	path := "/webhooks/" + esc(i.AppID) + "/" + esc(i.Token)
	if err := s.requestWithFiles(ctx, http.MethodPost, path, params, params.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// FollowupMessageEdit edits a followup message.
func (s *Session) FollowupMessageEdit(ctx context.Context, i *discord.Interaction, messageID string, edit *discord.WebhookEdit) (*discord.Message, error) {
	out := &discord.Message{}
	if err := s.requestWithFiles(ctx, http.MethodPatch, webhookMessagePath(i, messageID), edit, edit.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// FollowupMessageDelete deletes a followup message.
func (s *Session) FollowupMessageDelete(ctx context.Context, i *discord.Interaction, messageID string) error {
	return s.Request(ctx, http.MethodDelete, webhookMessagePath(i, messageID), nil, nil)
}

// ChannelMessageSendComplex sends a message to a channel.
func (s *Session) ChannelMessageSendComplex(ctx context.Context, channelID string, msg *discord.MessageSend) (*discord.Message, error) {
	out := &discord.Message{}
	path := "/channels/" + esc(channelID) + "/messages"
	if err := s.requestWithFiles(ctx, http.MethodPost, path, msg, msg.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ChannelMessageSend sends a text message to a channel.
func (s *Session) ChannelMessageSend(ctx context.Context, channelID, content string) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(ctx, channelID, &discord.MessageSend{Content: content})
}

// ChannelMessageSendReply sends a text message as a reply.
func (s *Session) ChannelMessageSendReply(ctx context.Context, channelID, content string, reference *discord.MessageReference) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(ctx, channelID, &discord.MessageSend{Content: content, Reference: reference})
}

// ChannelMessageSendEmbed sends an embed to a channel.
func (s *Session) ChannelMessageSendEmbed(ctx context.Context, channelID string, embed *discord.MessageEmbed) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(ctx, channelID, &discord.MessageSend{Embeds: []*discord.MessageEmbed{embed}})
}

// ChannelMessageSendEmbedReply sends an embed as a reply.
func (s *Session) ChannelMessageSendEmbedReply(ctx context.Context, channelID string, embed *discord.MessageEmbed, reference *discord.MessageReference) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(ctx, channelID, &discord.MessageSend{Embeds: []*discord.MessageEmbed{embed}, Reference: reference})
}
