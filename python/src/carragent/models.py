"""The shapes of the API's JSON, as typed dictionaries.

A response is the parsed JSON itself, so the keys are exactly the API's, camelCase where the API's are
(``waitsForOwner``, ``monthlyCents``), and a field the API adds later is still in the dictionary. These types are for
type checkers and editors; at run time they are plain ``dict`` objects. The OpenAPI document at
https://carragent.com/openapi.json is the reference.

Prices are integers in US cents. Times are ISO 8601 strings.
"""

from typing import Any, Literal, Optional, TypedDict

Lang = Literal["en", "es", "fr"]
"""The languages of the text fields: English (the default), Spanish and French."""

JobOperation = Literal["listAgents", "getAgent", "listPlans", "searchCatalog", "batch"]
"""The operations a job can run."""


class Agent(TypedDict):
    """One AI agent. ``area`` is one of ``money``, ``customers``, ``sales`` and ``operations``."""

    id: str
    name: str
    area: str
    status: str
    """``live``: it works today. ``soon``: it is being built."""
    summary: str
    runs: str
    """When it runs."""
    steps: list[str]
    waitsForOwner: str
    """What waits for the owner's approval."""
    never: str
    """What it never does."""
    tools: Optional[list[str]]
    """The tools it connects to, or ``None`` when there is nothing to connect."""


class AgentList(TypedDict):
    """A page of agents."""

    object: str
    data: list[Agent]
    nextCursor: Optional[str]
    """Pass it as ``cursor`` for the next page; ``None`` on the last page."""
    hasMore: bool


class PlanLimits(TypedDict):
    """What a plan includes. ``None`` means unlimited or quoted."""

    agents: Optional[int]
    connections: Optional[int]
    messagesPerMonth: Optional[int]
    seats: Optional[int]


class Plan(TypedDict):
    """One plan."""

    id: str
    name: str
    summary: str
    forWho: str
    recommended: bool
    currency: str
    """Always ``USD``. Prices are before applicable taxes."""
    monthlyCents: int
    """The price a month in cents; for a plan that is quoted, its starting price."""
    yearlyCents: Optional[int]
    """The price a year in cents, or ``None`` when the plan is quoted."""
    selfServe: bool
    """``False`` means the plan is quoted by email rather than bought on the site."""
    limits: PlanLimits


class PlanTerms(TypedDict):
    """The terms that apply to every plan."""

    trialDays: int
    annualRefundDays: int
    setupFee: int
    contract: bool
    currency: str


class PlanList(TypedDict):
    """Every plan and the terms. There is only one page."""

    object: str
    data: list[Plan]
    terms: PlanTerms
    nextCursor: None
    hasMore: bool


class SearchHit(TypedDict):
    """One match of a search: an agent or a plan."""

    kind: str
    """``agent`` or ``plan``."""
    id: str
    name: str
    description: str
    score: float
    """Higher is a better match. The scale is not part of the API."""


class SearchResults(TypedDict):
    """The matches of a search, best first. There is only one page."""

    object: str
    query: str
    data: list[SearchHit]
    nextCursor: None
    hasMore: bool


class Problem(TypedDict, total=False):
    """An RFC 9457 problem document, as it appears in a batch item's ``body``."""

    type: str
    status: int
    code: str
    title: str
    detail: str


class _BatchItemRequired(TypedDict):
    path: str


class BatchItem(_BatchItemRequired, total=False):
    """One read in a batch."""

    id: str
    """A name for the item, unique in the batch (1 to 64 of ``A-Za-z0-9_.:-``). The item's position when left out."""
    method: str
    """Only ``GET``, which is the default."""


class BatchResult(TypedDict):
    """The answer to one item of a batch."""

    id: str
    status: int
    """The HTTP status the single endpoint would have answered."""
    body: Any
    """What the single endpoint would have returned: its JSON, or a :class:`Problem` when ``status`` is 400 or more."""


class BatchResponse(TypedDict):
    """The answers of a batch, in the order of the requests."""

    object: str
    responses: list[BatchResult]


class JobLinks(TypedDict):
    """Where a job is."""

    self: str
    """The job's address, which :meth:`~carragent.Carragent.get_job` polls."""


class JobResult(TypedDict):
    """What a job's operation returned."""

    status: int
    """The HTTP status the operation answered, which can be a 4xx: the job still succeeded."""
    body: Any


class JobAccepted(TypedDict):
    """A job that has just been created: the answer to ``POST /api/v1/jobs``, a 202 Accepted."""

    object: str
    id: str
    status: str
    """``pending``."""
    operation: str
    createdAt: str
    expiresAt: str
    """When the job stops being available: 24 hours after it was created."""
    links: JobLinks


class Job(TypedDict):
    """A job, as polled: the answer to ``GET /api/v1/jobs/{id}``."""

    object: str
    id: str
    status: str
    """``succeeded``: the job ran. ``result`` says what the operation answered."""
    operation: str
    createdAt: str
    completedAt: str
    expiresAt: str
    result: JobResult


class Registration(TypedDict):
    """An anonymous agent registration (``POST /agent/identity``)."""

    registration_id: str
    registration_type: str
    identity_assertion: str
    """A signed assertion, good for 24 hours, that :meth:`~carragent.Carragent.use_anonymous_identity` trades in."""
    assertion_expires: str
    pre_claim_scopes: list[str]
    token_endpoint: str
    grant_type: str


class Token(TypedDict):
    """An access token for ``Authorization: Bearer``."""

    access_token: str
    token_type: str
    expires_in: int
    """Seconds until it expires: an hour."""
    scope: str
