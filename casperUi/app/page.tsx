import Link from "next/link";
import {
  ArrowRight,
  ArrowUpRight,
  Check,
  ChevronRight,
  Database,
  FileJson,
  Fingerprint,
  GitBranch,
  LayoutDashboard,
  Lock,
  RotateCcw,
  ShieldCheck,
  Sparkles,
} from "lucide-react";

export default function Home() {
  return (
    <div className="relative min-h-screen">
      <Header />

      <main className="relative">
        <Hero />
        <Problem />
        <HowItWorks />
        <Invariants />
        <ProductPreview />
        <Architecture />
        <CTA />
      </main>

      <Footer />
    </div>
  );
}

/* ---------- Header ---------- */

function Header() {
  return (
    <header className="sticky top-0 z-50 border-b border-border/60 bg-background/80 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
        <div className="flex items-center gap-2">
          <Logo />
          <span className="text-sm font-semibold tracking-tight">Casper</span>
          <span className="hidden rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[10px] font-medium tracking-wide text-muted-foreground sm:inline">
            v0.1 · alpha
          </span>
        </div>

        <nav className="hidden items-center gap-7 text-sm text-muted-foreground md:flex">
          <a href="#how" className="hover:text-foreground">How it works</a>
          <a href="#invariants" className="hover:text-foreground">Invariants</a>
          <a href="#product" className="hover:text-foreground">Product</a>
          <a href="#architecture" className="hover:text-foreground">Architecture</a>
        </nav>

        <div className="flex items-center gap-2">
          <a
            href="https://github.com/ASHUTOSH-SWAIN-GIT/casper"
            className="hidden items-center gap-1.5 rounded-md border border-border bg-muted/30 px-3 py-1.5 text-xs font-medium text-foreground/90 transition hover:bg-muted sm:inline-flex"
          >
            <GitBranch className="h-3.5 w-3.5" />
            GitHub
          </a>
          <Link
            href="/app"
            className="inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-xs font-semibold text-accent-foreground transition hover:opacity-90"
          >
            Open dashboard
            <ArrowRight className="h-3.5 w-3.5" />
          </Link>
        </div>
      </div>
    </header>
  );
}

function Logo() {
  return (
    <div className="flex h-6 w-6 items-center justify-center rounded-md border border-border bg-gradient-to-br from-muted to-background">
      <ShieldCheck className="h-3.5 w-3.5 text-accent" />
    </div>
  );
}

/* ---------- Hero ---------- */

function Hero() {
  return (
    <section className="relative pt-24 pb-20 md:pt-32 md:pb-28">
      <div className="mx-auto max-w-6xl px-6">
        <div className="mx-auto max-w-3xl text-center">
          <Link
            href="/app"
            className="group mx-auto mb-6 inline-flex items-center gap-2 rounded-full border border-border bg-muted/40 px-3 py-1 text-xs text-muted-foreground transition hover:bg-muted/60"
          >
            <span className="flex h-1.5 w-1.5 rounded-full bg-accent pulse-dot" />
            Now executing real RDS actions, end-to-end
            <ChevronRight className="h-3 w-3 transition group-hover:translate-x-0.5" />
          </Link>

          <h1 className="text-balance bg-gradient-to-b from-foreground to-foreground/70 bg-clip-text text-5xl font-semibold tracking-tight text-transparent md:text-6xl">
            Let LLMs propose.<br />
            <span className="text-accent">Verify, gate, and audit</span> before AWS sees a thing.
          </h1>

          <p className="mx-auto mt-6 max-w-2xl text-pretty text-base leading-relaxed text-muted-foreground md:text-lg">
            Casper is a trust layer that wraps a swappable LLM proposer with a deterministic Go core.
            Every action is schema-checked, policy-gated, executed against scoped credentials, and
            recorded in a hash-chained audit log — so the model can be wrong without being dangerous.
          </p>

          <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
            <Link
              href="/app"
              className="inline-flex items-center gap-2 rounded-md bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90"
            >
              <LayoutDashboard className="h-4 w-4" />
              Open the dashboard
            </Link>
            <a
              href="#how"
              className="inline-flex items-center gap-2 rounded-md border border-border bg-muted/30 px-4 py-2.5 text-sm font-medium text-foreground/90 transition hover:bg-muted/60"
            >
              How it works
              <ArrowRight className="h-3.5 w-3.5" />
            </a>
          </div>

          <div className="mt-10 flex flex-wrap items-center justify-center gap-x-6 gap-y-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1.5">
              <span className="h-1 w-1 rounded-full bg-accent" />
              Backends: Anthropic API · AWS Bedrock
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="h-1 w-1 rounded-full bg-accent" />
              10 typed RDS actions
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="h-1 w-1 rounded-full bg-accent" />
              Byte-for-byte replay
            </span>
          </div>
        </div>

        <HeroDiagram />
      </div>
    </section>
  );
}

