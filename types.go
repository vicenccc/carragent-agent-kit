package carragent

import (
	"encoding/json"
	"time"
)

// Lang is the language of the text the API returns. The empty Lang means English.
type Lang string

// The languages of the API's text fields.
const (
	English Lang = "en"
	Spanish Lang = "es"
	French  Lang = "fr"
)

// Agent is one AI agent.
type Agent struct {
	// ID is the agent's stable id, such as "collections".
	ID string `json:"id"`
	// Name is its display name.
	Name string `json:"name"`
	// Area is the business area: "money", "customers", "sales" or "operations".
	Area string `json:"area"`
	// Status is "live" when the agent works today and "soon" when it is being built.
	Status string `json:"status"`
	// Summary says what it does, in one line.
	Summary string `json:"summary"`
	// Runs says when it runs.
	Runs string `json:"runs"`
	// Steps is what it does, in order.
	Steps []string `json:"steps"`
	// WaitsForOwner is what waits for the owner's approval.
	WaitsForOwner string `json:"waitsForOwner"`
	// Never is what the agent never does.
	Never string `json:"never"`
	// Tools are the tools it connects to. It is nil when there is nothing to connect.
	Tools []string `json:"tools"`
}

// AgentList is a page of agents.
type AgentList struct {
	Object string  `json:"object"`
	Data   []Agent `json:"data"`
	// NextCursor goes in ListAgentsParams.Cursor to get the next page. It is empty on the last page.
	NextCursor string `json:"nextCursor"`
	HasMore    bool   `json:"hasMore"`
}

// PlanLimits is what a plan includes. A nil limit means unlimited, or quoted.
type PlanLimits struct {
	Agents           *int `json:"agents"`
	Connections      *int `json:"connections"`
	MessagesPerMonth *int `json:"messagesPerMonth"`
	Seats            *int `json:"seats"`
}

// Plan is one plan. Prices are whole US cents, before applicable taxes.
type Plan struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	ForWho      string `json:"forWho"`
	Recommended bool   `json:"recommended"`
	// Currency is always "USD".
	Currency string `json:"currency"`
	// MonthlyCents is the price a month in cents. For a plan that is quoted, it is the starting price.
	MonthlyCents int `json:"monthlyCents"`
	// YearlyCents is the price a year in cents. It is nil when the plan is quoted.
	YearlyCents *int `json:"yearlyCents"`
	// SelfServe is false when the plan is quoted by email rather than bought on the site.
	SelfServe bool       `json:"selfServe"`
	Limits    PlanLimits `json:"limits"`
}

// PlanTerms are the terms that apply to every plan.
type PlanTerms struct {
	TrialDays        int    `json:"trialDays"`
	AnnualRefundDays int    `json:"annualRefundDays"`
	SetupFee         int    `json:"setupFee"`
	Contract         bool   `json:"contract"`
	Currency         string `json:"currency"`
}

// PlanList is every plan and the terms. There is only one page.
type PlanList struct {
	Object string    `json:"object"`
	Data   []Plan    `json:"data"`
	Terms  PlanTerms `json:"terms"`
	// NextCursor is always empty.
	NextCursor string `json:"nextCursor"`
	HasMore    bool   `json:"hasMore"`
}

// SearchHit is one match of a search: an agent or a plan.
type SearchHit struct {
	// Kind is "agent" or "plan".
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Score is higher for a better match. Its scale is not part of the API.
	Score float64 `json:"score"`
}

// SearchResults are the matches of a search, best first. There is only one page.
type SearchResults struct {
	Object string      `json:"object"`
	Query  string      `json:"query"`
	Data   []SearchHit `json:"data"`
	// NextCursor is always empty.
	NextCursor string `json:"nextCursor"`
	HasMore    bool   `json:"hasMore"`
}

// BatchItem is one read in a batch.
type BatchItem struct {
	// ID names the item's answer: 1 to 64 of A-Za-z0-9_.:- and unique in the batch. The item's position when empty.
	ID string `json:"id,omitempty"`
	// Method is only "GET", which is also the default.
	Method string `json:"method,omitempty"`
	// Path is a read of the API relative to /api/v1: "/agents?limit=5", "/agents/collections", "/plans" or
	// "/search?q=late%20invoices".
	Path string `json:"path"`
}

// BatchResult is the answer to one item of a batch.
type BatchResult struct {
	ID string `json:"id"`
	// Status is the HTTP status the single endpoint would have answered.
	Status int `json:"status"`
	// Body is what the single endpoint would have returned: its JSON, or a problem when Status is 400 or more.
	Body json.RawMessage `json:"body"`
}

// BatchResponse is the answers of a batch, in the order of the requests.
type BatchResponse struct {
	Object    string        `json:"object"`
	Responses []BatchResult `json:"responses"`
}

// JobOperation is what a job runs.
type JobOperation string

// The operations a job can run.
const (
	OperationListAgents    JobOperation = "listAgents"
	OperationGetAgent      JobOperation = "getAgent"
	OperationListPlans     JobOperation = "listPlans"
	OperationSearchCatalog JobOperation = "searchCatalog"
	OperationBatch         JobOperation = "batch"
)

// JobLinks says where a job is.
type JobLinks struct {
	// Self is the job's address, which GetJob polls.
	Self string `json:"self"`
}

// JobAccepted is a job that has just been created: the answer to CreateJob, a 202 Accepted.
type JobAccepted struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	// Status is "pending".
	Status    string    `json:"status"`
	Operation string    `json:"operation"`
	CreatedAt time.Time `json:"createdAt"`
	// ExpiresAt is when the job stops being available: 24 hours after it was created.
	ExpiresAt time.Time `json:"expiresAt"`
	Links     JobLinks  `json:"links"`
}

// JobResult is what a job's operation returned.
type JobResult struct {
	// Status is the HTTP status the operation answered. It can be a 4xx: the job still succeeded.
	Status int `json:"status"`
	// Body is the operation's JSON, or a problem.
	Body json.RawMessage `json:"body"`
}

// Job is a job as polled: the answer to GetJob.
type Job struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	// Status is "succeeded" once the job has run. Result says what the operation answered.
	Status      string    `json:"status"`
	Operation   string    `json:"operation"`
	CreatedAt   time.Time `json:"createdAt"`
	CompletedAt time.Time `json:"completedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Result      JobResult `json:"result"`
}

// Registration is an anonymous agent registration, the answer to RegisterAnonymous.
type Registration struct {
	RegistrationID   string `json:"registration_id"`
	RegistrationType string `json:"registration_type"`
	// IdentityAssertion is a signed assertion, good for 24 hours, that UseAnonymousIdentity trades for a token.
	IdentityAssertion string    `json:"identity_assertion"`
	AssertionExpires  time.Time `json:"assertion_expires"`
	PreClaimScopes    []string  `json:"pre_claim_scopes"`
	TokenEndpoint     string    `json:"token_endpoint"`
	GrantType         string    `json:"grant_type"`
}

// Token is an access token for Authorization: Bearer.
type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	// ExpiresIn is the seconds until it expires: an hour.
	ExpiresIn int    `json:"expires_in"`
	Scope     string `json:"scope"`
}

// Decode reads the item's body into v, which is a pointer. It returns the item's problem instead when Status is 400
// or more, so an item that failed is an error and one that worked is data.
func (r BatchResult) Decode(v any) error {
	return decodeBody(r.Status, r.Body, v)
}

// Decode reads the operation's body into v, which is a pointer. It returns the operation's problem instead when
// its Status is 400 or more.
func (r JobResult) Decode(v any) error {
	return decodeBody(r.Status, r.Body, v)
}
