"""The reads: what the client sends for each call, and what it hands back."""

import unittest
from typing import Any
from unittest import mock

from carragent import USER_AGENT, Carragent, CarragentError, __version__

from . import samples
from .fake_server import ApiTestCase, Reply, problem


class ConstructorTest(unittest.TestCase):
    def test_defaults(self) -> None:
        client = Carragent()
        self.assertEqual(client.base_url, "https://carragent.com")
        self.assertIsNone(client.token)
        self.assertEqual(client.timeout, 10)
        self.assertEqual(client.user_agent, f"carragent-python/{__version__}")
        self.assertEqual(client.user_agent, USER_AGENT)

    def test_base_url_is_cleaned(self) -> None:
        self.assertEqual(Carragent("https://carragent.com/").base_url, "https://carragent.com")
        self.assertEqual(Carragent("https://example.com/prefix//").base_url, "https://example.com/prefix")
        self.assertEqual(Carragent("  https://carragent.com  ").base_url, "https://carragent.com")

    def test_http_is_only_for_this_machine(self) -> None:
        for url in ("http://localhost:3000", "http://127.0.0.1:8080", "http://[::1]:3000", "http://127.1.2.3"):
            self.assertEqual(Carragent(url).base_url, url)
        for url in (
            "http://carragent.com",
            "http://192.168.1.10",
            "http://localhost.example.com",
            "http://0.0.0.0:3000",
        ):
            with self.assertRaisesRegex(ValueError, "https"):
                Carragent(url)

    def test_base_url_must_be_an_address_and_nothing_else(self) -> None:
        bad: list[Any] = [
            "",
            "carragent.com",
            "//carragent.com",
            "ftp://carragent.com",
            "file:///etc/passwd",
            "https://",
            "https://user:pass@carragent.com",
            "https://user@carragent.com",
            "https://carragent.com?x=1",
            "https://carragent.com#top",
            "https://a..b",
            None,
            42,
        ]
        for url in bad:
            with self.subTest(url=url), self.assertRaises(ValueError):
                Carragent(url)

    def test_timeout_must_be_a_positive_number(self) -> None:
        for timeout in (0, -1, float("nan"), float("inf"), None, "10"):
            with self.subTest(timeout=timeout), self.assertRaisesRegex(ValueError, "timeout"):
                Carragent(timeout=timeout)  # type: ignore[arg-type]
        self.assertEqual(Carragent(timeout=0.5).timeout, 0.5)

    def test_repr_never_shows_the_token(self) -> None:
        client = Carragent(token=samples.FAKE_TOKEN)
        self.assertNotIn(samples.FAKE_TOKEN, repr(client))
        self.assertIn("token='***'", repr(client))
        self.assertIn("token=None", repr(Carragent()))


