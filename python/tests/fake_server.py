"""A server on this machine for the tests: it answers what a test tells it to, and records what it was asked."""

import json
import socketserver
import sys
import threading
import time
import unittest
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable, Optional, cast
from urllib.parse import parse_qsl

from carragent import Carragent


@dataclass
class Recorded:
    """One request the server received."""

    method: str
    target: str
    """The path and the query, exactly as the client wrote them."""
    headers: dict[str, str]
    """The headers, with lower-case names."""
    body: bytes

    @property
    def path(self) -> str:
        return self.target.split("?", 1)[0]

    @property
    def query(self) -> list[tuple[str, str]]:
        return parse_qsl(self.target.partition("?")[2], keep_blank_values=True)

    def json(self) -> Any:
        return json.loads(self.body)

    def form(self) -> dict[str, str]:
        return dict(parse_qsl(self.body.decode("ascii"), keep_blank_values=True))


@dataclass
class Reply:
    """What to answer: a status, headers and a body (a dict or list is sent as JSON, a str or bytes as it is)."""

    status: int = 200
    body: Any = None
    headers: dict[str, str] = field(default_factory=dict[str, str])
    content_type: Optional[str] = None
    delay: float = 0.0
    """Seconds to wait before answering."""
    short_by: int = 0
    """Announce this many more bytes than are sent, then hang up."""
    hang_up: bool = False
    """Close the connection without answering."""


def problem(
    status: int, code: str, title: str, detail: Optional[str] = None, headers: Optional[dict[str, str]] = None
) -> Reply:
    """An RFC 9457 answer like the API's."""
    body: dict[str, Any] = {"type": "about:blank", "status": status, "code": code, "title": title}
    if detail:
        body["detail"] = detail
    return Reply(status, body, dict(headers or {}), "application/problem+json; charset=utf-8")


class _Server(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, api: "FakeApi") -> None:
        super().__init__(("127.0.0.1", 0), _Handler)
        self.api = api

    def handle_error(self, request: Any, client_address: Any) -> None:
        # A client that gave up (a timeout test) leaves a broken pipe behind. Anything else is a bug in the test.
        if not isinstance(sys.exc_info()[1], (ConnectionError, TimeoutError)):
            super().handle_error(request, client_address)


class _Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _serve(self) -> None:
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else b""
        headers = {name.lower(): value for name, value in self.headers.items()}
        reply = cast(_Server, self.server).api.answer(Recorded(self.command, self.path, headers, body))
        if reply.delay:
            time.sleep(reply.delay)
        if reply.hang_up:
            self.close_connection = True
            return
        payload, content_type = _encode(reply)
        self.send_response(reply.status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload) + reply.short_by))
        for name, value in reply.headers.items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(payload)
        if reply.short_by:
            self.wfile.flush()
            self.close_connection = True

    do_GET = do_POST = do_PUT = do_DELETE = do_OPTIONS = _serve

    def log_message(self, format: str, *args: Any) -> None:
        pass


def _encode(reply: Reply) -> tuple[bytes, str]:
    body: Any = reply.body
    if isinstance(body, (dict, list)):
        return json.dumps(body).encode("utf-8"), reply.content_type or "application/json; charset=utf-8"
    if isinstance(body, bytes):
        return body, reply.content_type or "application/octet-stream"
    return str(body or "").encode("utf-8"), reply.content_type or "text/plain; charset=utf-8"


class FakeApi:
    """A server that answers the scripted replies in order (or whatever ``handler`` returns) and keeps every request."""

    def __init__(self, *replies: Reply, handler: Optional[Callable[[Recorded], Reply]] = None) -> None:
        self.requests: list[Recorded] = []
        self._replies = list(replies)
        self._handler = handler
        self._lock = threading.Lock()
        self._server = _Server(self)
        self._thread = threading.Thread(target=self._server.serve_forever, kwargs={"poll_interval": 0.02}, daemon=True)

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self._server.server_address[1]}"

    def start(self) -> None:
        self._thread.start()

    def stop(self) -> None:
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(5)

    def answer(self, recorded: Recorded) -> Reply:
        with self._lock:
            self.requests.append(recorded)
            if self._handler is not None:
                return self._handler(recorded)
            if self._replies:
                return self._replies.pop(0)
        return problem(500, "unscripted", "The test did not script this request")


class ApiTestCase(unittest.TestCase):
    """A test case that can start servers, which it stops afterwards."""

    def serve(self, *replies: Reply, handler: Optional[Callable[[Recorded], Reply]] = None) -> FakeApi:
        api = FakeApi(*replies, handler=handler)
        api.start()
        self.addCleanup(api.stop)
        return api

    def client(self, api: FakeApi, **options: Any) -> Carragent:
        return Carragent(api.url, **options)

    def free_port(self) -> int:
        """A port nothing listens on."""
        with socketserver.TCPServer(("127.0.0.1", 0), socketserver.BaseRequestHandler) as server:
            return int(server.server_address[1])
