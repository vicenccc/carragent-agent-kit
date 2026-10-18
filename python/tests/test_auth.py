"""Tokens: the anonymous agent identity, and an OAuth client's credentials."""

import base64
import urllib.parse

from carragent import Carragent, CarragentError, RateLimited

from . import samples
from .fake_server import ApiTestCase, Reply, problem

JWT_BEARER = "urn:ietf:params:oauth:grant-type:jwt-bearer"
OLD_TOKEN = "cat_old-fake-token"


class RegisterAnonymousTest(ApiTestCase):
    def test_it_posts_the_anonymous_type_and_returns_the_registration(self) -> None:
        api = self.serve(Reply(body=samples.REGISTRATION))
        registration = self.client(api).register_anonymous()
        request = api.requests[0]
        self.assertEqual((request.method, request.target), ("POST", "/agent/identity"))
        self.assertEqual(request.headers["content-type"], "application/json")
        self.assertEqual(request.json(), {"type": "anonymous"})
        self.assertEqual(registration, samples.REGISTRATION)

    def test_a_token_is_not_sent_to_the_identity_endpoint(self) -> None:
        api = self.serve(Reply(body=samples.REGISTRATION))
        self.client(api, token=OLD_TOKEN).register_anonymous()
        self.assertNotIn("authorization", api.requests[0].headers)

    def test_a_refusal_is_a_carragent_error(self) -> None:
        api = self.serve(
            problem(400, "unsupported_identity_type", "Unsupported identity type", "Only anonymous is offered."),
            problem(503, "temporarily_unavailable", "Registration is not configured"),
            problem(429, "rate_limited", "Too many requests", headers={"Retry-After": "30"}),
        )
        client = self.client(api)
        with self.assertRaises(CarragentError) as refused:
            client.register_anonymous()
        with self.assertRaises(CarragentError) as unavailable:
            client.register_anonymous()
        with self.assertRaises(RateLimited) as limited:
            client.register_anonymous()
        self.assertEqual((refused.exception.status, refused.exception.code), (400, "unsupported_identity_type"))
        self.assertEqual((unavailable.exception.status, unavailable.exception.code), (503, "temporarily_unavailable"))
        self.assertEqual(limited.exception.retry_after, 30.0)

    def test_an_answer_without_an_assertion_is_not_accepted(self) -> None:
        api = self.serve(Reply(body={"registration_id": "reg_x"}))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).register_anonymous()
        self.assertEqual(caught.exception.code, "invalid_response")


class UseAnonymousIdentityTest(ApiTestCase):
    def test_it_registers_exchanges_the_assertion_and_uses_the_token(self) -> None:
        api = self.serve(Reply(body=samples.REGISTRATION), Reply(body=samples.TOKEN), Reply(body=samples.PLANS))
        client = self.client(api)
        token = client.use_anonymous_identity()
        self.assertEqual(token, samples.TOKEN)
        self.assertEqual(client.token, samples.FAKE_TOKEN)

        register, exchange = api.requests
        self.assertEqual((register.method, register.target), ("POST", "/agent/identity"))
        self.assertEqual((exchange.method, exchange.target), ("POST", "/api/oauth2/token"))
        self.assertEqual(exchange.headers["content-type"], "application/x-www-form-urlencoded")
        self.assertEqual(exchange.form(), {"grant_type": JWT_BEARER, "assertion": "header.claims.signature"})
        self.assertNotIn("authorization", exchange.headers)

        client.list_plans()
        self.assertEqual(api.requests[2].headers["authorization"], f"Bearer {samples.FAKE_TOKEN}")

    def test_an_old_token_is_not_sent_while_getting_a_new_one(self) -> None:
        api = self.serve(Reply(body=samples.REGISTRATION), Reply(body=samples.TOKEN))
        client = self.client(api, token=OLD_TOKEN)
        client.use_anonymous_identity()
        self.assertTrue(all("authorization" not in request.headers for request in api.requests))
        self.assertEqual(client.token, samples.FAKE_TOKEN)

    def test_a_registration_you_kept_is_exchanged_without_registering_again(self) -> None:
        api = self.serve(Reply(body=samples.TOKEN))
        client = self.client(api)
        client.use_anonymous_identity(samples.REGISTRATION)
        self.assertEqual([request.target for request in api.requests], ["/api/oauth2/token"])
        self.assertEqual(client.token, samples.FAKE_TOKEN)

    def test_the_assertion_only_ever_goes_to_the_base_url(self) -> None:
        registration = {**samples.REGISTRATION, "token_endpoint": "https://evil.example/steal"}
        api = self.serve(Reply(body=registration), Reply(body=samples.TOKEN))
        self.client(api).use_anonymous_identity()
        self.assertEqual(api.requests[1].target, "/api/oauth2/token")

    def test_a_registration_without_an_assertion_is_a_value_error(self) -> None:
        api = self.serve()
        client = self.client(api)
        for registration in ({}, {"identity_assertion": ""}, {"identity_assertion": 5}):
            with self.subTest(registration=registration), self.assertRaises(ValueError):
                client.use_anonymous_identity(registration)  # type: ignore[arg-type]
        self.assertEqual(api.requests, [])

    def test_a_refused_assertion_leaves_the_token_alone(self) -> None:
        api = self.serve(
            Reply(body=samples.REGISTRATION),
            Reply(400, {"error": "invalid_grant", "error_description": "The assertion has expired."}),
        )
        client = self.client(api, token=OLD_TOKEN)
        with self.assertRaises(CarragentError) as caught:
            client.use_anonymous_identity()
        self.assertEqual((caught.exception.status, caught.exception.code), (400, "invalid_grant"))
        self.assertEqual(client.token, OLD_TOKEN)

    def test_a_token_response_without_a_token_is_not_accepted(self) -> None:
        api = self.serve(Reply(body={"token_type": "Bearer", "expires_in": 3600}))
        client = self.client(api)
        with self.assertRaises(CarragentError) as caught:
            client.use_anonymous_identity(samples.REGISTRATION)
        self.assertEqual(caught.exception.code, "invalid_response")
        self.assertIsNone(client.token)


