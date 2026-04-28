# Casper — Product Requirements Document

## 1. Overview

Casper is an **AI trust layer for cloud infrastructure**. It sits between an LLM and a real cloud account and enforces, in code, the properties that make autonomous infra action credible: predictability, reversibility, bounded authority, auditability, and accountability.

This is a side project, not a startup. It is optimized for learning depth, shippability, and proof-of-thesis — not for breadth of integrations or commercial readiness.

## 2. Problem

LLMs are already capable enough to operate cloud infrastructure. They can read metrics, reason about problems, and propose actions a competent SRE would propose. The reason autonomous AI infra agents aren't deployed in production isn't that the AI isn't smart enough — it's that there's no **trust layer** between the AI and the infrastructure.

Existing "AI for infra" products fall into two buckets:

- **Suggestion mode** — AI tells a human what to do, the human does it manually. Safe but slow; defeats the autonomy goal.
- **Raw execution mode** — AI is given API credentials and a tool-calling loop. Demos well, falls apart in production because nothing prevents catastrophic actions.

Both miss the actual product: the deterministic layer in between that lets the AI act, but enforces predictability, reversibility, and accountability on every action.

## 3. Thesis

The LLM is ~20% of the system. The other 80% is the deterministic infrastructure that makes its output safe to act on:

- A typed action format the LLM emits (not free-form code)
- A workflow engine that runs actions durably and idempotently
- A policy engine that decides what's allowed
- A simulator that predicts outcomes before commit
- An identity broker that gives short-lived, narrowly-scoped credentials per action
- A verifier that confirms the action did what was intended
- An audit log that captures the full causal chain

Build these well and the LLM becomes swappable. Build them badly and no model improvement saves you.

## 4. Goals (v1)

- Prove the trust layer works end-to-end against real AWS, on **one action**: RDS instance resize.
- Make the trust enforceable from the audit log alone — anyone reading the log can re-derive what happened and why.
- Keep the LLM strictly upstream of the trust layer. No LLM access to AWS at execution time.

### Non-goals (v1)

- Multi-tenancy, SSO, RBAC, organizations.
- Process/compliance layers (SOC 2, change management, on-call).
- Multiple cloud providers, multiple actions, runbooks, chat UI.
- Anything that requires "we promise" or "the team agrees" — software-only trust.

## 5. Users

- **Primary:** the project author (single tenant, single AWS account) and any technical reviewer evaluating the thesis.
- **Implicit:** an SRE or platform engineer assessing whether they'd trust this pattern for their own org.

## 6. Scope

**One cloud (AWS), one service (RDS), one write action (resize).** Everything else underneath the action — the trust layer — is built properly so that adding a second action is additive, not a rewrite.

## 7. User-facing requirements

A web app where:

1. A personal AWS account can be connected via cross-account role (CloudFormation template + external ID).
2. A natural-language intent like *"my orders-prod RDS is at 90% CPU"* can be typed in.
3. The system produces a structured proposal containing: action, reasoning, predicted blast radius, rollback plan, policy verdict.
4. The proposal can be reviewed in a UI and approved or rejected.
5. On approval, the action executes against real AWS under per-action scoped credentials.
6. The system verifies the outcome and rolls back automatically if verification fails.
7. Every step is in an append-only, hash-chained audit log that can be browsed.

## 8. Functional requirements

| Area | Requirement |
|---|---|
| Action contract | Typed JSON Schema for proposals; canonical serialization for stable hashing |
| Proposal layer | NL intent → one structured proposal via Claude Sonnet 4.6 tool use; single turn; no live AWS access |
| Simulator | Predicts blast radius (downtime, cost, reversibility class) before approval |
| Policy engine | OPA/Rego — allow / deny / needs-approval, with `irreversible` as a first-class input |
| Approval | Signed against the proposal hash; non-repudiable |
| Identity broker | STS AssumeRole with per-action session policy scoped to the exact resource; 15-minute credentials |
| Execution core | Single deterministic interpreter; only code that touches AWS; SDK retries disabled |
| Plans | Proposal compiles into typed `ExecutionPlan` (forward + rollback); both persisted before any AWS call |
| Verification | Post-execution verification is steps in the plan; failure auto-triggers rollback |
| Workflow / state | Postgres state machine; per-step transactional commits; resumable after crash |
| Audit log | Single append-only `audit_events` table; `hash = sha256(prev_hash \|\| canonical(payload))` |
| Live UI | Live execution view reads from the same rows that become the permanent audit log |
| Dry-run | Shares ~95% of code path with real execution; write calls swapped for `DryRun` or read-only equivalents |

## 9. Non-functional requirements

- **Determinism** — no SDK retries, no clock-dependent decisions inside steps, no AWS access outside the interpreter.
- **Tamper-evidence** — audit log hash chain verifiable end-to-end; AWS request IDs cross-reference CloudTrail.
- **Recoverability** — process can be killed mid-execution and resume from the last committed step.
- **Single-tenant, single-VPS** — hosted off AWS deliberately (Hetzner / Fly.io).

## 10. Architecture

