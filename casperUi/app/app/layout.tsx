import Link from "next/link";
import {
  Activity,
  ChevronDown,
  Cog,
  FileClock,
  KeyRound,
  LayoutDashboard,
  ShieldCheck,
  Sparkles,
} from "lucide-react";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="grid min-h-screen grid-cols-[16rem_1fr] bg-background">
      <Sidebar />
      <div className="flex min-h-screen flex-col">
        <Topbar />
        <main className="flex-1 overflow-y-auto">{children}</main>
      </div>
    </div>
  );
}

function Sidebar() {
  return (
    <aside className="flex flex-col border-r border-border bg-muted/10">
      <Link
        href="/"
        className="flex h-14 items-center gap-2 border-b border-border px-5 transition hover:bg-muted/30"
      >
        <div className="flex h-6 w-6 items-center justify-center rounded-md border border-border bg-gradient-to-br from-muted to-background">
          <ShieldCheck className="h-3.5 w-3.5 text-accent" />
        </div>
        <span className="text-sm font-semibold tracking-tight">Casper</span>
        <span className="ml-auto rounded-full border border-border bg-muted/40 px-1.5 py-0.5 text-[9px] font-medium tracking-wide text-muted-foreground">
          alpha
        </span>
      </Link>

      <div className="px-3 pt-5">
        <Workspace />
      </div>

      <nav className="mt-6 flex flex-col gap-0.5 px-3 text-sm">
        <SectionLabel>Run</SectionLabel>
        <NavItem href="/app" icon={<Sparkles className="h-4 w-4" />} label="New intent" active />
        <NavItem href="/app/runs" icon={<Activity className="h-4 w-4" />} label="Runs" />
        <NavItem href="/app/audit" icon={<FileClock className="h-4 w-4" />} label="Audit log" />

        <SectionLabel className="mt-5">Workspace</SectionLabel>
        <NavItem href="/app/credentials" icon={<KeyRound className="h-4 w-4" />} label="Credentials" />
        <NavItem href="/app/policies" icon={<ShieldCheck className="h-4 w-4" />} label="Policies" />
        <NavItem href="/app/settings" icon={<Cog className="h-4 w-4" />} label="Settings" />
      </nav>

      <div className="mt-auto px-3 pb-5">
        <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs">
          <div className="flex items-center gap-2">
            <span className="h-1.5 w-1.5 rounded-full bg-accent" />
            <span className="font-medium text-foreground/90">Trust layer active</span>
          </div>
          <p className="mt-1.5 text-muted-foreground">
            10 actions registered · OPA policies loaded · audit chain verified
          </p>
        </div>
      </div>
    </aside>
  );
}

function Workspace() {
  return (
    <button className="flex w-full items-center justify-between rounded-md border border-border bg-card px-3 py-2 text-left transition hover:bg-muted/40">
      <div>
        <p className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
          workspace
        </p>
        <p className="text-sm font-medium text-foreground/90">commenda · prod</p>
      </div>
      <ChevronDown className="h-4 w-4 text-muted-foreground" />
    </button>
  );
}

function SectionLabel({ children, className }: { children: string; className?: string }) {
  return (
    <p
      className={`px-2 pb-1 font-mono text-[10px] uppercase tracking-widest text-muted-foreground ${
        className ?? ""
      }`}
    >
      {children}
    </p>
  );
}

function NavItem({
  href,
  icon,
  label,
  active,
}: {
  href: string;
  icon: React.ReactNode;
  label: string;
  active?: boolean;
}) {
  return (
    <Link
      href={href}
      className={`flex items-center gap-2.5 rounded-md px-2 py-1.5 transition ${
        active
          ? "bg-accent/10 text-foreground"
          : "text-muted-foreground hover:bg-muted/30 hover:text-foreground"
      }`}
    >
      <span className={active ? "text-accent" : ""}>{icon}</span>
      <span>{label}</span>
    </Link>
  );
}

function Topbar() {
  return (
    <header className="flex h-14 items-center justify-between border-b border-border bg-background/80 px-6 backdrop-blur">
      <div className="flex items-center gap-2 text-sm">
        <LayoutDashboard className="h-4 w-4 text-muted-foreground" />
        <span className="text-muted-foreground">Run</span>
        <span className="text-muted-foreground">/</span>
        <span className="font-medium text-foreground">New intent</span>
      </div>
      <div className="flex items-center gap-3">
        <span className="hidden items-center gap-2 rounded-full border border-border bg-muted/30 px-2.5 py-1 text-[11px] text-muted-foreground sm:inline-flex">
          <span className="h-1.5 w-1.5 rounded-full bg-accent pulse-dot" />
          ap-south-1 · 492351590994
        </span>
        <div className="flex h-7 w-7 items-center justify-center rounded-full bg-accent/20 text-[11px] font-semibold text-accent">
          AS
        </div>
      </div>
    </header>
  );
}
