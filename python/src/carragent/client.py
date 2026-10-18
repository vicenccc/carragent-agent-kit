"""The client for Carragent's public, read-only API (https://carragent.com/developers).

It uses nothing but the standard library (``urllib``), so there is nothing to install besides this package.
"""

import base64
import http.client
import ipaddress
import json
import math
import re
import socket
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Iterator, Mapping, Sequence
from datetime import datetime, timezone
from email.message import Message
from email.utils import parsedate_to_datetime
from typing import IO, Any, NamedTuple, Optional, cast

from ._version import __version__
from .errors import CarragentError, ConnectionFailed, RateLimited
from .models import (
    Agent,
    AgentList,
    BatchItem,
    BatchResponse,
    Job,
    JobAccepted,
    JobOperation,
    Lang,
    PlanList,
    Registration,
    SearchResults,
    Token,
)

DEFAULT_BASE_URL = "https://carragent.com"
"""Where the API is."""

USER_AGENT = f"carragent-python/{__version__}"
"""What the client calls itself in the ``User-Agent`` header."""

# RFC 7523: the grant that exchanges an agent's identity assertion for an access token.
_JWT_BEARER_GRANT = "urn:ietf:params:oauth:grant-type:jwt-bearer"

# The API answers in a few kilobytes. A larger answer is not Carragent's, so it is refused instead of read into memory.
_MAX_BODY = 8 * 1024 * 1024


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Never follow a redirect: urllib would send the token to wherever it points and turn a POST into a GET."""

    def redirect_request(self, *args: Any, **kwargs: Any) -> None:
        return None


class Carragent:
    """A client for Carragent's public, read-only API: its AI agents for small businesses, and its plans and prices.

    Everything it reads is public and needs no account or key. A token is optional: it only raises the rate limit (60
    requests a minute per IP address without one, 300 a minute per client with one) and reads the same public catalog.
    Nothing here creates an account, starts a trial or touches a business's data.

    Example::

        client = Carragent()
        for hit in client.search("late invoices")["data"]:
            print(hit["kind"], hit["id"], hit["name"])

    A method that fails raises :class:`~carragent.CarragentError` or a subclass of it; one that is given an argument it
    cannot send raises :class:`ValueError`. The client keeps no connection open between calls and never retries by
    itself. The ``use_*`` methods replace :attr:`token`, so call them before you share a client between threads.

    Args:
        base_url: The API's address. It must be ``https``; ``http`` is accepted only for a server on this machine.
        token: An access token to send as ``Authorization: Bearer``, if you have one. :meth:`use_client_credentials`
            and :meth:`use_anonymous_identity` get one and set it.
        timeout: Seconds to wait for each network operation (the connection, then each read), not for the whole request.
        user_agent: The ``User-Agent`` header. It defaults to :data:`carragent.USER_AGENT`; to name your application,
            send ``"my-app/1.2 " + carragent.USER_AGENT``.

    Raises:
        ValueError: ``base_url`` is not an ``https`` address (or an ``http`` one for this machine), or ``timeout``
            is not a positive number.
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        token: Optional[str] = None,
        timeout: float = 10,
        *,
        user_agent: Optional[str] = None,
    ) -> None:
        if not (isinstance(timeout, (int, float)) and 0 < timeout < math.inf):
            raise ValueError("timeout must be a positive number of seconds.")
        self._base_url = _clean_base_url(base_url)
        self.token = token
        """The access token sent as ``Authorization: Bearer`` to the API; ``None`` sends none."""
        self.timeout = timeout
        """Seconds to wait for each network operation."""
        self.user_agent = user_agent or USER_AGENT
        """The ``User-Agent`` header."""
        self._opener = urllib.request.build_opener(_NoRedirect)

    @property
    def base_url(self) -> str:
        """The API's address, without a trailing slash."""
        return self._base_url

    def __repr__(self) -> str:
        token = "None" if self.token is None else "'***'"
        return f"Carragent(base_url={self._base_url!r}, token={token}, timeout={self.timeout!r})"

    # ---- the catalog ----------------------------------------------------------------------------------------------

    def list_agents(
        self,
        *,
        lang: Optional[Lang] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
    ) -> AgentList:
        """One page of the agents: ``GET /api/v1/agents``.

        Args:
            lang: The language of the text: ``en`` (the default), ``es`` or ``fr``.
            limit: The page size, 1 to 50 (the API's default is 20).
            cursor: The ``nextCursor`` of the previous page.
        """
        query = {"lang": lang, "limit": None if limit is None else str(limit), "cursor": cursor}
        return cast(AgentList, self._request("GET", "/api/v1/agents", query=query))

    def iter_agents(self, *, lang: Optional[Lang] = None, page_size: int = 50) -> Iterator[Agent]:
        """Every agent, one at a time. It follows ``nextCursor`` and asks for a page only when you reach it.

        Args:
            lang: The language of the text.
            page_size: How many agents each request asks for, 1 to 50.
        """
        cursor: Optional[str] = None
        while True:
            page = self.list_agents(lang=lang, limit=page_size, cursor=cursor)
            yield from page["data"]
            following = page.get("nextCursor")
            if not page.get("hasMore") or not following or following == cursor:
                return
            cursor = following

    def get_agent(self, id: str, *, lang: Optional[Lang] = None) -> Agent:
        """One agent by its id, such as ``collections``: ``GET /api/v1/agents/{id}``.

        Raises:
            CarragentError: ``agent_not_found`` (404) when there is no agent with that id.
            ValueError: ``id`` is empty.
        """
        path = "/api/v1/agents/" + _segment(id, "id")
        return cast(Agent, self._request("GET", path, query={"lang": lang}))

    def list_plans(self, *, lang: Optional[Lang] = None) -> PlanList:
        """The plans, with their prices in US cents (before taxes), their limits and the trial and refund terms.

        ``GET /api/v1/plans``. There is one page: every plan.
        """
        return cast(PlanList, self._request("GET", "/api/v1/plans", query={"lang": lang}))

    def search(self, q: str, *, lang: Optional[Lang] = None) -> SearchResults:
        """A keyword search over the agents and the plans, best match first: ``GET /api/v1/search``.

        It matches the words of ``q`` (those of three letters or more) against the text of the agents and plans in the
        language ``lang``, ignoring case and accents. It is not a semantic search, so use the words the agents use,
        such as ``late invoices`` or ``appointment reminders``. It returns at most 10 matches.

        Args:
            q: What to look for, 1 to 200 characters.
            lang: The language to search and answer in.
        """
        return cast(SearchResults, self._request("GET", "/api/v1/search", query={"q": q, "lang": lang}))

    # ---- several reads at once, and jobs ---------------------------------------------------------------------------

    def batch(self, requests: Sequence[BatchItem]) -> BatchResponse:
        """Up to 20 reads in one request: ``POST /api/v1/batch``.

        Each item is ``{"path": "/agents?limit=5"}``, with an optional ``"id"`` that names its answer. The paths are the
        API's reads (``/agents``, ``/agents/{id}``, ``/plans``, ``/search?q=...``), relative to ``/api/v1``. The answers
        come back in order. An item that failed carries its problem in its ``body`` and its own ``status``: this method
        raises only when the whole request is refused. Every item counts as one request against the rate limit.

        Example::

            answers = client.batch([{"id": "plans", "path": "/plans"}, {"path": "/agents/collections"}])
            for answer in answers["responses"]:
                print(answer["id"], answer["status"])
        """
        return cast(BatchResponse, self._request("POST", "/api/v1/batch", body={"requests": list(requests)}))

    def create_job(self, operation: JobOperation, params: Optional[Mapping[str, Any]] = None) -> JobAccepted:
        """Start a job: ``POST /api/v1/jobs``, which answers ``202 Accepted``.

        This is the REST pattern for slow operations, so that code written for one works here unchanged. The operations
        are fast reads: nothing is queued, and a job's result is worked out when you poll it, from the catalog as it is
        then. Poll with :meth:`get_job` or :meth:`wait_job`.

        Args:
            operation: ``listAgents``, ``getAgent``, ``listPlans``, ``searchCatalog`` or ``batch``.
            params: The operation's query parameters as strings (``lang``, ``limit``, ``cursor``, ``id``, ``q``), or
                ``{"requests": [...]}`` for ``batch``.
        """
        body = {"operation": operation, "params": dict(params) if params is not None else {}}
        return cast(JobAccepted, self._request("POST", "/api/v1/jobs", body=body))

    def get_job(self, id: str) -> Job:
        """A job, with its result once it has run: ``GET /api/v1/jobs/{id}``.

        Raises:
            CarragentError: ``job_not_found`` (404) for an id the API does not recognise, ``job_expired`` (410) once the
                job is older than 24 hours.
            ValueError: ``id`` is empty.
        """
        return cast(Job, self._request("GET", "/api/v1/jobs/" + _segment(id, "id")))

    def wait_job(self, id: str, timeout: float = 30.0, *, interval: float = 1.0) -> Job:
        """Poll a job until its status is no longer ``pending``, and return it.

        Today a job has its result on the first poll, so this returns at once. Every poll runs the job again and costs
        what the job costs against the rate limit: one request, or one per item of a batch.

        Args:
            id: The job's id.
            timeout: Seconds to wait in all before giving up.
            interval: Seconds between polls.

        Raises:
            TimeoutError: The job was still pending after ``timeout`` seconds.
            ValueError: ``id`` is empty, or ``timeout`` or ``interval`` is not a positive number.
        """
        if not (0 < timeout < math.inf and 0 < interval < math.inf):
            raise ValueError("timeout and interval must be positive numbers of seconds.")
        deadline = time.monotonic() + timeout
        while True:
            job = self.get_job(id)
            if job["status"] != "pending":
                return job
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError(f"Job {id} was still pending after {timeout:g} seconds.")
            time.sleep(min(interval, remaining))

    # ---- tokens (optional) ----------------------------------------------------------------------------------------

    def register_anonymous(self) -> Registration:
        """Register this agent anonymously: ``POST /agent/identity``.

        The answer holds an ``identity_assertion`` that is good for 24 hours. There is nothing to claim and no account
        behind it: it only gives your agent an identity of its own, so that its rate limit is its own. Keep the answer
        and pass it to :meth:`use_anonymous_identity` to get a token.

        Raises:
            CarragentError: ``unsupported_identity_type`` (400), ``rate_limited`` (429: registrations are limited to 10
                a minute per IP address), or ``temporarily_unavailable`` (503) when the server cannot sign assertions.
        """
        body = cast(Registration, self._request("POST", "/agent/identity", body={"type": "anonymous"}, bearer=False))
        if not isinstance(body.get("identity_assertion"), str):
            raise CarragentError(
                200, "invalid_response", "Unexpected response", "The answer has no identity_assertion."
            )
        return body

    def use_anonymous_identity(self, registration: Optional[Registration] = None) -> Token:
        """Get a token for an anonymous agent identity, and use it from now on.

        It registers first (:meth:`register_anonymous`), unless you pass the ``registration`` you kept from an earlier
        call. It then exchanges the assertion for an access token at ``/api/oauth2/token`` (RFC 7523, the
        ``jwt-bearer`` grant) and sets :attr:`token`. The token lasts an hour; call this again, with the same
        registration while it is valid, to get a new one. The assertion goes to this client's ``base_url``, whatever
        address the registration names.

        Returns:
            The token response; its ``expires_in`` is in seconds.

        Raises:
            CarragentError: ``invalid_grant`` (400) when the assertion is expired or not Carragent's.
            ValueError: ``registration`` has no ``identity_assertion``.
        """
        if registration is None:
            registration = self.register_anonymous()
        assertion = registration.get("identity_assertion")
        if not isinstance(assertion, str) or not assertion:
            raise ValueError("registration must be the answer of register_anonymous(), with an identity_assertion.")
        token = self._token({"grant_type": _JWT_BEARER_GRANT, "assertion": assertion})
        self.token = token["access_token"]
        return token

    def use_client_credentials(self, client_id: str, secret: str, *, scope: Optional[str] = None) -> Token:
        """Get a token with an OAuth client's id and secret, and use it from now on (RFC 6749, client credentials).

        The id and secret go to ``/api/oauth2/token`` as HTTP Basic credentials, and nowhere else. They are not kept.
        The token lasts an hour; call this again for a new one. Register a client at ``POST /api/oauth2/register``
        (https://carragent.com/auth.md has the steps).

        Args:
            client_id: The client's id.
            secret: The client's secret.
            scope: Space-separated scopes within the client's own (``agents:read plans:read search:read``). The client's
                own scopes when left out.

        Returns:
            The token response; its ``expires_in`` is in seconds.

        Raises:
            CarragentError: ``invalid_client`` (401) for an unknown client or a wrong secret, ``invalid_scope`` (400).
        """
        credentials = urllib.parse.quote(client_id, safe="") + ":" + urllib.parse.quote(secret, safe="")
        basic = base64.b64encode(credentials.encode("utf-8")).decode("ascii")
        form = {"grant_type": "client_credentials", **({"scope": scope} if scope else {})}
        token = self._token(form, {"Authorization": "Basic " + basic})
        self.token = token["access_token"]
        return token

    def _token(self, form: Mapping[str, str], headers: Optional[Mapping[str, str]] = None) -> Token:
        token = cast(Token, self._request("POST", "/api/oauth2/token", form=form, headers=headers, bearer=False))
        if not isinstance(token.get("access_token"), str):
            raise CarragentError(200, "invalid_response", "Unexpected response", "The answer has no access_token.")
        return token

    # ---- the request ----------------------------------------------------------------------------------------------

    def _request(
        self,
        method: str,
        path: str,
        *,
        query: Optional[Mapping[str, Optional[str]]] = None,
        body: Any = None,
        form: Optional[Mapping[str, str]] = None,
        headers: Optional[Mapping[str, str]] = None,
        bearer: bool = True,
    ) -> Any:
        """One request: the parsed JSON of a 2xx answer, else the exception for what went wrong."""
        url = self._base_url + path
        pairs = [(name, value) for name, value in (query or {}).items() if value is not None]
        if pairs:
            url += "?" + urllib.parse.urlencode(pairs, quote_via=urllib.parse.quote)

        sent: dict[str, str] = {"Accept": "application/json, application/problem+json", "User-Agent": self.user_agent}
        data: Optional[bytes] = None
        if body is not None:
            data = json.dumps(body, separators=(",", ":")).encode("utf-8")
            sent["Content-Type"] = "application/json"
        elif form is not None:
            data = urllib.parse.urlencode(form).encode("ascii")
            sent["Content-Type"] = "application/x-www-form-urlencoded"
        if bearer and self.token:
            sent["Authorization"] = "Bearer " + self.token
        sent.update(headers or {})

        answer = self._exchange(urllib.request.Request(url, data=data, headers=sent, method=method))
        if len(answer.raw) > _MAX_BODY:
            raise CarragentError(
                answer.status,
                "response_too_large",
                "Response too large",
                "The answer is over 8 MiB, so it is not Carragent's.",
            )
        if 200 <= answer.status < 300:
            parsed = _json_or_none(answer.raw)
            if parsed is None:
                raise CarragentError(
                    answer.status, "invalid_response", "Unexpected response", "The API answered, but not with JSON."
                )
            return parsed
        raise _error_for(answer.status, answer.raw, answer.retry_after, answer.location)

    def _exchange(self, request: urllib.request.Request) -> "_Answer":
        """Send a request and read the whole answer, whatever its status. No answer at all is ConnectionFailed."""
        try:
            try:
                with self._opener.open(request, timeout=self.timeout) as response:
                    return _read_answer(response.status, response.headers, response)
            except urllib.error.HTTPError as error:
                # An error status is an answer too: read it, then let go of the connection.
                try:
                    return _read_answer(error.code, error.headers, error.fp)
                finally:
                    error.close()
        except (OSError, http.client.HTTPException) as failure:
            raise _connection_failed(failure) from failure