function HeroDiagram() {
  return (
    <div className="relative mx-auto mt-16 max-w-4xl">
      <div className="relative overflow-hidden rounded-2xl border border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border bg-muted/30 px-4 py-2.5">
          <span className="h-2.5 w-2.5 rounded-full bg-red-400/40" />
          <span className="h-2.5 w-2.5 rounded-full bg-yellow-400/40" />
          <span className="h-2.5 w-2.5 rounded-full bg-green-400/40" />
          <span className="ml-2 font-mono text-[11px] text-muted-foreground">
            casper.app/run · scale up casper-testt, CPU is at 90%
          </span>
        </div>
        <div className="grid grid-cols-1 gap-px bg-border md:grid-cols-5">
          <Stage
            step="1"
            title="Intent"
            sub="Free-form English"
            tone="neutral"
            icon={<Sparkles className="h-4 w-4" />}
            line="scale up casper-testt"
          />
          <Stage
            step="2"
            title="Router"
            sub="Haiku · classifier"
            tone="neutral"
            icon={<GitBranch className="h-4 w-4" />}
            line="rds_resize · high"
          />
          <Stage
            step="3"
            title="Fetch + Propose"
            sub="Sonnet · typed JSON"
            tone="neutral"
            icon={<FileJson className="h-4 w-4" />}
            line="t4g.small → t4g.medium"
          />
          <Stage
            step="4"
            title="Policy gate"
            sub="OPA · Rego rules"
            tone="accent"
            icon={<ShieldCheck className="h-4 w-4" />}
            line="allow · in-family upsize"
          />
          <Stage
            step="5"
            title="Execute"
            sub="Deterministic Go"
            tone="accent"
            icon={<Check className="h-4 w-4" />}
            line="forward · 8 steps · verified"
          />
        </div>
        <div className="border-t border-border bg-muted/20 px-4 py-3 font-mono text-[11px] text-muted-foreground">
          <span className="text-accent">●</span> 20 events · chain verified · proposal hash{" "}
          <span className="text-foreground/80">f94446ab…</span>
        </div>
      </div>
    </div>
  );
}

function Stage({
  step,
  title,
  sub,
  line,
  icon,
  tone,
}: {
  step: string;
  title: string;
  sub: string;
  line: string;
  icon: React.ReactNode;
  tone: "neutral" | "accent";
}) {
  return (
    <div className="flex flex-col gap-2 bg-card px-4 py-4">
      <div className="flex items-center justify-between">
        <span
          className={
            tone === "accent"
              ? "text-accent"
              : "text-muted-foreground"
          }
        >
          {icon}
        </span>
        <span className="font-mono text-[10px] text-muted-foreground">{step}</span>
      </div>
      <div>
        <div className="text-sm font-semibold">{title}</div>
        <div className="text-[11px] text-muted-foreground">{sub}</div>
      </div>
      <div className="mt-1 truncate font-mono text-[11px] text-foreground/70">
        {line}
      </div>
    </div>
  );
}

/* ---------- Problem ---------- */

