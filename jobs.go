package carragent

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Batch runs up to 20 reads in one request: POST /api/v1/batch.
//
// The answers come back in order. An item that failed carries its problem in its Body and its own Status: Batch
// returns an error only when the whole request is refused. BatchResult.Decode reads an item's body, or returns its
// problem. Every item counts as one request against the rate limit.
//
// Each item's Path is relative to /api/v1: "/agents", "/agents/collections", "/plans" or "/search?q=late%20invoices".
func (c *Client) Batch(ctx context.Context, items []BatchItem) (*BatchResponse, error) {
	if items == nil {
		items = []BatchItem{}
	}
	var response BatchResponse
	body := struct {
		Requests []BatchItem `json:"requests"`
	}{items}
	if err := c.do(ctx, request{method: http.MethodPost, path: "/api/v1/batch", json: body}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateJob starts a job: POST /api/v1/jobs, which answers 202 Accepted.
//
// This is the REST pattern for slow operations, so that code written for one works here unchanged. The operations
// are fast reads: nothing is queued, and a job's result is worked out when you poll it, from the catalog as it is then.
// Poll with GetJob or WaitJob.
//
// params are the operation's query parameters as strings ("lang", "limit", "cursor", "id", "q"), or
// map[string]any{"requests": []BatchItem{...}} for OperationBatch. They may be nil.
func (c *Client) CreateJob(ctx context.Context, operation JobOperation, params map[string]any) (*JobAccepted, error) {
	if params == nil {
		params = map[string]any{}
	}
	body := struct {
		Operation JobOperation   `json:"operation"`
		Params    map[string]any `json:"params"`
	}{operation, params}
	var job JobAccepted
	if err := c.do(ctx, request{method: http.MethodPost, path: "/api/v1/jobs", json: body}, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// GetJob returns a job, with its result once it has run: GET /api/v1/jobs/{id}.
//
// The error is a *Problem with the code "job_not_found" (404) for an id the API does not recognise, and
// "job_expired" (410) once the job is older than 24 hours.
func (c *Client) GetJob(ctx context.Context, id string) (*Job, error) {
	segment, err := pathSegment(id, "job id")
	if err != nil {
		return nil, err
	}
	var job Job
	if err := c.get(ctx, "/api/v1/jobs/"+segment, url.Values{}, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// WaitJob polls a job until its status is no longer "pending", and returns it. It gives up with ctx's error when ctx
// is done, so give ctx a deadline.
//
// Today a job has its result on the first poll, so this returns at once. Every poll runs the job again and costs
// what the job costs against the rate limit: one request, or one per item of a batch. interval is the time between
// polls; zero means one second.
func (c *Client) WaitJob(ctx context.Context, id string, interval time.Duration) (*Job, error) {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		job, err := c.GetJob(ctx, id)
		if err != nil {
			return nil, err
		}
		if job.Status != "pending" {
			return job, nil
		}
		if err := sleep(ctx, interval); err != nil {
			return nil, err
		}
	}
}