# ---- helpers ------------------------------------------------------------------------------------------------------


class _Answer(NamedTuple):
    """What came back: the status, the body, and the two headers the client reads."""

    status: int
    raw: bytes
    retry_after: Optional[str]
    location: Optional[str]


def _read_answer(status: int, headers: Message, body: Optional[IO[bytes]]) -> _Answer:
    """Read a body up to the limit, plus one byte to tell it is over. A body shorter than announced is a failure."""
    raw = body.read(_MAX_BODY + 1) if body is not None else b""
    missing = getattr(body, "length", None)  # http.client: the bytes Content-Length promised that have not come
    if len(raw) <= _MAX_BODY and missing:
        raise http.client.IncompleteRead(raw, missing)
    return _Answer(status, raw, headers.get("Retry-After"), headers.get("Location"))


def _clean_base_url(value: str) -> str:
    """The base URL without its trailing slash, or ValueError: https (http for this machine), nothing but an address."""
    if not isinstance(value, str):
        raise ValueError(f"base_url must be an https address such as {DEFAULT_BASE_URL!r}.")
    value = value.strip()
    parts = urllib.parse.urlsplit(value)
    host = parts.hostname
    if parts.scheme not in ("http", "https") or not host:
        raise ValueError(f"base_url must be an https address such as {DEFAULT_BASE_URL!r}, not {value!r}.")
    if parts.username is not None or parts.password is not None or parts.query or parts.fragment:
        raise ValueError("base_url must be an address only: no credentials, query or fragment.")
    if parts.scheme == "http" and not _is_local(host):
        raise ValueError(
            "base_url must be https, so that a token never crosses the network in the clear (http works for localhost)."
        )
    try:
        host.encode("idna")
    except UnicodeError:
        raise ValueError(f"base_url has an invalid host name: {host!r}.") from None
    return value.rstrip("/")