class ReadsTest(ApiTestCase):
    def test_list_agents_sends_only_what_it_is_given(self) -> None:
        api = self.serve(Reply(body=samples.AGENT_PAGES[2]), Reply(body=samples.AGENT_PAGES[2]))
        client = self.client(api)
        self.assertEqual(client.list_agents(), samples.AGENT_PAGES[2])
        client.list_agents(lang="es", limit=5, cursor="Mg")
        first, second = api.requests
        self.assertEqual((first.method, first.path, first.query), ("GET", "/api/v1/agents", []))
        self.assertEqual((second.method, second.path), ("GET", "/api/v1/agents"))
        self.assertEqual(sorted(second.query), [("cursor", "Mg"), ("lang", "es"), ("limit", "5")])

    def test_every_request_says_who_is_asking(self) -> None:
        api = self.serve(Reply(body=samples.PLANS))
        self.client(api).list_plans()
        headers = api.requests[0].headers
        self.assertEqual(headers["user-agent"], f"carragent-python/{__version__}")
        self.assertIn("application/json", headers["accept"])
        self.assertNotIn("authorization", headers)
        self.assertNotIn("content-type", headers)

    def test_the_user_agent_can_be_replaced(self) -> None:
        api = self.serve(Reply(body=samples.PLANS))
        self.client(api, user_agent=f"my-app/2 {USER_AGENT}").list_plans()
        self.assertEqual(api.requests[0].headers["user-agent"], f"my-app/2 carragent-python/{__version__}")

    def test_a_token_is_sent_as_a_bearer(self) -> None:
        api = self.serve(Reply(body=samples.PLANS))
        self.client(api, token=samples.FAKE_TOKEN).list_plans()
        self.assertEqual(api.requests[0].headers["authorization"], f"Bearer {samples.FAKE_TOKEN}")

    def test_the_token_can_be_set_or_removed_later(self) -> None:
        api = self.serve(handler=lambda request: Reply(body=samples.PLANS))
        client = self.client(api)
        client.token = samples.FAKE_TOKEN
        client.list_plans()
        client.token = None
        client.list_plans()
        self.assertEqual(api.requests[0].headers.get("authorization"), f"Bearer {samples.FAKE_TOKEN}")
        self.assertNotIn("authorization", api.requests[1].headers)

    def test_get_agent_quotes_the_id_as_one_path_segment(self) -> None:
        api = self.serve(handler=lambda request: Reply(body=samples.AGENT))
        client = self.client(api)
        self.assertEqual(client.get_agent("collections", lang="fr"), samples.AGENT)
        client.get_agent("a/b c?d#e%f")
        client.get_agent("facturación")
        self.assertEqual(api.requests[0].target, "/api/v1/agents/collections?lang=fr")
        self.assertEqual(api.requests[1].target, "/api/v1/agents/a%2Fb%20c%3Fd%23e%25f")
        self.assertEqual(api.requests[2].target, "/api/v1/agents/facturaci%C3%B3n")

    def test_get_agent_refuses_an_id_that_cannot_name_an_agent(self) -> None:
        api = self.serve()
        client = self.client(api)
        for value in ("", ".", "..", None, 3):
            with self.subTest(value=value), self.assertRaises(ValueError):
                client.get_agent(value)  # type: ignore[arg-type]
        self.assertEqual(api.requests, [])

    def test_an_unknown_agent_is_a_carragent_error(self) -> None:
        api = self.serve(problem(404, "agent_not_found", "No agent with that id", "See /api/v1/agents for the ids."))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).get_agent("nope")
        error = caught.exception
        self.assertEqual((error.status, error.code, error.title), (404, "agent_not_found", "No agent with that id"))
        self.assertEqual(error.detail, "See /api/v1/agents for the ids.")

    def test_list_plans_and_search(self) -> None:
        api = self.serve(Reply(body=samples.PLANS), Reply(body=samples.SEARCH), Reply(body=samples.SEARCH))
        client = self.client(api)
        self.assertEqual(client.list_plans(lang="es"), samples.PLANS)
        self.assertEqual(client.search("late invoices"), samples.SEARCH)
        client.search("facturación & recibos=1+2 #3", lang="es")
        plans, plain, awkward = api.requests
        self.assertEqual(plans.target, "/api/v1/plans?lang=es")
        self.assertEqual(plain.target, "/api/v1/search?q=late%20invoices")
        self.assertEqual(awkward.query, [("q", "facturación & recibos=1+2 #3"), ("lang", "es")])
        self.assertNotIn(" ", awkward.target)
        self.assertNotIn("#", awkward.target)

    def test_a_search_without_a_query_is_left_to_the_api(self) -> None:
        api = self.serve(problem(400, "invalid_parameter", "Invalid q", "q is required, up to 200 characters."))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).search("")
        self.assertEqual(caught.exception.code, "invalid_parameter")
        self.assertEqual(api.requests[0].target, "/api/v1/search?q=")

    def test_a_base_url_with_a_path_keeps_it(self) -> None:
        api = self.serve(Reply(body=samples.PLANS))
        Carragent(api.url + "/prefix/").list_plans()
        self.assertEqual(api.requests[0].path, "/prefix/api/v1/plans")

    def test_the_answer_is_the_parsed_json_untouched(self) -> None:
        extra = {**samples.PLANS, "somethingNew": {"added": ["later"]}}
        api = self.serve(Reply(body=extra))
        plans: Any = self.client(api).list_plans()
        self.assertEqual(plans["somethingNew"], {"added": ["later"]})


