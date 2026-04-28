# Trust Layer Design

The trust layer is the part of Casper that decides whether an action is allowed to happen, what it will cost if it does, who is permitted to authorize it, what credentials it runs under, and how the whole sequence is recorded so the decision can be re-examined later.

The execution layer is the *hands*. The trust layer is everything that decides whether the hands move.

This document describes what trust means in software-only form, the five properties the layer enforces, the components that enforce them, and how those components compose.

---

## The frame: software-only trust

In a real enterprise deployment, trust comes from a mix of software and process — SOC 2 reports, signed change-management procedures, legal indemnification, on-call rotations, internal review boards. Casper v1 deliberately ignores all of that. The bet is that **the software-enforceable subset is itself sufficient to make autonomous AI infra action credible**, and that the paperwork is layered on top later.

That means everything in this document must be enforced by code, observable in the system, and falsifiable from the audit log. If a property requires "we promise" or "the team agrees," it does not belong in v1.

---

## The five properties

A human delegate is trusted when five things are true. The trust layer encodes each as a software invariant.

| Property | Human meaning | Software invariant |
|---|---|---|
| **Predictability** | The delegate does what they said they'd do | The action that *executes* must be byte-equivalent to the action that was *approved* |
| **Reversibility** | Mistakes can be undone | Every action carries a rollback plan, executed automatically on verification failure |
| **Bounded authority** | They can only act within agreed scope | Each action runs under credentials narrowly scoped to that single action; nothing else is reachable |
| **Auditability** | There's a record of what they did and why | Every input, decision, API call, and response is captured in a tamper-evident log |
| **Skin in the game** | Someone is accountable | Every state transition has a named, authenticated principal attached — LLM, policy version, approver |

Each row maps to a component. The remainder of this document walks through each one.

---

## Component 1 — Predictability

**Goal:** what runs is what was approved. No drift, no late edits, no "the LLM regenerated the proposal."

**Mechanism:**

- The proposal is a typed object, hashed at the moment it enters the trust layer.
- Policy verdict, simulator output, and approval all reference that hash.
- The plan compiler input is the hashed proposal. The compiled plan is itself hashed and stored.
- The execution interpreter only runs plans whose source-proposal hash matches a *currently approved* proposal.
- If anything between proposal-time and execute-time touched the bytes, the hash mismatches and execution refuses to start.

**What this kills:**

- "We approved version A but ran version B."
- "The LLM was re-prompted between approval and execution and quietly changed the parameters."
- "Someone tweaked the params in the DB before the worker picked it up."

**What this requires from the schema:** a canonical serialization (sorted keys, fixed numeric formatting) so the hash is stable. Easy in JSON Schema land, easy to get wrong if you don't think about it day one.

---

## Component 2 — Reversibility

**Goal:** the system can undo what it did, automatically, when the action did not achieve its intent.

**Mechanism:**

