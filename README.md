# Aegis — Deterministic Execution Layer for AI Agents

**Let AI recommend actions. Let Aegis decide whether they happen.**

```
go run .
```

That's the whole demo. No API key required.

---

## The problem

You want an AI agent to handle refund requests. Direct tool access is out —
prompt injection or a hallucination can drain an account. So people reach
for **redaction/tokenization**: mask the real amount before it hits the
model.

That breaks in a specific, boring, expensive way:

```
Refund request: $600
Gateway masks it → sends the AI amount = $100 (or a meaningless token)
AI sees $100, which is under the $500 auto-approve limit → approves
Gateway executes the REAL payment: $600
```

**Redaction protects privacy and destroys business logic at the same time.**
The AI made a perfectly logical decision — on fabricated data.

## The idea

Don't send the AI real data, and don't send it fake data either. Send it
**categories**, computed deterministically from the real data by code you
control:

```
$600, account since 2018, 0 refunds this month
        │
        ▼  (deterministic, in your infra, never touched by the model)
amount_tier: HIGH · customer_status: VETERAN · return_history: CLEAN
        │
        ▼
   AI sees only this. Decides: ESCALATE.
        │
        ▼
Your code applies ESCALATE to the REAL $600 request.
```

The $500 threshold lives in a YAML file your engineers edit — never in a
prompt, never inferred by a model. The AI's job is bounded to judgment
calls ("is this combination of categories suspicious?"), never arithmetic
on real money. This also means there's nothing to "de-anonymize": the
model's answer is a decision label (`ALLOW` / `DENY` / `ESCALATE`), not
reconstructed data, so there's no rehydration step to get wrong.

**Scope note, read this before you trust it with money:** Aegis protects
data flowing through *tool calls* — the execution layer. If your user
types "please refund my $600" directly into the chat, that number already
reached the model's context the moment the agent read the message; no
tool-layer gateway can undo that. Aegis is the right layer for what the
*agent does*, not a substitute for input-side PII handling on what the
*user says*. Combine both if you need end-to-end coverage.

---

## Architecture

```
                         AI Agent (Claude, or anything MCP-compatible)
                                       │
                          tool_call: request_refund(request_id)
                                       ▼
┌──────────────────────────── Aegis ──────────────────────────────────┐
│                                                                     │
│   MCP Server (internal/mcpserver)                                   │
│        │                                                            │
│        ▼                                                            │
│   Store (internal/store) ── fetches REAL facts, never exposed  ─────┼──► never leaves this box
│        │                                                            │
│        ▼                                                            │
│   Context Engine (internal/context) ── real facts → categories      │
│        │                                                            │
│        ▼                                                            │
│   Agent.Decide() ── categories only ────────────────────► [ AI ]    │
│        │                                            (sees no real   │
│        ▼                                             data at all)   │
│   Decision Engine (internal/decision) ── hard_rules can override    │
│        │                                                            │
│        ▼                                                            │
│   Executor (internal/executor) ── acts on REAL data                 │
│        │                                                            │
│        ▼                                                            │
│   Audit Logger (internal/audit) ── JSONL, tied to policy_version    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

Each piece is a separate package so you can unit-test the category logic
with zero mocks of an LLM, and swap the `Agent` interface for anything —
Claude, GPT, a local model, or a plain rules engine — without touching
anything else.

**Zero external dependencies.** The YAML-subset parser (`internal/yamlite`)
and the whole pipeline are stdlib-only, so `go build` works offline. If you
need full YAML or CEL expressiveness in production, swap
`internal/yamlite` for `gopkg.in/yaml.v3` and/or `github.com/google/cel-go`
— `internal/policy` is the only caller.

---

## Try it

```bash
go run .                          # HIGH-amount case → escalated to a human
go run . -request req_002         # low amount, flagged history → denied
cat audit.log                     # every decision, tied to the policy version that made it
```

### With a real model instead of the built-in mock

```bash
export ANTHROPIC_API_KEY=sk-ant-...
go run .
```

`internal/decision/agent.go` has zero fields for amounts, names, or IDs
anywhere near the API call — that's not a policy, it's the type system.

### As a real MCP server (Claude Desktop / Claude Code)

```bash
go build -o aegis .
```

Add to your MCP client config (e.g. `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "aegis": {
      "command": "/absolute/path/to/aegis",
      "args": ["-serve"],
      "env": { "ANTHROPIC_API_KEY": "sk-ant-..." }
    }
  }
}
```

Now any MCP-compatible agent can call `request_refund(request_id)` and will
only ever get categories and a decision back — check
`internal/mcpserver/server.go`, the response text is built from
`trace.Categories` and `trace.FinalDecision`, never from the real request.

---

## Configuring it for your own business rules

Everything you'll touch day-to-day is `policies/refund_v1.yaml` — no Go
required:

```yaml
categories:
  amount_tier:
    field: amount          # any numeric field present in your facts
    tiers:
      - label: HIGH
        gte: 500            # first match wins, order strictest first
      - label: LOW
        gte: 0

hard_rules:
  - when: { amount_tier: HIGH }
    force_action: ESCALATE   # wins over the AI no matter what it said
    reason: "..."
```

To wire in your real database instead of `examples/refund/mock_db.json`,
implement the same two-method shape as `internal/store.Store` against your
actual DB/payments API — `Lookup(requestID) (Request, Customer, error)` —
and pass it into `pipeline.Pipeline`.

To add a new decision domain (KYC checks, claims, credit limits — anything
with numeric thresholds and a small action set), add a new policy file and
a new `Store`-like lookup; the pipeline, MCP server, and audit log are
domain-agnostic.

---

## Honest limitations (please read before a "Show HN")

- **Not a general-purpose PII redaction gateway.** It's narrower and more
  rigorous *within* that scope: automated decisioning with numeric
  thresholds. For open-ended chat, you still want text-level PII masking
  (e.g. Microsoft Presidio) upstream of this.
- **Free-text nuance is not yet categorized.** The demo only turns numeric
  facts into tiers. A real deployment probably also wants a
  `reason_category` derived from the complaint text (`DEFECTIVE_ITEM` vs
  `CHANGED_MIND`) — that's a natural next module, not yet built here.
- **Category boundaries are business logic and need governance.** If you
  change `amount_tier: HIGH` from `$500` to `$600`, bump the policy
  `version` — the audit log is only useful if it can answer "which rules
  were live at decision time."
- **This pattern isn't unclaimed territory.** Business rule engines
  (Drools, InRule, Taktile and others) are actively adding "AI as an
  isolated, swappable node, never the final authority" to their own
  platforms in 2026. Aegis is a small, readable, MIT-licensed reference
  implementation of that pattern for the MCP era — not a claim to have
  invented decision governance.