class IterAgentsTest(ApiTestCase):
    def test_it_follows_the_cursor_until_the_last_page(self) -> None:
        api = self.serve(*(Reply(body=page) for page in samples.AGENT_PAGES))
        ids = [agent["id"] for agent in self.client(api).iter_agents(lang="en", page_size=2)]
        self.assertEqual(ids, ["collections", "scheduling", "reviews", "leads"])
        self.assertEqual(
            [r.query for r in api.requests],
            [
                [("lang", "en"), ("limit", "2")],
                [("lang", "en"), ("limit", "2"), ("cursor", "Mg")],
                [("lang", "en"), ("limit", "2"), ("cursor", "NA")],
            ],
        )

    def test_it_asks_for_a_page_only_when_it_is_reached(self) -> None:
        api = self.serve(*(Reply(body=page) for page in samples.AGENT_PAGES))
        agents = self.client(api).iter_agents()
        self.assertEqual(api.requests, [])
        self.assertEqual(next(agents)["id"], "collections")
        self.assertEqual(len(api.requests), 1)
        self.assertEqual(next(agents)["id"], "scheduling")
        self.assertEqual(len(api.requests), 1)
        self.assertEqual(next(agents)["id"], "reviews")
        self.assertEqual(len(api.requests), 2)

    def test_the_default_page_size_is_the_largest(self) -> None:
        api = self.serve(Reply(body=samples.AGENT_PAGES[2]))
        list(self.client(api).iter_agents())
        self.assertEqual(api.requests[0].query, [("limit", "50")])

    def test_a_cursor_that_does_not_move_ends_the_loop(self) -> None:
        stuck = {"object": "list", "data": [samples.AGENT], "nextCursor": "same", "hasMore": True}
        api = self.serve(handler=lambda request: Reply(body=stuck))
        agents = list(self.client(api).iter_agents())
        self.assertEqual(len(agents), 2)
        self.assertEqual(len(api.requests), 2)

    def test_an_error_on_a_later_page_is_raised(self) -> None:
        api = self.serve(
            Reply(body=samples.AGENT_PAGES[0]),
            problem(429, "rate_limited", "Too many requests", headers={"Retry-After": "3"}),
        )
        agents = self.client(api).iter_agents()
        self.assertEqual(next(agents)["id"], "collections")
        self.assertEqual(next(agents)["id"], "scheduling")
        with self.assertRaises(CarragentError) as caught:
            next(agents)
        self.assertEqual(caught.exception.status, 429)


BATCH_ANSWER: dict[str, Any] = {
    "object": "batch",
    "responses": [
        {"id": "plans", "status": 200, "body": samples.PLANS},
        {
            "id": "1",
            "status": 404,
            "body": {"type": "about:blank", "status": 404, "code": "agent_not_found", "title": "No agent"},
        },
    ],
}


class BatchTest(ApiTestCase):
    def test_it_posts_the_items_as_json_and_returns_every_answer(self) -> None:
        api = self.serve(Reply(body=BATCH_ANSWER))
        items: list[Any] = [{"id": "plans", "path": "/plans"}, {"path": "/agents/nope"}]
        answer = self.client(api).batch(items)
        request = api.requests[0]
        self.assertEqual((request.method, request.target), ("POST", "/api/v1/batch"))
        self.assertEqual(request.headers["content-type"], "application/json")
        self.assertEqual(request.json(), {"requests": items})
        self.assertEqual(answer, BATCH_ANSWER)

    def test_an_item_that_failed_is_data_not_an_exception(self) -> None:
        api = self.serve(Reply(body=BATCH_ANSWER))
        answer = self.client(api).batch([{"path": "/plans"}, {"path": "/agents/nope"}])
        self.assertEqual(answer["responses"][1]["status"], 404)
        self.assertEqual(answer["responses"][1]["body"]["code"], "agent_not_found")

    def test_a_refused_batch_is_an_exception(self) -> None:
        api = self.serve(problem(400, "invalid_request", "Invalid batch", "requests must have 1 to 20 items."))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).batch([])
        self.assertEqual((caught.exception.status, caught.exception.code), (400, "invalid_request"))

    def test_any_sequence_of_items_will_do(self) -> None:
        api = self.serve(Reply(body=BATCH_ANSWER))
        self.client(api).batch(({"path": "/plans"},))
        self.assertEqual(api.requests[0].json(), {"requests": [{"path": "/plans"}]})


