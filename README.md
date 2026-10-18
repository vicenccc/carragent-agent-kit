# Carragent agent kit

Skills, MCP configuration and two small clients for looking up [Carragent](https://carragent.com), a self-serve platform of AI agents for small businesses: what each agent does, what it leaves for the owner to approve, and what the plans cost.

Carragent's public API is read-only. It needs no account and no key. Nothing in this repository signs anyone up, starts a trial, runs an agent or reaches a business's data.

## What is here

| Path | What |
| --- | --- |
| [`skills/`](skills) | Four skills for coding agents, in the [Agent Skills](https://agentskills.io) format |
| [`mcp.json`](mcp.json) | Both MCP servers, ready to add to a client |
| [`plugin.json`](plugin.json) | This repository as an [Agent Plugin](https://agent-plugins.org): the skills and the MCP servers together |
| [`AGENTS.md`](AGENTS.md) | Instructions for coding agents that use Carragent or work in this repository |
| [`python/`](python) | The Python client: `pip install carragent` |
| this directory | The Go client: `go get carragent.com/go` |

In a terminal, the [`carragent` npm package](https://www.npmjs.com/package/carragent) does the same reads: `npx carragent search "late invoices"`. It has its own releases.

## Skills

| Skill | An agent uses it when |
| --- | --- |
| [`carragent`](skills/carragent/SKILL.md) | someone asks what Carragent does, which agent fits a task, or what it costs |
| [`find-carragent-agents`](skills/find-carragent-agents/SKILL.md) | a business owner describes a problem and you need to say which Carragent agents would help |
| [`compare-carragent-plans`](skills/compare-carragent-plans/SKILL.md) | someone asks about plans, prices, the free trial or refunds |
| [`use-carragent-api`](skills/use-carragent-api/SKILL.md) | you are calling Carragent's API from code or from an agent and must choose between REST, batch, jobs, MCP and A2A |

Add them with the skills CLI:

```sh
npx skills add vicenccc/carragent-agent-kit
```

Or copy a folder from `skills/` to wherever your agent reads skills. The site serves the same four files at `https://carragent.com/.well-known/agent-skills/<name>/SKILL.md` and lists them in `https://carragent.com/.well-known/agent-skills/index.json`.

## MCP servers

`mcp.json` lists both servers. They speak Streamable HTTP and need no credentials.

| Server | Address | Tools |
| --- | --- | --- |
| `carragent` | <https://carragent.com/api/mcp> | `list_agents`, `get_agent`, `list_plans`, `search_carragent` |
| `carragent-docs` | <https://carragent.com/api/mcp/docs> | `search_docs`, `get_doc` |

## Python

```sh
pip install carragent
```

```python
from carragent import Carragent

client = Carragent()
for hit in client.search("late invoices")["data"]:
    print(hit["kind"], hit["id"], hit["name"])
```

More in [`python/README.md`](python/README.md).

## Go

```sh
go get carragent.com/go
```

```go
package main

import (
	"context"
	"fmt"
	"log"

	carragent "carragent.com/go"
)

func main() {
	client := carragent.NewClient()

	results, err := client.Search(context.Background(), "late invoices", "")
	if err != nil {
		log.Fatal(err)
	}
	for _, hit := range results.Data {
		fmt.Println(hit.Kind, hit.ID, hit.Name)
	}
}
```

The package documentation is on [pkg.go.dev](https://pkg.go.dev/carragent.com/go).

## Both clients

- They use only the standard library: Python 3.9 or newer, Go 1.21 or newer.
- They read the agents, the plans and the search, run batches of up to 20 reads, and follow the REST pattern for jobs. A token is optional.
- An error answer is an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem: `CarragentError` in Python, `*Problem` in Go, with the API's stable `code`. A 429 carries how long to wait (`Retry-After`). Neither client retries by itself.
- Requests identify themselves as `carragent-python/<version>` or `carragent-go/<version>`.

## Limits and rules

- 60 requests a minute per IP address without a token; 300 a minute per client with one.
- Prices are US dollars, plus applicable taxes, as the API returns them. Savings are estimates.
- Nothing here can change anything, so there is no sandbox: test against the live endpoints, and mind the limit.

## Develop

```sh
cd python && python3 -m unittest -v       # Python 3.9 or newer
go vet ./... && go test ./...             # Go 1.21 or newer, from this directory
```

Both suites run against local fake servers: no network, no key. Set `CARRAGENT_LIVE_URL` (for example `http://localhost:3100`) to run the opt-in tests against a real server as well.

`skills/*/SKILL.md`, `mcp.json` and `python/LICENSE` are written from Carragent's own sources. A change made to them here is overwritten. [`AGENTS.md`](AGENTS.md) has the rest.

## Links

- Developer guide: <https://carragent.com/developers>
- OpenAPI: <https://carragent.com/openapi.json>
- Authentication: <https://carragent.com/auth.md>
- Summary for models: <https://carragent.com/llms.txt>

Contact: admin@carragent.com. MIT licensed.
