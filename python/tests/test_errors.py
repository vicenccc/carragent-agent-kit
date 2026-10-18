"""What goes wrong: error answers, rate limits, and requests that get no answer."""

import copy
import email.utils
import pickle
import socket
import time
import unittest
from unittest import mock

import carragent.client
from carragent import Carragent, CarragentError, ConnectionFailed, RateLimited

from . import samples
from .fake_server import ApiTestCase, Reply, problem


class ExceptionsTest(unittest.TestCase):
    def test_carragent_error_keeps_its_fields_and_reads_well(self) -> None:
        error = CarragentError(404, "agent_not_found", "No agent with that id", "See /api/v1/agents.")
        self.assertEqual(
            (error.status, error.code, error.title, error.detail),
            (404, "agent_not_found", "No agent with that id", "See /api/v1/agents."),
        )
        self.assertEqual(str(error), "No agent with that id (agent_not_found, HTTP 404): See /api/v1/agents.")
        self.assertEqual(str(CarragentError(500, "http_error", "Server Error")), "Server Error (http_error, HTTP 500)")
        self.assertIsInstance(error, Exception)

    def test_rate_limited_is_a_carragent_error_with_the_wait(self) -> None:
        error = RateLimited(7, detail="Limit: 60 requests every 60 seconds.")
        self.assertIsInstance(error, CarragentError)
        self.assertEqual(
            (error.status, error.code, error.title, error.retry_after), (429, "rate_limited", "Too many requests", 7)
        )
        self.assertEqual(
            str(error),
            "Too many requests (rate_limited, HTTP 429): Limit: 60 requests every 60 seconds. Retry after 7 seconds.",
        )
        self.assertEqual(str(RateLimited(2.5)), "Too many requests (rate_limited, HTTP 429). Retry after 2.5 seconds.")
        self.assertEqual(str(RateLimited(7.0)), "Too many requests (rate_limited, HTTP 429). Retry after 7 seconds.")
        self.assertEqual(str(RateLimited(1)), "Too many requests (rate_limited, HTTP 429). Retry after 1 second.")
        self.assertEqual(str(RateLimited(None)), "Too many requests (rate_limited, HTTP 429)")
        self.assertIsNone(RateLimited().retry_after)

    def test_connection_failed_has_no_status(self) -> None:
        error = ConnectionFailed("timeout", "Request timed out", "Try again.")
        self.assertIsInstance(error, CarragentError)
        self.assertEqual((error.status, error.code), (0, "timeout"))
        self.assertEqual(str(error), "Request timed out (timeout): Try again.")

    def test_they_survive_pickle_and_copy(self) -> None:
        errors: list[CarragentError] = [
            CarragentError(404, "agent_not_found", "No agent with that id", "See /api/v1/agents."),
            RateLimited(7.5, "Slow down", "Later.", "custom_code"),
            RateLimited(),
            ConnectionFailed("connection_failed", "Could not reach the API", "refused"),
        ]
        for error in errors:
            clones: list[CarragentError] = [pickle.loads(pickle.dumps(error)), copy.copy(error), copy.deepcopy(error)]
            for clone in clones:
                with self.subTest(error=repr(error), clone=type(clone).__name__):
                    self.assertIs(type(clone), type(error))
                    self.assertEqual(vars(clone), vars(error))
                    self.assertEqual(str(clone), str(error))