- Every action's typed schema requires a `reversibility` classification: `reversible`, `partially_reversible`, or `irreversible`. This is computed by the simulator, not declared by the LLM.
- The plan compiler emits *two* plans: a forward plan and a rollback plan. The rollback plan is itself a typed sequence of steps the interpreter can execute.
- Verification is the trigger. If post-execution verification fails (the symptom didn't resolve, the metric didn't move, the resource didn't reach the expected state), the rollback plan runs automatically through the same interpreter.
- For `irreversible` actions, the policy engine treats `irreversible` as a first-class verdict input. The default policy denies them outright; a permissive policy can require an extra approval step.

**What this kills:**

- "The system silently assumed everything could be undone."
- "Rollback was a script someone had to run by hand."
- "The rollback was different code from the forward path so we don't know if it actually works."

**Honest limit:** some AWS operations cannot be reversed (storage shrink, certain deletions, parameter group changes that require restarts). Casper's job is not to make them reversible — it's to refuse them, or to surface "irreversible" as a label the human sees before approving. Lying about reversibility is worse than not having it.

---

## Component 3 — Bounded authority

**Goal:** an action can only do what it claims to do. If a proposal says "resize this one DB," the credentials it executes under cannot delete a different DB, modify a different service, or read unrelated data.

**Mechanism — the identity broker:**

- The user connects their AWS account by deploying a CloudFormation template that creates a role trusting Casper's account, with a required external ID and a *broad* permission set (the boundary of everything Casper might need to do across all action types).
- That role is never used directly to act.
- Per-action, the broker computes a **session policy** — an inline IAM policy attached to a single `AssumeRole` call — that narrows the role's effective permissions to *exactly* what this one action needs. For RDS resize on `orders-prod`: `rds:ModifyDBInstance` on `arn:aws:rds:…:db:orders-prod` and the read-only describes needed for verification. Nothing else.
- The session is short-lived (15 minutes). Credentials are passed only to the step interpreter and discarded after the plan completes.
- The session policy is hashed and recorded with every step execution. Cross-referencing AWS CloudTrail with the recorded session-name and policy-hash produces an external witness that the credentials had the claimed scope.

**What this kills:**

- "The agent had `*` permissions because scoping was hard."
- "The credentials were stored long-lived in our DB."
- "We can't tell from CloudTrail which proposal made which call."

**Why session policies and not separate roles:** a separate role per action type would still over-grant when called for a specific resource. Session policies let you scope to the *resource* the proposal names, not just the action class.

---

## Component 4 — Auditability

**Goal:** everything that happened can be re-derived from a single, append-only, tamper-evident log.

**Mechanism:**

- One `audit_events` table. Every state transition, every policy verdict, every LLM proposal, every approval, every AWS API call (request + response, verbatim, with AWS request ID), every verification result, every rollback step.
- Each row contains `prev_hash` and `hash = sha256(prev_hash || canonical(payload))`. Any tampering with a past row breaks the chain forward of that row.
- The audit log is the same data that drives the live execution UI. There is no separate "live log" and "audit log" — what the operator watched is what is permanent.
- Every event references the proposal hash (predictability), the policy version (decision provenance), the session name and policy hash (authority provenance), and the principal that triggered the transition (accountability).

**What an auditor can answer from the log alone:**

- What was proposed and by whom?
- What policy version evaluated it, and what did it return?
- Who approved it, and against which exact proposal-hash?
- What did the simulator predict?
- What credentials ran it, with what scope?
- What did each AWS call request and return, verbatim?
- Did verification pass or fail, on what data?
- If rollback ran, what did it do?
- Is the log itself intact? (chain check)

**What this kills:**

- "Logs captured what happened but not why."
- "Decisions and actions are in different systems."
- "We can prove the action ran but not that it ran with the right scope."

---

## Component 5 — Skin in the game

This is the hardest property to encode in software, and the place where v1 is most honestly "the start of an answer."

**Goal:** for every action that touched infrastructure, there is a named, authenticated principal accountable for the decision.

**Mechanism:**

- Every state transition records a principal: `llm:claude-sonnet-4.6@<prompt-version-hash>` for proposals, `policy:<version-hash>` for verdicts, `user:<authenticated-user-id>` for approvals.
- Approvals are signed: the approver's authentication token (or, in a stronger version, a webauthn signature) is attached to the approval event. The approval references the proposal hash. Non-repudiation by construction — an approver cannot later claim they approved something different.
- Policy versions are hashed and stored. "The policy that allowed this" is a verifiable artifact, not "the policy that was running at the time, probably."
- LLM proposals record the model ID, prompt version hash, and tool schema hash. "The model said this under these exact instructions" is reproducible.

**Honest limit:** software cannot create accountability where none exists; it can only make accountability *unambiguous*. If an organization wants the LLM's proposer to "be accountable," that's a process question, not a software one. Casper's job is to make sure that whatever the accountability story is, the *facts* the story relies on are tamper-evidently recorded.

---

## How the components compose

The trust layer is not a pipeline; it is a set of invariants that must all hold by the time execution begins. The order in which they are evaluated is:

```
proposal arrives
   │
   ▼
[predictability]  hash the proposal; freeze the bytes
   │
   ▼
[reversibility]   simulator classifies; rollback plan compiled
   │
   ▼
[bounded authority]   policy evaluates with the proposal + simulator output;
                      verdict references the proposal hash and policy version
   │
   ▼
human approval (if needed)   signed against the proposal hash
   │
   ▼
[bounded authority]   identity broker mints session-scoped credentials
   │
   ▼
execution begins   under credentials narrowed to this proposal
   │
   ▼
[auditability]    every step writes hash-chained audit events
[skin in the game] every transition names its principal
```

If any invariant is missing — proposal not hashed, no rollback plan, no policy verdict, no signed approval, no scoped credentials, no audit chain — execution refuses to start. These are not warnings. They are preconditions enforced by the worker.

---

## What the trust layer is *not*

Worth saying explicitly, because the failure mode of projects in this space is to keep adding things until the trust layer becomes another vague "platform":

- **Not an agent framework.** It does not orchestrate the LLM. The LLM is upstream, produces a proposal, and is done.
- **Not a policy DSL design exercise.** Rego exists; v1 uses it.
- **Not a runbook system.** It does not store or execute pre-canned procedures. Every action is a fresh proposal evaluated fresh.
- **Not a chat interface.** The NL intent is a one-shot input to the proposal layer. The trust layer never has a "conversation" with the user.
- **Not a monitoring product.** It reads metrics for verification, but it does not page, alert, or own dashboards.
- **Not a secrets manager.** It mints short-lived credentials per action; long-lived secrets are not its concern.

Each of these is a real product category. Casper sits next to them, not on top of them.

---

## What "the trust layer works" looks like

Three concrete demos prove the layer:

1. **The honest path.** A reasonable proposal flows through, gets a clean policy verdict, gets approved, executes, verifies, completes. The audit log is browsable and complete.
2. **The verification-failure path.** A reasonable proposal executes, but the success metric does not move. Rollback runs automatically. The audit log shows the forward run and the rollback run as two coherent sequences against the same proposal.
3. **The adversarial-LLM path.** The proposal is crafted (or coaxed from the LLM) to do something out of scope — a different DB, a different service, a wider blast radius than predicted. Each invariant catches it independently: schema validation rejects malformed proposals, the simulator flags the mismatch, the policy engine denies it, and (in the worst case where all upstream checks were misconfigured) the session policy on the AWS credentials makes the offending API call fail at AWS itself.

The third demo is the important one. A trust layer is not credible because it works in the happy case. It is credible because it has *defense in depth*: each invariant is independently sufficient to stop a bad action, so a bug in any one of them does not collapse the whole.

---

## What v2 adds (deliberately deferred)

For completeness, so v1 can stay narrow:

- **Policy-as-code with versioned bundles** — Rego pulled from git, signed, distributed.
- **Multi-party approval** — quorum, separation-of-duty, role-based approval routing.
- **Hardware-rooted signatures** — webauthn or YubiKey on approvals.
- **External audit witness** — periodic audit-log root hash published to an external store (S3 with object lock, or a transparency log).
- **Per-tenant isolation** — separate hash chains, separate credentials, separate UIs.
- **Action-class registry** — discovery and certification of action types, versioned independently.

None of these change the v1 architecture. They are extensions of components that already exist.

---

## Summary

The trust layer is five invariants — predictability, reversibility, bounded authority, auditability, accountability — each enforced by a specific piece of software (proposal hashing, rollback plans, identity broker, audit chain, signed-principal events) and composed so that every one of them must hold before the execution layer is allowed to act.

The LLM produces proposals. The trust layer produces *trust*. The execution layer produces *evidence*. Together they make autonomous infrastructure action something a human can sign their name next to without flinching.