class JobsTest(ApiTestCase):
    def test_create_job_posts_the_operation_and_its_params(self) -> None:
        api = self.serve(
            Reply(202, samples.JOB_ACCEPTED, {"Location": "https://carragent.com/api/v1/jobs/job_abc.def"})
        )
        job = self.client(api).create_job("searchCatalog", {"q": "late invoices", "lang": "en"})
        request = api.requests[0]
        self.assertEqual((request.method, request.target), ("POST", "/api/v1/jobs"))
        self.assertEqual(request.json(), {"operation": "searchCatalog", "params": {"q": "late invoices", "lang": "en"}})
        self.assertEqual(job, samples.JOB_ACCEPTED)

    def test_params_are_optional(self) -> None:
        api = self.serve(Reply(202, samples.JOB_ACCEPTED))
        self.client(api).create_job("listPlans")
        self.assertEqual(api.requests[0].json(), {"operation": "listPlans", "params": {}})

    def test_a_batch_job_carries_its_requests(self) -> None:
        api = self.serve(Reply(202, samples.JOB_ACCEPTED))
        self.client(api).create_job("batch", {"requests": [{"path": "/plans"}]})
        self.assertEqual(api.requests[0].json()["params"], {"requests": [{"path": "/plans"}]})

    def test_get_job_quotes_the_id(self) -> None:
        api = self.serve(Reply(body=samples.JOB_DONE))
        job: Any = self.client(api).get_job("job_abc.def")
        self.assertEqual(api.requests[0].target, "/api/v1/jobs/job_abc.def")
        self.assertEqual(job["result"]["status"], 200)
        api = self.serve(Reply(body=samples.JOB_DONE))
        self.client(api).get_job("job_a/b")
        self.assertEqual(api.requests[0].target, "/api/v1/jobs/job_a%2Fb")

    def test_get_job_refuses_an_empty_id(self) -> None:
        with self.assertRaises(ValueError):
            Carragent().get_job("")

    def test_unknown_and_expired_jobs_are_errors(self) -> None:
        api = self.serve(
            problem(404, "job_not_found", "No such job"),
            problem(410, "job_expired", "The job has expired", "Jobs last 24 hours."),
        )
        client = self.client(api)
        with self.assertRaises(CarragentError) as unknown:
            client.get_job("job_x")
        with self.assertRaises(CarragentError) as expired:
            client.get_job("job_y")
        self.assertEqual((unknown.exception.status, unknown.exception.code), (404, "job_not_found"))
        self.assertEqual((expired.exception.status, expired.exception.code), (410, "job_expired"))

    def test_wait_job_returns_at_once_when_the_job_has_run(self) -> None:
        api = self.serve(Reply(body=samples.JOB_DONE))
        with mock.patch("time.sleep") as sleep:
            job = self.client(api).wait_job("job_abc.def")
        self.assertEqual(job["status"], "succeeded")
        self.assertEqual(len(api.requests), 1)
        sleep.assert_not_called()

    def test_wait_job_polls_until_the_job_is_no_longer_pending(self) -> None:
        api = self.serve(Reply(body=samples.JOB_PENDING), Reply(body=samples.JOB_PENDING), Reply(body=samples.JOB_DONE))
        job = self.client(api).wait_job("job_abc.def", interval=0.01)
        self.assertEqual(job["status"], "succeeded")
        self.assertEqual(len(api.requests), 3)

    def test_wait_job_gives_up_after_its_timeout(self) -> None:
        api = self.serve(handler=lambda request: Reply(body=samples.JOB_PENDING))
        with self.assertRaisesRegex(TimeoutError, "job_abc.def"):
            self.client(api).wait_job("job_abc.def", timeout=0.1, interval=0.02)
        self.assertGreaterEqual(len(api.requests), 2)

    def test_wait_job_checks_its_own_arguments_before_it_asks(self) -> None:
        api = self.serve()
        client = self.client(api)
        for timeout, interval in ((0, 1), (-1, 1), (1, 0), (1, -1), (float("inf"), 1), (1, float("nan"))):
            with self.subTest(timeout=timeout, interval=interval), self.assertRaises(ValueError):
                client.wait_job("job_x", timeout, interval=interval)
        self.assertEqual(api.requests, [])

    def test_wait_job_raises_what_get_job_raises(self) -> None:
        api = self.serve(problem(410, "job_expired", "The job has expired"))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).wait_job("job_abc.def")
        self.assertEqual(caught.exception.code, "job_expired")


if __name__ == "__main__":
    unittest.main()
