# Starling: full agent context

Starling is a Go runtime for building LLM agents where every run is
recorded as an append-only, BLAKE3-chained, Merkle-rooted event log.
Replay re-executes a recorded run byte-for-byte against the same
agent wiring. Resume reconstructs state from the log after a crash.

This block is the operational truth. Use it when writing Starling code.

---

## Imports you almost always need

```go
import (
    starling "github.com/jerkeyray/starling"
    "github.com/jerkeyray/starling/eventlog"
    "github.com/jerkeyray/starling/provider/openai"   // or anthropic / gemini / openrouter
    "github.com/jerkeyray/starling/replay"
    "github.com/jerkeyray/starling/step"
    "github.com/jerkeyray/starling/tool"
)
```

---

## 1. Agent

```go
type Agent struct {
    Provider  provider.Provider // required
    Tools     []tool.Tool
    Log       eventlog.EventLog // required
    Budget    *Budget           // nil disables every axis
    Config    Config
    Namespace string            // optional run-id prefix; must not contain "/"
    Metrics   *Metrics          // nil disables metrics
}

type Config struct {
    Model                  string             // required, non-empty
    SystemPrompt           string             // hashed into RunStarted
    Params                 cborenc.RawMessage // vendor-specific
    MaxTurns               int                // 0 = unlimited (don't ship 0)
    RequireRawResponseHash bool               // audit-grade
    AppVersion             string
    EmitTimeout            time.Duration      // 0 = no timeout
    SkipSchemaCheck        bool               // tests only
    Logger                 *slog.Logger       // nil → slog.Default()
}
```

**`Run` validates:** Provider non-nil, Log non-nil, Config.Model
non-empty, Namespace has no `/`, all tool `Name()`s unique. Failure
returns a typed error before the first event is appended.

```go
func (a *Agent) Run(ctx context.Context, goal string) (*RunResult, error)
func (a *Agent) Resume(ctx context.Context, runID, extraMessage string) (*RunResult, error)
func (a *Agent) ResumeWith(ctx context.Context, runID, extraMessage string, opts ...ResumeOption) (*RunResult, error)

// ResumeOption: WithReissueTools(bool); default true.

type RunResult struct {
    RunID         string
    FinalText     string
    TurnCount     int
    ToolCallCount int
    TotalCostUSD  float64
    InputTokens   int64
    OutputTokens  int64
    Duration      time.Duration
    TerminalKind  event.Kind  // RunCompleted | RunFailed | RunCancelled
    MerkleRoot    []byte
}
```

**Sentinel errors (root package):**

- `starling.ErrNonDeterminism`     — replay diverged
- `starling.ErrPartialToolCall`    — Resume saw pending tools, WithReissueTools(false)
- `starling.ErrRunNotFound`        — Resume on unknown runID
- `starling.ErrRunAlreadyTerminal` — Resume on terminal run
- `starling.ErrRunInUse`           — chain advanced between read and append
- `starling.ErrSchemaVersionMismatch` — recorded schema unsupported

---

## 2. Determinism contract — `step.*`

Anything non-deterministic inside the agent loop OR inside a tool MUST
go through `step.*`. Replay re-executes tool bodies live, so
unwrapped non-determinism diverges every time.

```go
now := step.Now(ctx)            // time.Time; recorded once, replayed
n   := step.Random(ctx)         // uint64;    recorded once, replayed
val, err := step.SideEffect(ctx, "stable-name", func() (T, error) {
    // any HTTP, filesystem, RPC, MCP call
})
```

**Reserved names:** `"now"` (used by step.Now), `"rand"` (used by
step.Random). Don't use these as your own SideEffect names.

`step.Now`, `step.Random`, and `step.SideEffect` PANIC if `ctx` was
not derived from an active agent run. Don't fork goroutines inside a
tool without propagating ctx.

The `name` argument to `step.SideEffect` is the lookup key on replay.
Same logical effect → same name. Different name on replay than on
record → divergence.

---

## 3. Tools

```go
type Tool interface {
    Name() string
    Description() string
    Schema() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

func tool.Typed[In, Out any](
    name, description string,
    fn func(context.Context, In) (Out, error),
) Tool
```

`In` must be a struct (panics at construction otherwise). `Out` is
JSON-marshalled. `Execute` recovers panics and wraps with
`tool.ErrPanicked`.

**Retries on transient errors:**

```go
import "github.com/jerkeyray/starling/tool"

return out, fmt.Errorf("upstream timeout: %w", tool.ErrTransient)
```

Caller declares retry policy:

