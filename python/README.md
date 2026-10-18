# carragent

A Python client for [Carragent](https://carragent.com)'s public, read-only API: its AI agents for small businesses, and its plans and prices.

The API needs no account and no key. This package reads it. It does not sign anyone up, start a trial, change anything or reach a business's data. It has no dependencies besides Python (3.9 or newer).

## Install

```sh
pip install carragent
```

## Use

```python
from carragent import Carragent

client = Carragent()

# Which agent follows up late invoices?
for hit in client.search("late invoices")["data"]:
    print(hit["kind"], hit["id"], hit["name"])

# What does it do, and what does it leave to the owner?
agent = client.get_agent("collections")
print(agent["summary"])
print("Waits for the owner:", agent["waitsForOwner"])
print("Never:", agent["never"])

# What does it cost? Prices are whole US cents, before taxes.
plans = client.list_plans()
for plan in plans["data"]:
    if plan["selfServe"]:
        print(f"{plan['name']}: ${plan['monthlyCents'] / 100:.2f} a month")
print("Free trial:", plans["terms"]["trialDays"], "days")
```

Text comes in English, Spanish or French: pass `lang="en"`, `"es"` or `"fr"`.

## What it can do

| Method | Endpoint |
| --- | --- |
| `list_agents(lang=, limit=, cursor=)` | `GET /api/v1/agents` |
| `iter_agents(lang=, page_size=)` | every agent, following the cursor |
| `get_agent(id, lang=)` | `GET /api/v1/agents/{id}` |
| `list_plans(lang=)` | `GET /api/v1/plans` |
| `search(q, lang=)` | `GET /api/v1/search` |
| `batch(requests)` | `POST /api/v1/batch` |
| `create_job(operation, params)`, `get_job(id)`, `wait_job(id, timeout=)` | `POST /api/v1/jobs`, `GET /api/v1/jobs/{id}` |
| `register_anonymous()`, `use_anonymous_identity()` | `POST /agent/identity`, then `POST /api/oauth2/token` |
| `use_client_credentials(client_id, secret)` | `POST /api/oauth2/token` |

Every method returns the API's JSON as plain dictionaries. The keys are the API's own (`waitsForOwner`, `monthlyCents`), and the package ships type hints for them (`carragent.Agent`, `carragent.Plan` and the rest), so your editor and type checker know the shapes. The [OpenAPI document](https://carragent.com/openapi.json) is the reference.

### Several reads at once

```python
answers = client.batch(
    [
        {"id": "plans", "path": "/plans"},
        {"id": "one", "path": "/agents/collections"},
        {"path": "/search?q=appointment%20reminders"},
    ]
)
for answer in answers["responses"]:
    print(answer["id"], answer["status"])
```

A batch takes 1 to 20 reads. An item that fails carries its own `status` and its problem in `body`; the call raises only when the whole request is refused. Every item counts as one request against the rate limit.

### Jobs

```python
job = client.create_job("searchCatalog", {"q": "late invoices"})
done = client.wait_job(job["id"])
print(done["result"]["status"], done["result"]["body"]["data"])
```

This is the REST pattern for slow operations, so code written for one works unchanged. These operations are fast reads: nothing is queued, and a job's result is worked out when you poll it.

## Errors

A request that fails raises `CarragentError`. The API's errors are [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problems; branch on `code`, which is stable, not on `title` or `detail`, which are for people.

```python
import time

from carragent import Carragent, CarragentError, RateLimited

client = Carragent()
try:
    client.get_agent("nope")
except RateLimited as error:
    time.sleep(error.retry_after or 1)  # seconds, from the Retry-After header
except CarragentError as error:
    print(error.status, error.code, error.title)  # 404 agent_not_found No agent with that id
```

- `RateLimited` (HTTP 429) is a `CarragentError` with `retry_after`. The client never retries by itself.
- `ConnectionFailed` is a `CarragentError` for a request that got no answer: `code` is `timeout` or `connection_failed`, and `status` is 0.
- A bad argument (an empty id) raises `ValueError`. `wait_job` raises `TimeoutError` if the job is still pending when its time is up.

## Tokens (optional)

Nothing here needs one. Without a token the limit is 60 requests a minute per IP address; with one it is 300 a minute per client. A token lasts an hour and reads the same public catalog.

```python
client = Carragent()
client.use_anonymous_identity()  # registers this agent anonymously, then gets a token and uses it
```

An anonymous registration has no account behind it: it only gives your agent a rate limit of its own. Keep the answer of `register_anonymous()` (it is good for 24 hours) and pass it to `use_anonymous_identity(registration)` to get a new token each hour without registering again.

If you have an OAuth client (`POST /api/oauth2/register`, see [auth.md](https://carragent.com/auth.md)):

```python
client.use_client_credentials(client_id, client_secret)
```

When a token expires, the API answers 401 `invalid_token` even though it is public: get a new one, or set `client.token = None`.

## Behaviour worth knowing

- `Carragent(base_url="https://carragent.com", token=None, timeout=10)`. The timeout is for each network operation (connecting, then each read), not for a whole request.
- `base_url` must be `https`; `http` is accepted only for a server on your own machine (`localhost`, `127.0.0.1`), so a token never crosses the network in the clear.
- Redirects are not followed: the client would send your token wherever they pointed. A redirect raises `CarragentError` with the code `unexpected_redirect`.
- Answers over 8 MiB are refused. The proxy settings in your environment (`HTTPS_PROXY`, `NO_PROXY`) apply, as they do for anything built on `urllib`.
- Requests identify themselves as `carragent-python/<version>`; pass `user_agent=` to add your application's name.

## Links

- Developer guide: <https://carragent.com/developers>
- This package is part of the [Carragent agent kit](https://github.com/vicenccc/carragent-agent-kit), which also holds a Go client, the MCP configuration and four skills for coding agents.

MIT licensed.
