"""Opt-in checks against a real server. They are skipped unless you say where it is:

    CARRAGENT_LIVE_URL=http://localhost:3100 python3 -m unittest tests.test_live

Nothing here changes anything: the API is read-only. A feature the server does not have yet (a 404) skips its test.
"""

import json
import os
import unittest
import urllib.request
from typing import Any

from carragent import Agent, Carragent, CarragentError, Job, JobAccepted, PlanList, SearchResults, Token

URL = os.environ.get("CARRAGENT_LIVE_URL")


def shaped_like(case: unittest.TestCase, value: Any, typed: Any) -> None:
    """The keys of an answer are exactly the keys of its type (an optional key may be missing)."""
    keys = set(value)
    case.assertEqual(keys - set(typed.__annotations__), set(), f"{typed.__name__}: keys the type lacks")
    case.assertEqual(typed.__required_keys__ - keys, set(), f"{typed.__name__}: keys the answer lacks")


@unittest.skipUnless(URL, "set CARRAGENT_LIVE_URL to try the client against a real server")
class LiveTest(unittest.TestCase):
    def setUp(self) -> None:
        assert URL
        self.client = Carragent(URL, timeout=30)

    def test_agents(self) -> None:
        page = self.client.list_agents(limit=3)
        self.assertEqual(len(page["data"]), 3)
        self.assertTrue(page["hasMore"])
        self.assertIsInstance(page["nextCursor"], str)
        for agent in page["data"]:
            shaped_like(self, agent, Agent)
        second = self.client.list_agents(limit=3, cursor=page["nextCursor"])
        self.assertNotEqual(second["data"][0]["id"], page["data"][0]["id"])

    def test_every_agent_once(self) -> None:
        ids = [agent["id"] for agent in self.client.iter_agents(page_size=4)]
        self.assertGreater(len(ids), 4)
        self.assertEqual(len(ids), len(set(ids)))
        self.assertEqual(ids, [agent["id"] for agent in self.client.list_agents(limit=50)["data"]])

    def test_one_agent_and_an_unknown_one(self) -> None:
        first = self.client.list_agents(limit=1)["data"][0]
        agent = self.client.get_agent(first["id"], lang="es")
        shaped_like(self, agent, Agent)
        self.assertEqual(agent["id"], first["id"])
        with self.assertRaises(CarragentError) as caught:
            self.client.get_agent("no-such-agent")
        self.assertEqual((caught.exception.status, caught.exception.code), (404, "agent_not_found"))

    def test_a_bad_parameter_is_a_problem(self) -> None:
        with self.assertRaises(CarragentError) as caught:
            self.client.list_agents(limit=500)
        self.assertEqual((caught.exception.status, caught.exception.code), (400, "invalid_parameter"))

    def test_plans(self) -> None:
        plans: PlanList = self.client.list_plans()
        shaped_like(self, plans, PlanList)
        shaped_like(self, plans["terms"], PlanList.__annotations__["terms"])
        self.assertTrue(plans["data"])
        for plan in plans["data"]:
            self.assertEqual(plan["currency"], "USD")
            self.assertIsInstance(plan["monthlyCents"], int)

    def test_search(self) -> None:
        results: SearchResults = self.client.search("late invoices")
        shaped_like(self, results, SearchResults)
        self.assertTrue(results["data"])
        self.assertIn(results["data"][0]["kind"], ("agent", "plan"))

    def test_batch(self) -> None:
        try:
            answers = self.client.batch(
                [{"id": "plans", "path": "/plans"}, {"id": "nope", "path": "/agents/no-such-agent"}]
            )
        except CarragentError as error:
            self.skipIfMissing(error)
            raise
        self.assertEqual([answer["id"] for answer in answers["responses"]], ["plans", "nope"])
        self.assertEqual([answer["status"] for answer in answers["responses"]], [200, 404])

    def test_a_job(self) -> None:
        try:
            accepted: JobAccepted = self.client.create_job("searchCatalog", {"q": "late invoices"})
        except CarragentError as error:
            self.skipIfMissing(error)
            raise
        shaped_like(self, accepted, JobAccepted)
        job: Job = self.client.wait_job(accepted["id"])
        shaped_like(self, job, Job)
        self.assertEqual(job["status"], "succeeded")
        self.assertEqual(job["result"]["status"], 200)

    def test_an_anonymous_identity_gets_a_token(self) -> None:
        try:
            token: Token = self.client.use_anonymous_identity()
        except CarragentError as error:
            self.skipIfMissing(error)
            raise
        shaped_like(self, token, Token)
        self.assertEqual(self.client.token, token["access_token"])
        self.assertTrue(self.client.list_plans()["data"])

    def test_client_credentials_get_a_token(self) -> None:
        client_id, secret = self.register_client()
        token = self.client.use_client_credentials(client_id, secret)
        shaped_like(self, token, Token)
        self.assertTrue(self.client.list_plans()["data"])
        with self.assertRaises(CarragentError) as caught:
            self.client.use_client_credentials(client_id, "wrong-secret")
        self.assertEqual((caught.exception.status, caught.exception.code), (401, "invalid_client"))

    def register_client(self) -> tuple[str, str]:
        """An OAuth client (RFC 7591); the client has no method for it, since it is only needed once."""
        assert URL
        request = urllib.request.Request(
            URL + "/api/oauth2/register",
            data=json.dumps({"client_name": "carragent-python live test"}).encode(),
            headers={"Content-Type": "application/json", "Idempotency-Key": "live-test-0123456789abcdef"},
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                body = json.load(response)
        except OSError as error:
            self.skipTest(f"this server cannot register a client: {error}")
        return str(body["client_id"]), str(body["client_secret"])

    def skipIfMissing(self, error: CarragentError) -> None:
        """404, or a redirect to a page: the server is older than the feature. 503: it is not set up to sign."""
        if error.status in (404, 503) or error.code == "unexpected_redirect":
            self.skipTest(f"this server does not have it ({error})")


if __name__ == "__main__":
    unittest.main()