class UseClientCredentialsTest(ApiTestCase):
    def test_it_sends_basic_credentials_and_uses_the_token(self) -> None:
        api = self.serve(Reply(body=samples.TOKEN), Reply(body=samples.PLANS))
        client = self.client(api)
        token = client.use_client_credentials("cag_FAKEFAKEFAKEFAKE_7", "fake-secret")
        self.assertEqual(token, samples.TOKEN)
        self.assertEqual(client.token, samples.FAKE_TOKEN)

        request = api.requests[0]
        self.assertEqual((request.method, request.target), ("POST", "/api/oauth2/token"))
        self.assertEqual(request.headers["content-type"], "application/x-www-form-urlencoded")
        self.assertEqual(request.form(), {"grant_type": "client_credentials"})
        self.assertEqual(_basic(request.headers["authorization"]), "cag_FAKEFAKEFAKEFAKE_7:fake-secret")

        client.list_plans()
        self.assertEqual(api.requests[1].headers["authorization"], f"Bearer {samples.FAKE_TOKEN}")

    def test_scope_is_passed_when_given(self) -> None:
        api = self.serve(Reply(body=samples.TOKEN), Reply(body=samples.TOKEN))
        client = self.client(api)
        client.use_client_credentials("cag_x_7", "s", scope="agents:read plans:read")
        client.use_client_credentials("cag_x_7", "s", scope="")
        self.assertEqual(
            api.requests[0].form(), {"grant_type": "client_credentials", "scope": "agents:read plans:read"}
        )
        self.assertEqual(api.requests[1].form(), {"grant_type": "client_credentials"})

    def test_credentials_are_form_encoded_before_they_are_joined(self) -> None:
        # RFC 6749 section 2.3.1: a colon or a percent sign in an id or secret must not change where one ends.
        api = self.serve(Reply(body=samples.TOKEN))
        self.client(api).use_client_credentials("cag_a:b%c", "s e+cret/ü")
        decoded = base64.b64decode(api.requests[0].headers["authorization"].split(" ", 1)[1]).decode("ascii")
        encoded_id, encoded_secret = decoded.split(":")
        self.assertEqual(urllib.parse.unquote(encoded_id), "cag_a:b%c")
        self.assertEqual(urllib.parse.unquote(encoded_secret), "s e+cret/ü")

    def test_an_old_token_is_not_sent_while_getting_a_new_one(self) -> None:
        api = self.serve(Reply(body=samples.TOKEN))
        client = self.client(api, token=OLD_TOKEN)
        client.use_client_credentials("cag_x_7", "s")
        self.assertTrue(_basic(api.requests[0].headers["authorization"]).startswith("cag_x_7:"))
        self.assertEqual(client.token, samples.FAKE_TOKEN)

    def test_a_wrong_secret_is_a_carragent_error_and_leaves_the_token_alone(self) -> None:
        body = {"error": "invalid_client", "error_description": "Unknown client or wrong secret."}
        api = self.serve(Reply(401, body, {"WWW-Authenticate": 'Basic realm="carragent"'}))
        client = self.client(api, token=OLD_TOKEN)
        with self.assertRaises(CarragentError) as caught:
            client.use_client_credentials("cag_x_7", "wrong")
        self.assertEqual((caught.exception.status, caught.exception.code), (401, "invalid_client"))
        self.assertEqual(client.token, OLD_TOKEN)

    def test_a_scope_beyond_the_clients_is_refused(self) -> None:
        body = {"error": "invalid_scope", "error_description": "This client may ask for: agents:read."}
        api = self.serve(Reply(400, body))
        with self.assertRaises(CarragentError) as caught:
            self.client(api).use_client_credentials("cag_x_1", "s", scope="plans:read")
        self.assertEqual(caught.exception.code, "invalid_scope")

    def test_the_secret_is_not_kept(self) -> None:
        secret = "fake-secret-that-must-not-linger"
        api = self.serve(Reply(401, {"error": "invalid_client", "error_description": "no"}), Reply(body=samples.TOKEN))
        client = self.client(api)
        with self.assertRaises(CarragentError) as caught:
            client.use_client_credentials("cag_x_7", secret)
        client.use_client_credentials("cag_x_7", secret)
        for text in (repr(client), repr(vars(client)), str(caught.exception), repr(caught.exception.args)):
            self.assertNotIn(secret, text)
        self.assertNotIn(secret, repr({key: value for key, value in vars(client).items() if key != "_opener"}))

    def test_no_token_is_needed_at_all(self) -> None:
        api = self.serve(Reply(body=samples.PLANS))
        Carragent(api.url).list_plans()
        self.assertNotIn("authorization", api.requests[0].headers)


def _basic(header: str) -> str:
    scheme, _, value = header.partition(" ")
    assert scheme == "Basic", header
    return base64.b64decode(value).decode("ascii")
