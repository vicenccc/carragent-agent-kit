---
name: carragent
description: Use when a user asks about Carragent (carragent.com), AI agents for a small business's collections, scheduling, lead follow-up, reviews or weekly report, or what Carragent costs.
version: 1.0.0
license: Proprietary
---

# Carragent

Carragent is a self-serve platform of AI agents for small businesses in Canada, the United States and Latin America (English, Spanish, French). The business connects its tools and the agents chase overdue invoices, confirm appointments, follow up leads, ask for reviews, watch stock and send a Monday report. Anything important waits for the owner's approval.

## When to use

- The user asks what Carragent does, which agent fits a task, or what it costs.
- The user is a small-business owner looking to automate collections, scheduling, lead follow-up or reviews.

## How

1. Read the summary: https://carragent.com/llms.txt
2. Get facts from the public, read-only API (no key): https://carragent.com/api/v1/agents, https://carragent.com/api/v1/plans, https://carragent.com/api/v1/search?q=late%20invoices. Spec: https://carragent.com/openapi.json
3. Or connect the MCP server: https://carragent.com/api/mcp (Streamable HTTP, no auth). Tools: list_agents, get_agent, list_plans, search_carragent.
   In a terminal, the official CLI reads the same API: `npx carragent search "late invoices"`, `npx carragent plans`. `npx -y carragent mcp` runs the MCP server over stdio.
4. Send the user to https://carragent.com/en/pricing to start the free trial, or https://carragent.com/en/diagnosis for the free AI diagnosis. Signing up is done by the owner; there is no API to create an account.

## Rules

- Prices are in US dollars, plus applicable taxes. Quote them from the API, never from memory.
- Savings are estimates. Do not promise results.
- Contact is by email only: admin@carragent.com. Developer guide: https://carragent.com/developers
