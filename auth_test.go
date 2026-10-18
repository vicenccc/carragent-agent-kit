package carragent_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	carragent "carragent.com/go"
)

const (
	grantJWT      = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	earlierAccess = "cat_old-fake-token"
)

var registrationJSON = map[string]any{
	"registration_id":    "reg_AAAAAAAAAAAAAAAAAAAAAA",
	"registration_type":  "anonymous",
	"identity_assertion": "header.claims.signature",
	"assertion_expires":  "2026-10-17T12:00:00.000Z",
	"pre_claim_scopes":   []string{"agents:read", "plans:read", "search:read"},
	"token_endpoint":     "https://carragent.com/api/oauth2/token",
	"grant_type":         grantJWT,
}

func TestRegisterAnonymousPostsTheTypeAndReturnsTheRegistration(t *testing.T) {
	f := newFake(t, ok(registrationJSON))
	c := f.client()
	c.Token = earlierAccess
	registration, err := c.RegisterAnonymous(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	req := f.Requests()[0]
	if req.Method != http.MethodPost || req.URI != "/agent/identity" || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("request = %s %s (%s)", req.Method, req.URI, req.Header.Get("Content-Type"))
	}
	if got := string(req.Body); got != `{"type":"anonymous"}` {
		t.Errorf("body = %s", got)
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("the token was sent to the registration: %q", req.Header.Get("Authorization"))
	}
	want := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	if registration.RegistrationID != "reg_AAAAAAAAAAAAAAAAAAAAAA" || registration.IdentityAssertion != "header.claims.signature" ||
		!registration.AssertionExpires.Equal(want) || registration.GrantType != grantJWT ||
		!reflect.DeepEqual(registration.PreClaimScopes, []string{"agents:read", "plans:read", "search:read"}) {
		t.Errorf("registration = %+v", registration)
	}
}

func TestRegisterAnonymousRefusals(t *testing.T) {
	f := newFake(t,
		problem(400, "unsupported_identity_type", "Unsupported identity type", "Only anonymous is offered."),
		problem(503, "temporarily_unavailable", "Registration is not configured", ""),
		problem(429, "rate_limited", "Too many requests", "", "Retry-After", "30"),
		ok(map[string]any{"registration_id": "reg_x"}),
	)
	c := f.client()
	wantCodes := []string{"unsupported_identity_type", "temporarily_unavailable", "rate_limited", "invalid_response"}
	for _, code := range wantCodes {
		_, err := c.RegisterAnonymous(context.Background())
		var p *carragent.Problem
		if !errors.As(err, &p) || p.Code != code {
			t.Errorf("err = %v, want %s", err, code)
		}
		if code == "rate_limited" && p.RetryAfter != 30*time.Second {
			t.Errorf("RetryAfter = %v", p.RetryAfter)
		}
	}
}

