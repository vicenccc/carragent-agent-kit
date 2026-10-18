package carragent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is where the API is.
	DefaultBaseURL = "https://carragent.com"

	// Version is this library's version. The client names it in its User-Agent header.
	Version = "1.0.0"

	userAgent      = "carragent-go/" + Version
	defaultTimeout = 10 * time.Second

	// The API answers in a few kilobytes. A larger answer is not Carragent's, so it is refused instead of read.
	maxBody = 8 << 20
)

// Client reads Carragent's public, read-only API: its AI agents for small businesses, and its plans and prices.
//
// Everything it reads is public and needs no account or key. A token is optional: it only raises the rate limit (60
// requests a minute per IP address without one, 300 a minute per client with one) and reads the same public catalog.
// Nothing here creates an account, starts a trial or touches a business's data.
//
// The zero Client works, with the defaults NewClient sets. A Client keeps no connection of its own and never
// retries a request. It is safe for concurrent use as long as its fields are not changed meanwhile; UseAnonymousIdentity
// and UseClientCredentials change Token, so call them before sharing the client between goroutines.
type Client struct {
	// BaseURL is the API's address, without a path or with the prefix of a proxy. It must be https; http is
	// accepted only for a server on this machine. Empty means DefaultBaseURL.
	BaseURL string

	// HTTPClient sends the requests. Nil means a client with a 10 second timeout that does not follow redirects:
	// a redirect would send your token wherever it points, so it is returned as an error (code "unexpected_redirect").
	// If you set your own, you choose its timeout and its redirect policy.
	HTTPClient *http.Client

	// Token is an access token to send as Authorization: Bearer. Empty sends none. UseAnonymousIdentity and
	// UseClientCredentials get one and set it. A token lasts an hour; after that the API answers 401 invalid_token,
	// even though it is public, until you get a new token or clear this one.
	Token string

	// UserAgent is the User-Agent header. Empty means "carragent-go/" and the Version. To name your application,
	// start it with your own product token: "my-app/1.2 carragent-go/1.0.0".
	UserAgent string
}

// NewClient returns a Client with the defaults: DefaultBaseURL, a 10 second timeout and no token.
func NewClient() *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: defaultTimeout, CheckRedirect: noRedirect},
		UserAgent:  userAgent,
	}
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// String describes the client without its token.
func (c *Client) String() string {
	token := "none"
	if c.Token != "" {
		token = "***"
	}
	return fmt.Sprintf("carragent.Client{BaseURL: %q, Token: %s}", c.baseOrDefault(), token)
}

// GoString is String, so that %#v does not print the token either.
func (c *Client) GoString() string { return c.String() }

// ListAgentsParams are the optional parameters of ListAgents. The zero value asks for the first page in English.
type ListAgentsParams struct {
	// Lang is the language of the text. Empty means English.
	Lang Lang
	// Limit is the page size, 1 to 50. Zero means the API's default, 20.
	Limit int
	// Cursor is the NextCursor of the previous page. Empty means the first page.
	Cursor string
}

