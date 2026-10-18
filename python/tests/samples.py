"""Answers shaped like the API's, for the tests. The text is made up; the keys are the API's."""

from typing import Any

from carragent import Agent, Job, JobAccepted, PlanList, Registration, SearchResults, Token

AGENT: Agent = {
    "id": "collections",
    "name": "Collections",
    "area": "money",
    "status": "live",
    "summary": "Follows up late invoices on a fixed schedule.",
    "runs": "Every morning.",
    "steps": ["Reads open invoices.", "Writes the reminder.", "Waits for your OK over the limit."],
    "waitsForOwner": "Anything over your limit.",
    "never": "Never threatens or discloses a debt.",
    "tools": ["Gmail", "Google Sheets"],
}

AGENT_PAGES: list[dict[str, Any]] = [
    {
        "object": "list",
        "data": [AGENT, {**AGENT, "id": "scheduling", "name": "Scheduling", "tools": None}],
        "nextCursor": "Mg",
        "hasMore": True,
    },
    {"object": "list", "data": [{**AGENT, "id": "reviews", "name": "Reviews"}], "nextCursor": "NA", "hasMore": True},
    {"object": "list", "data": [{**AGENT, "id": "leads", "name": "Leads"}], "nextCursor": None, "hasMore": False},
]

PLANS: PlanList = {
    "object": "list",
    "data": [
        {
            "id": "team",
            "name": "Team",
            "summary": "For a small team.",
            "forWho": "Teams of up to 10.",
            "recommended": True,
            "currency": "USD",
            "monthlyCents": 4900,
            "yearlyCents": 47000,
            "selfServe": True,
            "limits": {"agents": 5, "connections": 10, "messagesPerMonth": 1000, "seats": 5},
        }
    ],
    "terms": {"trialDays": 14, "annualRefundDays": 30, "setupFee": 0, "contract": False, "currency": "USD"},
    "nextCursor": None,
    "hasMore": False,
}

SEARCH: SearchResults = {
    "object": "list",
    "query": "late invoices",
    "data": [
        {
            "kind": "agent",
            "id": "collections",
            "name": "Collections",
            "description": "Follows up late invoices.",
            "score": 2,
        }
    ],
    "nextCursor": None,
    "hasMore": False,
}

JOB_ACCEPTED: JobAccepted = {
    "object": "job",
    "id": "job_abc.def",
    "status": "pending",
    "operation": "searchCatalog",
    "createdAt": "2026-10-16T12:00:00.000Z",
    "expiresAt": "2026-10-17T12:00:00.000Z",
    "links": {"self": "https://carragent.com/api/v1/jobs/job_abc.def"},
}

# The API answers a poll with the finished job. A slow API could answer "pending" first, and wait_job copes with that.
JOB_PENDING: dict[str, Any] = {
    "object": "job",
    "id": "job_abc.def",
    "status": "pending",
    "operation": "searchCatalog",
    "createdAt": "2026-10-16T12:00:00.000Z",
    "expiresAt": "2026-10-17T12:00:00.000Z",
}

JOB_DONE: Job = {
    "object": "job",
    "id": "job_abc.def",
    "status": "succeeded",
    "operation": "searchCatalog",
    "createdAt": "2026-10-16T12:00:00.000Z",
    "completedAt": "2026-10-16T12:00:01.000Z",
    "expiresAt": "2026-10-17T12:00:00.000Z",
    "result": {"status": 200, "body": SEARCH},
}

REGISTRATION: Registration = {
    "registration_id": "reg_AAAAAAAAAAAAAAAAAAAAAA",
    "registration_type": "anonymous",
    "identity_assertion": "header.claims.signature",
    "assertion_expires": "2026-10-17T12:00:00.000Z",
    "pre_claim_scopes": ["agents:read", "plans:read", "search:read"],
    "token_endpoint": "https://carragent.com/api/oauth2/token",
    "grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
}

# Obviously fake: nothing here is, or looks like, a real credential.
FAKE_TOKEN = "cat_fake-token-for-tests"

TOKEN: Token = {
    "access_token": FAKE_TOKEN,
    "token_type": "Bearer",
    "expires_in": 3600,
    "scope": "agents:read plans:read search:read",
}