def _is_local(host: str) -> bool:
    if host == "localhost":
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def _segment(value: str, what: str) -> str:
    """A value as one path segment, or ValueError when it cannot name anything."""
    if not isinstance(value, str) or value in ("", ".", ".."):
        raise ValueError(f"{what} must be a non-empty string, such as 'collections'.")
    return urllib.parse.quote(value, safe="")


def _connection_failed(failure: BaseException) -> ConnectionFailed:
    reason = failure.reason if isinstance(failure, urllib.error.URLError) else failure
    if isinstance(reason, (socket.timeout, TimeoutError)):
        return ConnectionFailed(
            "timeout", "Request timed out", "The API did not answer in time. Raise timeout, or try again."
        )
    return ConnectionFailed("connection_failed", "Could not reach the API", str(reason) or type(reason).__name__)


def _json_or_none(raw: bytes) -> Any:
    try:
        return json.loads(raw.decode("utf-8")) if raw else None
    except (UnicodeDecodeError, ValueError):
        return None


def _retry_after(value: Optional[str]) -> Optional[float]:
    """The ``Retry-After`` header as seconds: delta-seconds or an HTTP date (RFC 9110), else None."""
    if value is None:
        return None
    value = value.strip()
    if re.fullmatch(r"[0-9]+", value):
        seconds = float(value)
        return seconds if math.isfinite(seconds) else None
    try:
        when = parsedate_to_datetime(value)
    except (TypeError, ValueError):
        return None
    if when.tzinfo is None:
        when = when.replace(tzinfo=timezone.utc)
    return max(0.0, (when - datetime.now(timezone.utc)).total_seconds())