class ErrorAnswersTest(ApiTestCase):
    def fail_with(self, *replies: Reply) -> CarragentError:
        api = self.serve(*replies)
        with self.assertRaises(CarragentError) as caught:
            self.client(api).list_plans()
        return caught.exception

    def test_a_problem_document_becomes_a_carragent_error(self) -> None:
        error = self.fail_with(problem(400, "invalid_parameter", "Invalid lang", "lang must be one of en, es, fr."))
        self.assertIs(type(error), CarragentError)
        self.assertEqual(
            (error.status, error.code, error.title, error.detail),
            (400, "invalid_parameter", "Invalid lang", "lang must be one of en, es, fr."),
        )

    def test_a_problem_without_detail_has_none(self) -> None:
        self.assertIsNone(self.fail_with(problem(404, "not_found", "No such endpoint")).detail)

    def test_401_and_403_carry_the_apis_code(self) -> None:
        for status, code in ((401, "invalid_token"), (403, "insufficient_scope")):
            with self.subTest(status=status):
                error = self.fail_with(
                    problem(status, code, "Nope", "x", headers={"WWW-Authenticate": 'Bearer error="x"'})
                )
                self.assertEqual((error.status, error.code), (status, code))

    def test_an_oauth_error_uses_its_error_as_the_code(self) -> None:
        body = {"error": "invalid_client", "error_description": "Unknown client or wrong secret."}
        error = self.fail_with(Reply(401, body, {"WWW-Authenticate": 'Basic realm="carragent"'}))
        self.assertEqual(
            (error.status, error.code, error.title), (401, "invalid_client", "Unknown client or wrong secret.")
        )
        self.assertIsNone(error.detail)

    def test_an_oauth_error_without_a_description_still_has_a_title(self) -> None:
        error = self.fail_with(Reply(400, {"error": "invalid_request"}))
        self.assertEqual((error.code, error.title), ("invalid_request", "invalid_request"))

    def test_a_page_that_is_not_json_is_an_http_error(self) -> None:
        error = self.fail_with(Reply(502, "<html>Bad gateway</html>", content_type="text/html"))
        self.assertEqual(
            (error.status, error.code, error.title, error.detail), (502, "http_error", "Bad Gateway", None)
        )

    def test_an_empty_error_body_is_an_http_error(self) -> None:
        error = self.fail_with(Reply(503))
        self.assertEqual((error.status, error.code, error.title), (503, "http_error", "Service Unavailable"))

    def test_an_error_body_of_the_wrong_shape_is_an_http_error(self) -> None:
        for body in ([1, 2], {"code": 5, "title": ["x"]}, "just text"):
            with self.subTest(body=body):
                error = self.fail_with(Reply(500, body))
                self.assertEqual((error.status, error.code, error.title), (500, "http_error", "Internal Server Error"))

    def test_an_unusual_status_still_gets_a_title(self) -> None:
        self.assertEqual(self.fail_with(Reply(599)).title, "HTTP 599")

    def test_a_success_that_is_not_json_is_an_invalid_response(self) -> None:
        for reply in (
            Reply(200, "<html>hello</html>", content_type="text/html"),
            Reply(200),
            Reply(200, "null"),
            Reply(204),
        ):
            with self.subTest(status=reply.status, body=reply.body):
                error = self.fail_with(reply)
                self.assertEqual((error.status, error.code), (reply.status, "invalid_response"))


class RateLimitTest(ApiTestCase):
    def limited(self, *headers: str) -> RateLimited:
        api = self.serve(
            problem(
                429,
                "rate_limited",
                "Too many requests",
                "Limit: 60 requests every 60 seconds.",
                dict(h.split(": ", 1) for h in headers),
            )
        )
        with self.assertRaises(RateLimited) as caught:
            self.client(api).search("anything")
        return caught.exception

    def test_a_429_is_rate_limited_with_retry_after_in_seconds(self) -> None:
        error = self.limited("Retry-After: 7")
        self.assertEqual(
            (error.status, error.code, error.title, error.retry_after), (429, "rate_limited", "Too many requests", 7.0)
        )
        self.assertEqual(error.detail, "Limit: 60 requests every 60 seconds.")
        self.assertIsInstance(error, CarragentError)

    def test_retry_after_may_be_an_http_date(self) -> None:
        later = email.utils.formatdate(time.time() + 30, usegmt=True)
        wait = self.limited(f"Retry-After: {later}").retry_after
        assert wait is not None
        self.assertTrue(25 <= wait <= 31, wait)

    def test_an_http_date_in_the_old_minus_zero_zone_is_utc(self) -> None:
        later = email.utils.formatdate(time.time() + 30)  # "-0000": the zone is unknown, and Python reads it as naive
        self.assertTrue(later.endswith("-0000"))
        wait = self.limited(f"Retry-After: {later}").retry_after
        assert wait is not None
        self.assertTrue(25 <= wait <= 31, wait)

    def test_a_date_in_the_past_means_no_wait(self) -> None:
        self.assertEqual(self.limited("Retry-After: Wed, 21 Oct 2015 07:28:00 GMT").retry_after, 0.0)

    def test_a_retry_after_it_cannot_read_is_none(self) -> None:
        for value in ("soon", "-5", "1e3", "1_0", "5s", "", "nan", "inf", "\u00b2", "9" * 400):
            with self.subTest(value=value):
                self.assertIsNone(self.limited(f"Retry-After: {value}").retry_after)

    def test_no_retry_after_is_none(self) -> None:
        self.assertIsNone(self.limited().retry_after)

    def test_a_429_that_is_not_a_problem_is_still_rate_limited(self) -> None:
        api = self.serve(Reply(429, "<html>slow down</html>", {"Retry-After": "12"}, "text/html"))
        with self.assertRaises(RateLimited) as caught:
            self.client(api).list_plans()
        error = caught.exception
        self.assertEqual((error.code, error.title, error.retry_after), ("rate_limited", "Too many requests", 12.0))

    def test_the_client_does_not_retry_by_itself(self) -> None:
        api = self.serve(
            problem(429, "rate_limited", "Too many requests", headers={"Retry-After": "1"}), Reply(body=samples.PLANS)
        )
        with self.assertRaises(RateLimited), mock.patch("time.sleep") as sleep:
            self.client(api).list_plans()
        self.assertEqual(len(api.requests), 1)
        sleep.assert_not_called()


