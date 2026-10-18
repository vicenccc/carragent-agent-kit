"""Python client for Carragent's public, read-only API: its AI agents for small businesses, and its plans and prices.

The API is public and needs no key. Nothing in this package signs anyone up, changes anything or reaches a
business's data.

Example::

    from carragent import Carragent

    client = Carragent()
    for hit in client.search("late invoices")["data"]:
        print(hit["kind"], hit["id"], hit["name"])

Docs: https://carragent.com/developers
"""

from ._version import __version__
from .client import DEFAULT_BASE_URL, USER_AGENT, Carragent
from .errors import CarragentError, ConnectionFailed, RateLimited
from .models import (
    Agent,
    AgentList,
    BatchItem,
    BatchResponse,
    BatchResult,
    Job,
    JobAccepted,
    JobLinks,
    JobOperation,
    JobResult,
    Lang,
    Plan,
    PlanLimits,
    PlanList,
    PlanTerms,
    Problem,
    Registration,
    SearchHit,
    SearchResults,
    Token,
)

__all__ = [
    "DEFAULT_BASE_URL",
    "USER_AGENT",
    "Agent",
    "AgentList",
    "BatchItem",
    "BatchResponse",
    "BatchResult",
    "Carragent",
    "CarragentError",
    "ConnectionFailed",
    "Job",
    "JobAccepted",
    "JobLinks",
    "JobOperation",
    "JobResult",
    "Lang",
    "Plan",
    "PlanLimits",
    "PlanList",
    "PlanTerms",
    "Problem",
    "RateLimited",
    "Registration",
    "SearchHit",
    "SearchResults",
    "Token",
    "__version__",
]
