package carragent_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	carragent "carragent.com/go"
)

// failWith sends one request that the server answers with r, and returns the *Problem it gets.
func failWith(t *testing.T, r reply) *carragent.Problem {
	t.Helper()
	f := newFake(t, r)
	_, err := f.client().ListPlans(context.Background(), "")
	var p *carragent.Problem
	if !errors.As(err, &p) {
		t.Fatalf("err = %v (%T), want a *Problem", err, err)
	}
	return p
}

func TestAProblemDocumentBecomesAProblem(t *testing.T) {
	p := failWith(t, problem(400, "invalid_parameter", "Invalid lang", "lang must be one of en, es, fr."))
	if p.Status != 400 || p.Code != "invalid_parameter" || p.Title != "Invalid lang" || p.Detail != "lang must be one of en, es, fr." || p.Type != "about:blank" {
		t.Errorf("problem = %+v", p)
	}
	if p.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v", p.RetryAfter)
	}
}

func TestAProblemWithoutDetailHasNone(t *testing.T) {
	p := failWith(t, problem(404, "not_found", "No such endpoint", ""))
	if p.Detail != "" || p.Error() != "carragent: No such endpoint (not_found, HTTP 404)" {
		t.Errorf("problem = %+v, Error() = %q", p, p.Error())
	}
}

func TestAuthenticationProblemsCarryTheAPIsCode(t *testing.T) {
	for status, code := range map[int]string{401: "invalid_token", 403: "insufficient_scope"} {
		p := failWith(t, problem(status, code, "Nope", "x", "WWW-Authenticate", `Bearer error="x"`))
		if p.Status != status || p.Code != code {
			t.Errorf("%d: problem = %+v", status, p)
		}
	}
}

func TestAnOAuthErrorUsesItsErrorAsTheCode(t *testing.T) {
	body := map[string]any{"error": "invalid_client", "error_description": "Unknown client or wrong secret."}
	p := failWith(t, reply{status: 401, body: body, header: map[string]string{"WWW-Authenticate": `Basic realm="carragent"`}})
	if p.Status != 401 || p.Code != "invalid_client" || p.Title != "Unknown client or wrong secret." || p.Detail != "" {
		t.Errorf("problem = %+v", p)
	}
	p = failWith(t, reply{status: 400, body: map[string]any{"error": "invalid_request"}})
	if p.Code != "invalid_request" || p.Title != "invalid_request" {
		t.Errorf("problem = %+v", p)
	}
}

func TestAnErrorThatIsNotAProblemIsAnHTTPError(t *testing.T) {
	cases := []struct {
		name  string
		reply reply
		title string
	}{
		{"a gateway's page", reply{status: 502, body: "<html>Bad gateway</html>", contentType: "text/html"}, "Bad Gateway"},
		{"an empty body", reply{status: 503}, "Service Unavailable"},
		{"a list", reply{status: 500, body: []any{1, 2}}, "Internal Server Error"},
		{"fields of the wrong type", reply{status: 500, body: map[string]any{"code": 5, "title": []string{"x"}}}, "Internal Server Error"},
		{"plain text", reply{status: 500, body: "just text"}, "Internal Server Error"},
		{"an unusual status", reply{status: 599}, "HTTP 599"},
	}
	for _, tc := range cases {
		p := failWith(t, tc.reply)
		if p.Code != "http_error" || p.Title != tc.title || p.Status != tc.reply.status || p.Detail != "" {
			t.Errorf("%s: problem = %+v, want http_error %q", tc.name, p, tc.title)
		}
	}
}

func TestASuccessThatIsNotJSONIsAnInvalidResponse(t *testing.T) {
	for name, r := range map[string]reply{
		"html":  {status: 200, body: "<html>hello</html>", contentType: "text/html"},
		"empty": {status: 200},
		"null":  {status: 200, body: "null"},
		"204":   {status: 204},
		"array": {status: 200, body: "[1,2,3]"},
	} {
		p := failWith(t, r)
		if p.Code != "invalid_response" || p.Status != r.status {
			t.Errorf("%s: problem = %+v", name, p)
		}
	}
}