| Layer | Responsibility | Stack |
|---|---|---|
| Action contract | Typed JSON Schema for proposals; LLM output target | JSON Schema + Go structs |
| Execution core | Runs typed plans against AWS; verifies; rolls back | Go + `aws-sdk-go-v2` |
| Policy engine | Allow / deny / needs-approval decisions | OPA (Rego) embedded |
| Simulator | Predicted blast radius (downtime, cost, reversibility) | Go + AWS Pricing API |
| Workflow / state | Durable state machine, idempotent execution | Postgres + `pgx` |
| Identity broker | Per-action short-lived scoped AWS credentials | STS AssumeRole + session policies |
| Proposer | NL intent → structured proposal | Starling + Anthropic API (Claude Sonnet 4.6) |
| Audit log | Append-only, hash-chained, tamper-evident | Postgres |
| UI | Connect AWS, submit intent, review, approve, watch, audit | Next.js + Tailwind + shadcn/ui |
| Hosting | Single VPS — kept off AWS deliberately | Hetzner / Fly.io |

Component-level designs live in:

- [`TRUST_LAYER.md`](./TRUST_LAYER.md) — the five invariants and how they compose.
- [`EXECUTION_LAYER.md`](./EXECUTION_LAYER.md) — plans, the interpreter, persistence, transparency design.
- [`PROPOSER.md`](./PROPOSER.md) — the single LLM-driven component, built on Starling.

## 11. Success criteria

The v1 demo is "done" when, against a real AWS account, the system can show three flows end-to-end:

1. **Honest path** — reasonable proposal → policy allow → approval → execution → verification → done. Audit log is browsable and complete.
2. **Verification-failure path** — proposal executes, success metric does not move, rollback runs automatically, audit log shows forward + rollback as two coherent sequences against the same proposal.
3. **Adversarial path** — a malformed or out-of-scope proposal is independently rejected by schema validation, simulator, policy engine, *and* (in the worst case) the AWS-side session policy. Defense in depth is observable.

The execution layer is "done" when, looking only at the audit log, anyone can answer:

- What was proposed?
- What was the plan?
- Which credentials executed it (with what scope)?
- What did each AWS call return, verbatim?
- Did verification pass? On what data?
- If it rolled back, what did the rollback do?
- Has the log been tampered with?

## 12. Phased plan

Each phase has a single thing it proves. Don't move on until that thing is real.

- **Phase 0 — Spec the action.** Inputs, preconditions, observable effects, failure modes, rollback, success post-conditions for RDS resize. On paper.
- **Phase 1 — Typed action layer (no AI).** CLI takes a hand-written proposal JSON, validates, executes, verifies, rolls back. *Proves: the deterministic core works end-to-end on real infra.*
- **Phase 2 — Policy + simulation.** OPA-embedded policy engine + per-action predictor for blast radius. *Proves: the system reasons about an action before it runs, independent of who proposed it.*
- **Phase 3 — Durable workflow + audit log.** Postgres state machine, hash-chained audit. *Proves: recoverability and auditability, not just happy-path functionality.*
- **Phase 4 — Identity broker.** Cross-account role assumption with per-action session policies; 15-minute credentials. *Proves: bounded authority is real.*
- **Phase 5 — Proposer.** Starling + Claude Sonnet 4.6 tool use against the action schema; read-only AWS state in context. *Proves: the central thesis — the LLM is the swappable 20%.*
- **Phase 6 — Web UI + approval flow.** Connect AWS, submit intent, review proposal with reasoning + blast radius + policy verdict + rollback plan, approve, watch live timeline, browse audit. *Proves: a non-builder sees the thesis in 60 seconds.*
- **Phase 7 — Second action (optional).** A differently-shaped action (e.g. ECS service scaling) through the same trust layer. *Proves: extensibility is real.*

**Order matters: trust layer first, LLM last.** Building the LLM first forces corner-cutting on the trust layer to make demos work — exactly the failure mode this project is criticizing.

## 13. Risks

- **The one-action trap** — building so tightly around RDS resize that the trust layer takes its shape. Mitigation: design schema and policy assuming a second action exists.
- **Verification is harder than execution** — calling `ModifyDBInstance` is one call; confirming the symptom resolved requires polling, threshold definitions, and time.
- **Not everything is reversible** — some AWS operations are one-way (storage shrink, deletions). Make `irreversible` a first-class policy verdict, not a silent assumption.
- **The LLM happy path lies** — single intents demo fine; the trust layer's value shows up under ambiguous or adversarial intents. Test those deliberately.
- **Simulator drift** — predictions that don't match reality are anti-trust. Keep predictions narrow and verifiable.
- **Durable workflow is where projects rot** — "what happens if the server dies mid-resize" has no demo payoff and is easy to skip.
- **IAM is a tar pit** — per-action scoped roles always take longer than expected the first time.
- **Audit log as afterthought** — bolted on, it captures state but not causality. Designed in week 1, it captures both.
- **Scope creep through SRE features** — runbooks, on-call, Slack bots. None prove the thesis. The thesis is proven by *one action, fully trusted*.
- **Premature workflow engine** — Temporal is the architecturally right answer and the wrong week-one answer. Postgres state machine first.
- **Treating the LLM proposal as truth** — the proposal is an *input* to the trust layer, not a trusted artifact. Every guarantee must hold even if the LLM is adversarial.

## 14. Open questions

- Action schema: per-type structs, or generic intent-envelope?
- Rollback model: separate computed action, per-type inverse, or snapshot/restore primitive?
- Verification window: fixed timeout, adaptive, or user-specified?
- LLM structured output: tool use (chosen), JSON mode, or constrained generation?
- Durability: hand-written Postgres state machine for v1; Temporal later if Phase 7 demands it.
- AWS connection: cross-account role with external ID (chosen) over stored access keys.
- LLM read access to live infra state: yes, but as prompt context, not as tools (no multi-turn).
- Reasoning in the audit log: store the final structured proposal + the LLM's stated rationale; not the raw chain-of-thought.

## 15. Status

Pre-Phase-0. Specifying the action contract next.