function Problem() {
  return (
    <section className="border-t border-border/60 bg-muted/10 py-20">
      <div className="mx-auto max-w-6xl px-6">
        <div className="grid gap-12 md:grid-cols-2">
          <div>
            <p className="font-mono text-xs uppercase tracking-widest text-accent">
              The problem
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
              An LLM with shell access is a liability — not a teammate.
            </h2>
            <p className="mt-5 text-pretty text-muted-foreground">
              Today&apos;s &quot;AI for ops&quot; products hand the model a credential and hope.
              One bad token, one drifted prompt, one prompt-injection in a log line, and it&apos;s your
              database that gets dropped. There&apos;s no schema for what the model is allowed to do,
              no policy gate before it acts, no audit trail you&apos;d trust in a postmortem.
            </p>
          </div>

          <div className="relative rounded-xl border border-border bg-card/60 p-6">
            <div className="mb-4 flex items-center gap-2 text-xs text-muted-foreground">
              <span className="h-1.5 w-1.5 rounded-full bg-red-400" />
              The shape of the bug
            </div>
            <ol className="space-y-3 text-sm">
              {[
                "Operator types: \"clean up old test instances\"",
                "Agent calls DescribeDBInstances, decides what \"old\" means",
                "Agent calls DeleteDBInstance with no policy in front",
                "Production DB matched the regex",
                "No predictable proposal, no gate, no rollback, no audit",
              ].map((line, i) => (
                <li key={i} className="flex gap-3">
                  <span className="mt-1 inline-block h-1 w-1 shrink-0 rounded-full bg-muted-foreground" />
                  <span className="text-foreground/85">{line}</span>
                </li>
              ))}
            </ol>
          </div>
        </div>
      </div>
    </section>
  );
}

/* ---------- How it works ---------- */

