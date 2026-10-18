---
name: compare-carragent-plans
description: Use when a user asks what Carragent costs, which plan to start with, how the plans differ, whether there is a free trial or what the refund terms are. Reads the plans, prices and terms from Carragent's public API every time, so the numbers are never from memory.
license: MIT
compatibility: Needs network access to carragent.com. No account, key or login.
metadata:
  author: Carragent
  version: "1.0.0"
---

# Compare Carragent's plans

Carragent's plans, prices and terms are public. Read them from the API every time: they can change, and memory does not.

## When to use

- The user asks what Carragent costs, which plan fits, or how the plans differ.
- The user asks about the free trial, refunds, a setup fee or a contract.

## Steps

1. Fetch the plans. MCP: the `list_plans` tool. Pass `lang=es` or `lang=fr` for Spanish or French text.

   ```sh
   curl "https://carragent.com/api/v1/plans?lang=en"
   ```

2. Read the answer:
   - `data` has one object per plan: `id`, `name`, `summary`, `forWho`, `recommended`, `selfServe`, `monthlyCents`, `yearlyCents` and `limits`.
   - Prices are integers in US cents (divide by 100 for dollars) and `currency` is always `USD`. `yearlyCents` is null when the plan is quoted.
   - `selfServe: false` means the plan is quoted by email (admin@carragent.com), and `monthlyCents` is its starting price.
   - `limits` has `agents`, `connections`, `messagesPerMonth` and `seats`. null means unlimited or quoted.
   - `recommended` marks the plan Carragent suggests starting with.
   - `terms` has `trialDays` (the free trial), `annualRefundDays` (a yearly plan cancelled within that many days is refunded in full), `setupFee` (0) and `contract` (false).
3. Compare on what the user cares about: the monthly and yearly price, and the limits that would bind (agents, connections, messages, seats). If you work out a difference, such as twelve times `monthlyCents` against `yearlyCents`, show the numbers and say it is arithmetic on the listed prices.
4. Answer in plain sentences with the currency and the tax note, then the trial and refund terms.

## Rules

- Prices are in US dollars, plus applicable taxes. Say so each time you quote one.
- Quote the API's answer from this conversation, never a price from memory or from an older page.
- Do not promise results, savings or a return. Any figure about time or money saved is an estimate, and it comes from the owner's own numbers.
- Do not sign the user up or ask for payment details. The owner starts the free trial at https://carragent.com/en/pricing, which also says what the trial needs and how to cancel: point there instead of guessing.
