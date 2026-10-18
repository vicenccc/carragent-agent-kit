package carragent_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	carragent "carragent.com/go"
)

// These tests talk to a real server, and run only when you say where it is:
//
//	CARRAGENT_LIVE_URL=http://localhost:3100 go test -run Live ./...
//
// Nothing here changes anything: the API is read-only. A feature the server does not have yet skips its test.
func liveClient(t *testing.T) *carragent.Client {
	t.Helper()
	base := os.Getenv("CARRAGENT_LIVE_URL")
	if base == "" {
		t.Skip("set CARRAGENT_LIVE_URL to try the client against a real server")
	}
	c := carragent.NewClient()
	c.BaseURL = base
	c.HTTPClient.Timeout = 30 * time.Second
	return c
}

func skipIfMissing(t *testing.T, err error) {
	t.Helper()
	var p *carragent.Problem
	if errors.As(err, &p) && (p.Status == 404 || p.Status == 503 || p.Code == "unexpected_redirect") {
		t.Skipf("this server does not have it: %v", err)
	}
}

func TestLiveAgentsPlansAndSearch(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	page, err := c.ListAgents(ctx, &carragent.ListAgentsParams{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 3 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	it := c.IterAgents(&carragent.ListAgentsParams{Limit: 4})
	seen := map[string]bool{}
	for it.Next(ctx) {
		if seen[it.Agent().ID] {
			t.Errorf("%s came twice", it.Agent().ID)
		}
		seen[it.Agent().ID] = true
	}
	if it.Err() != nil || len(seen) <= 4 {
		t.Errorf("iterated %d agents, err %v", len(seen), it.Err())
	}

	agent, err := c.GetAgent(ctx, page.Data[0].ID, carragent.Spanish)
	if err != nil || agent.ID != page.Data[0].ID || agent.WaitsForOwner == "" {
		t.Errorf("agent = %+v, err %v", agent, err)
	}
	_, err = c.GetAgent(ctx, "no-such-agent", "")
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 404 || p.Code != "agent_not_found" {
		t.Errorf("err = %v", err)
	}
	_, err = c.ListAgents(ctx, &carragent.ListAgentsParams{Limit: 500})
	if !errors.As(err, &p) || p.Status != 400 || p.Code != "invalid_parameter" {
		t.Errorf("err = %v", err)
	}

	plans, err := c.ListPlans(ctx, "")
	if err != nil || len(plans.Data) == 0 || plans.Terms.Currency != "USD" || plans.Data[0].Currency != "USD" {
		t.Errorf("plans = %+v, err %v", plans, err)
	}
	results, err := c.Search(ctx, "late invoices", "")
	if err != nil || len(results.Data) == 0 {
		t.Errorf("results = %+v, err %v", results, err)
	}
}

// strict decodes an answer with no field left over: it fails when the API has a field the type lacks.
func strict(t *testing.T, c *carragent.Client, path string, into any) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		t.Errorf("%s: %v", path, err)
	}
}

func TestLiveTheTypesHaveEveryFieldTheAPIHas(t *testing.T) {
	c := liveClient(t)
	strict(t, c, "/api/v1/agents?limit=50", &carragent.AgentList{})
	strict(t, c, "/api/v1/agents/collections", &carragent.Agent{})
	strict(t, c, "/api/v1/plans", &carragent.PlanList{})
	strict(t, c, "/api/v1/search?q=invoices", &carragent.SearchResults{})
}

func TestLiveBatchAndJobs(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	answers, err := c.Batch(ctx, []carragent.BatchItem{{ID: "plans", Path: "/plans"}, {ID: "nope", Path: "/agents/no-such-agent"}})
	skipIfMissing(t, err)
	if err != nil {
		t.Fatal(err)
	}
	if len(answers.Responses) != 2 || answers.Responses[0].Status != 200 || answers.Responses[1].Status != 404 {
		t.Errorf("answers = %+v", answers)
	}

	accepted, err := c.CreateJob(ctx, carragent.OperationSearchCatalog, map[string]any{"q": "late invoices"})
	skipIfMissing(t, err)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.WaitJob(ctx, accepted.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var results carragent.SearchResults
	if job.Status != "succeeded" || job.Result.Decode(&results) != nil || len(results.Data) == 0 {
		t.Errorf("job = %+v", job)
	}
}

func TestLiveTokens(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	if _, err := c.UseAnonymousIdentity(ctx, nil); err != nil {
		skipIfMissing(t, err)
		t.Fatal(err)
	}
	if c.Token == "" || !strings.HasPrefix(c.Token, "cat_") {
		t.Errorf("Token = %q", c.Token)
	}
	if plans, err := c.ListPlans(ctx, ""); err != nil || len(plans.Data) == 0 {
		t.Errorf("plans with a token: %+v, %v", plans, err)
	}
}

func TestLiveClientCredentials(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	// The client has no method for registering an OAuth client (RFC 7591), since it is only needed once.
	registration := strings.NewReader(`{"client_name":"carragent-go live test"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/oauth2/register", registration)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "live-test-0123456789abcdef")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Skipf("this server cannot register a client: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var client struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if resp.StatusCode != http.StatusCreated || json.NewDecoder(resp.Body).Decode(&client) != nil || client.ClientID == "" {
		t.Skipf("this server cannot register a client: HTTP %d", resp.StatusCode)
	}

	if _, err := c.UseClientCredentials(ctx, client.ClientID, client.ClientSecret); err != nil {
		t.Fatal(err)
	}
	if plans, err := c.ListPlans(ctx, ""); err != nil || len(plans.Data) == 0 {
		t.Errorf("plans with a token: %+v, %v", plans, err)
	}
	_, err = c.UseClientCredentials(ctx, client.ClientID, "wrong-secret")
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 401 || p.Code != "invalid_client" {
		t.Errorf("err = %v", err)
	}
}
