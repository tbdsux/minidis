package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tbdsux/minidis/discord"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
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
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}

	out := fmt.Sprintf("discord: %s %s: %d %s (code %d)", e.Method, e.Path, e.StatusCode, msg, e.Code)
	if len(e.Errors) > 0 {
		out += ": " + string(e.Errors)
	}

	return out
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
		contentType := f.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition",
			fmt.Sprintf(`form-data; name="files[%d]"; filename="%s"`, i, multipartEscaper.Replace(f.Name)))
		header.Set("Content-Type", contentType)

		part, err := w.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err = io.Copy(part, f.Reader); err != nil {
			return nil, "", fmt.Errorf("discord: read file %q: %w", f.Name, err)
		}
	}

	if err = w.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), w.FormDataContentType(), nil
}

func (s *Session) send(ctx context.Context, method, path, contentType string, payload []byte, out any) error {
	route, major := routeKey(method, path)
	safePath := redactPath(path)

	// Interaction callbacks are not bound by the global limit and each one is
	// sent once, so they must not queue behind each other.
	limited := !strings.HasPrefix(path, "/interactions/")

	rateLimited, serverErrors := 0, 0
	for {
		var b *bucket
		var err error
		if limited {
			if b, err = s.limiter.acquire(ctx, route); err != nil {
				return err
			}
		}

		status, header, data, err := s.roundTrip(ctx, method, path, contentType, payload)
		if limited {
			s.limiter.release(b, route, major, header)
		}
		if err != nil {
			return fmt.Errorf("discord: %s %s: %w", method, safePath, err)
		}

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

		case (status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout) &&
			serverErrors < maxServerRetries:
			serverErrors++
			if err := sleep(ctx, serverRetryDelay<<(serverErrors-1)); err != nil {
				return err
			}

		default:
			return newRESTError(method, safePath, status, data)
		}
	}
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
	_ = json.Unmarshal(data, &body) // Cloudflare's 429s are not JSON

	global := body.Global || h.Get("X-RateLimit-Global") == "true"

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

func commandsPath(appID, guildID string) string {
	if guildID == "" {
		return "/applications/" + appID + "/commands"
	}
	return "/applications/" + appID + "/guilds/" + guildID + "/commands"
}

// ApplicationCommands lists an app's commands. An empty guildID means global commands.
func (s *Session) ApplicationCommands(appID, guildID string) ([]*discord.ApplicationCommand, error) {
	var out []*discord.ApplicationCommand
	err := s.Request(context.Background(), http.MethodGet, commandsPath(appID, guildID), nil, &out)
	return out, err
}

