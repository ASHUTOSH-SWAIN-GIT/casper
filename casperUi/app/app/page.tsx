"use client";

import { useState } from "react";
import {
  ArrowRight,
  Check,
  Database,
  FileJson,
  GitBranch,
  Loader2,
  Sparkles,
} from "lucide-react";

const SAMPLE_INTENTS = [
  "casper-testt is sustained at 90% CPU, scale it up",
  "snapshot casper-testt before tonight's deploy",
  "raise backup retention on casper-testt to 14 days",
  "spin up a read replica of casper-testt for analytics",
];

export default function NewIntentPage() {
  const [intent, setIntent] = useState("");
  const [region, setRegion] = useState("ap-south-1");
  const [backend, setBackend] = useState<"anthropic" | "bedrock">("anthropic");
  const [stage, setStage] = useState<"idle" | "routing" | "fetching" | "proposing" | "ready">(
    "idle"
  );

  function generate() {
    if (!intent.trim() || stage !== "idle") return;
    setStage("routing");
    setTimeout(() => setStage("fetching"), 700);
    setTimeout(() => setStage("proposing"), 1500);
    setTimeout(() => setStage("ready"), 3200);
  }

  return (
    <div className="mx-auto max-w-5xl px-8 py-10">
      <div className="mb-8">
        <h1 className="text-2xl font-semibold tracking-tight">New intent</h1>
        <p className="mt-1.5 text-sm text-muted-foreground">
          Describe the change you want. Casper will route it, fetch live state from AWS, and
          produce a typed proposal you can review.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[1.4fr_1fr]">
        <div className="space-y-5">
          <Card label="01" title="Intent">
            <textarea
              value={intent}
              onChange={(e) => setIntent(e.target.value)}
              rows={4}
              placeholder="e.g. casper-testt is slow under load"
              className="w-full resize-none rounded-md border border-border bg-background px-3 py-2.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-accent/60 focus:outline-none"
            />
            <div className="mt-3 flex flex-wrap gap-2">
              {SAMPLE_INTENTS.map((s) => (
                <button
                  key={s}
                  onClick={() => setIntent(s)}
                  className="rounded-full border border-border bg-muted/30 px-3 py-1 text-[11px] text-muted-foreground transition hover:border-foreground/40 hover:text-foreground"
                >
                  {s}
                </button>
              ))}
            </div>
          </Card>

          <Card label="02" title="Context">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Region">
                <select
                  value={region}
                  onChange={(e) => setRegion(e.target.value)}
                  className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm focus:border-accent/60 focus:outline-none"
                >
                  <option value="ap-south-1">ap-south-1 (Mumbai)</option>
                  <option value="us-east-1">us-east-1 (N. Virginia)</option>
                  <option value="us-west-2">us-west-2 (Oregon)</option>
                  <option value="eu-west-1">eu-west-1 (Ireland)</option>
                </select>
              </Field>
              <Field label="LLM backend">
                <div className="flex rounded-md border border-border bg-background p-0.5">
                  <button
                    onClick={() => setBackend("anthropic")}
                    className={`flex-1 rounded-[5px] px-3 py-1.5 text-xs font-medium transition ${
                      backend === "anthropic"
                        ? "bg-accent/15 text-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    Anthropic
                  </button>
                  <button
                    onClick={() => setBackend("bedrock")}
                    className={`flex-1 rounded-[5px] px-3 py-1.5 text-xs font-medium transition ${
                      backend === "bedrock"
                        ? "bg-accent/15 text-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    Bedrock
                  </button>
                </div>
              </Field>
            </div>
          </Card>

          <button
            onClick={generate}
            disabled={!intent.trim() || stage !== "idle"}
            className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {stage === "idle" || stage === "ready" ? (
              <>
                <Sparkles className="h-4 w-4" />
                Generate proposal
              </>
            ) : (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                {stage === "routing" && "Routing intent…"}
                {stage === "fetching" && "Fetching live state…"}
                {stage === "proposing" && "Proposer running…"}
              </>
            )}
          </button>
        </div>

        <PipelineSidebar stage={stage} />
      </div>

      {stage === "ready" && (
        <div className="mt-8">
          <ProposalCard />
        </div>
      )}
    </div>
  );
}

function Card({
  label,
  title,
  children,
}: {
  label: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-5 py-3">
        <h3 className="text-sm font-semibold">{title}</h3>
        <span className="font-mono text-[10px] text-muted-foreground">{label}</span>
      </div>
      <div className="p-5">{children}</div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1.5 block font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
        {label}
      </label>
      {children}
    </div>
  );
}

function PipelineSidebar({ stage }: { stage: string }) {
  const steps = [
    { id: "routing", label: "Router classifies", sub: "Haiku · ~$0.0003", icon: GitBranch },
    { id: "fetching", label: "Fetcher reads AWS", sub: "DescribeDBInstances", icon: Database },
    { id: "proposing", label: "Proposer emits JSON", sub: "Sonnet · typed schema", icon: FileJson },
    { id: "ready", label: "Ready for review", sub: "policy gate next", icon: Check },
  ];
  const order = ["idle", "routing", "fetching", "proposing", "ready"];
  const currentIdx = order.indexOf(stage);

  return (
    <div className="rounded-xl border border-border bg-card/60 p-5">
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Pipeline</h3>
        <span className="font-mono text-[10px] text-muted-foreground">live</span>
      </div>
      <ol className="space-y-3">
        {steps.map((s, i) => {
          const stepIdx = order.indexOf(s.id);
          const isDone = currentIdx > stepIdx;
          const isActive = currentIdx === stepIdx;
          const Icon = s.icon;
          return (
            <li key={s.id} className="flex items-start gap-3">
              <div
                className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border ${
                  isDone
                    ? "border-accent/50 bg-accent/15 text-accent"
                    : isActive
                    ? "border-accent/50 bg-accent/10 text-accent"
                    : "border-border bg-muted/30 text-muted-foreground"
                }`}
              >
                {isActive ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : isDone ? (
                  <Check className="h-3 w-3" />
                ) : (
                  <Icon className="h-3 w-3" />
                )}
              </div>
              <div>
                <p
                  className={`text-sm ${
                    isDone || isActive ? "text-foreground" : "text-muted-foreground"
                  }`}
                >
                  {s.label}
                </p>
                <p className="text-xs text-muted-foreground">{s.sub}</p>
              </div>
            </li>
          );
        })}
      </ol>

      <div className="mt-5 rounded-md border border-border bg-muted/30 p-3 text-xs">
        <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
          trust layer
        </p>
        <p className="mt-1 text-foreground/80">
          Nothing touches AWS until you approve the proposal. Read-only fetcher excluded.
        </p>
      </div>
    </div>
  );
}

function ProposalCard() {
  return (
    <div className="overflow-hidden rounded-xl border border-accent/40 bg-card shadow-2xl shadow-accent/5">
      <div className="flex items-center justify-between border-b border-border bg-muted/20 px-5 py-3">
        <div className="flex items-center gap-2">
          <FileJson className="h-4 w-4 text-accent" />
          <h3 className="text-sm font-semibold">Proposal</h3>
          <span className="rounded-md border border-border bg-muted/30 px-2 py-0.5 font-mono text-[10px] text-muted-foreground">
            rds_resize
          </span>
        </div>
        <span className="font-mono text-[11px] text-muted-foreground">
          hash <span className="text-foreground/80">f94446ab…2418</span>
        </span>
      </div>

      <div className="grid gap-px bg-border md:grid-cols-2">
        <div className="bg-card p-5">
          <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
            change
          </p>
          <div className="mt-2 flex items-center gap-2 font-mono text-sm">
            <span className="rounded bg-muted/50 px-2 py-0.5 text-foreground/80">db.t4g.small</span>
            <ArrowRight className="h-4 w-4 text-muted-foreground" />
            <span className="rounded bg-accent/15 px-2 py-0.5 text-accent">db.t4g.medium</span>
          </div>

          <div className="mt-5 space-y-2 text-sm">
            <Row k="instance" v="casper-testt" />
            <Row k="region" v="ap-south-1" />
            <Row k="apply_immediately" v="true" />
            <Row k="success metric" v="CPUUtilization < 60% over 5m" />
          </div>
        </div>

        <div className="bg-card p-5">
          <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
            reasoning
          </p>
          <p className="mt-2 text-sm leading-relaxed text-foreground/85">
            Burstable t4g.small is exhausting CPU credits under sustained load. Upsizing one step in
            family to t4g.medium doubles vCPUs and memory while keeping the same engine and storage
            characteristics. Reversible by resizing back; rollback plan generated.
          </p>

          <div className="mt-5 grid grid-cols-3 gap-2 text-xs">
            <Stat k="model" v="sonnet-4-6" />
            <Stat k="tokens" v="3.5k / 350" />
            <Stat k="cost" v="$0.0157" />
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between border-t border-border bg-muted/20 px-5 py-3">
        <div className="flex items-center gap-2 text-xs">
          <span className="rounded-md border border-accent/40 bg-accent/10 px-2 py-0.5 font-mono text-[10px] text-accent">
            policy: allow
          </span>
          <span className="text-muted-foreground">safe in-family one-step upsize</span>
        </div>
        <div className="flex items-center gap-2">
          <button className="rounded-md border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground/80 transition hover:border-foreground/40">
            Reject
          </button>
          <button className="inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-xs font-semibold text-accent-foreground transition hover:opacity-90">
            <Check className="h-3.5 w-3.5" />
            Approve & run
          </button>
        </div>
      </div>
    </div>
  );
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <div className="flex items-center justify-between font-mono text-xs">
      <span className="text-muted-foreground">{k}</span>
      <span className="text-foreground/85">{v}</span>
    </div>
  );
}

function Stat({ k, v }: { k: string; v: string }) {
  return (
    <div className="rounded-md border border-border bg-background px-2.5 py-1.5">
      <p className="font-mono text-[9px] uppercase tracking-widest text-muted-foreground">{k}</p>
      <p className="font-mono text-[11px] text-foreground/85">{v}</p>
    </div>
  );
}