class NoAnswerTest(ApiTestCase):
    def test_a_refused_connection(self) -> None:
        client = Carragent(f"http://127.0.0.1:{self.free_port()}", timeout=2)
        with self.assertRaises(ConnectionFailed) as caught:
            client.list_plans()
        error = caught.exception
        self.assertEqual((error.status, error.code, error.title), (0, "connection_failed", "Could not reach the API"))
        self.assertTrue(error.detail)
        self.assertIsInstance(error.__cause__, OSError)

    def test_a_name_that_does_not_resolve(self) -> None:
        failure = socket.gaierror(-2, "Name or service not known")
        with mock.patch("socket.getaddrinfo", side_effect=failure), self.assertRaises(ConnectionFailed) as caught:
            Carragent("https://carragent.example").list_plans()
        self.assertEqual(caught.exception.code, "connection_failed")
        self.assertIn("Name or service not known", caught.exception.detail or "")

    def test_a_server_that_answers_too_slowly_is_a_timeout(self) -> None:
        api = self.serve(Reply(body=samples.PLANS, delay=1.5))
        started = time.monotonic()
        with self.assertRaises(ConnectionFailed) as caught:
            self.client(api, timeout=0.2).list_plans()
        self.assertEqual((caught.exception.status, caught.exception.code), (0, "timeout"))
        self.assertLess(time.monotonic() - started, 1.2)

    def test_a_server_that_hangs_up_without_answering(self) -> None:
        api = self.serve(Reply(hang_up=True))
        with self.assertRaises(ConnectionFailed) as caught:
            self.client(api).list_plans()
        self.assertEqual(caught.exception.code, "connection_failed")

    def test_an_answer_cut_short_is_a_connection_failure_not_bad_json(self) -> None:
        api = self.serve(Reply(body=samples.PLANS, short_by=50))
        with self.assertRaises(ConnectionFailed) as caught:
            self.client(api).list_plans()
        self.assertEqual(caught.exception.code, "connection_failed")

    def test_an_error_answer_cut_short_is_a_connection_failure_too(self) -> None:
        api = self.serve(Reply(500, {"code": "x", "title": "y"}, short_by=50))
        with self.assertRaises(ConnectionFailed):
            self.client(api).list_plans()


class SafetyTest(ApiTestCase):
    def test_a_redirect_is_not_followed_and_the_token_goes_nowhere_else(self) -> None:
        elsewhere = self.serve(Reply(body=samples.PLANS))
        api = self.serve(Reply(302, headers={"Location": elsewhere.url + "/api/v1/plans"}))
        with self.assertRaises(CarragentError) as caught:
            self.client(api, token=samples.FAKE_TOKEN).list_plans()
        error = caught.exception
        self.assertEqual((error.status, error.code), (302, "unexpected_redirect"))
        self.assertIn(elsewhere.url, error.detail or "")
        self.assertEqual(elsewhere.requests, [])
        self.assertEqual(len(api.requests), 1)

    def test_every_redirect_status_is_refused_for_posts_too(self) -> None:
        elsewhere = self.serve(Reply(body=samples.PLANS))
        for status in (301, 303, 307, 308):
            with self.subTest(status=status):
                api = self.serve(Reply(status, headers={"Location": elsewhere.url}))
                with self.assertRaises(CarragentError) as caught:
                    self.client(api).batch([{"path": "/plans"}])
                self.assertEqual((caught.exception.status, caught.exception.code), (status, "unexpected_redirect"))
        self.assertEqual(elsewhere.requests, [])

    def test_a_redirect_without_a_location_is_explained_too(self) -> None:
        api = self.serve(Reply(302))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).list_plans()
        self.assertEqual(caught.exception.code, "unexpected_redirect")

    def test_an_answer_that_is_too_large_is_refused(self) -> None:
        api = self.serve(Reply(200, b"{" + b" " * 4096 + b"}", content_type="application/json"))
        with mock.patch.object(carragent.client, "_MAX_BODY", 1024), self.assertRaises(CarragentError) as caught:
            self.client(api).list_plans()
        self.assertEqual((caught.exception.status, caught.exception.code), (200, "response_too_large"))

    def test_an_error_answer_that_is_too_large_is_refused_too(self) -> None:
        api = self.serve(Reply(500, b"x" * 4096))
        with mock.patch.object(carragent.client, "_MAX_BODY", 1024), self.assertRaises(CarragentError) as caught:
            self.client(api).list_plans()
        self.assertEqual(caught.exception.code, "response_too_large")

    def test_the_limit_is_eight_mebibytes(self) -> None:
        self.assertEqual(carragent.client._MAX_BODY, 8 * 1024 * 1024)  # pyright: ignore[reportPrivateUsage]

    def test_a_token_that_could_split_a_header_is_not_sent(self) -> None:
        api = self.serve(Reply(body=samples.PLANS))
        client = self.client(api, token="cat_x\r\nX-Injected: 1")
        with self.assertRaises(ValueError):
            client.list_plans()
        self.assertEqual(api.requests, [])


if __name__ == "__main__":
    unittest.main()
