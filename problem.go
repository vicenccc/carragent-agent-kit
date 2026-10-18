package carragent

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Problem is an error answer from the API: an RFC 9457 problem document (application/problem+json), or an OAuth
// error (RFC 6749) from the token endpoint. Every method of Client returns it as a *Problem, so use errors.As:
//
//	var problem *carragent.Problem
//	if errors.As(err, &problem) && problem.Code == "agent_not_found" {
//		// ...
//	}
//
// Branch on Code, which is stable, rather than on Title or Detail, which are for people.
//
// A request that gets no answer at all (no network, a refused connection, a timeout) is not a Problem: the method
// returns the transport's error, which wraps the standard ones, so errors.Is(err, context.DeadlineExceeded) works.
type Problem struct {
	// Type is the problem's type URI, "about:blank" for the API's own.
	Type string `json:"type,omitempty"`
	// Status is the HTTP status.
	Status int `json:"status"`
	// Code is the API's stable, machine-readable error code, such as "agent_not_found", "invalid_parameter" or
	// "rate_limited"; for the OAuth endpoints, the RFC 6749 error ("invalid_client", "invalid_grant").
	// The client adds four of its own: "http_error" (an error answer that is not a problem document, such as a
	// gateway's HTML page), "invalid_response" (a success answer that is not the JSON it should be),
	// "unexpected_redirect" and "response_too_large".
	Code string `json:"code"`
	// Title is a short summary of the problem.
	Title string `json:"title"`
	// Detail says what went wrong this time, when the API says.
	Detail string `json:"detail,omitempty"`
	// RetryAfter is how long to wait before trying again, from the Retry-After header of a 429 answer. It is zero
	// when there was no such header, or when it said to retry at once.
	RetryAfter time.Duration `json:"-"`
}

// Error describes the problem on one line.
func (p *Problem) Error() string {
	var b strings.Builder
	b.WriteString("carragent: ")
	b.WriteString(p.Title)
	fmt.Fprintf(&b, " (%s, HTTP %d)", p.Code, p.Status)
	if p.Detail != "" {
		b.WriteString(": ")
		b.WriteString(p.Detail)
	}
	if p.RetryAfter > 0 {
		text := strings.TrimRight(b.String(), ".")
		return fmt.Sprintf("%s. Retry after %s.", text, p.RetryAfter)
	}
	return b.String()
}

// RateLimited reports whether the API answered 429, too many requests. Wait RetryAfter and try again: the client
// never retries by itself.
func (p *Problem) RateLimited() bool {
	return p.Status == http.StatusTooManyRequests
}

// problemFrom builds the Problem for an error answer from its body, which may be anything.
func problemFrom(status int, body []byte, header http.Header) *Problem {
	p := &Problem{Status: status, Code: "http_error", Title: http.StatusText(status)}
	if p.Title == "" {
		p.Title = "HTTP " + strconv.Itoa(status)
	}
	var fields map[string]any
	if json.Unmarshal(body, &fields) == nil {
		text := func(key string) string {
			value, _ := fields[key].(string)
			return value
		}
		switch {
		case text("code") != "":
			p.Code = text("code")
			if title := text("title"); title != "" {
				p.Title = title
			}
			p.Type, p.Detail = text("type"), text("detail")
		case text("error") != "": // RFC 6749 section 5.2
			p.Code = text("error")
			p.Title = p.Code
			if description := text("error_description"); description != "" {
				p.Title = description
			}
		}
	}
	switch {
	case status == http.StatusTooManyRequests:
		if p.Code == "http_error" {
			p.Code, p.Title = "rate_limited", "Too many requests"
		}
		p.RetryAfter = parseRetryAfter(header.Get("Retry-After"), time.Now())
	case status >= 300 && status < 400:
		where := "The API redirected."
		if location := header.Get("Location"); location != "" {
			where = "The API redirected to " + location + "."
		}
		p.Code, p.Title = "unexpected_redirect", "Unexpected redirect"
		p.Detail = where + " The client does not follow redirects: set BaseURL to the final address."
	}
	return p
}

// parseRetryAfter reads a Retry-After value: delta-seconds or an HTTP date (RFC 9110). Zero when it is neither.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if isDigits(value) {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds > math.MaxInt64/int64(time.Second) {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// decodeBody reads the body of a batch item or a job result into v, or returns its problem.
func decodeBody(status int, body json.RawMessage, v any) error {
	if status >= 400 {
		return problemFrom(status, body, http.Header{})
	}
	if err := json.Unmarshal(body, v); err != nil {
		return &Problem{Status: status, Code: "invalid_response", Title: "Unexpected response", Detail: "The body is not the JSON it should be: " + err.Error()}
	}
	return nil
}
