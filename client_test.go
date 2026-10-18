package carragent_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	carragent "carragent.com/go"
)

func TestNewClientDefaults(t *testing.T) {
	c := carragent.NewClient()
	if carragent.DefaultBaseURL != "https://carragent.com" {
		t.Errorf("DefaultBaseURL = %q", carragent.DefaultBaseURL)
	}
	if c.BaseURL != carragent.DefaultBaseURL {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.UserAgent != "carragent-go/"+carragent.Version {
		t.Errorf("UserAgent = %q", c.UserAgent)
	}
	if c.Token != "" {
		t.Errorf("Token = %q, want none", c.Token)
	}
	if c.HTTPClient == nil || c.HTTPClient.Timeout.Seconds() != 10 {
		t.Errorf("HTTPClient = %+v, want a 10 second timeout", c.HTTPClient)
	}
}

func TestZeroClientWorks(t *testing.T) {
	f := newFake(t, ok(plansJSON))
	c := &carragent.Client{BaseURL: f.URL}
	if _, err := c.ListPlans(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[0].Header.Get("User-Agent"); got != "carragent-go/"+carragent.Version {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestBaseURLIsChecked(t *testing.T) {
	for _, base := range []string{"carragent.com", "ftp://carragent.com", "https://", "//carragent.com", "http://carragent.com", "http://192.168.1.10",
		"http://localhost.example.com", "http://0.0.0.0:3000", "https://user:pass@carragent.com", "https://user@carragent.com", "https://carragent.com?x=1",
		"https://carragent.com?", "https://carragent.com#top", "https://carragent.com#", "https://carragent.com/%zz"} {
		c := carragent.NewClient()
		c.BaseURL = base
		if _, err := c.ListPlans(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "BaseURL") {
			t.Errorf("BaseURL %q: err = %v, want a BaseURL error", base, err)
		}
	}
}

func TestBaseURLWithAPathKeepsIt(t *testing.T) {
	f := newFake(t, ok(plansJSON))
	c := f.client()
	c.BaseURL += "/prefix//"
	if _, err := c.ListPlans(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[0].Path(); got != "/prefix/api/v1/plans" {
		t.Errorf("path = %q", got)
	}
}

func TestHTTPIsAcceptedForThisMachine(t *testing.T) {
	// httptest serves on 127.0.0.1, and the other loopback names are checked without a server.
	f := newFake(t, ok(plansJSON))
	if !strings.HasPrefix(f.URL, "http://127.0.0.1:") {
		t.Fatalf("unexpected test server address %q", f.URL)
	}
	if _, err := f.client().ListPlans(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"http://localhost:1", "http://[::1]:1", "http://127.1.2.3:1"} {
		c := carragent.NewClient()
		c.BaseURL = base
		_, err := c.ListPlans(context.Background(), "")
		if err != nil && strings.Contains(err.Error(), "must be https") {
			t.Errorf("BaseURL %q was refused: %v", base, err)
		}
	}
}

func TestListAgentsSendsOnlyWhatItIsGiven(t *testing.T) {
	f := newFake(t, ok(agentPage([]string{"collections"}, "")), ok(agentPage([]string{"collections"}, "")), ok(agentPage([]string{"collections"}, "")))
	c := f.client()
	if _, err := c.ListAgents(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListAgents(context.Background(), &carragent.ListAgentsParams{}); err != nil {
		t.Fatal(err)
	}
	list, err := c.ListAgents(context.Background(), &carragent.ListAgentsParams{Lang: carragent.Spanish, Limit: 5, Cursor: "Mg"})
	if err != nil {
		t.Fatal(err)
	}
	got := f.Requests()
	if got[0].URI != "/api/v1/agents" || got[1].URI != "/api/v1/agents" {
		t.Errorf("empty params sent %q and %q, want no query", got[0].URI, got[1].URI)
	}
	if want := (url.Values{"cursor": {"Mg"}, "lang": {"es"}, "limit": {"5"}}); !reflect.DeepEqual(got[2].Query(t), want) {
		t.Errorf("query = %v, want %v", got[2].Query(t), want)
	}
	if len(list.Data) != 1 || list.Data[0].ID != "collections" || list.HasMore || list.NextCursor != "" {
		t.Errorf("list = %+v", list)
	}
}

func TestEveryRequestSaysWhoIsAsking(t *testing.T) {
	f := newFake(t, ok(plansJSON), ok(plansJSON), ok(plansJSON))
	c := f.client()
	ctx := context.Background()
	if _, err := c.ListPlans(ctx, ""); err != nil {
		t.Fatal(err)
	}
	c.UserAgent = "my-app/2 carragent-go/" + carragent.Version
	c.Token = fakeAccess
	if _, err := c.ListPlans(ctx, ""); err != nil {
		t.Fatal(err)
	}
	c.Token = ""
	if _, err := c.ListPlans(ctx, ""); err != nil {
		t.Fatal(err)
	}
	got := f.Requests()
	if got[0].Header.Get("User-Agent") != "carragent-go/1.0.0" {
		t.Errorf("User-Agent = %q", got[0].Header.Get("User-Agent"))
	}
	if !strings.Contains(got[0].Header.Get("Accept"), "application/json") {
		t.Errorf("Accept = %q", got[0].Header.Get("Accept"))
	}
	if got[0].Header.Get("Authorization") != "" || got[0].Header.Get("Content-Type") != "" {
		t.Errorf("a plain read sent Authorization %q and Content-Type %q", got[0].Header.Get("Authorization"), got[0].Header.Get("Content-Type"))
	}
	if got[1].Header.Get("Authorization") != "Bearer "+fakeAccess || got[1].Header.Get("User-Agent") != "my-app/2 carragent-go/1.0.0" {
		t.Errorf("headers = %v", got[1].Header)
	}
	if got[2].Header.Get("Authorization") != "" {
		t.Errorf("a cleared token was still sent: %q", got[2].Header.Get("Authorization"))
	}
}

func TestGetAgentEscapesTheIDAsOneSegment(t *testing.T) {
	f := newFakeFunc(t, func(int, recorded) reply { return ok(agentJSON) })
	c := f.client()
	agent, err := c.GetAgent(context.Background(), "collections", carragent.French)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Collections" || agent.Area != "money" || agent.WaitsForOwner == "" || len(agent.Tools) != 2 || len(agent.Steps) != 2 {
		t.Errorf("agent = %+v", agent)
	}
	for _, id := range []string{"a/b c?d#e%f", "facturación"} {
		if _, err := c.GetAgent(context.Background(), id, ""); err != nil {
			t.Fatal(err)
		}
	}
	got := f.Requests()
	want := []string{"/api/v1/agents/collections?lang=fr", "/api/v1/agents/a%2Fb%20c%3Fd%23e%25f", "/api/v1/agents/facturaci%C3%B3n"}
	for i, uri := range want {
		if got[i].URI != uri {
			t.Errorf("request %d = %q, want %q", i, got[i].URI, uri)
		}
	}
}

func TestGetAgentRefusesAnIDThatCannotNameAnAgent(t *testing.T) {
	f := newFake(t)
	for _, id := range []string{"", ".", ".."} {
		if _, err := f.client().GetAgent(context.Background(), id, ""); err == nil {
			t.Errorf("GetAgent(%q) worked", id)
		}
	}
	if n := len(f.Requests()); n != 0 {
		t.Errorf("%d requests were sent", n)
	}
}

func TestAnUnknownAgentIsAProblem(t *testing.T) {
	f := newFake(t, problem(404, "agent_not_found", "No agent with that id", "See /api/v1/agents for the ids."))
	_, err := f.client().GetAgent(context.Background(), "nope", "")
	var p *carragent.Problem
	if !errors.As(err, &p) {
		t.Fatalf("err = %v, want a *Problem", err)
	}
	if p.Status != 404 || p.Code != "agent_not_found" || p.Title != "No agent with that id" || p.Detail != "See /api/v1/agents for the ids." {
		t.Errorf("problem = %+v", p)
	}
	if want := "carragent: No agent with that id (agent_not_found, HTTP 404): See /api/v1/agents for the ids."; p.Error() != want {
		t.Errorf("Error() = %q, want %q", p.Error(), want)
	}
	if p.RateLimited() {
		t.Error("a 404 is not rate limiting")
	}
}

func TestPlansAreTyped(t *testing.T) {
	f := newFake(t, ok(plansJSON))
	plans, err := f.client().ListPlans(context.Background(), carragent.Spanish)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Requests()[0].URI; got != "/api/v1/plans?lang=es" {
		t.Errorf("URI = %q", got)
	}
	team, custom := plans.Data[0], plans.Data[1]
	if team.MonthlyCents != 4900 || team.YearlyCents == nil || *team.YearlyCents != 47000 || !team.SelfServe || !team.Recommended || team.Currency != "USD" {
		t.Errorf("team = %+v", team)
	}
	if team.Limits.Agents == nil || *team.Limits.Agents != 5 || team.Limits.MessagesPerMonth != nil {
		t.Errorf("team limits = %+v, want 5 agents and unlimited messages", team.Limits)
	}
	if custom.YearlyCents != nil || custom.SelfServe || custom.Limits.Agents != nil {
		t.Errorf("custom = %+v, want a quoted plan", custom)
	}
	if plans.Terms.TrialDays != 14 || plans.Terms.AnnualRefundDays != 30 || plans.Terms.SetupFee != 0 || plans.Terms.Contract || plans.Terms.Currency != "USD" {
		t.Errorf("terms = %+v", plans.Terms)
	}
	if plans.HasMore || plans.NextCursor != "" {
		t.Errorf("plans paginate: %+v", plans)
	}
}

func TestSearchEncodesTheQuery(t *testing.T) {
	f := newFake(t, ok(searchJSON), ok(searchJSON))
	c := f.client()
	results, err := c.Search(context.Background(), "late invoices", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Data) != 1 || results.Data[0].Kind != "agent" || results.Data[0].ID != "collections" || results.Data[0].Score != 2 || results.Query != "late invoices" {
		t.Errorf("results = %+v", results)
	}
	const awkward = "facturación & recibos=1+2 #3"
	if _, err := c.Search(context.Background(), awkward, carragent.Spanish); err != nil {
		t.Fatal(err)
	}
	got := f.Requests()
	if got[0].URI != "/api/v1/search?q=late%20invoices" {
		t.Errorf("URI = %q", got[0].URI)
	}
	if got[1].Query(t).Get("q") != awkward || got[1].Query(t).Get("lang") != "es" {
		t.Errorf("query = %v", got[1].Query(t))
	}
	if strings.ContainsAny(got[1].URI, " #") {
		t.Errorf("URI %q has a raw space or hash", got[1].URI)
	}
}

func TestAnEmptySearchIsLeftToTheAPI(t *testing.T) {
	f := newFake(t, problem(400, "invalid_parameter", "Invalid q", "q is required, up to 200 characters."))
	_, err := f.client().Search(context.Background(), "", "")
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Code != "invalid_parameter" {
		t.Fatalf("err = %v", err)
	}
	if got := f.Requests()[0].URI; got != "/api/v1/search?q=" {
		t.Errorf("URI = %q", got)
	}
}

func TestTheAnswerKeepsWhatTheTypeDoesNotKnow(t *testing.T) {
	// A field the API adds later must not break an old client.
	body := map[string]any{"object": "list", "data": []any{}, "nextCursor": nil, "hasMore": false, "somethingNew": map[string]any{"added": "later"}}
	f := newFake(t, ok(body))
	if _, err := f.client().ListAgents(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestIterAgentsFollowsTheCursorUntilTheLastPage(t *testing.T) {
	f := newFake(t, ok(agentPage([]string{"a", "b"}, "Mg")), ok(agentPage([]string{"c"}, "NA")), ok(agentPage([]string{"d"}, "")))
	it := f.client().IterAgents(&carragent.ListAgentsParams{Lang: carragent.English, Limit: 2})
	var ids []string
	for it.Next(context.Background()) {
		ids = append(ids, it.Agent().ID)
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	got := f.Requests()
	if len(got) != 3 {
		t.Fatalf("%d requests, want 3", len(got))
	}
	for i, cursor := range []string{"", "Mg", "NA"} {
		query := got[i].Query(t)
		if query.Get("limit") != "2" || query.Get("lang") != "en" || query.Get("cursor") != cursor {
			t.Errorf("request %d query = %v, want limit 2, lang en, cursor %q", i, query, cursor)
		}
	}
	if it.Next(context.Background()) {
		t.Error("Next went past the end")
	}
}

func TestIterAgentsAsksForAPageOnlyWhenItIsReached(t *testing.T) {
	f := newFake(t, ok(agentPage([]string{"a", "b"}, "Mg")), ok(agentPage([]string{"c"}, "")))
	it := f.client().IterAgents(nil)
	if n := len(f.Requests()); n != 0 {
		t.Fatalf("%d requests before Next", n)
	}
	steps := []struct {
		id       string
		requests int
	}{{"a", 1}, {"b", 1}, {"c", 2}}
	for _, step := range steps {
		if !it.Next(context.Background()) || it.Agent().ID != step.id {
			t.Fatalf("Next() = false or agent %q, want %q (err %v)", it.Agent().ID, step.id, it.Err())
		}
		if n := len(f.Requests()); n != step.requests {
			t.Errorf("after %q: %d requests, want %d", step.id, n, step.requests)
		}
	}
}

func TestIterAgentsDefaultPageSizeIsTheLargest(t *testing.T) {
	f := newFake(t, ok(agentPage([]string{"a"}, "")))
	it := f.client().IterAgents(nil)
	for it.Next(context.Background()) {
	}
	if got := f.Requests()[0].URI; got != "/api/v1/agents?limit=50" {
		t.Errorf("URI = %q", got)
	}
}

func TestIterAgentsCanStartFromACursor(t *testing.T) {
	f := newFake(t, ok(agentPage([]string{"c"}, "")))
	it := f.client().IterAgents(&carragent.ListAgentsParams{Cursor: "Mg", Limit: 10})
	for it.Next(context.Background()) {
	}
	if got := f.Requests()[0].Query(t); got.Get("cursor") != "Mg" || got.Get("limit") != "10" {
		t.Errorf("query = %v", got)
	}
}

func TestIterAgentsStopsOnACursorThatDoesNotMove(t *testing.T) {
	f := newFakeFunc(t, func(int, recorded) reply { return ok(agentPage([]string{"a"}, "same")) })
	it := f.client().IterAgents(nil)
	count := 0
	for it.Next(context.Background()) {
		count++
	}
	if it.Err() != nil || count != 2 || len(f.Requests()) != 2 {
		t.Errorf("count = %d, requests = %d, err = %v; want 2, 2, nil", count, len(f.Requests()), it.Err())
	}
}

func TestIterAgentsReportsAnErrorFromALaterPage(t *testing.T) {
	f := newFake(t, ok(agentPage([]string{"a"}, "Mg")), problem(429, "rate_limited", "Too many requests", "", "Retry-After", "3"))
	it := f.client().IterAgents(nil)
	if !it.Next(context.Background()) || it.Agent().ID != "a" {
		t.Fatalf("the first agent is missing: %v", it.Err())
	}
	if it.Next(context.Background()) {
		t.Fatal("Next worked after an error")
	}
	var p *carragent.Problem
	if !errors.As(it.Err(), &p) || p.Status != 429 {
		t.Fatalf("Err() = %v, want a 429 problem", it.Err())
	}
	if it.Next(context.Background()) {
		t.Error("Next works again after an error")
	}
}

func TestBatchPostsTheItemsAndReturnsEveryAnswer(t *testing.T) {
	answer := map[string]any{"object": "batch", "responses": []any{
		map[string]any{"id": "plans", "status": 200, "body": plansJSON},
		map[string]any{"id": "1", "status": 404, "body": map[string]any{"type": "about:blank", "status": 404, "code": "agent_not_found", "title": "No agent with that id"}},
	}}
	f := newFake(t, ok(answer))
	response, err := f.client().Batch(context.Background(), []carragent.BatchItem{{ID: "plans", Path: "/plans"}, {Path: "/agents/nope"}})
	if err != nil {
		t.Fatal(err)
	}
	req := f.Requests()[0]
	if req.Method != http.MethodPost || req.URI != "/api/v1/batch" || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("request = %s %s (%s)", req.Method, req.URI, req.Header.Get("Content-Type"))
	}
	if got, want := string(req.Body), `{"requests":[{"id":"plans","path":"/plans"},{"path":"/agents/nope"}]}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if len(response.Responses) != 2 || response.Responses[0].ID != "plans" || response.Responses[1].Status != 404 {
		t.Fatalf("response = %+v", response)
	}
	var plans carragent.PlanList
	if err := response.Responses[0].Decode(&plans); err != nil || len(plans.Data) != 2 {
		t.Errorf("Decode(plans) = %v, %+v", err, plans)
	}
	var agent carragent.Agent
	err = response.Responses[1].Decode(&agent)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 404 || p.Code != "agent_not_found" || p.Title != "No agent with that id" {
		t.Errorf("Decode(failed item) = %v, want its problem", err)
	}
}

func TestBatchWithNoItemsSendsAnEmptyList(t *testing.T) {
	f := newFake(t, problem(400, "invalid_request", "Invalid batch", "requests must have 1 to 20 items."))
	_, err := f.client().Batch(context.Background(), nil)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 400 || p.Code != "invalid_request" {
		t.Fatalf("err = %v", err)
	}
	if got := string(f.Requests()[0].Body); got != `{"requests":[]}` {
		t.Errorf("body = %s", got)
	}
}

func TestADecodeOfTheWrongShapeIsAProblem(t *testing.T) {
	result := carragent.BatchResult{ID: "x", Status: 200, Body: []byte(`["not","an","agent"]`)}
	var agent carragent.Agent
	err := result.Decode(&agent)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Code != "invalid_response" {
		t.Errorf("err = %v, want invalid_response", err)
	}
}

func TestAnIteratorAndAClientSurviveACancelledContext(t *testing.T) {
	f := newFake(t, ok(plansJSON))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.client().ListPlans(ctx, "")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if n := len(f.Requests()); n != 0 {
		t.Errorf("%d requests were sent with a cancelled context", n)
	}
}

func TestConcurrentReadsDoNotRace(t *testing.T) {
	f := newFakeFunc(t, func(int, recorded) reply { return ok(plansJSON) })
	c := f.client()
	errs := make(chan error, 16)
	for i := 0; i < cap(errs); i++ {
		go func() {
			_, err := c.ListPlans(context.Background(), "")
			errs <- err
		}()
	}
	for i := 0; i < cap(errs); i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestStringNeverShowsTheToken(t *testing.T) {
	c := carragent.NewClient()
	c.Token = fakeAccess
	for _, text := range []string{c.String(), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprint(c)} {
		if strings.Contains(text, fakeAccess) {
			t.Errorf("%q shows the token", text)
		}
		if !strings.Contains(text, "***") {
			t.Errorf("%q does not say there is a token", text)
		}
	}
	if text := carragent.NewClient().String(); !strings.Contains(text, "Token: none") {
		t.Errorf("String() = %q", text)
	}
}

func TestTheZeroClientNamesTheDefaultBaseURL(t *testing.T) {
	if text := (&carragent.Client{}).String(); !strings.Contains(text, carragent.DefaultBaseURL) {
		t.Errorf("String() = %q", text)
	}
}