// ApplicationCommand fetches one command. An empty guildID means a global command.
func (s *Session) ApplicationCommand(appID, guildID, commandID string) (*discord.ApplicationCommand, error) {
	out := &discord.ApplicationCommand{}
	if err := s.Request(context.Background(), http.MethodGet, commandsPath(appID, guildID)+"/"+commandID, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApplicationCommandCreate creates a command, or replaces the one with the same
// name and type. An empty guildID means a global command.
func (s *Session) ApplicationCommandCreate(appID, guildID string, cmd *discord.ApplicationCommand) (*discord.ApplicationCommand, error) {
	out := &discord.ApplicationCommand{}
	if err := s.Request(context.Background(), http.MethodPost, commandsPath(appID, guildID), cmd, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApplicationCommandEdit changes a command; fields left empty in cmd are kept.
// An empty guildID means a global command.
func (s *Session) ApplicationCommandEdit(appID, guildID, commandID string, cmd *discord.ApplicationCommand) (*discord.ApplicationCommand, error) {
	out := &discord.ApplicationCommand{}
	if err := s.Request(context.Background(), http.MethodPatch, commandsPath(appID, guildID)+"/"+commandID, cmd, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuildApplicationCommandsPermissions lists the permission overwrites of an
// app's commands in a guild. Editing them needs a user's Bearer token with the
// applications.commands.permissions.update scope, which a bot token cannot
// provide, so it has no typed method; use Request with such a Session.
func (s *Session) GuildApplicationCommandsPermissions(appID, guildID string) ([]*discord.GuildApplicationCommandPermissions, error) {
	var out []*discord.GuildApplicationCommandPermissions
	err := s.Request(context.Background(), http.MethodGet, commandsPath(appID, guildID)+"/permissions", nil, &out)
	return out, err
}

// ApplicationCommandPermissions returns the permission overwrites of one
// command in a guild. See GuildApplicationCommandsPermissions about editing.
func (s *Session) ApplicationCommandPermissions(appID, guildID, commandID string) (*discord.GuildApplicationCommandPermissions, error) {
	out := &discord.GuildApplicationCommandPermissions{}
	if err := s.Request(context.Background(), http.MethodGet, commandsPath(appID, guildID)+"/"+commandID+"/permissions", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApplicationCommandDelete deletes a command. An empty guildID means a global command.
func (s *Session) ApplicationCommandDelete(appID, guildID, commandID string) error {
	return s.Request(context.Background(), http.MethodDelete, commandsPath(appID, guildID)+"/"+commandID, nil, nil)
}

// ApplicationCommandBulkOverwrite replaces all of an app's commands in one call.
// An empty guildID means global commands.
func (s *Session) ApplicationCommandBulkOverwrite(appID, guildID string, commands []*discord.ApplicationCommand) ([]*discord.ApplicationCommand, error) {
	if commands == nil {
		commands = []*discord.ApplicationCommand{}
	}

	var out []*discord.ApplicationCommand
	err := s.Request(context.Background(), http.MethodPut, commandsPath(appID, guildID), commands, &out)
	return out, err
}

// InteractionRespond sends the initial response to an interaction.
func (s *Session) InteractionRespond(i *discord.Interaction, res *discord.InteractionResponse) error {
	var files []*discord.File
	if res.Data != nil {
		files = res.Data.Files
	}

	return s.requestWithFiles(context.Background(), http.MethodPost, callbackPath(i), res, files, nil)
}

// InteractionRespondWithResponse is InteractionRespond, returning what the
// response created, such as the message sent.
func (s *Session) InteractionRespondWithResponse(i *discord.Interaction, res *discord.InteractionResponse) (*discord.InteractionCallbackResponse, error) {
	var files []*discord.File
	if res.Data != nil {
		files = res.Data.Files
	}

	out := &discord.InteractionCallbackResponse{}
	if err := s.requestWithFiles(context.Background(), http.MethodPost, callbackPath(i)+"?with_response=true", res, files, out); err != nil {
		return nil, err
	}
	return out, nil
}

func callbackPath(i *discord.Interaction) string {
	return "/interactions/" + i.ID + "/" + i.Token + "/callback"
}

// InteractionResponse fetches the original interaction response.
func (s *Session) InteractionResponse(i *discord.Interaction) (*discord.Message, error) {
	return s.FollowupMessage(i, "@original")
}

// FollowupMessage fetches a followup message.
func (s *Session) FollowupMessage(i *discord.Interaction, messageID string) (*discord.Message, error) {
	out := &discord.Message{}
	if err := s.Request(context.Background(), http.MethodGet, webhookMessagePath(i, messageID), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

func webhookMessagePath(i *discord.Interaction, messageID string) string {
	return "/webhooks/" + i.AppID + "/" + i.Token + "/messages/" + messageID
}

// InteractionResponseEdit edits the original interaction response.
func (s *Session) InteractionResponseEdit(i *discord.Interaction, edit *discord.WebhookEdit) (*discord.Message, error) {
	return s.FollowupMessageEdit(i, "@original", edit)
}

// InteractionResponseDelete deletes the original interaction response.
func (s *Session) InteractionResponseDelete(i *discord.Interaction) error {
	return s.FollowupMessageDelete(i, "@original")
}

// FollowupMessageCreate sends a followup message to an interaction.
func (s *Session) FollowupMessageCreate(i *discord.Interaction, params *discord.WebhookParams) (*discord.Message, error) {
	out := &discord.Message{}
	path := "/webhooks/" + i.AppID + "/" + i.Token
	if err := s.requestWithFiles(context.Background(), http.MethodPost, path, params, params.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// FollowupMessageEdit edits a followup message.
func (s *Session) FollowupMessageEdit(i *discord.Interaction, messageID string, edit *discord.WebhookEdit) (*discord.Message, error) {
	out := &discord.Message{}
	if err := s.requestWithFiles(context.Background(), http.MethodPatch, webhookMessagePath(i, messageID), edit, edit.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// FollowupMessageDelete deletes a followup message.
func (s *Session) FollowupMessageDelete(i *discord.Interaction, messageID string) error {
	return s.Request(context.Background(), http.MethodDelete, webhookMessagePath(i, messageID), nil, nil)
}

// ChannelMessageSendComplex sends a message to a channel.
func (s *Session) ChannelMessageSendComplex(channelID string, msg *discord.MessageSend) (*discord.Message, error) {
	out := &discord.Message{}
	path := "/channels/" + channelID + "/messages"
	if err := s.requestWithFiles(context.Background(), http.MethodPost, path, msg, msg.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ChannelMessageSend sends a text message to a channel.
func (s *Session) ChannelMessageSend(channelID, content string) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(channelID, &discord.MessageSend{Content: content})
}

// ChannelMessageSendReply sends a text message as a reply.
func (s *Session) ChannelMessageSendReply(channelID, content string, reference *discord.MessageReference) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(channelID, &discord.MessageSend{Content: content, Reference: reference})
}

// ChannelMessageSendEmbed sends an embed to a channel.
func (s *Session) ChannelMessageSendEmbed(channelID string, embed *discord.MessageEmbed) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(channelID, &discord.MessageSend{Embeds: []*discord.MessageEmbed{embed}})
}

// ChannelMessageSendEmbedReply sends an embed as a reply.
func (s *Session) ChannelMessageSendEmbedReply(channelID string, embed *discord.MessageEmbed, reference *discord.MessageReference) (*discord.Message, error) {
	return s.ChannelMessageSendComplex(channelID, &discord.MessageSend{Embeds: []*discord.MessageEmbed{embed}, Reference: reference})
}
