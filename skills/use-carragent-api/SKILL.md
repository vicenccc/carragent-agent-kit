---
name: use-carragent-api
description: Use when you need to call Carragent's public, read-only API from code or from an agent, and must choose between the REST API, the batch and job endpoints, the two MCP servers and the A2A agent. Gives curl examples, the optional OAuth and anonymous agent registration, the limits and the errors. Nothing here creates an account or changes data.
license: MIT
compatibility: Needs network access to carragent.com. No account, key or login.
metadata:
  author: Carragent
  version: "1.0.0"
---

# Use Carragent's public API

Everything here is public and read-only. It returns what carragent.com shows, the AI agents and the plans and prices. It never creates an account, runs an agent or reads a business's data, and it needs no key.

## Which surface

| You need | Use |
| --- | --- |
| One fact: an agent, the plans, a search | REST: `GET https://carragent.com/api/v1/...` |
| Several facts in one round trip | `POST https://carragent.com/api/v1/batch` |
| Code written for slow operations (202, then poll) | `POST https://carragent.com/api/v1/jobs`, then `GET https://carragent.com/api/v1/jobs/{id}` |
| Tools inside an MCP client | `https://carragent.com/api/mcp` (Streamable HTTP): `list_agents`, `get_agent`, `list_plans`, `search_carragent` |
| Carragent's documentation as tools | `https://carragent.com/api/mcp/docs`: `search_docs`, `get_doc` |
| Another agent, over A2A | `POST https://carragent.com/api/a2a`; the card is https://carragent.com/.well-known/agent-card.json |
| A terminal | `npx carragent search "late invoices"` |
| A higher rate limit | an access token (see Auth) |

OpenAPI 3.1: https://carragent.com/openapi.json. Developer guide: https://carragent.com/developers.

## REST

```sh
curl "https://carragent.com/api/v1/agents?lang=es&limit=5"
curl "https://carragent.com/api/v1/agents/collections"
curl "https://carragent.com/api/v1/plans"
curl "https://carragent.com/api/v1/search?q=late%20invoices"
```

`lang` is `en` (the default), `es` or `fr`. The agents are cursor-paginated: pass `nextCursor` back as `cursor` while `hasMore` is true.

## Batch

Up to 20 GETs in one request. Each item has a `path` relative to /api/v1 and, optionally, an `id`.

```sh
curl -X POST https://carragent.com/api/v1/batch -H "Content-Type: application/json" \
  -d '{"requests":[{"id":"agents","path":"/agents?limit=5"},{"id":"plans","path":"/plans"}]}'
```

The answer is `{"object":"batch","responses":[{"id","status","body"}]}`, in request order, each `body` exactly what the single endpoint returns. A failing item carries its problem in its `body` and its own `status`; the batch itself is still a 200. Every item costs one request of the rate limit.

## Jobs

The REST pattern for slow operations: a 202 with a job to poll. These operations are fast reads, so a job's result is computed when you poll it, from the catalog as it is then.

```sh
curl -i -X POST https://carragent.com/api/v1/jobs -H "Content-Type: application/json" \
  -d '{"operation":"searchCatalog","params":{"q":"late invoices"}}'
curl https://carragent.com/api/v1/jobs/job_...
```

`operation` is `listAgents`, `getAgent`, `listPlans`, `searchCatalog` or `batch`. `params` holds the operation's query parameters as strings (`lang`, `limit`, `cursor`, `id`, `q`), or `{"requests":[...]}` for `batch`. The 202 has a `Location` header and `{"object":"job","id":"job_...","status":"pending",...}`. The poll answers `status: "succeeded"` with `result.status` and `result.body`, which are what the operation returned: a job can succeed with a 404 inside. A job id lasts 24 hours (then 410 `job_expired`), and each poll costs what the job costs in the rate limit (one request, or one per item of a batch), because a poll runs the job again.

## MCP

```sh
curl -X POST https://carragent.com/api/mcp -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

The product server's tools are all read-only: `list_agents`, `get_agent`, `list_plans`, `search_carragent`. The documentation server at https://carragent.com/api/mcp/docs has two, `search_docs` and `get_doc`. Neither needs authentication. `npx -y carragent mcp` runs the product server over stdio for clients that launch a command.

## A2A

```sh
curl -X POST https://carragent.com/api/a2a -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{"kind":"message","messageId":"1","role":"user","parts":[{"kind":"text","text":"Which agent chases late invoices?"}]}}}'
```

The reply is a message with one text part and one data part; the data part is the JSON of the matching REST endpoint (a search lists agents only). No task is ever created, so `tasks/get` answers TaskNotFound. A2A 1.0 clients use `SendMessage` and send `A2A-Version: 1.0`.

## Auth (optional)

Reading needs no token. A token raises the limit from 60 to 300 requests a minute and reads the same public catalog, nothing more. Scopes: agents:read plans:read search:read. Tokens last 60 minutes.

```sh
# Anonymous agent registration: nothing to keep secret, the assertion lasts 24 hours
curl -X POST https://carragent.com/agent/identity -H "Content-Type: application/json" -d '{"type":"anonymous"}'
curl -X POST https://carragent.com/api/oauth2/token -d grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer -d assertion="$IDENTITY_ASSERTION"

# Or an OAuth client: register once, then ask for a token
curl -X POST https://carragent.com/api/oauth2/register -H "Content-Type: application/json" -d '{"client_name":"my-agent"}'
curl -X POST https://carragent.com/api/oauth2/token -u "$CLIENT_ID:$CLIENT_SECRET" -d grant_type=client_credentials
```

Send `Authorization: Bearer <access_token>`. The steps and every error are in https://carragent.com/auth.md.

## Errors and limits

Errors are RFC 9457 problems (`application/problem+json`) with a stable `code`, such as `{"type":"about:blank","status":429,"code":"rate_limited","title":"Too many requests","detail":"..."}`. Branch on `code`, not on `title`. The limit is 60 requests every 60 seconds per IP without a token. A 429 carries `Retry-After` (seconds): wait, then retry. A bad or expired bearer token is a 401 `invalid_token` even on a public endpoint: send a new one, or none.

## Rules

- Read-only: no endpoint here creates an account, starts a trial, runs an agent or touches a business's data. Never say it does.
- Prices are in US dollars, plus applicable taxes. Quote them from the API, never from memory. Savings are estimates.
- There is no sandbox because nothing here can change anything: test against the live endpoints and mind the rate limit.