// ListAgents returns one page of the agents: GET /api/v1/agents. params may be nil.
func (c *Client) ListAgents(ctx context.Context, params *ListAgentsParams) (*AgentList, error) {
	query := url.Values{}
	if params != nil {
		setIf(query, "lang", string(params.Lang))
		if params.Limit != 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		setIf(query, "cursor", params.Cursor)
	}
	var list AgentList
	if err := c.get(ctx, "/api/v1/agents", query, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// GetAgent returns one agent by its id, such as "collections": GET /api/v1/agents/{id}. lang may be empty.
//
// The error is a *Problem with the code "agent_not_found" (404) when there is no agent with that id.
func (c *Client) GetAgent(ctx context.Context, id string, lang Lang) (*Agent, error) {
	segment, err := pathSegment(id, "agent id")
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	setIf(query, "lang", string(lang))
	var agent Agent
	if err := c.get(ctx, "/api/v1/agents/"+segment, query, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// ListPlans returns the plans, with their prices in US cents (before taxes), their limits and the trial and refund
// terms: GET /api/v1/plans. There is one page: every plan. lang may be empty.
func (c *Client) ListPlans(ctx context.Context, lang Lang) (*PlanList, error) {
	query := url.Values{}
	setIf(query, "lang", string(lang))
	var list PlanList
	if err := c.get(ctx, "/api/v1/plans", query, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// Search is a keyword search over the agents and the plans, best match first: GET /api/v1/search.
//
// It matches the words of q (those of three letters or more) against the text of the agents and plans in the language
// lang, ignoring case and accents. It is not a semantic search, so use the words the agents use, such as "late
// invoices" or "appointment reminders". It returns at most 10 matches. q is 1 to 200 characters; lang may be empty.
func (c *Client) Search(ctx context.Context, q string, lang Lang) (*SearchResults, error) {
	query := url.Values{"q": {q}}
	setIf(query, "lang", string(lang))
	var results SearchResults
	if err := c.get(ctx, "/api/v1/search", query, &results); err != nil {
		return nil, err
	}
	return &results, nil
}

// AgentIterator walks every agent, page by page. Make one with Client.IterAgents.
//
//	it := client.IterAgents(nil)
//	for it.Next(ctx) {
//		fmt.Println(it.Agent().Name)
//	}
//	if err := it.Err(); err != nil {
//		// ...
//	}
//
// It is not safe for concurrent use.
type AgentIterator struct {
	client *Client
	params ListAgentsParams
	page   []Agent
	agent  Agent
	err    error
	done   bool
}

// IterAgents returns an iterator over every agent. It follows NextCursor from page to page and asks for a page only
// when Next needs it. params may be nil; its Cursor is where to start, and its Limit is the page size (50 by default,
// which is the largest).
func (c *Client) IterAgents(params *ListAgentsParams) *AgentIterator {
	it := &AgentIterator{client: c}
	if params != nil {
		it.params = *params
	}
	if it.params.Limit == 0 {
		it.params.Limit = 50
	}
	return it
}

// Next moves to the next agent, asking for the next page when it must. It returns false at the end, and on an error;
// Err tells which.
func (it *AgentIterator) Next(ctx context.Context) bool {
	if it.err != nil {
		return false
	}
	for len(it.page) == 0 {
		if it.done {
			return false
		}
		list, err := it.client.ListAgents(ctx, &it.params)
		if err != nil {
			it.err = err
			return false
		}
		it.page = list.Data
		// A cursor that does not move would loop for ever.
		it.done = !list.HasMore || list.NextCursor == "" || list.NextCursor == it.params.Cursor
		it.params.Cursor = list.NextCursor
	}
	it.agent, it.page = it.page[0], it.page[1:]
	return true
}

// Agent returns the agent Next moved to.
func (it *AgentIterator) Agent() Agent { return it.agent }

// Err returns the error that stopped the iteration, or nil if it reached the last agent.
func (it *AgentIterator) Err() error { return it.err }

// request is one call to the API.
type request struct {
	method string
	path   string // with its segments already escaped
	query  url.Values
	json   any        // a body to send as JSON
	form   url.Values // a body to send as a form
	header http.Header
	// noBearer keeps Token off this request: the token endpoints and the registration take their own credentials.
	noBearer bool
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, request{method: http.MethodGet, path: path, query: query}, out)
}

// do sends a request and decodes a 2xx answer into out. Any other answer is a *Problem.
func (c *Client) do(ctx context.Context, r request, out any) error {
	base, err := cleanBaseURL(c.baseOrDefault())
	if err != nil {
		return err
	}
	target := base + r.path
	if len(r.query) > 0 {
		target += "?" + encodeQuery(r.query)
	}
	var (
		body        io.Reader
		contentType string
	)
	switch {
	case r.json != nil:
		encoded, err := json.Marshal(r.json)
		if err != nil {
			return fmt.Errorf("carragent: encoding the request: %w", err)
		}
		body, contentType = bytes.NewReader(encoded), "application/json"
	case r.form != nil:
		body, contentType = strings.NewReader(r.form.Encode()), "application/x-www-form-urlencoded"
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return fmt.Errorf("carragent: %w", err)
	}
	req.Header.Set("Accept", "application/json, application/problem+json")
	req.Header.Set("User-Agent", c.userAgentOrDefault())
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != "" && !r.noBearer {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for name, values := range r.header {
		req.Header[name] = values
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("carragent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("carragent: reading the answer: %w", err)
	}
	if len(raw) > maxBody {
		return &Problem{Status: resp.StatusCode, Code: "response_too_large", Title: "Response too large", Detail: "The answer is over 8 MiB, so it is not Carragent's."}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return problemFrom(resp.StatusCode, raw, resp.Header)
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || json.Unmarshal(raw, out) != nil {
		return &Problem{Status: resp.StatusCode, Code: "invalid_response", Title: "Unexpected response", Detail: "The API answered, but not with the JSON it should."}
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: defaultTimeout, CheckRedirect: noRedirect}
}

func (c *Client) baseOrDefault() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

func (c *Client) userAgentOrDefault() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return userAgent
}

// cleanBaseURL returns the base URL without its trailing slash, or an error: https (http for this machine), and
// nothing but an address.
func cleanBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("carragent: BaseURL must be an https address such as %q, not %q", DefaultBaseURL, value)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(value, "#") {
		return "", errors.New("carragent: BaseURL must be an address only: no credentials, query or fragment")
	}
	if u.Scheme == "http" && !isLocal(u.Hostname()) {
		return "", errors.New("carragent: BaseURL must be https, so that a token never crosses the network in the clear (http works for localhost)")
	}
	return strings.TrimRight(value, "/"), nil
}

func isLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// pathSegment escapes a value as one path segment, or says it cannot name anything.
func pathSegment(value, what string) (string, error) {
	if value == "" || value == "." || value == ".." {
		return "", fmt.Errorf("carragent: %s must not be empty", what)
	}
	return escape(value), nil
}

// escape percent-encodes everything but A-Z a-z 0-9 - _ . ~, with %20 for a space.
func escape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// encodeQuery is url.Values.Encode (keys sorted) with %20 for a space, as the API's own docs write it.
func encodeQuery(query url.Values) string {
	return strings.ReplaceAll(query.Encode(), "+", "%20")
}

func setIf(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

// sleep waits for d or for ctx, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
