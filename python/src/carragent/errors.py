"""What goes wrong, as exceptions.

A request that fails raises a :class:`CarragentError`, so ``except CarragentError`` catches them all:

* an error answer from the API: an RFC 9457 problem (``application/problem+json``) becomes a plain
  :class:`CarragentError` carrying its ``status``, ``code``, ``title`` and ``detail``; a 429 becomes
  :class:`RateLimited`;
* a request that never got an answer (no network, a refused connection, a timeout) becomes :class:`ConnectionFailed`.

Branch on ``code``, which is stable, rather than on ``title`` or ``detail``, which are for people.

Two more exceptions are Python's own: :class:`ValueError` for an argument the client cannot send (an empty id, say), and
:class:`TimeoutError` from :meth:`~carragent.Carragent.wait_job` when its own time runs out.
"""

from typing import Optional


class CarragentError(Exception):
    """An error answer from the API, or a failure to get an answer.

    Attributes:
        status: The HTTP status, or ``0`` when there was no answer.
        code: The API's stable, machine-readable error code, such as ``agent_not_found``, ``invalid_parameter`` or
            ``rate_limited``; for the OAuth endpoints, the RFC 6749 ``error`` (``invalid_client``, ``invalid_grant``).
            The client adds four of its own: ``http_error`` (an error answer that is not a problem document, such as a
            gateway's HTML page), ``invalid_response`` (a success answer that is not the JSON it should be),
            ``unexpected_redirect`` and ``response_too_large``.
        title: A short summary of the problem.
        detail: What went wrong this time, when the API says; otherwise ``None``.
    """

    def __init__(self, status: int, code: str, title: str, detail: Optional[str] = None) -> None:
        # Exactly the constructor's arguments, so the exception can be copied and pickled.
        super().__init__(status, code, title, detail)
        self.status = status
        self.code = code
        self.title = title
        self.detail = detail

    def __str__(self) -> str:
        where = f"{self.code}, HTTP {self.status}" if self.status else self.code
        text = f"{self.title} ({where})"
        return f"{text}: {self.detail}" if self.detail else text


class RateLimited(CarragentError):
    """The API answered 429: too many requests.

    Without a token the limit is per IP address, with one it is per client, both over a window of a minute.
    Wait ``retry_after`` seconds and try again. The client never retries by itself.

    Attributes:
        retry_after: The ``Retry-After`` header, in seconds, or ``None`` when the answer had none.
    """

    def __init__(
        self,
        retry_after: Optional[float] = None,
        title: str = "Too many requests",
        detail: Optional[str] = None,
        code: str = "rate_limited",
    ) -> None:
        super().__init__(429, code, title, detail)
        # The constructor's own arguments, so the exception can be copied and pickled.
        self.args = (retry_after, title, detail, code)
        self.retry_after = retry_after

    def __str__(self) -> str:
        text = super().__str__()
        if self.retry_after is None:
            return text
        wait = float(self.retry_after)
        seconds = int(wait) if wait.is_integer() else wait
        unit = "second" if seconds == 1 else "seconds"
        return f"{text.rstrip('.')}. Retry after {seconds} {unit}."


class ConnectionFailed(CarragentError):
    """The request got no answer: the host could not be reached, the connection broke, or it timed out.

    ``status`` is ``0``. ``code`` is ``timeout`` when the time given to :class:`~carragent.Carragent` ran out and
    ``connection_failed`` for anything else; the underlying error is ``__cause__``.
    """

    def __init__(self, code: str, title: str, detail: Optional[str] = None) -> None:
        super().__init__(0, code, title, detail)
        self.args = (code, title, detail)