def _problem_fields(body: Any) -> tuple[Optional[str], Optional[str], Optional[str]]:
    """The code, title and detail of an RFC 9457 problem or an RFC 6749 error body; None for what is not there."""
    if not isinstance(body, dict):
        return None, None, None
    fields = cast("dict[str, Any]", body)

    def text(key: str) -> Optional[str]:
        value = fields.get(key)
        return value if isinstance(value, str) and value else None

    if text("code") is None and text("error") is not None:
        return text("error"), text("error_description") or text("error"), None
    return text("code"), text("title"), text("detail")


def _error_for(status: int, raw: bytes, retry_after: Optional[str], location: Optional[str]) -> CarragentError:
    """The exception for an error answer."""
    code, title, detail = _problem_fields(_json_or_none(raw))
    if status == 429:
        return RateLimited(_retry_after(retry_after), title or "Too many requests", detail, code or "rate_limited")
    if 300 <= status < 400:
        where = f"The API redirected to {location}. " if location else "The API redirected. "
        return CarragentError(
            status,
            "unexpected_redirect",
            "Unexpected redirect",
            where + "The client does not follow redirects: set base_url to the final address.",
        )
    return CarragentError(
        status, code or "http_error", title or http.client.responses.get(status, f"HTTP {status}"), detail
    )