func TestA429IsRateLimitedWithRetryAfter(t *testing.T) {
	later := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	cases := []struct {
		name   string
		header string
		min    time.Duration
		max    time.Duration
	}{
		{"seconds", "7", 7 * time.Second, 7 * time.Second},
		{"seconds with spaces", " 12 ", 12 * time.Second, 12 * time.Second},
		{"a date", later, 25 * time.Second, 31 * time.Second},
		{"a date in the past", "Wed, 21 Oct 2015 07:28:00 GMT", 0, 0},
		{"zero", "0", 0, 0},
		{"nothing", "", 0, 0},
		{"words", "soon", 0, 0},
		{"negative", "-5", 0, 0},
		{"exponent", "1e3", 0, 0},
		{"underscore", "1_0", 0, 0},
		{"a unit", "5s", 0, 0},
		{"a number too big to wait", "99999999999999999999999999", 0, 0},
		{"a number that overflows a duration", "9999999999", 0, 0},
	}
	for _, tc := range cases {
		var headers []string
		if tc.header != "" {
			headers = []string{"Retry-After", tc.header}
		}
		p := failWith(t, problem(429, "rate_limited", "Too many requests", "Limit: 60 requests every 60 seconds.", headers...))
		if !p.RateLimited() || p.Status != 429 || p.Code != "rate_limited" {
			t.Errorf("%s: problem = %+v", tc.name, p)
		}
		if p.RetryAfter < tc.min || p.RetryAfter > tc.max {
			t.Errorf("%s: RetryAfter = %v, want between %v and %v", tc.name, p.RetryAfter, tc.min, tc.max)
		}
	}
}

func TestARateLimitedErrorSaysHowLongToWait(t *testing.T) {
	p := failWith(t, problem(429, "rate_limited", "Too many requests", "Limit: 60 requests every 60 seconds.", "Retry-After", "7"))
	if want := "carragent: Too many requests (rate_limited, HTTP 429): Limit: 60 requests every 60 seconds. Retry after 7s."; p.Error() != want {
		t.Errorf("Error() = %q, want %q", p.Error(), want)
	}
	p = failWith(t, problem(429, "rate_limited", "Too many requests", "", "Retry-After", "2"))
	if want := "carragent: Too many requests (rate_limited, HTTP 429). Retry after 2s."; p.Error() != want {
		t.Errorf("Error() = %q, want %q", p.Error(), want)
	}
}

func TestA429ThatIsNotAProblemIsStillRateLimited(t *testing.T) {
	p := failWith(t, reply{status: 429, body: "<html>slow down</html>", contentType: "text/html", header: map[string]string{"Retry-After": "12"}})
	if !p.RateLimited() || p.Code != "rate_limited" || p.Title != "Too many requests" || p.RetryAfter != 12*time.Second {
		t.Errorf("problem = %+v", p)
	}
}

func TestTheClientDoesNotRetryByItself(t *testing.T) {
	f := newFake(t, problem(429, "rate_limited", "Too many requests", "", "Retry-After", "1"), ok(plansJSON))
	if _, err := f.client().ListPlans(context.Background(), ""); err == nil {
		t.Fatal("no error")
	}
	if n := len(f.Requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestARefusedConnectionIsNotAProblem(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	base := server.URL
	server.Close()
	c := carragent.NewClient()
	c.BaseURL = base
	_, err := c.ListPlans(context.Background(), "")
	assertTransportError(t, err)
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Errorf("err = %v, want it to wrap a *url.Error", err)
	}
}

func TestAContextThatRunsOutIsReportedAsSuch(t *testing.T) {
	f := newFake(t, reply{body: plansJSON, status: 200, delay: 500 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := f.client().ListPlans(ctx, "")
	assertTransportError(t, err)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
}

func TestTheClientsOwnTimeoutIsReported(t *testing.T) {
	f := newFake(t, reply{body: plansJSON, status: 200, delay: 500 * time.Millisecond})
	c := f.client()
	c.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	_, err := c.ListPlans(context.Background(), "")
	assertTransportError(t, err)
	var timeout interface{ Timeout() bool }
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Errorf("err = %v, want a timeout", err)
	}
}

func TestAServerThatHangsUpIsNotAProblem(t *testing.T) {
	f := newFake(t, reply{hangUp: true})
	_, err := f.client().ListPlans(context.Background(), "")
	assertTransportError(t, err)
}

func TestAnAnswerCutShortIsATransportErrorNotBadJSON(t *testing.T) {
	f := newFake(t, reply{status: 200, body: plansJSON, shortBy: 50})
	_, err := f.client().ListPlans(context.Background(), "")
	assertTransportError(t, err)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("err = %v, want it to wrap io.ErrUnexpectedEOF", err)
	}
}

func TestAnErrorAnswerCutShortIsATransportErrorToo(t *testing.T) {
	f := newFake(t, reply{status: 500, body: map[string]any{"code": "x", "title": "y"}, shortBy: 50})
	_, err := f.client().ListPlans(context.Background(), "")
	assertTransportError(t, err)
}

func assertTransportError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("no error")
	}
	var p *carragent.Problem
	if errors.As(err, &p) {
		t.Fatalf("err = %v, want the transport's error, not a Problem", err)
	}
	if !strings.HasPrefix(err.Error(), "carragent: ") {
		t.Errorf("err = %q, want the package's prefix", err)
	}
}

