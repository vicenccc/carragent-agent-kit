package carragent_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	carragent "carragent.com/go"
)

// fakeAccess is obviously fake: nothing here is, or looks like, a real credential.
const fakeAccess = "cat_fake-token-for-tests"

// recorded is one request the fake server received.
type recorded struct {
	Method string
	URI    string // the path and query, exactly as the client wrote them
	Header http.Header
	Body   []byte
}

// Query returns the parsed query of the request.
func (r recorded) Query(t *testing.T) url.Values {
	t.Helper()
	u, err := url.ParseRequestURI(r.URI)
	if err != nil {
		t.Fatalf("bad request URI %q: %v", r.URI, err)
	}
	return u.Query()
}

// Path returns the request URI without its query.
func (r recorded) Path() string {
	for i := 0; i < len(r.URI); i++ {
		if r.URI[i] == '?' {
			return r.URI[:i]
		}
	}
	return r.URI
}

// JSON decodes the request body.
func (r recorded) JSON(t *testing.T) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(r.Body, &v); err != nil {
		t.Fatalf("request body %q is not a JSON object: %v", r.Body, err)
	}
	return v
}

// Form decodes the request body as a form.
func (r recorded) Form(t *testing.T) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatalf("request body %q is not a form: %v", r.Body, err)
	}
	return v
}

// reply is what the fake server answers.
type reply struct {
	status      int
	body        any // a map or slice is sent as JSON, a string or []byte as it is
	header      map[string]string
	contentType string
	delay       time.Duration
	shortBy     int  // announce this many more bytes than are sent, then hang up
	hangUp      bool // close the connection without answering
}

func ok(body any) reply { return reply{status: 200, body: body} }

// problem is an RFC 9457 answer like the API's.
func problem(status int, code, title, detail string, header ...string) reply {
	body := map[string]any{"type": "about:blank", "status": status, "code": code, "title": title}
	if detail != "" {
		body["detail"] = detail
	}
	r := reply{status: status, body: body, contentType: "application/problem+json; charset=utf-8", header: map[string]string{}}
	for i := 0; i+1 < len(header); i += 2 {
		r.header[header[i]] = header[i+1]
	}
	return r
}

// fake is a server on this machine that answers the scripted replies in order, or whatever handler returns.
type fake struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
	replies  []reply
	handler  func(n int, r recorded) reply
}

func newFake(t *testing.T, replies ...reply) *fake {
	t.Helper()
	f := &fake{replies: replies}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func newFakeFunc(t *testing.T, handler func(n int, r recorded) reply) *fake {
	t.Helper()
	f := newFake(t)
	f.handler = handler
	return f
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rec := recorded{Method: r.Method, URI: r.RequestURI, Header: r.Header.Clone(), Body: body}
	f.mu.Lock()
	n := len(f.requests)
	f.requests = append(f.requests, rec)
	var answer reply
	switch {
	case f.handler != nil:
		answer = f.handler(n, rec)
	case len(f.replies) > 0:
		answer, f.replies = f.replies[0], f.replies[1:]
	default:
		answer = problem(500, "unscripted", "The test did not script this request", "")
	}
	f.mu.Unlock()

	if answer.delay > 0 {
		time.Sleep(answer.delay)
	}
	if answer.hangUp {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	payload, contentType := encode(answer)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)+answer.shortBy))
	for name, value := range answer.header {
		w.Header().Set(name, value)
	}
	w.WriteHeader(answer.status)
	// With shortBy the body is shorter than the Content-Length announced, and the server hangs up when the handler ends.
	_, _ = w.Write(payload)
}

func encode(r reply) ([]byte, string) {
	contentType := r.contentType
	switch body := r.body.(type) {
	case nil:
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		return nil, contentType
	case string:
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		return []byte(body), contentType
	case []byte:
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return body, contentType
	default:
		encoded, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		if contentType == "" {
			contentType = "application/json; charset=utf-8"
		}
		return encoded, contentType
	}
}

// Requests returns the requests received so far.
func (f *fake) Requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.requests...)
}

// client returns a client for the fake server.
func (f *fake) client() *carragent.Client {
	c := carragent.NewClient()
	c.BaseURL = f.URL
	return c
}

// A few answers shaped like the API's. The text is made up; the keys are the API's.
var (
	agentJSON = map[string]any{
		"id": "collections", "name": "Collections", "area": "money", "status": "live",
		"summary": "Follows up late invoices on a fixed schedule.", "runs": "Every morning.",
		"steps":         []string{"Reads open invoices.", "Writes the reminder."},
		"waitsForOwner": "Anything over your limit.", "never": "Never threatens.",
		"tools": []string{"Gmail", "Google Sheets"},
	}
	plansJSON = map[string]any{
		"object": "list",
		"data": []any{map[string]any{
			"id": "team", "name": "Team", "summary": "For a small team.", "forWho": "Teams of up to 10.",
			"recommended": true, "currency": "USD", "monthlyCents": 4900, "yearlyCents": 47000, "selfServe": true,
			"limits": map[string]any{"agents": 5, "connections": 10, "messagesPerMonth": nil, "seats": 5},
		}, map[string]any{
			"id": "custom", "name": "Custom", "summary": "Quoted.", "forWho": "Larger teams.",
			"recommended": false, "currency": "USD", "monthlyCents": 29900, "yearlyCents": nil, "selfServe": false,
			"limits": map[string]any{"agents": nil, "connections": nil, "messagesPerMonth": nil, "seats": nil},
		}},
		"terms":      map[string]any{"trialDays": 14, "annualRefundDays": 30, "setupFee": 0, "contract": false, "currency": "USD"},
		"nextCursor": nil, "hasMore": false,
	}
	searchJSON = map[string]any{
		"object": "list", "query": "late invoices", "nextCursor": nil, "hasMore": false,
		"data": []any{map[string]any{"kind": "agent", "id": "collections", "name": "Collections", "description": "Follows up late invoices.", "score": 2}},
	}
	tokenJSON = map[string]any{"access_token": fakeAccess, "token_type": "Bearer", "expires_in": 3600, "scope": "agents:read plans:read search:read"}
)

func agentPage(ids []string, next string) map[string]any {
	data := make([]any, 0, len(ids))
	for _, id := range ids {
		agent := map[string]any{}
		for k, v := range agentJSON {
			agent[k] = v
		}
		agent["id"], agent["name"] = id, id
		data = append(data, agent)
	}
	page := map[string]any{"object": "list", "data": data, "hasMore": next != ""}
	if next == "" {
		page["nextCursor"] = nil
	} else {
		page["nextCursor"] = next
	}
	return page
}