function HowItWorks() {
  const steps: {
    n: string;
    title: string;
    body: string;
    icon: React.ReactNode;
  }[] = [
    {
      n: "01",
      title: "Free-form intent",
      body: "Operator describes the change in English. Casper never lets the model touch AWS — it touches the proposer.",
      icon: <Sparkles className="h-4 w-4" />,
    },
    {
      n: "02",
      title: "Router classifies",
      body: "A cheap Haiku-tier model picks one of the registered action types and extracts the named resource. Confidence and reasoning are recorded.",
      icon: <GitBranch className="h-4 w-4" />,
    },
    {
      n: "03",
      title: "Live state, not flag fictions",
      body: "Casper deterministically pulls the current resource state from AWS — so the proposer reasons against reality, not whatever the operator typed.",
      icon: <Database className="h-4 w-4" />,
    },
    {
      n: "04",
      title: "Proposer emits typed JSON",
      body: "A single-tool agent produces a strict-schema proposal. No prose, no shell commands — just current → target values plus reasoning.",
      icon: <FileJson className="h-4 w-4" />,
    },
    {
      n: "05",
      title: "Policy gate",
      body: "Rego rules per action decide allow / needs_approval / deny. Irreversible actions default to deny. The verdict is part of the audit chain.",
      icon: <ShieldCheck className="h-4 w-4" />,
    },
    {
      n: "06",
      title: "Bounded execution",
      body: "Forward + rollback plans compile from the proposal. STS mints 15-minute credentials scoped to the resource ARN. The interpreter walks the plan.",
      icon: <Lock className="h-4 w-4" />,
    },
    {
      n: "07",
      title: "Hash-chained audit",
      body: "Every event — proposed, evaluated, compiled, executed, verified — is sha256-linked to the previous one. Tampering shows up immediately.",
      icon: <Fingerprint className="h-4 w-4" />,
    },
    {
      n: "08",
      title: "Reversible by default",
      body: "If a forward step fails after a mutating call, the rollback plan runs automatically. Both halves were planned before AWS saw anything.",
      icon: <RotateCcw className="h-4 w-4" />,
    },
  ];

  return (
    <section id="how" className="py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="max-w-2xl">
          <p className="font-mono text-xs uppercase tracking-widest text-accent">
            How it works
          </p>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
            One command. Eight invariants enforced by code, not by hope.
          </h2>
          <p className="mt-5 text-muted-foreground">
            The LLM is the swappable 20%. The 80% that decides what runs against your cloud is
            deterministic Go — predictable, replayable, and reviewable.
          </p>
        </div>

        <div className="mt-12 grid gap-px overflow-hidden rounded-xl border border-border bg-border md:grid-cols-2 lg:grid-cols-4">
          {steps.map((s) => (
            <div key={s.n} className="bg-card p-6">
              <div className="flex items-center justify-between">
                <span className="text-accent">{s.icon}</span>
                <span className="font-mono text-[10px] text-muted-foreground">{s.n}</span>
              </div>
              <h3 className="mt-4 text-base font-semibold">{s.title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

/* ---------- Invariants ---------- */

function Invariants() {
  const items = [
    {
      title: "Predictability",
      body: "Same input, same proposal, same plan. The proposer is single-tool, single-turn — replayable byte-for-byte.",
    },
    {
      title: "Reversibility",
      body: "Forward and rollback plans are compiled together. If a mutating step fails, the undo runs without asking.",
    },
    {
      title: "Bounded authority",
      body: "STS mints 15-minute credentials scoped to the proposal's exact resource ARN. The model never sees them.",
    },
    {
      title: "Auditability",
      body: "Every event is hash-chained. Verifying the chain is a single function call; tampering is detectable.",
    },
    {
      title: "Accountability",
      body: "Every action is bound to a model run, a policy verdict, and a credential session. Postmortems write themselves.",
    },
  ];

  return (
    <section id="invariants" className="border-t border-border/60 bg-muted/10 py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="flex flex-col items-start justify-between gap-6 md:flex-row md:items-end">
          <div className="max-w-xl">
            <p className="font-mono text-xs uppercase tracking-widest text-accent">
              Trust-layer invariants
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
              Five properties Casper guarantees on every run.
            </h2>
          </div>
          <p className="max-w-md text-sm text-muted-foreground">
            Take any one of these away and you don&apos;t have a trust layer — you have a
            wrapper around an LLM with a shell.
          </p>
        </div>

        <div className="mt-12 grid gap-4 md:grid-cols-2 lg:grid-cols-5">
          {items.map((it, i) => (
            <div
              key={it.title}
              className="rounded-xl border border-border bg-card p-5 transition hover:border-accent/50"
            >
              <div className="flex items-center gap-2">
                <span className="font-mono text-[10px] text-muted-foreground">
                  {String(i + 1).padStart(2, "0")}
                </span>
                <span className="h-px flex-1 bg-border" />
              </div>
              <h3 className="mt-3 text-sm font-semibold">{it.title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{it.body}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

/* ---------- Product preview ---------- */

function ProductPreview() {
  return (
    <section id="product" className="py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="max-w-2xl">
          <p className="font-mono text-xs uppercase tracking-widest text-accent">
            The product
          </p>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
            A dashboard built around the proposal, not the prompt.
          </h2>
          <p className="mt-5 text-muted-foreground">
            Operators submit an intent. The trust layer turns it into a typed proposal you can read,
            diff, gate, and approve — without ever touching a terminal. Every run renders the same
            three artifacts: the proposal, the live execution, and the chained audit.
          </p>
        </div>

        <div className="mt-12 grid gap-6 lg:grid-cols-3">
          <PreviewCard
            label="Step 1"
            title="Submit an intent"
            body="A textarea, a region, a submit button. The router classifies and the fetcher pulls live state before the proposer runs."
            preview={<IntentPreview />}
          />
          <PreviewCard
            label="Step 2"
            title="Review the proposal"
            body="Current → target diff card with the model's reasoning. Approve or reject. Irreversible actions surface a second confirmation."
            preview={<ProposalPreview />}
            highlight
          />
          <PreviewCard
            label="Step 3"
            title="Watch it run"
            body="Forward plan steps stream in live with green/red pills. The hash-chained audit log is one click away — copyable, verifiable."
            preview={<RunPreview />}
          />
        </div>

        <div className="mt-10 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <Link
            href="/app"
            className="inline-flex items-center gap-2 rounded-md bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90"
          >
            <LayoutDashboard className="h-4 w-4" />
            Open the dashboard
          </Link>
          <span className="text-xs text-muted-foreground">
            Live mock — no AWS calls until you connect a workspace.
          </span>
        </div>
      </div>
    </section>
  );
}

function PreviewCard({
  label,
  title,
  body,
  preview,
  highlight,
}: {
  label: string;
  title: string;
  body: string;
  preview: React.ReactNode;
  highlight?: boolean;
}) {
  return (
    <div
      className={`relative overflow-hidden rounded-xl border bg-card/80 ${
        highlight ? "border-accent/50" : "border-border"
      }`}
    >
      {highlight && (
        <div className="pointer-events-none absolute inset-0 bg-gradient-to-b from-accent/8 to-transparent" />
      )}
      <div className="relative">
        <div className="border-b border-border bg-muted/30 px-5 py-4">
          <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
            {label}
          </p>
          <h3 className="mt-1 text-base font-semibold">{title}</h3>
          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{body}</p>
        </div>
        <div className="p-4">{preview}</div>
      </div>
    </div>
  );
}

function IntentPreview() {
  return (
    <div className="space-y-3">
      <div className="rounded-md border border-border bg-background p-3">
        <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
          intent
        </p>
        <p className="mt-2 text-sm text-foreground/85">
          casper-testt is slow under load
        </p>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div className="rounded-md border border-border bg-background px-3 py-2">
          <p className="font-mono text-[10px] uppercase text-muted-foreground">region</p>
          <p className="text-xs text-foreground/85">ap-south-1</p>
        </div>
        <div className="rounded-md border border-border bg-background px-3 py-2">
          <p className="font-mono text-[10px] uppercase text-muted-foreground">backend</p>
          <p className="text-xs text-foreground/85">Anthropic</p>
        </div>
      </div>
      <button className="w-full rounded-md bg-accent py-2 text-xs font-semibold text-accent-foreground">
        Generate proposal
      </button>
    </div>
  );
}

function ProposalPreview() {
  return (
    <div className="space-y-3 font-mono text-[11px]">
      <div className="flex items-center justify-between rounded-md border border-border bg-background px-3 py-2">
        <span className="text-muted-foreground">action</span>
        <span className="text-accent">rds_resize</span>
      </div>
      <div className="rounded-md border border-border bg-background p-3">
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground">instance class</span>
        </div>
        <div className="mt-2 flex items-center gap-2">
          <span className="rounded bg-muted/50 px-1.5 py-0.5 text-foreground/70">db.t4g.small</span>
          <ArrowRight className="h-3 w-3 text-muted-foreground" />
          <span className="rounded bg-accent/15 px-1.5 py-0.5 text-accent">db.t4g.medium</span>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <button className="rounded-md border border-border bg-background py-2 text-[11px] font-sans font-medium text-foreground/80 hover:border-foreground/50">
          Reject
        </button>
        <button className="rounded-md bg-accent py-2 text-[11px] font-sans font-semibold text-accent-foreground">
          Approve & run
        </button>
      </div>
    </div>
  );
}

function RunPreview() {
  const steps = [
    { id: "describe-pre", t: "381ms", ok: true },
    { id: "modify", t: "758ms", ok: true },
    { id: "poll-available", t: "289s", ok: true },
    { id: "verify-metric", t: "CPU 5.28%", ok: true },
  ];
  return (
    <div className="space-y-2 font-mono text-[11px]">
      {steps.map((s) => (
        <div
          key={s.id}
          className="flex items-center justify-between rounded-md border border-border bg-background px-3 py-2"
        >
          <div className="flex items-center gap-2">
            <Check className="h-3 w-3 text-accent" />
            <span className="text-foreground/85">{s.id}</span>
          </div>
          <span className="text-muted-foreground">{s.t}</span>
        </div>
      ))}
      <div className="flex items-center justify-between rounded-md border border-accent/40 bg-accent/5 px-3 py-2">
        <span className="text-accent">chain verified</span>
        <span className="text-muted-foreground">20 events</span>
      </div>
    </div>
  );
}

/* ---------- Architecture ---------- */

function Architecture() {
  return (
    <section id="architecture" className="border-t border-border/60 bg-muted/10 py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="max-w-2xl">
          <p className="font-mono text-xs uppercase tracking-widest text-accent">
            Architecture
          </p>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
            Deterministic Go core. Swappable LLM upstream.
          </h2>
          <p className="mt-5 text-muted-foreground">
            The boundary is sharp on purpose. Anything that can hallucinate is in the proposer.
            Anything that touches AWS is in the interpreter. The two never cross.
          </p>
        </div>

        <div className="mt-12 grid gap-6 lg:grid-cols-3">
          <Pillar
            label="Upstream · LLM"
            title="Proposer"
            body="Single-tool, single-turn agents. One per action type. Anthropic API or AWS Bedrock — the proposer doesn't care."
            tags={["Anthropic", "Bedrock", "Starling"]}
          />
          <Pillar
            label="Boundary · typed JSON"
            title="Action registry"
            body="10 RDS actions today. Each one ships its own schema, plan compiler, Rego rules, and IAM session policy. Adding a service is mechanical."
            tags={["Schemas", "Rego", "Plan IR"]}
            highlight
          />
          <Pillar
            label="Downstream · Go"
            title="Interpreter"
            body="Walks compiled plans against AWS via aws-sdk-go-v2. Forward and rollback share one dispatcher. STS mints scoped credentials per run."
            tags={["aws-sdk-go-v2", "STS", "OPA"]}
          />
        </div>
      </div>
    </section>
  );
}

function Pillar({
  label,
  title,
  body,
  tags,
  highlight,
}: {
  label: string;
  title: string;
  body: string;
  tags: string[];
  highlight?: boolean;
}) {
  return (
    <div
      className={`relative rounded-xl border bg-card p-6 ${
        highlight ? "border-accent/50" : "border-border"
      }`}
    >
      {highlight && (
        <div className="absolute -inset-px rounded-xl bg-gradient-to-b from-accent/20 to-transparent opacity-40 blur-md" />
      )}
      <div className="relative">
        <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
          {label}
        </p>
        <h3 className="mt-2 text-lg font-semibold">{title}</h3>
        <p className="mt-3 text-sm leading-relaxed text-muted-foreground">{body}</p>
        <div className="mt-5 flex flex-wrap gap-1.5">
          {tags.map((t) => (
            <span
              key={t}
              className="rounded-md border border-border bg-muted/30 px-2 py-0.5 font-mono text-[10px] text-foreground/80"
            >
              {t}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

/* ---------- CTA ---------- */

function CTA() {
  return (
    <section className="py-24">
      <div className="mx-auto max-w-4xl px-6">
        <div className="relative overflow-hidden rounded-2xl border border-border bg-card p-10 text-center">
          <div className="pointer-events-none absolute inset-0 bg-gradient-to-b from-accent/10 via-transparent to-transparent" />
          <div className="relative">
            <h2 className="text-3xl font-semibold tracking-tight md:text-4xl">
              Stop trusting the model.<br />
              <span className="text-accent">Start verifying its work.</span>
            </h2>
            <p className="mx-auto mt-4 max-w-xl text-muted-foreground">
              Connect a workspace, point Casper at an AWS account, and let your team submit changes
              through a dashboard that enforces the trust layer on every run.
            </p>
            <div className="mt-7 flex flex-col items-center justify-center gap-3 sm:flex-row">
              <Link
                href="/app"
                className="inline-flex items-center gap-2 rounded-md bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90"
              >
                <LayoutDashboard className="h-4 w-4" />
                Open the dashboard
                <ArrowRight className="h-3.5 w-3.5" />
              </Link>
              <a
                href="https://github.com/ASHUTOSH-SWAIN-GIT/casper"
                className="inline-flex items-center gap-2 rounded-md border border-border bg-muted/30 px-4 py-2.5 text-sm font-medium text-foreground/90 transition hover:bg-muted/60"
              >
                <GitBranch className="h-4 w-4" />
                View on GitHub
                <ArrowUpRight className="h-3.5 w-3.5" />
              </a>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

/* ---------- Footer ---------- */

function Footer() {
  return (
    <footer className="border-t border-border/60 bg-muted/10">
      <div className="mx-auto flex max-w-6xl flex-col items-start justify-between gap-6 px-6 py-10 md:flex-row md:items-center">
        <div className="flex items-center gap-2">
          <Logo />
          <span className="text-sm font-semibold tracking-tight">Casper</span>
          <span className="text-xs text-muted-foreground">— AI Trust Layer for Cloud Infrastructure</span>
        </div>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs text-muted-foreground">
          <a href="#how" className="hover:text-foreground">How it works</a>
          <a href="#invariants" className="hover:text-foreground">Invariants</a>
          <a href="#product" className="hover:text-foreground">Product</a>
          <a href="#architecture" className="hover:text-foreground">Architecture</a>
          <Link href="/app" className="hover:text-foreground">Dashboard</Link>
          <a
            href="https://github.com/ASHUTOSH-SWAIN-GIT/casper"
            className="inline-flex items-center gap-1 hover:text-foreground"
          >
            GitHub <ArrowUpRight className="h-3 w-3" />
          </a>
        </div>
      </div>
    </footer>
  );
}
