# The Proposer Agent

Casper has exactly one LLM-driven component: the **Proposer**. This document specifies what it does, what it deliberately does *not* do, and how it is built using the [Starling](https://starling.jerkeyray.com) framework.

The Proposer is intentionally tiny. The thesis of Casper is that the LLM is ~20% of the system; the Proposer is the entire ~20%. Resist the urge to grow it.

---

## What the Proposer does

**One job:** turn a natural-language intent + a read-only snapshot of relevant infra state into exactly one structured proposal that conforms to Casper's action JSON Schema.

Inputs:

- An NL intent string (e.g. *"orders-prod RDS is at 90% CPU"*)
- A read-only context bundle: current RDS instance class, recent CloudWatch metrics, tags, related resources, time window
- The action schema (JSON Schema for `RDSResizeProposal`, etc.)

Output:

- One validated proposal object, persisted with `status='proposed'`
- A short, human-readable rationale string captured alongside it
- The model ID, prompt-version hash, tool-schema hash, and Starling run ID (for the audit log)

That's it. The Proposer never executes anything, never approves anything, never decides whether something is safe. It writes a row in the `proposals` table and exits.

## What the Proposer does *not* do

- **No multi-turn reasoning.** Single shot. If the model wants more context, the answer is to put more context in the prompt, not to give it tools to fetch more.
- **No write tools.** The only tool exposed is the proposal-emitting tool. There is no `modify_db`, no `delete_anything`, no `assume_role`.
- **No reading AWS at agent-time.** The infra snapshot is collected by deterministic Go code *before* the agent runs and passed as prompt context. The agent has no live AWS access.
- **No memory across runs.** Each Proposer invocation is independent. State lives in Casper's DB, not in the agent.
- **No decisions about reversibility, cost, or policy.** Those are the simulator's and policy engine's jobs. The Proposer can mention concerns in its rationale, but it has no veto.

If a future feature seems to require any of the above, that's a signal to add a *deterministic* component, not to grow the agent.

---

## Why Starling

The Proposer is the only LLM-driven component in Casper, and it sits *upstream* of the trust layer. That means a malformed, drifted, or non-reproducible Proposer run is not just an agent bug — it's a hole in the trust story. Whatever runtime hosts the Proposer has to make the agent run itself a tamper-evident, replayable, bounded artifact, or the rest of Casper's invariants are partially aspirational.

We could write that runtime by hand. The substantive pieces would be: a per-run event log, a deterministic replay path, hard budget enforcement, a provider abstraction, and tool-call plumbing. Each is non-trivial and easy to get subtly wrong. None of them are the point of Casper. Building them ourselves is exactly the kind of yak-shaving the project's "trust layer first, LLM last" ordering is designed to avoid.

[Starling](https://starling.jerkeyray.com) is an event-sourced agent runtime for Go that already does this work, with three properties that map one-to-one onto Casper's invariants:

- **Hash-chained event log (BLAKE3, Merkle root) → Auditability.** Every Starling run produces a tamper-evident log of every state change — prompts, tool calls, tool results, assistant messages — committed to a Merkle root in the terminal event. This is the same shape as Casper's `audit_events` chain. We treat the Starling log as the *proposer-scoped* audit and link it from `audit_events` by run ID. Without this, the Proposer would be the one component in the system whose internal behavior is *not* tamper-evidently recorded — a gap directly under the LLM, which is the least-trusted component.

- **Byte-for-byte replay → Predictability.** `starling.Replay` re-executes a run against the recorded events and surfaces the first divergence as a typed `replay.Divergence`. The predictability invariant says "the action that executes must be byte-equivalent to the action that was approved." Replay lets us prove, months later, that the proposal on file was actually what the recorded model run produced — not something edited, regenerated, or quietly substituted. Without replay, "the LLM said this under these inputs" is a claim we cannot verify after the fact.

- **Budget enforcement (tokens, USD, wall-clock) → Bounded authority for the agent itself.** The trust layer bounds what the *infra action* can do. Budgets bound what the *agent run* can do. A runaway Proposer that loops, calls tools repeatedly, or burns context is a runtime fault we want stopped by the host, not by code we wrote. Token, USD, and wall-clock ceilings are first-class in Starling and enforced by the runtime, so a misbehaving prompt fails closed instead of silently costing money or producing noisy partial output.

Two more practical reasons reinforce the choice:

- **Go-native.** Casper is Go end-to-end (interpreter, policy, identity broker, persistence). A Python or TS agent runtime would mean two languages, two deployment surfaces, and an RPC seam between the Proposer and the proposal-persistence path. Starling lets the `propose_action` tool implementation live in the same process as the rest of Casper and write to Postgres directly.
- **MCP adapter available.** v1 passes the infra snapshot as plain prompt text. If we ever want to expose read-only AWS context as an MCP server (so the agent can pull a fresh snapshot at run-time, with the snapshot fetch itself audited), Starling's MCP adapter is the path. We don't need it now, but choosing a runtime that can grow that direction is cheap.

What we are *not* getting from Starling: policy decisions, schema validation, simulation, execution, rollback, or anything downstream of "the model emitted a proposal." Those are Casper's job and stay Casper's job. Starling's scope ends at "the agent run is auditable and bounded"; that is exactly the seam we need.

---

## Architecture in one picture

```
NL intent ──┐
            │
infra       ▼
snapshot ─► Context Builder (Go, deterministic)
            │
            ▼
          Starling Agent
          ┌──────────────────────┐
          │ Provider: Anthropic   │
          │ Tools: [propose]      │
          │ Config: MaxTurns=1    │
          │ Log: hash-chained     │
          │ Budget: tight         │
          └──────────────────────┘
            │
            ▼
        Tool call: propose_action(<schema-validated payload>)
            │
            ▼
       Proposer adapter
            │
            ▼
   Insert into proposals table
   Link Starling run-id into audit_events
            │
            ▼
        status='proposed'
```

Downstream of `status='proposed'`, the simulator, policy engine, approval flow, and execution layer take over — none of them call the LLM.

---

## How Starling is wired

Based on Starling's quickstart and MCP docs:

### Imports

```go
import (
    "github.com/jerkeyray/starling"
    "github.com/jerkeyray/starling/eventlog"
    "github.com/jerkeyray/starling/provider/anthropic"
    "github.com/jerkeyray/starling/step"
    "github.com/jerkeyray/starling/tool"
)
```

(Provider package path follows the same pattern as `provider/openai` shown in the quickstart; confirm exact path when wiring.)

### Provider

```go
prov, err := anthropic.New(
    anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
)
```

Model: `claude-sonnet-4-6` (or the latest Sonnet at build time). Sonnet is the right tier — it follows tool-schema constraints reliably and is meaningfully cheaper than Opus for a one-shot tool call.

### The single tool

The Proposer exposes exactly one tool: `propose_action`. Its input schema *is* Casper's action JSON Schema. Forcing tool use guarantees the model cannot emit free text we'd have to parse.

```go
type ProposeInput struct {
    ActionType  string          `json:"action_type"` // e.g. "rds_resize"
    Payload     json.RawMessage `json:"payload"`     // validated against schema for ActionType
    Reasoning   string          `json:"reasoning"`
}

type ProposeOutput struct {
    ProposalID string `json:"proposal_id"`
}

propose := tool.Typed(
    "propose_action",
    "Emit exactly one structured infrastructure-change proposal. This is the only action you can take.",
    func(ctx context.Context, in ProposeInput) (ProposeOutput, error) {
        if err := validateAgainstSchema(in.ActionType, in.Payload); err != nil {
            return ProposeOutput{}, err
        }
        id, err := persistProposal(ctx, in)
        if err != nil {
            return ProposeOutput{}, err
        }
        return ProposeOutput{ProposalID: id}, nil
    },
)
```

The tool implementation is the *adapter* between Starling and Casper: it validates against the action JSON Schema, hashes the proposal, inserts the `proposals` row, and emits the corresponding `audit_events` row referencing the Starling run ID.

### The agent

```go
a := &starling.Agent{
    Provider: prov,
    Tools:    []tool.Tool{propose},
    Log:      eventlog.NewPostgres(db),  // or NewInMemory + persist run after
    Config: starling.Config{
        Model:    "claude-sonnet-4-6",
        MaxTurns: 1,
    },
}

res, err := a.Run(ctx, buildPrompt(intent, snapshot))
```

`MaxTurns: 1` is deliberate. The model gets one turn to call `propose_action`. If it doesn't, the run fails and we surface the failure in the UI — there is no "try again with a different prompt" inside the agent.

### The prompt

The user message contains, in order:

1. The NL intent (verbatim from the human)
2. A serialized infra snapshot (current resource state, recent metrics, tags)
3. The list of available action types and a pointer to the schema (the schema itself is the tool definition)
4. Hard constraints: must call `propose_action` exactly once; must not write free-form text outside the tool call

The system prompt is fixed across runs and hashed. Its hash goes into `audit_events` so we can prove which prompt produced which proposal.

We use Anthropic prompt caching on the system prompt and tool schema — both are stable across runs and dominate token cost.

### Budget

```go
Budget: starling.Budget{
    MaxTokens:    20_000,
    MaxUSD:       0.20,
    MaxWallClock: 30 * time.Second,
},
```

A single Sonnet call with a few KB of context fits easily under these. The point isn't cost; it's that a runaway agent has a hard ceiling enforced by the runtime.

### Replay

After a proposal is approved and executed, the Proposer's Starling run can be replayed:

```go
divergence, err := starling.Replay(ctx, recordedEvents, freshAgent)
```

If the recorded events still produce the same proposal byte-for-byte, the run is verifiable. If not, `divergence` tells us exactly where the model output drifted (which would suggest model regression, prompt drift, or tampering). For v1 we wire replay as a CLI command, not an automatic check — but the property exists and can be exercised.

---

## How the Proposer's events link into Casper's audit log

Two log chains exist:

- **Starling event log** — proposer-scoped, written by the Starling runtime, hash-chained internally.
- **Casper `audit_events`** — system-scoped, written by every layer.

The link: when the `propose_action` tool fires, the adapter writes one `audit_events` row of kind `proposer.proposed` whose payload includes the Starling run ID, the run's terminal Merkle root, the model ID, and the prompt-version hash. From any proposal in Casper, you can pivot into the full Starling trace by run ID, and from the Starling trace you can verify it produced exactly the bytes that became the proposal.

This is the right separation of concerns: Starling guarantees the *agent run* is auditable; Casper guarantees the *system* is auditable; the run ID is the join key.

---

## Failure modes and how they're handled

- **Model refuses to call the tool.** Run ends with no proposal; UI shows "the agent declined to propose an action" with the model's free-text response (if any) for the human to read. No retry inside the agent.
- **Model emits invalid payload (schema violation).** Tool returns an error; Starling records the tool error event; agent's single turn is over. UI shows the validation error verbatim.
- **Budget exceeded.** Starling kills the run; UI shows "budget exhausted." No partial proposals are persisted.
- **Provider error / network failure.** Starling marks the run as transient; the *user* can retry, but the agent itself does not auto-retry. We do not want silent retries producing different proposals.
- **Schema drift between recording and replay.** `replay.Divergence` surfaces it; the recorded proposal is still valid (it's the bytes on file), but we know the agent code has moved.

In every failure mode, the principle is the same: a failed agent run produces an audit record of the failure and stops. It does not produce a proposal that any other layer might act on.

---

## Build order

1. Define the action JSON Schema for `RDSResizeProposal` (Block 1 of the project — already on the roadmap).
2. Stand up Starling with a stub `propose_action` tool that just persists what it receives. Hard-code an intent and snapshot. Confirm the proposal row appears.
3. Build the context builder — Go code that reads RDS describe + CloudWatch and produces the snapshot. Deterministic, no LLM.
4. Wire the real prompt: system prompt, user message, tool definition, budget.
5. Wire the audit-events join: Starling run ID and Merkle root into `audit_events`.
6. Add the replay CLI command.
7. Hook the Proposer to the UI's "submit intent" form.

Estimated scope: a long weekend if the action schema and DB tables already exist. The LLM part is genuinely the small part of this project — Starling does most of the heavy lifting we'd otherwise build by hand (event log, replay, budget, provider abstraction).

---

## When (not if) to add a second agent

The one place a second agent is plausibly worth building, in v2:

- **The Critic.** A separate Starling run that reads the Proposer's structured output and tries to find reasons to reject it — wrong resource, blast radius understated, success criteria too loose, irreversible action mislabeled. It cannot approve anything. Its output is a list of warnings the policy engine and human reviewer see.

The Critic would use Starling identically — single tool (`emit_warnings`), one turn, tight budget, hash-chained log. Its run ID joins to the same proposal in `audit_events`.

This is genuinely useful (adversarial review is a strong trust pattern) but it is *not* required to prove the v1 thesis. Build it only after the v1 demo lands.

---

## Summary

One agent. One tool. One turn. Starling handles the runtime, the audit chain, the replay, and the budget. The action schema handles structure. Casper handles everything that happens after the proposal is written.

The agent is the small part of the system. That is the point.
