package carragent_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	carragent "carragent.com/go"
)

var (
	jobAcceptedJSON = map[string]any{
		"object": "job", "id": "job_abc.def", "status": "pending", "operation": "searchCatalog",
		"createdAt": "2026-10-16T12:00:00.000Z", "expiresAt": "2026-10-17T12:00:00.000Z",
		"links": map[string]any{"self": "https://carragent.com/api/v1/jobs/job_abc.def"},
	}
	jobPendingJSON = map[string]any{
		"object": "job", "id": "job_abc.def", "status": "pending", "operation": "searchCatalog",
		"createdAt": "2026-10-16T12:00:00.000Z", "expiresAt": "2026-10-17T12:00:00.000Z",
	}
	jobDoneJSON = map[string]any{
		"object": "job", "id": "job_abc.def", "status": "succeeded", "operation": "searchCatalog",
		"createdAt": "2026-10-16T12:00:00.000Z", "completedAt": "2026-10-16T12:00:01.000Z", "expiresAt": "2026-10-17T12:00:00.000Z",
		"result": map[string]any{"status": 200, "body": searchJSON},
	}
)

func TestCreateJobPostsTheOperationAndItsParams(t *testing.T) {
	f := newFake(t, reply{status: 202, body: jobAcceptedJSON, header: map[string]string{"Location": "https://carragent.com/api/v1/jobs/job_abc.def", "Retry-After": "1"}})
	job, err := f.client().CreateJob(context.Background(), carragent.OperationSearchCatalog, map[string]any{"q": "late invoices", "lang": "en"})
	if err != nil {
		t.Fatal(err)
	}
	req := f.Requests()[0]
	if req.Method != http.MethodPost || req.URI != "/api/v1/jobs" || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("request = %s %s (%s)", req.Method, req.URI, req.Header.Get("Content-Type"))
	}
	if got, want := string(req.Body), `{"operation":"searchCatalog","params":{"lang":"en","q":"late invoices"}}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if job.ID != "job_abc.def" || job.Status != "pending" || job.Operation != "searchCatalog" || job.Links.Self != "https://carragent.com/api/v1/jobs/job_abc.def" {
		t.Errorf("job = %+v", job)
	}
	if want := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC); !job.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v", job.ExpiresAt)
	}
}

func TestJobParamsAreOptional(t *testing.T) {
	f := newFake(t, reply{status: 202, body: jobAcceptedJSON})
	if _, err := f.client().CreateJob(context.Background(), carragent.OperationListPlans, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := string(f.Requests()[0].Body), `{"operation":"listPlans","params":{}}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestABatchJobCarriesItsRequests(t *testing.T) {
	f := newFake(t, reply{status: 202, body: jobAcceptedJSON})
	params := map[string]any{"requests": []carragent.BatchItem{{Path: "/plans"}}}
	if _, err := f.client().CreateJob(context.Background(), carragent.OperationBatch, params); err != nil {
		t.Fatal(err)
	}
	if got, want := string(f.Requests()[0].Body), `{"operation":"batch","params":{"requests":[{"path":"/plans"}]}}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestTheOperationNamesAreTheAPIs(t *testing.T) {
	names := map[carragent.JobOperation]string{
		carragent.OperationListAgents:    "listAgents",
		carragent.OperationGetAgent:      "getAgent",
		carragent.OperationListPlans:     "listPlans",
		carragent.OperationSearchCatalog: "searchCatalog",
		carragent.OperationBatch:         "batch",
	}
	for operation, name := range names {
		if string(operation) != name {
			t.Errorf("%q != %q", operation, name)
		}
	}
}

func TestGetJobEscapesTheIDAndReadsTheResult(t *testing.T) {
	f := newFake(t, ok(jobDoneJSON), ok(jobDoneJSON))
	c := f.client()
	job, err := c.GetJob(context.Background(), "job_abc.def")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetJob(context.Background(), "job_a/b"); err != nil {
		t.Fatal(err)
	}
	got := f.Requests()
	if got[0].URI != "/api/v1/jobs/job_abc.def" || got[1].URI != "/api/v1/jobs/job_a%2Fb" {
		t.Errorf("URIs = %q, %q", got[0].URI, got[1].URI)
	}
	if job.Status != "succeeded" || job.CompletedAt.IsZero() || job.Result.Status != 200 {
		t.Fatalf("job = %+v", job)
	}
	var results carragent.SearchResults
	if err := job.Result.Decode(&results); err != nil || len(results.Data) != 1 || results.Data[0].ID != "collections" {
		t.Errorf("Decode = %v, %+v", err, results)
	}
}

func TestAJobWhoseOperationFailedStillSucceeded(t *testing.T) {
	body := map[string]any{"object": "job", "id": "job_x", "status": "succeeded", "operation": "getAgent",
		"createdAt": "2026-10-16T12:00:00.000Z", "completedAt": "2026-10-16T12:00:01.000Z", "expiresAt": "2026-10-17T12:00:00.000Z",
		"result": map[string]any{"status": 404, "body": map[string]any{"type": "about:blank", "status": 404, "code": "agent_not_found", "title": "No agent with that id"}}}
	f := newFake(t, ok(body))
	job, err := f.client().GetJob(context.Background(), "job_x")
	if err != nil {
		t.Fatal(err)
	}
	var agent carragent.Agent
	err = job.Result.Decode(&agent)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 404 || p.Code != "agent_not_found" {
		t.Errorf("Decode = %v, want the operation's problem", err)
	}
}

func TestGetJobRefusesAnEmptyID(t *testing.T) {
	f := newFake(t)
	if _, err := f.client().GetJob(context.Background(), ""); err == nil {
		t.Error("no error")
	}
	if n := len(f.Requests()); n != 0 {
		t.Errorf("%d requests", n)
	}
}

func TestUnknownAndExpiredJobsAreProblems(t *testing.T) {
	f := newFake(t, problem(404, "job_not_found", "No such job", ""), problem(410, "job_expired", "The job has expired", "Jobs last 24 hours."))
	for status, code := range []string{"job_not_found", "job_expired"} {
		_, err := f.client().GetJob(context.Background(), "job_x")
		var p *carragent.Problem
		if !errors.As(err, &p) || p.Code != code {
			t.Errorf("%d: err = %v, want %s", status, err, code)
		}
	}
}

func TestWaitJobReturnsAtOnceWhenTheJobHasRun(t *testing.T) {
	f := newFake(t, ok(jobDoneJSON))
	started := time.Now()
	job, err := f.client().WaitJob(context.Background(), "job_abc.def", time.Hour)
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("job = %+v, err = %v", job, err)
	}
	if time.Since(started) > 2*time.Second || len(f.Requests()) != 1 {
		t.Errorf("took %v and %d requests", time.Since(started), len(f.Requests()))
	}
}

func TestWaitJobPollsUntilTheJobIsNoLongerPending(t *testing.T) {
	f := newFake(t, ok(jobPendingJSON), ok(jobPendingJSON), ok(jobDoneJSON))
	job, err := f.client().WaitJob(context.Background(), "job_abc.def", 5*time.Millisecond)
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("job = %+v, err = %v", job, err)
	}
	if n := len(f.Requests()); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
}

func TestWaitJobGivesUpWhenTheContextDoes(t *testing.T) {
	f := newFakeFunc(t, func(int, recorded) reply { return ok(jobPendingJSON) })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := f.client().WaitJob(ctx, "job_abc.def", 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if n := len(f.Requests()); n < 2 {
		t.Errorf("%d polls, want at least 2", n)
	}
}

func TestWaitJobRaisesWhatGetJobRaises(t *testing.T) {
	f := newFake(t, problem(410, "job_expired", "The job has expired", ""))
	_, err := f.client().WaitJob(context.Background(), "job_abc.def", time.Millisecond)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Code != "job_expired" {
		t.Errorf("err = %v", err)
	}
}

func TestWaitJobDefaultsItsIntervalToASecond(t *testing.T) {
	f := newFakeFunc(t, func(int, recorded) reply { return ok(jobPendingJSON) })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := f.client().WaitJob(ctx, "job_abc.def", 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.Requests()); n != 1 {
		t.Errorf("%d polls in 300ms with the default interval, want 1", n)
	}
}

func TestAJobTheAPIRefusesIsAProblem(t *testing.T) {
	f := newFake(t, problem(400, "invalid_parameter", "Invalid operation", "operation must be one of listAgents, getAgent, listPlans, searchCatalog, batch."))
	_, err := f.client().CreateJob(context.Background(), "nope", nil)
	var p *carragent.Problem
	if !errors.As(err, &p) || p.Status != 400 || p.Code != "invalid_parameter" {
		t.Errorf("err = %v", err)
	}
}

func TestParamsThatCannotBeEncodedAreAnErrorBeforeAnyRequest(t *testing.T) {
	f := newFake(t)
	_, err := f.client().CreateJob(context.Background(), carragent.OperationListPlans, map[string]any{"x": make(chan int)})
	if err == nil {
		t.Fatal("no error")
	}
	if n := len(f.Requests()); n != 0 {
		t.Errorf("%d requests", n)
	}
}
