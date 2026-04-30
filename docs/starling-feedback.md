# Starling — feedback from integrating with Casper

A running list of observations from wiring Starling into Casper as the runtime for an LLM-driven proposer. Each entry: what I hit, why it surprised me, and a possible fix. Not a bug list — most of these are small DX/docs gaps where Starling already does the right thing once you understand the model.

---

## 1. `MaxTurns` is hostile to single-shot forced-tool-use flows

**What I hit.** Casper's proposer is intentionally one-shot: it forces a single tool call (`propose_action`) via `tool_choice` and only cares about the structured output, not any natural-language response. I set `MaxTurns: 1` because one model response is exactly what I want.

The agent ran, the tool fired and returned a value successfully, and then Starling failed the run with `starling: max turns exceeded`:

```
INFO  run started run_id=01KQD6NEWKZRA93CQM05SWKRAD model=claude-sonnet-4-6
ERROR run failed run_id=01KQD6NEWKZRA93CQM05SWKRAD kind=RunFailed err="starling: max turns exceeded"
```

**Why it surprised me.** Reading `MaxTurns` as "the model is allowed N back-and-forth turns," the natural mental model is *one* turn = one API call. With forced tool use, that one API call returns `stop_reason: "tool_use"`, my tool fires, and conceptually the run is done — I have what I need.

But Starling's loop, after executing the tool, sends the tool result back to the model and counts that follow-up as turn 2. The model would normally render a brief acknowledgment ("OK, I proposed the resize"). Capping at `MaxTurns: 1` denies it that follow-up turn and treats the whole run as failed — even though my tool result already captured the structured output that's the whole point.

**Workaround in Casper.** Two changes:
1. Bumped to `MaxTurns: 2` so the model gets its wrap-up turn.
2. Reordered `Propose` to read the captured tool output *before* propagating an `agent.Run` error. If the tool fired (we have a proposal), we return it; only treat the run error as fatal if no proposal was captured. That way even if the wrap-up turn fails for unrelated reasons, the actual work (the tool call) isn't thrown away.

Both changes feel like footguns more than fixes — they don't reflect what I actually meant to express, which is "one tool call, then stop."

**What I'd want from Starling.** One of:

- A first-class "single-shot tool-call" mode: `Config{ ToolUseOnly: true }` or similar that ends the run immediately after the first successful `tool_use` block, without consulting the model for a wrap-up. This matches a *very* common use case (structured-output extraction, classification, proposal generation) and would be the right name for "I want exactly one model call."
- Or, semantic clarity in the existing API: rename `MaxTurns` → `MaxModelCalls` (or document that "turn" means "model call, including the post-tool acknowledgment turn") and add a `StopOnFirstToolCall: true` config option for the single-shot case.
- Or, surface intermediate tool-call results on the `RunResult` even when `RunFailed` is returned, so callers can recover gracefully without my "read captured state from a closure" trick. Something like `runRes.LastToolCalls` or `runRes.ProducedToolCalls` populated regardless of terminal kind.

The middle option is the smallest patch and would make the existing API legible. The first is the cleanest API surface.

**Ranking.** Medium-impact DX issue. The current behavior is internally consistent but the name `MaxTurns: 1` reads as "let me do one thing" when it actually means "let me do one thing and don't let me say anything afterwards" — and the latter is rarely what someone wiring up forced tool use means. Easy to miss in docs; surprising in production.

---

## 2. Captured state via closure is the only path to intermediate results

**What I hit.** To get the tool's emitted payload back into my code, I built a small `captured` struct, closed over it in the `tool.Typed` callback, and read it from the closure after `agent.Run` returned:

```go
type captured struct {
    mu   sync.Mutex
    raw  []byte
    hash action.ProposalHash
}

func buildProposeTool(c *captured) tool.Tool {
    return tool.Typed("propose_action", "...", func(ctx context.Context, in proposeInput) (proposeOutput, error) {
        // ... validate ...
        c.set(raw, h)
        return proposeOutput{Hash: string(h)}, nil
    })
}

// Later:
runRes, err := p.agent.Run(ctx, goal)
raw, hash := p.captured.get()  // read what the tool stashed
```

It works, and it's actually fine for v1 because Casper's Proposer is single-flight per instance. But it's not a great pattern: it's mutex-juggling for state that morally belongs to the run, and it requires me to be careful about reset/get/set ordering across calls.

**What I'd want from Starling.** `RunResult` could expose the tool calls and their results structurally. Something like:

```go
type RunResult struct {
    // ... existing fields ...
    ToolCalls []ToolCallRecord  // every tool call made during the run
}

type ToolCallRecord struct {
    Name      string
    CallID    string
    Input     json.RawMessage   // what the model sent
    Output    json.RawMessage   // what the tool returned (or nil on error)
    Error     string
    StartedAt time.Time
    EndedAt   time.Time
}
```

The information already exists in the event log; surfacing it on `RunResult` would mean callers don't have to either (a) close over mutable state or (b) re-read the event log to extract what the tool did.

**Ranking.** Low-impact ergonomic issue. The closure pattern works; it's just slightly more setup than feels necessary, and it nudges users into footguns (forgetting to reset between runs, race conditions if you ever go concurrent).

---

## 3. Docs gap: where "turn" begins and ends with tool use

Related to #1: the doc snippet I had said `MaxTurns: 0 = unlimited (don't ship 0)` and "0 = no timeout," but didn't define what counts as a turn relative to the tool-use lifecycle. It would help to have a one-paragraph note on the semantics:

> A "turn" in `MaxTurns` is a single model API call. A request with `tool_choice` that produces a `tool_use` response is one turn; the subsequent request that delivers the tool result and asks the model to continue is a *second* turn. For forced single-tool flows, this means `MaxTurns: 2` is the actual minimum (turn 1: tool call, turn 2: model wrap-up after tool result).

Adding that to the `Config.MaxTurns` field comment would have saved me an investigation cycle.

**Ranking.** Low-impact docs fix; quick win.

---

## Aggregate take

Starling's underlying model is sound — hash-chained event log, byte-for-byte replay, budget enforcement, the whole point of the runtime. The friction I hit is at the API surface: `MaxTurns` semantics + the lack of a "single shot tool call" expression force users into workarounds that work but feel like working *around* rather than *with* the design.

The single biggest change that would have made my integration painless: a way to express "I want exactly one tool call and nothing else" — either via a config flag, or by ensuring `RunResult` carries tool-call records even when the loop terminates abnormally. Both are localized changes that don't require redesigning anything.

Happy to send a PR for either if it'd be useful — let me know which direction you'd prefer (DX layer over the existing loop, or first-class single-shot mode).