```go
type ToolCall struct {
    CallID      string                              // optional; minted if empty
    TurnID      string                              // required
    Name        string                              // required
    Args        json.RawMessage                     // required
    Idempotent  bool                                // false → no retries
    MaxAttempts int                                 // 0 → 1 (no retries)
    Backoff     func(attempt int) time.Duration    // nil → 100ms × 2^n, 25% jitter, cap 10s
}

step.CallTool(ctx, c)             // sequential
step.CallTools(ctx, calls)        // fan-out, semaphore = step.DefaultMaxParallelTools (8)
```

Each retry attempt emits its own `ToolCallScheduled` +
`ToolCallCompleted/Failed` pair under the same `CallID` with
incrementing `Attempt`.

**Built-in tools (`tool/builtin/`):**

```go
import "github.com/jerkeyray/starling/tool/builtin"

builtin.Fetch()                    // HTTP GET; 15s timeout, 1 MiB cap
builtin.ReadFile(baseDir string)   // (tool.Tool, error); rejects "..", absolute, escaping symlinks
```

**MCP tools (`tool/mcp/`):**

```go
import mcptool "github.com/jerkeyray/starling/tool/mcp"

func mcptool.New(ctx, transport, opts...) (*Client, error)
func mcptool.NewCommand(ctx, *exec.Cmd, opts...) (*Client, error)
func mcptool.NewHTTP(ctx, endpoint, *http.Client, opts...) (*Client, error)

// Options:
mcptool.WithClientInfo(name, version)
mcptool.WithToolNamePrefix(prefix)
mcptool.WithIncludeTools(names...)        // allowlist (pre-prefix names)
mcptool.WithExcludeTools(names...)        // denylist (pre-prefix names)
mcptool.WithCallTimeout(d)                // per-call deadline; 0 = ctx-only
mcptool.WithMaxOutputBytes(n)             // ≤0 → 1 MiB default
mcptool.WithTextOnly(true)                // reject non-text content
mcptool.WithTransientErrorClassifier(fn)  // mark transport errors retryable

// Client methods:
client.Tools(ctx) ([]tool.Tool, error)
client.RefreshTools(ctx) ([]tool.Tool, error)
client.Close() error
```

MCP calls go through `step.SideEffect` keyed on `mcp/<remote-name>` so
replay never re-contacts the server.

---

## 4. Providers

```go
type Provider interface {
    Info() Info
    Stream(ctx context.Context, req *Request) (EventStream, error)
}

type Capabler interface {
    Capabilities() Capabilities
}

type Capabilities struct {
    Tools, ToolChoice, Reasoning, StopSequences, CacheControl, RequestID bool
}

type Request struct {
    Model           string
    SystemPrompt    string
    Messages        []Message
    Tools           []ToolDefinition
    ToolChoice      string         // "" | "auto" | "any" | "none" | tool_name
    StopSequences   []string
    TopK            *int
    MaxOutputTokens int
    Params          cborenc.RawMessage  // vendor-specific
}
```

**OpenAI (and OpenAI-compatible: Groq, Together, Ollama, vLLM, Azure):**

```go
import "github.com/jerkeyray/starling/provider/openai"
openai.New(
    openai.WithAPIKey(key),
    openai.WithBaseURL("https://api.groq.com/openai/v1"), // optional
    openai.WithOrganization(org),                          // optional
    openai.WithAPIVersion("v1"),                           // default "v1"
    openai.WithProviderID("groq"),                         // optional
    openai.WithHTTPClient(client),                         // optional
)
```

**Anthropic:**

```go
import "github.com/jerkeyray/starling/provider/anthropic"
anthropic.New(
    anthropic.WithAPIKey(key),
    anthropic.WithBaseURL(url),
    anthropic.WithAPIVersion("2023-06-01"),  // default
    anthropic.WithProviderID(id),
    anthropic.WithHTTPClient(client),
)
```

**Gemini:**

```go
import "github.com/jerkeyray/starling/provider/gemini"
gemini.New(
    gemini.WithAPIKey(key),
    gemini.WithBaseURL(url),
    gemini.WithAPIVersion("v1beta"),         // default
    gemini.WithProviderID(id),
    gemini.WithHTTPClient(client),
)
```

**OpenRouter:**

```go
import "github.com/jerkeyray/starling/provider/openrouter"
openrouter.New(
    openrouter.WithAPIKey(key),
    openrouter.WithHTTPReferer("https://your-app.com"),  // attribution
    openrouter.WithXTitle("Your App"),                   // attribution
    openrouter.WithBaseURL(url),
    openrouter.WithProviderID(id),
    openrouter.WithHTTPClient(client),
)
```

