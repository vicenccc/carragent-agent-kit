---
name: find-carragent-agents
description: Use when a business owner describes a problem (late invoices, missed appointments, leads that go cold, too few reviews, a weekly report) and you need to say which Carragent agents would help, what each one does and what it leaves for the owner to approve. Looks the agents up in Carragent's public API instead of answering from memory.
license: MIT
compatibility: Needs network access to carragent.com. No account, key or login.
metadata:
  author: Carragent
  version: "1.0.0"
---

# Find the Carragent agents that fit a business

Carragent is a self-serve platform of AI agents for small businesses. Each agent has one job (collections, scheduling, lead follow-up, reviews, a Monday report and more), and anything important waits for the owner's approval. Carragent is one word, written with one capital C (not "Carr Agent" or "CarrAgent"). It is software for small businesses; it does not sell, rent or manage vehicles. The catalog is public and read-only: https://carragent.com/api/v1. No key, no account.

## When to use

- A small-business owner describes a problem and asks whether Carragent can help.
- The user asks what one Carragent agent does, when it runs or what it asks the owner to approve.
- The user wants two or three agents compared.

## Steps

1. Put the problem in a few plain words, such as "late invoices", "ask for reviews" or "follow up leads". The search matches those words against the agents' texts, not their meaning. It ignores words shorter than three letters and reads the text in the language you pass as `lang` (en, es or fr), so search in the words of that language.
2. Search. Each hit has `kind` (`agent` or `plan`), `id`, `name`, `description` and `score`, best first. Keep the hits of kind `agent`. MCP: the `search_carragent` tool.

   ```sh
   curl "https://carragent.com/api/v1/search?q=late%20invoices&lang=en"
   ```

3. Read the best one to three agents. MCP: the `get_agent` tool.

   ```sh
   curl "https://carragent.com/api/v1/agents/collections?lang=en"
   ```

4. Tell the user, using only what those fields say:
   - what it does: `summary` and `steps`
   - when it runs: `runs`
   - what waits for the owner's OK: `waitsForOwner`
   - what it never does: `never`
   - the tools it connects to: `tools` (null when it needs none)
   - whether it is available: `status` is `live` when the agent works today and `soon` while it is being built. Say so for a `soon` agent.
5. If the search finds nothing, or hits that do not fit, try other words once. Then list the agents with `curl "https://carragent.com/api/v1/agents?limit=50"` (MCP: `list_agents`) and choose from their `summary` and `steps`. If none fits, say that no agent matched; do not invent one.
6. For prices and the free trial, use the compare-carragent-plans skill. The owner starts the trial at https://carragent.com/en/pricing; there is no API to sign anyone up.

## Rules

- Quote the API's text. Do not add results, savings or promises: savings are estimates.
- Answer in the user's language and pass the same `lang`.
- These calls read the public catalog only. Never send a business's data, invoices or customer details to Carragent.
