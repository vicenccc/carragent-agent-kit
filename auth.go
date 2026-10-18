package carragent

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
)

// grantTypeJWT is the grant (RFC 7523) that trades an agent's identity assertion for an access token.
const grantTypeJWT = "urn:ietf:params:oauth:grant-type:jwt-bearer"

// RegisterAnonymous registers this agent anonymously: POST /agent/identity.
//
// The answer holds an IdentityAssertion that is good for 24 hours. There is nothing to claim and no account behind
// it: it only gives your agent an identity of its own, so that its rate limit is its own. Keep the answer and pass it
// to UseAnonymousIdentity to get a token. Registrations are limited to 10 a minute per IP address.
//
// Token is not sent to the registration.
func (c *Client) RegisterAnonymous(ctx context.Context) (*Registration, error) {
	var registration Registration
	body := map[string]string{"type": "anonymous"}
	if err := c.do(ctx, request{method: http.MethodPost, path: "/agent/identity", json: body, noBearer: true}, &registration); err != nil {
		return nil, err
	}
	if registration.IdentityAssertion == "" {
		return nil, &Problem{Status: http.StatusOK, Code: "invalid_response", Title: "Unexpected response", Detail: "The answer has no identity_assertion."}
	}
	return &registration, nil
}

// UseAnonymousIdentity gets a token for an anonymous agent identity, and uses it from now on.
//
// It registers first (RegisterAnonymous), unless you pass the registration you kept from an earlier call. It then
// exchanges the assertion for an access token at /api/oauth2/token (RFC 7523, the jwt-bearer grant) and sets Token.
// The token lasts an hour; call this again, with the same registration while it is valid, to get a new one. The
// assertion goes to the client's BaseURL, whatever address the registration names.
//
// The error is a *Problem with the code "invalid_grant" (400) when the assertion is expired or not Carragent's.
// Token is left alone when it fails.
func (c *Client) UseAnonymousIdentity(ctx context.Context, registration *Registration) (*Token, error) {
	if registration == nil {
		var err error
		if registration, err = c.RegisterAnonymous(ctx); err != nil {
			return nil, err
		}
	}
	if registration.IdentityAssertion == "" {
		return nil, errors.New("carragent: the registration has no IdentityAssertion; pass the result of RegisterAnonymous")
	}
	form := url.Values{"grant_type": {grantTypeJWT}, "assertion": {registration.IdentityAssertion}}
	return c.useToken(ctx, request{method: http.MethodPost, path: "/api/oauth2/token", form: form, noBearer: true})
}

// A TokenOption changes a token request.
type TokenOption func(*tokenRequest)

type tokenRequest struct{ scope string }

// WithScope asks for these space-separated scopes ("agents:read plans:read search:read"), within the client's own.
// Without it, the token gets all of the client's scopes.
func WithScope(scope string) TokenOption {
	return func(r *tokenRequest) { r.scope = scope }
}

// UseClientCredentials gets a token with an OAuth client's id and secret, and uses it from now on (RFC 6749,
// client credentials).
//
// The id and secret go to /api/oauth2/token as HTTP Basic credentials, and nowhere else. They are not kept. The token
// lasts an hour; call this again for a new one. Register a client at POST /api/oauth2/register (https://carragent.com/auth.md
// has the steps).
//
// The error is a *Problem with the code "invalid_client" (401) for an unknown client or a wrong secret, and
// "invalid_scope" (400) for a scope beyond the client's. Token is left alone when it fails.
func (c *Client) UseClientCredentials(ctx context.Context, clientID, secret string, options ...TokenOption) (*Token, error) {
	var asked tokenRequest
	for _, option := range options {
		option(&asked)
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	setIf(form, "scope", asked.scope)
	// RFC 6749 section 2.3.1: form-encode both, so that a colon or a percent sign cannot change where one ends.
	basic := escape(clientID) + ":" + escape(secret)
	r := request{method: http.MethodPost, path: "/api/oauth2/token", form: form, noBearer: true, header: http.Header{}}
	r.header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(basic)))
	return c.useToken(ctx, r)
}

// useToken sends a token request and keeps the token it gets.
func (c *Client) useToken(ctx context.Context, r request) (*Token, error) {
	var token Token
	if err := c.do(ctx, r, &token); err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		return nil, &Problem{Status: http.StatusOK, Code: "invalid_response", Title: "Unexpected response", Detail: "The answer has no access_token."}
	}
	c.Token = token.AccessToken
	return &token, nil
}
