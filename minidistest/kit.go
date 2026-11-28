// Package minidistest tests the handlers of a minidis bot without connecting
// it to Discord. A Kit answers the bot's REST calls in memory and records
// them, and feeds the bot interactions, messages and other gateway events,
// which run through the bot's middleware and error handlers as they would
// when sent by Discord:
//
//	bot := minidis.New("token")
//	bot.AddCommand(&minidis.Command{Name: "ping", Execute: func(c *minidis.Context) error {
//		return c.ReplyWith(minidis.Message{Content: "pong", Ephemeral: true})
//	}})
//
//	kit := minidistest.New(t, bot.Session)
//	res := kit.Interact(minidistest.Slash("ping"))
//	if len(res) != 1 || res[0].Content != "pong" || !res[0].Ephemeral {
//		t.Errorf("responses = %+v", res)
//	}
//
// It works the same with the bot of the interactions package, or with a
// client.Session alone.
package minidistest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// Kit is a fake Discord for one bot. Its methods are safe for concurrent use.
type Kit struct {
	t       testing.TB
	session *client.Session
	prefix  string // the path of Session.BaseURL, such as "/api/v10"

	mu       sync.Mutex
	requests []*Request
	answers  map[string]answer // by "METHOD /path"
}

type answer struct {
	status int
	body   []byte
}

// New makes s answer its REST calls in memory, by replacing its HTTPClient,
// and makes it ready, as connecting would: it dispatches a READY event, which
// sets s.User to the bot user, whose ID is AppID, and s.ApplicationID. Add
// the bot's handlers first, as the READY ones run then.
func New(t testing.TB, s *client.Session) *Kit {
	t.Helper()

	base, err := url.Parse(s.BaseURL)
	if err != nil {
		t.Fatalf("minidistest: invalid Session.BaseURL: %v", err)
	}

	k := &Kit{t: t, session: s, prefix: strings.TrimSuffix(base.Path, "/"), answers: map[string]answer{}}
	s.HTTPClient = &http.Client{Transport: k}

	k.dispatch("READY", &discord.Ready{
		Version:     client.APIVersion,
		User:        &discord.User{ID: AppID, Username: "bot", Bot: true},
		SessionID:   "session",
		Application: &discord.Application{ID: AppID},
	})
	return k
}

// Interact sends the bot an interaction, such as one made with Slash, and
// returns once its handlers have, with the responses they sent to it.
func (k *Kit) Interact(i *discord.Interaction) []*Response {
	k.t.Helper()

	type alias discord.Interaction
	data, err := json.Marshal(struct {
		*alias
		Data any `json:"data,omitempty"` // which Interaction leaves out
	}{(*alias)(i), i.Data})
	if err != nil {
		k.t.Fatalf("minidistest: encoding the interaction: %v", err)
	}

	k.session.Dispatch("INTERACTION_CREATE", data)
	return k.Responses(i)
}

// Dispatch sends the bot a gateway event, such as a discord.GuildMemberAdd
// or a discord.MessageCreate with a message made with Message, and returns
// once its handlers have. It panics if E is not an event type.
func Dispatch[E any](k *Kit, e *E) {
	k.t.Helper()

	if ic, ok := any(e).(*discord.InteractionCreate); ok {
		k.Interact(ic.Interaction)
		return
	}

	name := client.EventName[E]()
	if name == "" {
		panic(fmt.Sprintf("minidistest: %T is not a gateway event", e))
	}
	k.dispatch(name, e)
}

func (k *Kit) dispatch(event string, payload any) {
	k.t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		k.t.Fatalf("minidistest: encoding %s: %v", event, err)
	}
	k.session.Dispatch(event, data)
}

// Respond sets what the REST call with the given method and path, such as
// "GET" and "/guilds/2000/members/4000", is answered with: the status and
// body encoded as JSON, or no body if body is nil. Other calls are answered
// with 200 and their own body with an "id" added, which is enough for
// those that create something; GET /users/@me gets the bot user and DELETE
// gets 204.
func (k *Kit) Respond(method, path string, status int, body any) {
	k.t.Helper()

	var data []byte
	if body != nil {
		var err error
		if data, err = json.Marshal(body); err != nil {
			k.t.Fatalf("minidistest: encoding the answer to %s %s: %v", method, path, err)
		}
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	k.answers[method+" "+path] = answer{status, data}
}

// Requests returns the REST calls made so far, in order.
func (k *Kit) Requests() []*Request {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Clone(k.requests)
}

// Request is a REST call the bot made.
type Request struct {
	Method string
	Path   string // such as "/channels/3000/messages", without the API version and the query
	Query  url.Values

	// Body is the JSON body, or the payload_json of a request with files.
	Body json.RawMessage
}

// Decode decodes the body into v.
func (r *Request) Decode(v any) error {
	return json.Unmarshal(r.Body, v)
}

// RoundTrip implements http.RoundTripper, for Session.HTTPClient.
func (k *Kit) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := requestBody(r)
	if err != nil {
		return nil, err
	}

	req := &Request{
		Method: r.Method,
		Path:   strings.TrimPrefix(r.URL.Path, k.prefix),
		Query:  r.URL.Query(),
		Body:   body,
	}

	k.mu.Lock()
	id := fmt.Sprint(firstID + len(k.requests)) // IDs of what the calls create
	k.requests = append(k.requests, req)
	a, ok := k.answers[req.Method+" "+req.Path]
	k.mu.Unlock()

	if !ok {
		a = defaultAnswer(req, id)
	}

	res := &http.Response{
		StatusCode: a.status,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(a.body)),
		Request:    r,
	}
	if a.body != nil {
		res.Header.Set("Content-Type", "application/json")
	}
	return res, nil
}