func TestUseAnonymousIdentityRegistersExchangesAndUsesTheToken(t *testing.T) {
	f := newFake(t, ok(registrationJSON), ok(tokenJSON), ok(plansJSON))
	c := f.client()
	token, err := c.UseAnonymousIdentity(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != fakeAccess || token.TokenType != "Bearer" || token.ExpiresIn != 3600 || c.Token != fakeAccess {
		t.Errorf("token = %+v, c.Token = %q", token, c.Token)
	}
	got := f.Requests()
	if got[0].Method != http.MethodPost || got[0].URI != "/agent/identity" {
		t.Errorf("first request = %s %s", got[0].Method, got[0].URI)
	}
	exchange := got[1]
	if exchange.Method != http.MethodPost || exchange.URI != "/api/oauth2/token" || exchange.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("second request = %s %s (%s)", exchange.Method, exchange.URI, exchange.Header.Get("Content-Type"))
	}
	if want := (url.Values{"grant_type": {grantJWT}, "assertion": {"header.claims.signature"}}); !reflect.DeepEqual(exchange.Form(t), want) {
		t.Errorf("form = %v, want %v", exchange.Form(t), want)
	}
	if exchange.Header.Get("Authorization") != "" {
		t.Errorf("the exchange carried Authorization %q", exchange.Header.Get("Authorization"))
	}
	if _, err := c.ListPlans(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[2].Header.Get("Authorization"); got != "Bearer "+fakeAccess {
		t.Errorf("Authorization = %q", got)
	}
}

func TestAnOldTokenIsNotSentWhileGettingANewOne(t *testing.T) {
	f := newFake(t, ok(registrationJSON), ok(tokenJSON))
	c := f.client()
	c.Token = earlierAccess
	if _, err := c.UseAnonymousIdentity(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	for _, req := range f.Requests() {
		if req.Header.Get("Authorization") != "" {
			t.Errorf("%s carried Authorization %q", req.URI, req.Header.Get("Authorization"))
		}
	}
}

func TestARegistrationYouKeptIsExchangedWithoutRegisteringAgain(t *testing.T) {
	f := newFake(t, ok(tokenJSON))
	c := f.client()
	registration := &carragent.Registration{IdentityAssertion: "kept.assertion.value"}
	if _, err := c.UseAnonymousIdentity(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	got := f.Requests()
	if len(got) != 1 || got[0].URI != "/api/oauth2/token" || got[0].Form(t).Get("assertion") != "kept.assertion.value" {
		t.Errorf("requests = %+v", got)
	}
	if c.Token != fakeAccess {
		t.Errorf("Token = %q", c.Token)
	}
}

func TestTheAssertionOnlyEverGoesToTheBaseURL(t *testing.T) {
	registration := map[string]any{}
	for k, v := range registrationJSON {
		registration[k] = v
	}
	registration["token_endpoint"] = "https://evil.example/steal"
	f := newFake(t, ok(registration), ok(tokenJSON))
	if _, err := f.client().UseAnonymousIdentity(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[1].URI; got != "/api/oauth2/token" {
		t.Errorf("exchange went to %q", got)
	}
}

func TestARegistrationWithoutAnAssertionIsRefusedBeforeAnyRequest(t *testing.T) {
	f := newFake(t)
	if _, err := f.client().UseAnonymousIdentity(context.Background(), &carragent.Registration{}); err == nil {
		t.Error("no error")
	}
	if n := len(f.Requests()); n != 0 {
		t.Errorf("%d requests", n)
	}
}

func TestARefusedAssertionLeavesTheTokenAlone(t *testing.T) {
	f := newFake(t, ok(registrationJSON), reply{status: 400, body: map[string]any{"error": "invalid_grant", "error_description": "The assertion has expired."}})
	c := f.client()
	c.Token = earlierAccess
	_, err := c.UseAnonymousIdentity(context.Background(), nil)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 400 || p.Code != "invalid_grant" {
		t.Fatalf("err = %v", err)
	}
	if c.Token != earlierAccess {
		t.Errorf("Token = %q, want the old one", c.Token)
	}
}

func TestATokenResponseWithoutATokenIsNotAccepted(t *testing.T) {
	f := newFake(t, ok(map[string]any{"token_type": "Bearer", "expires_in": 3600}))
	c := f.client()
	_, err := c.UseAnonymousIdentity(context.Background(), &carragent.Registration{IdentityAssertion: "a.b.c"})
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Code != "invalid_response" {
		t.Fatalf("err = %v", err)
	}
	if c.Token != "" {
		t.Errorf("Token = %q", c.Token)
	}
}

func basicCredentials(t *testing.T, header string) string {
	t.Helper()
	value, ok := strings.CutPrefix(header, "Basic ")
	if !ok {
		t.Fatalf("Authorization = %q, want Basic", header)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}

func TestUseClientCredentialsSendsBasicCredentialsAndUsesTheToken(t *testing.T) {
	f := newFake(t, ok(tokenJSON), ok(plansJSON))
	c := f.client()
	token, err := c.UseClientCredentials(context.Background(), "cag_FAKEFAKEFAKEFAKE_7", "fake-secret")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != fakeAccess || c.Token != fakeAccess {
		t.Errorf("token = %+v, c.Token = %q", token, c.Token)
	}
	req := f.Requests()[0]
	if req.Method != http.MethodPost || req.URI != "/api/oauth2/token" || req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("request = %s %s (%s)", req.Method, req.URI, req.Header.Get("Content-Type"))
	}
	if want := (url.Values{"grant_type": {"client_credentials"}}); !reflect.DeepEqual(req.Form(t), want) {
		t.Errorf("form = %v, want %v", req.Form(t), want)
	}
	if got := basicCredentials(t, req.Header.Get("Authorization")); got != "cag_FAKEFAKEFAKEFAKE_7:fake-secret" {
		t.Errorf("credentials = %q", got)
	}
	if _, err := c.ListPlans(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[1].Header.Get("Authorization"); got != "Bearer "+fakeAccess {
		t.Errorf("Authorization = %q", got)
	}
}

func TestUseClientCredentialsPassesAScopeWhenGiven(t *testing.T) {
	f := newFake(t, ok(tokenJSON), ok(tokenJSON), ok(tokenJSON))
	c := f.client()
	ctx := context.Background()
	if _, err := c.UseClientCredentials(ctx, "cag_x_7", "s", carragent.WithScope("agents:read plans:read")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UseClientCredentials(ctx, "cag_x_7", "s", carragent.WithScope("")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UseClientCredentials(ctx, "cag_x_7", "s"); err != nil {
		t.Fatal(err)
	}
	got := f.Requests()
	if want := (url.Values{"grant_type": {"client_credentials"}, "scope": {"agents:read plans:read"}}); !reflect.DeepEqual(got[0].Form(t), want) {
		t.Errorf("form = %v, want %v", got[0].Form(t), want)
	}
	for _, req := range got[1:] {
		if req.Form(t).Has("scope") {
			t.Errorf("form = %v, want no scope", req.Form(t))
		}
	}
}

func TestCredentialsAreFormEncodedBeforeTheyAreJoined(t *testing.T) {
	// RFC 6749 section 2.3.1: a colon or a percent sign in an id or secret must not change where one ends.
	f := newFake(t, ok(tokenJSON))
	if _, err := f.client().UseClientCredentials(context.Background(), "cag_a:b%c", "s e+cret/ü"); err != nil {
		t.Fatal(err)
	}
	pair := basicCredentials(t, f.Requests()[0].Header.Get("Authorization"))
	id, secret, found := strings.Cut(pair, ":")
	if !found || strings.Contains(secret, ":") {
		t.Fatalf("credentials = %q", pair)
	}
	if got, _ := url.PathUnescape(id); got != "cag_a:b%c" {
		t.Errorf("id decodes to %q", got)
	}
	if got, _ := url.PathUnescape(secret); got != "s e+cret/ü" {
		t.Errorf("secret decodes to %q", got)
	}
}

func TestAnOldTokenIsNotSentWhileGettingANewOneWithCredentials(t *testing.T) {
	f := newFake(t, ok(tokenJSON))
	c := f.client()
	c.Token = earlierAccess
	if _, err := c.UseClientCredentials(context.Background(), "cag_x_7", "s"); err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[0].Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
		t.Errorf("Authorization = %q", got)
	}
}

func TestAWrongSecretIsAProblemAndLeavesTheTokenAlone(t *testing.T) {
	body := map[string]any{"error": "invalid_client", "error_description": "Unknown client or wrong secret."}
	f := newFake(t, reply{status: 401, body: body, header: map[string]string{"WWW-Authenticate": `Basic realm="carragent"`}},
		reply{status: 400, body: map[string]any{"error": "invalid_scope", "error_description": "This client may ask for: agents:read."}})
	c := f.client()
	c.Token = earlierAccess
	for _, code := range []string{"invalid_client", "invalid_scope"} {
		_, err := c.UseClientCredentials(context.Background(), "cag_x_7", "wrong")
		var p *carragent.Problem
		if !errors.As(err, &p) || p.Code != code {
			t.Errorf("err = %v, want %s", err, code)
		}
	}
	if c.Token != earlierAccess {
		t.Errorf("Token = %q, want the old one", c.Token)
	}
}

func TestTheSecretIsNotInTheError(t *testing.T) {
	const secret = "fake-secret-that-must-not-linger"
	f := newFake(t, reply{status: 401, body: map[string]any{"error": "invalid_client", "error_description": "no"}})
	_, err := f.client().UseClientCredentials(context.Background(), "cag_x_7", secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("err = %v", err)
	}
}

func TestAFailedRegistrationStopsUseAnonymousIdentity(t *testing.T) {
	f := newFake(t, problem(429, "rate_limited", "Too many requests", "", "Retry-After", "30"))
	c := f.client()
	_, err := c.UseAnonymousIdentity(context.Background(), nil)
	var p *carragent.Problem
	if !errors.As(err, &p) || !p.RateLimited() {
		t.Fatalf("err = %v, want the registration's 429", err)
	}
	if n := len(f.Requests()); n != 1 || c.Token != "" {
		t.Errorf("%d requests, Token %q; want 1 and none", n, c.Token)
	}
}