All `New` return `(provider.Provider, error)` (the interface, not a
struct pointer). Always check the error.

**Stream chunk kinds** (validated by `step.LLMCall`):

```
ChunkText | ChunkReasoning | ChunkRedactedThinking
ChunkToolUseStart | ChunkToolUseDelta | ChunkToolUseEnd
ChunkUsage | ChunkEnd
```

Violations surface as `step.ErrInvalidStream`.

Changing provider, model, or `Params` invalidates existing replay
fixtures (RunStarted carries ProviderID, ModelID, ParamsHash).

---

## 5. Event log

```go
type EventLog interface {
    Append(ctx, runID, event.Event) error
    Read(ctx, runID) ([]event.Event, error)
    Stream(ctx, runID) (<-chan event.Event, error)
    Close() error
}

type RunLister interface {
    ListRuns(ctx) ([]RunSummary, error)
}

type RunSummary struct {
    RunID        string
    StartedAt    time.Time
    LastSeq      uint64
    TerminalKind event.Kind
}
```

**Constructors:**

```go
import "github.com/jerkeyray/starling/eventlog"

// Tests, demos. No persistence.
log := eventlog.NewInMemory()

// Single-host. WAL + per-run _txlock=immediate. Auto-migrates on open.
log, err := eventlog.NewSQLite("starling.db",
    eventlog.WithReadOnly(),  // optional, inspector mode
)

// Multi-host. Postgres ≥ 11.
log, err := eventlog.NewPostgres(db,
    eventlog.WithAutoMigratePG(),  // run InstallSchema on connect
    eventlog.WithReadOnlyPG(),     // optional, inspector mode
)
```

**Migrations:**

```go
v, err := eventlog.SchemaVersion(ctx, log)
report, err := eventlog.Migrate(ctx, log)                       // forward-only
report, err := eventlog.Migrate(ctx, log, eventlog.WithDryRun()) // CI safety
```

CLI: `starling migrate <db>`, `starling schema-version <db>`,
`starling migrate -dry-run <db>`.

**Preflight:** Run and Resume call `eventlog.Preflight(ctx, log)` on
startup. Returns `ErrSchemaOutdated` (run migrate) or `ErrSchemaTooNew`
(deploy newer binary).

**Validation:** `eventlog.Validate(events)` checks seq monotonicity,
hash chain, terminal placement, Merkle root, turn pairing, tool-call
pairing, schema version.

**Sentinels:** `ErrLogClosed`, `ErrLogCorrupt`, `ErrInvalidAppend`,
`ErrReadOnly`, `ErrSchemaOutdated`, `ErrSchemaTooNew`.

`Stream` delivers historical events first, then live; closes on ctx
cancel, log close, or buffer overflow (256 events).

---

## 6. Budgets

```go
type Budget struct {
    MaxInputTokens  int64           // pre-call
    MaxOutputTokens int64           // mid-stream on ChunkUsage
    MaxUSD          float64         // mid-stream, per-model price table
    MaxWallClock    time.Duration   // ctx.WithDeadline
}
```

Zero on any field disables that axis. nil Budget disables all four.

**BudgetExceeded payload:**

```go
type BudgetExceeded struct {
    Limit         string  // "input_tokens" | "output_tokens" | "usd" | "wall_clock"
    Cap           float64
    Actual        float64
    Where         string  // "pre_call" | "mid_stream"
    TurnID        string
    CallID        string
    PartialText   string  // mid-stream only
    PartialTokens int64   // mid-stream only
}
```

A trip emits `BudgetExceeded` (which carries the `Limit`/`Cap`/`Actual`
context) then unwinds with `RunFailed{ErrorType:"budget"}`. `RunFailed`
itself has fields `Error`, `ErrorType`, `MerkleRoot`, `DurationMs` —
look at the preceding `BudgetExceeded` for the axis-specific detail.

**USD pricing** is package-private in `budget/prices.go`. Models not
in the table skip USD enforcement. Always set token caps as
defense-in-depth.

`Config.MaxTurns` is separate: caps the ReAct loop and unwinds with
`RunFailed{ErrorType:"max_turns"}`. Use both in production.

---

## 7. Replay