// firstID is the ID of what the first REST call creates, well above those of
// the builders.
const firstID = 900000

func defaultAnswer(r *Request, id string) answer {
	switch {
	case r.Method == http.MethodDelete:
		return answer{http.StatusNoContent, nil}
	case r.Method == http.MethodGet && r.Path == "/users/@me":
		return answer{http.StatusOK, []byte(`{"id":"` + AppID + `","username":"bot","bot":true}`)}
	}

	// An object is echoed with an ID, such as a message that was sent; an
	// array, such as commands being overwritten, as it is.
	var fields map[string]any
	if json.Unmarshal(r.Body, &fields) != nil || fields == nil { // nil for a JSON null
		if bytes.HasPrefix(bytes.TrimSpace(r.Body), []byte("[")) {
			return answer{http.StatusOK, r.Body}
		}
		fields = map[string]any{}
	}
	fields["id"] = id
	data, _ := json.Marshal(fields)
	return answer{http.StatusOK, data}
}

// requestBody reads the JSON of a request, from its payload_json part if it
// has files.
func requestBody(r *http.Request) (json.RawMessage, error) {
	if r.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 {
		return nil, err
	}

	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		return data, nil
	}

	mr := multipart.NewReader(bytes.NewReader(data), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil, fmt.Errorf("minidistest: no payload_json in the request: %w", err)
		}
		if part.FormName() == "payload_json" {
			return io.ReadAll(part)
		}
	}
}

// ResponseKind is what a Response does.
type ResponseKind string

const (
	Callback ResponseKind = "callback" // the initial response, of the given Type
	Edit     ResponseKind = "edit"     // an edit of the original response or a followup
	Followup ResponseKind = "followup" // a followup message
	Delete   ResponseKind = "delete"   // a deletion of the original response or a followup
)

// Response is what a handler sent in answer to an interaction.
type Response struct {
	Kind ResponseKind

	// Type is the type of a Callback, such as
	// discord.InteractionResponseChannelMessageWithSource for a reply and
	// discord.InteractionResponseDeferredChannelMessageWithSource for a
	// deferral.
	Type discord.InteractionResponseType

	// MessageID is what an Edit or a Delete is of: "@original" or the ID of
	// a followup.
	MessageID string

	Content    string
	Flags      discord.MessageFlags
	Ephemeral  bool // Flags has discord.MessageFlagsEphemeral
	Embeds     []*discord.MessageEmbed
	Components discord.ComponentList
	Choices    []*discord.ApplicationCommandOptionChoice // of an autocomplete result

	CustomID string // of a modal
	Title    string // of a modal

	Request *Request
}

// responseBody holds the fields of the bodies a Response is read from.
type responseBody struct {
	Content    string                                    `json:"content"`
	Flags      discord.MessageFlags                      `json:"flags"`
	Embeds     []*discord.MessageEmbed                   `json:"embeds"`
	Components discord.ComponentList                     `json:"components"`
	Choices    []*discord.ApplicationCommandOptionChoice `json:"choices"`
	CustomID   string                                    `json:"custom_id"`
	Title      string                                    `json:"title"`
}

// Responses returns the responses sent to i so far, in order, such as those
// sent from a goroutine after Interact returned.
func (k *Kit) Responses(i *discord.Interaction) []*Response {
	k.t.Helper()

	var out []*Response
	for _, r := range k.Requests() {
		res := &Response{Request: r}
		var body responseBody
		raw := r.Body

		parts := strings.Split(strings.TrimPrefix(r.Path, "/"), "/")
		switch {
		case len(parts) == 4 && parts[0] == "interactions" && parts[2] == i.Token && parts[3] == "callback":
			var cb struct {
				Type discord.InteractionResponseType `json:"type"`
				Data json.RawMessage                 `json:"data"`
			}
			if err := r.Decode(&cb); err != nil {
				k.t.Fatalf("minidistest: decoding %s %s: %v", r.Method, r.Path, err)
			}
			res.Kind, res.Type, raw = Callback, cb.Type, cb.Data
		case len(parts) == 3 && parts[0] == "webhooks" && parts[2] == i.Token && r.Method == http.MethodPost:
			res.Kind = Followup
		case len(parts) == 5 && parts[0] == "webhooks" && parts[2] == i.Token && parts[3] == "messages" &&
			(r.Method == http.MethodPatch || r.Method == http.MethodDelete):
			res.Kind, res.MessageID = Edit, parts[4]
			if r.Method == http.MethodDelete {
				res.Kind = Delete
			}
		default:
			continue
		}

		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &body); err != nil {
				k.t.Fatalf("minidistest: decoding %s %s: %v", r.Method, r.Path, err)
			}
		}
		res.Content, res.Flags, res.Embeds, res.Components = body.Content, body.Flags, body.Embeds, body.Components
		res.Choices, res.CustomID, res.Title = body.Choices, body.CustomID, body.Title
		res.Ephemeral = body.Flags&discord.MessageFlagsEphemeral != 0
		out = append(out, res)
	}
	return out
}