func TestARedirectIsNotFollowedAndTheTokenGoesNowhereElse(t *testing.T) {
	elsewhere := newFake(t, ok(plansJSON))
	f := newFake(t, reply{status: 302, header: map[string]string{"Location": elsewhere.URL + "/api/v1/plans"}})
	c := f.client()
	c.Token = fakeAccess
	_, err := c.ListPlans(context.Background(), "")
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 302 || p.Code != "unexpected_redirect" || !strings.Contains(p.Detail, elsewhere.URL) {
		t.Fatalf("err = %v, want an unexpected_redirect problem naming %s", err, elsewhere.URL)
	}
	if n := len(elsewhere.Requests()); n != 0 {
		t.Errorf("the redirect was followed: %d requests elsewhere", n)
	}
	if n := len(f.Requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestEveryRedirectStatusIsRefusedForPostsToo(t *testing.T) {
	elsewhere := newFake(t, ok(plansJSON))
	for _, status := range []int{301, 303, 307, 308} {
		f := newFake(t, reply{status: status, header: map[string]string{"Location": elsewhere.URL}})
		_, err := f.client().Batch(context.Background(), []carragent.BatchItem{{Path: "/plans"}})
		var p *carragent.Problem
		if !errors.As(err, &p) || p.Status != status || p.Code != "unexpected_redirect" {
			t.Errorf("%d: err = %v", status, err)
		}
	}
	if n := len(elsewhere.Requests()); n != 0 {
		t.Errorf("a redirect was followed: %d requests elsewhere", n)
	}
}

func TestARedirectWithoutALocationIsExplainedToo(t *testing.T) {
	p := failWith(t, reply{status: 302})
	if p.Code != "unexpected_redirect" || p.Detail == "" {
		t.Errorf("problem = %+v", p)
	}
}

func TestAnAnswerThatIsTooLargeIsRefused(t *testing.T) {
	huge := []byte("{" + strings.Repeat(" ", 8<<20+10) + "}")
	for _, status := range []int{200, 500} {
		p := failWith(t, reply{status: status, body: huge, contentType: "application/json"})
		if p.Code != "response_too_large" || p.Status != status {
			t.Errorf("%d: problem = %+v", status, p)
		}
	}
}

func TestAnAnswerJustUnderTheLimitIsRead(t *testing.T) {
	body := []byte(`{"object":"list","data":[],"nextCursor":null,"hasMore":false,"pad":"` + strings.Repeat("x", 8<<20-200) + `"}`)
	f := newFake(t, reply{status: 200, body: body, contentType: "application/json"})
	if _, err := f.client().ListAgents(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestACustomHTTPClientIsUsed(t *testing.T) {
	f := newFake(t, ok(plansJSON))
	used := false
	c := f.client()
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	if _, err := c.ListPlans(context.Background(), ""); err != nil || !used {
		t.Errorf("err = %v, used = %v", err, used)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestANilContextIsAnErrorNotAPanic(t *testing.T) {
	f := newFake(t)
	//lint:ignore SA1012 a nil context is what this test passes on purpose
	_, err := f.client().ListPlans(nil, "") //nolint:staticcheck
	if err == nil {
		t.Error("no error")
	}
}