```go
// Root package wrapper, takes *Agent.
func starling.Replay(ctx, log, runID, *Agent) error

// Subpackage, takes Agent interface (good for stubs in tests).
func replay.Verify(ctx, log, runID, replay.Agent) error
func replay.Stream(ctx, factory replay.Factory, log, runID) (<-chan ReplayStep, error)

type Divergence struct {
    RunID        string
    Seq          uint64        // 1-based event index
    Kind         event.Kind    // emitted kind
    ExpectedKind event.Kind    // recorded kind
    Class        step.MismatchClass
    Reason       string
}
// Class is one of: "exhausted" | "kind" | "payload" | "turn_id".

var replay.ErrNonDeterminism = errors.New("replay: non-determinism detected")
// Re-exported as starling.ErrNonDeterminism.

type Factory func(ctx context.Context) (replay.Agent, error)

type ReplayStep struct {
    Index            uint64        // 0-based
    Recorded         event.Event   // always set
    Produced         event.Event   // zero on divergence
    Diverged         bool          // true only on the final step on mismatch
    DivergenceReason string
    Divergence       *Divergence   // nil unless Diverged && step.MismatchError
}
```

**Mechanics:**

- `starling.Replay` shallow-clones the Agent and overrides `Provider`
  with a synthetic replay provider that yields chunks from the
  recording. Your real provider is never contacted.
- Original `Agent.Provider` MUST be non-nil (validate runs before the
  swap). Any stub satisfies this.
- Tools **re-execute live**. Their output bytes are compared against
  the recorded `ToolCallCompleted` payload. Wrap non-determinism in
  `step.*` or replay diverges.
- `step.Now`, `step.Random`, `step.SideEffect` return recorded values
  in both the loop and tool bodies.
- TurnIDs and CallIDs are reused from the recording so byte
  comparison is exact.

**Canonical replay test:**

```go
const goldenRunID = "REPLACE_ME"

func TestAgentMatchesRecording(t *testing.T) {
    log, err := eventlog.NewSQLite("testdata/golden.db", eventlog.WithReadOnly())
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = log.Close() })

    a := newAgent(stubProvider(), log) // any non-nil provider; Replay swaps it
    err = starling.Replay(context.Background(), log, goldenRunID, a)
    if err == nil { return }

    var d *replay.Divergence
    if errors.As(err, &d) {
        t.Fatalf("diverged at seq=%d class=%s kind=%s expected=%s reason=%s",
            d.Seq, d.Class, d.Kind, d.ExpectedKind, d.Reason)
    }
    t.Fatal(err)
}
```

---

## 8. Inspector

```go
import "github.com/jerkeyray/starling/inspect"

srv, err := inspect.New(log,
    inspect.WithAuth(inspect.BearerAuth(os.Getenv("STARLING_INSPECT_TOKEN"))),
    inspect.WithReplayer(factory),  // optional, enables replay re-execution
)
http.ListenAndServe(":8080", srv)
```

Read-only by construction. CSRF on the two replay POST endpoints.
Front with TLS for non-loopback access.

Standalone binary: `cmd/starling-inspect <db>` opens any SQLite log
read-only with no replay capability.

---

## 9. Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

reg := prometheus.NewRegistry()
metrics := starling.NewMetrics(reg)
http.Handle("/metrics", starling.MetricsHandler(prometheus.DefaultGatherer))
// MetricsHandler takes a prometheus.Gatherer; pass DefaultGatherer
// (or your own *prometheus.Registry, which satisfies Gatherer).

a := &starling.Agent{ /* ... */ Metrics: metrics }
```

Labels (verified):

- `starling_runs_started_total`                — no labels
- `starling_runs_in_flight`                    — no labels
- `starling_run_duration_seconds`              — `status`
- `starling_run_terminal_total`                — `status`, `error_type`
- `starling_provider_calls_total`              — `model`, `status`
- `starling_provider_call_duration_seconds`    — `model`
- `starling_provider_tokens_total`             — `model`, `type`
- `starling_tool_calls_total`                  — `tool`, `status`, `error_type`
- `starling_tool_call_duration_seconds`        — `tool`
- `starling_eventlog_appends_total`            — `kind`, `status`
- `starling_eventlog_append_duration_seconds`  — `kind`
- `starling_budget_exceeded_total`             — `axis`

OTel spans: `agent.run` → `agent.turn × N` → `provider.stream` +
`step.tool × M`.

---

## 10. CLI (`cmd/starling`)

```bash
starling validate <db> [<runID>]   # hash chain + Merkle check; full log if no runID
starling export   <db> <runID>     # NDJSON event dump to stdout
starling inspect  [flags] <db>     # local web inspector (read-only)
starling replay   <db> <runID>     # headless replay (dual-mode binary only)
starling migrate  [-dry-run] <db>  # apply pending schema migrations (single dash; -dry-run)
starling schema-version <db>       # print the current schema version
```

Stock binary is SQLite-only. Build a dual-mode binary (link your
agent factory) to enable `starling replay` and `starling inspect`
with replay re-execution.

---

## 11. Anti-patterns (don't write these)

```go
// 1. Reading wall-clock or RNG inside a tool without step.*
fn := func(ctx context.Context, in In) (Out, error) {
    return Out{When: time.Now()}, nil  // BAD — replay diverges
}

// Fix:
fn := func(ctx context.Context, in In) (Out, error) {
    return Out{When: step.Now(ctx)}, nil
}

// 2. Forking goroutines without ctx propagation
go someThing()                       // BAD — step.* will panic if it touches step
g, gctx := errgroup.WithContext(ctx)
g.Go(func() error { /* uses gctx */ return nil })  // OK

// 3. Wrapping non-transient errors with tool.ErrTransient
if status == 401 {
    return out, fmt.Errorf("auth: %w", tool.ErrTransient)  // BAD — infinite retries
}
return out, fmt.Errorf("auth failed: %d", status)         // OK

// 4. step.SideEffect name that varies per run
key := fmt.Sprintf("fetch-%d", time.Now().Unix())  // BAD — name not stable
val, _ := step.SideEffect(ctx, key, fn)
// Fix:
val, _ := step.SideEffect(ctx, "fetch/"+stableID, fn)

// 5. MaxTurns: 0 in production
Config{MaxTurns: 0}     // BAD — runaway loops
Config{MaxTurns: 12}    // OK

// 6. Discarding constructor errors
prov, _ := openai.New(openai.WithAPIKey(""))  // BAD — silent misconfig
prov, err := openai.New(openai.WithAPIKey(key))
if err != nil { return err }                  // OK

// 7. Mocking the provider in replay tests
// BAD — replay swaps Provider with a synthetic one already.
// You only need a NON-NIL provider stub to pass validate().

// 8. Multiple processes writing the same SQLite file
// BAD — SQLite has no cross-host locking.
// Use Postgres for multi-writer.

// 9. SkipSchemaCheck: true in production
Config{SkipSchemaCheck: true}  // BAD — hides forgotten migrations

// 10. Reusing a runID across runs
a.Run(ctx, "...")        // mints fresh ULID
// Once a run terminates, its ID is retired. Start fresh for the next run.
```

---

## 12. Quick reference: copy-pasteable agent skeleton

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "time"

    starling "github.com/jerkeyray/starling"
    "github.com/jerkeyray/starling/eventlog"
    "github.com/jerkeyray/starling/provider/openai"
    "github.com/jerkeyray/starling/step"
    "github.com/jerkeyray/starling/tool"
)

type customerIn struct {
    ID string `json:"id" jsonschema:"description=Customer id"`
}
type customerOut struct {
    Name string `json:"name"`
    Plan string `json:"plan"`
    At   string `json:"at"`
}

func newCustomerLookup() tool.Tool {
    return tool.Typed(
        "customer_lookup",
        "Fetch a customer record by id.",
        func(ctx context.Context, in customerIn) (customerOut, error) {
            out, err := step.SideEffect(ctx, "customer/"+in.ID, func() (customerOut, error) {
                return fetchCustomer(in.ID) // your real call
            })
            if err != nil { return customerOut{}, err }
            out.At = step.Now(ctx).UTC().Format(time.RFC3339)
            return out, nil
        },
    )
}

func main() {
    prov, err := openai.New(openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")))
    if err != nil { panic(err) }

    log, err := eventlog.NewSQLite("starling.db")
    if err != nil { panic(err) }
    defer log.Close()

    a := &starling.Agent{
        Provider: prov,
        Tools:    []tool.Tool{newCustomerLookup()},
        Log:      log,
        Config: starling.Config{
            Model:    "gpt-4o-mini",
            MaxTurns: 12,
            Logger:   slog.Default(),
        },
        Budget: &starling.Budget{
            MaxInputTokens:  100_000,
            MaxOutputTokens: 8_000,
            MaxUSD:          1.50,
            MaxWallClock:    2 * time.Minute,
        },
    }

    res, err := a.Run(context.Background(), "Look up customer 42 and summarize.")
    if err != nil { panic(err) }
    println(res.RunID, "→", res.FinalText)
}

func fetchCustomer(id string) (customerOut, error) {
    // your real HTTP call
    return customerOut{Name: "Ada", Plan: "pro"}, nil
}
```