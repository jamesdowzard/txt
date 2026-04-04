import Image from "next/image";

import { CommandBlock } from "../components/command-block";
import { SiteFooter } from "../components/site-footer";
import { SiteHeader } from "../components/site-header";
import {
  aiBlocks,
  claudeMcpCommand,
  downloadUrl,
  productSignals,
  repoUrl,
  setupColumns,
  workflowSteps
} from "../data/site-content";

const heroStats = [
  { label: "Routes live now", value: "SMS, RCS, WhatsApp" },
  { label: "Runtime", value: "Local SQLite + MCP" },
  { label: "Built with", value: "Go, Swift, Web UI" }
];

const primaryActionClassName =
  "inline-flex items-center justify-center rounded-full bg-[var(--accent)] px-6 py-3.5 text-sm font-semibold text-[var(--bg-deep)] transition-transform hover:-translate-y-0.5";
const secondaryActionClassName =
  "inline-flex items-center justify-center rounded-full border border-[var(--border)] bg-[color:rgba(13,23,40,0.82)] px-6 py-3.5 text-sm font-semibold text-[var(--text-primary)] transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--bg-hover)]";
const panelClassName =
  "rounded-[2rem] border border-[var(--border)] bg-[color:rgba(13,23,40,0.74)] p-7 shadow-[0_18px_70px_rgba(4,12,24,0.24)] lg:p-8";
const sectionBodyClassName = "mt-5 max-w-[28rem] text-base leading-7 text-[var(--text-secondary)]";

function ActionLink({
  children,
  href,
  minWidthClassName = "",
  openInNewTab = false,
  variant = "secondary"
}) {
  const className =
    variant === "primary"
      ? `${primaryActionClassName} ${minWidthClassName}`.trim()
      : `${secondaryActionClassName} ${minWidthClassName}`.trim();
  const linkProps = openInNewTab ? { rel: "noreferrer", target: "_blank" } : {};

  return (
    <a href={href} className={className} {...linkProps}>
      {children}
    </a>
  );
}

function SectionIntro({ body, eyebrow, title }) {
  return (
    <div className="max-w-[42rem]">
      <span className="eyebrow">{eyebrow}</span>
      <h2 className="mt-5 text-[clamp(2.2rem,4vw,3.8rem)] font-semibold leading-[0.98] tracking-[-0.06em]">
        {title}
      </h2>
      {body ? <p className={sectionBodyClassName}>{body}</p> : null}
    </div>
  );
}

function HeroStat({ label, value }) {
  return (
    <div>
      <div className="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-[var(--text-muted)]">
        {label}
      </div>
      <div className="mt-2 font-medium text-[var(--text-primary)]">{value}</div>
    </div>
  );
}

function ProductSignalCard({ index, signal, total }) {
  const isLast = index === total - 1;
  const className = isLast
    ? "px-0 py-8 lg:px-8"
    : "border-b border-[var(--border)] px-0 py-8 lg:border-b-0 lg:border-r lg:px-8";

  return (
    <article className={className}>
      <div className="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-[var(--text-muted)]">
        {String(index + 1).padStart(2, "0")}
      </div>
      <h3 className="mt-4 text-2xl font-semibold tracking-[-0.045em] text-[var(--text-primary)]">
        {signal.title}
      </h3>
      <p className="mt-4 max-w-[24rem] text-base leading-7 text-[var(--text-secondary)]">
        {signal.body}
      </p>
    </article>
  );
}

function WorkflowStepCard({ step }) {
  return (
    <div className="grid gap-4 border-b border-[var(--border)] pb-8 last:border-b-0 last:pb-0 md:grid-cols-[96px_minmax(0,1fr)]">
      <div className="text-5xl font-semibold tracking-[-0.08em] text-[var(--accent-strong)]">
        {step.number}
      </div>
      <div>
        <h3 className="text-2xl font-semibold tracking-[-0.045em] text-[var(--text-primary)]">
          {step.title}
        </h3>
        <p className="mt-3 max-w-[42rem] text-base leading-7 text-[var(--text-secondary)]">
          {step.body}
        </p>
      </div>
    </div>
  );
}

function SetupCard({ column }) {
  return (
    <section className={panelClassName}>
      <div className="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-[var(--accent-strong)]">
        {column.eyebrow}
      </div>
      <h3 className="mt-4 text-[2rem] font-semibold tracking-[-0.05em] text-[var(--text-primary)]">
        {column.title}
      </h3>
      <p className="mt-4 max-w-[34rem] text-base leading-7 text-[var(--text-secondary)]">
        {column.body}
      </p>

      <ul className="mt-6 space-y-3 text-sm leading-6 text-[var(--text-secondary)]">
        {column.bullets.map((bullet) => (
          <li key={bullet} className="flex gap-3">
            <span className="mt-2 h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />
            <span>{bullet}</span>
          </li>
        ))}
      </ul>

      <div className="mt-7 grid gap-4">
        {column.commands.map((command) => (
          <CommandBlock key={command.label} label={command.label}>
            {command.code}
          </CommandBlock>
        ))}
      </div>
    </section>
  );
}

function AIBlockCard({ block }) {
  return (
    <article className="rounded-[2rem] border border-[var(--border)] bg-[color:rgba(13,23,40,0.74)] p-7">
      <h3 className="text-[1.45rem] font-semibold tracking-[-0.045em] text-[var(--text-primary)]">
        {block.title}
      </h3>
      <p className="mt-3 text-sm leading-7 text-[var(--text-secondary)]">{block.body}</p>
      <div className="mt-5 rounded-2xl border border-[var(--border)] bg-[var(--bg-deep)] px-4 py-3 font-mono text-sm text-[var(--accent-strong)]">
        {block.command}
      </div>
    </article>
  );
}

export default function HomePage() {
  return (
    <main className="min-h-screen">
      <SiteHeader />

      <section className="relative overflow-hidden border-b border-[var(--border)]">
        <div className="absolute inset-x-0 top-[-18rem] h-[34rem] bg-[radial-gradient(circle_at_top,rgba(79,108,255,0.22),transparent_62%)]" />
        <div className="mx-auto grid max-w-[1360px] gap-16 px-6 pb-20 pt-18 lg:grid-cols-[minmax(0,520px)_minmax(0,1fr)] lg:px-10 lg:pb-28 lg:pt-24">
          <div className="relative z-10 flex max-w-[34rem] flex-col justify-center">
            <div className="animate-fade-up">
              <span className="eyebrow">Local-first messaging workspace</span>
            </div>
            <h1 className="animate-fade-up mt-6 text-[clamp(3.4rem,7vw,6.8rem)] font-semibold leading-[0.94] tracking-[-0.065em] text-[var(--text-primary)] [animation-delay:120ms]">
              One desktop surface
              <span className="block text-[var(--accent-strong)]">for your real messages.</span>
            </h1>
            <p className="animate-fade-up mt-6 max-w-[32rem] text-lg leading-8 text-[var(--text-secondary)] [animation-delay:220ms]">
              OpenMessage brings Google Messages and WhatsApp into one local workspace, then
              exposes the same context to AI assistants through a local MCP endpoint.
            </p>

            <div className="animate-fade-up mt-9 flex flex-col gap-4 sm:flex-row [animation-delay:320ms]">
              <ActionLink href={downloadUrl} minWidthClassName="min-w-[220px]" variant="primary">
                Download for macOS
              </ActionLink>
              <ActionLink
                href={repoUrl}
                minWidthClassName="min-w-[220px]"
                openInNewTab
              >
                View the repo
              </ActionLink>
            </div>

            <div className="animate-fade-up mt-12 grid gap-4 border-y border-[var(--border)] py-5 text-sm text-[var(--text-secondary)] sm:grid-cols-3 [animation-delay:420ms]">
              {heroStats.map((stat) => (
                <HeroStat key={stat.label} label={stat.label} value={stat.value} />
              ))}
            </div>
          </div>

          <div className="relative flex items-center justify-center lg:justify-end">
            <div className="absolute right-8 top-10 hidden h-48 w-48 rounded-full bg-[var(--accent-glow)] blur-3xl lg:block" />
            <div className="relative w-full max-w-[840px] animate-fade-up [animation-delay:180ms]">
              <div className="animate-float-slow absolute -bottom-10 left-[-2rem] hidden rounded-3xl border border-[var(--border)] bg-[color:rgba(13,23,40,0.9)] p-5 shadow-[var(--panel-shadow)] lg:block">
                <div className="text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-[var(--text-muted)]">
                  AI control layer
                </div>
                <p className="mt-3 max-w-[18rem] text-sm leading-6 text-[var(--text-secondary)]">
                  Local search, thread reads, sends, drafts, and summaries through the same
                  message surface.
                </p>
              </div>

              <div className="overflow-hidden rounded-[2rem] border border-[var(--border)] bg-[color:rgba(13,23,40,0.86)] shadow-[var(--panel-shadow)]">
                <Image
                  src="/hero-product-dark.png"
                  alt="OpenMessage desktop workspace showing a grouped contact with WhatsApp and SMS lanes"
                  width={1600}
                  height={1100}
                  priority
                  className="h-auto w-full"
                />
              </div>
            </div>
          </div>
        </div>
      </section>

      <section id="features" className="border-b border-[var(--border)]">
        <div className="mx-auto max-w-[1360px] px-6 py-20 lg:px-10">
          <SectionIntro
            eyebrow="Why it feels different"
            title="It behaves like a real messaging client, not a browser hack with an AI sticker on it."
          />

          <div className="mt-14 grid border-y border-[var(--border)] lg:grid-cols-3">
            {productSignals.map((signal, index) => (
              <ProductSignalCard
                key={signal.title}
                index={index}
                signal={signal}
                total={productSignals.length}
              />
            ))}
          </div>
        </div>
      </section>

      <section className="border-b border-[var(--border)]">
        <div className="mx-auto max-w-[1360px] px-6 py-20 lg:px-10">
          <div className="grid gap-12 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
            <SectionIntro
              eyebrow="Workflow"
              title="Pair once. Stay in flow."
              body="OpenMessage is built around the thread workspace itself: routes on the left, messages in the center, local automation at the edge."
            />

            <div className="grid gap-10 border-t border-[var(--border)] pt-8 lg:pt-0">
              {workflowSteps.map((step) => (
                <WorkflowStepCard key={step.number} step={step} />
              ))}
            </div>
          </div>
        </div>
      </section>

      <section id="setup" className="border-b border-[var(--border)]">
        <div className="mx-auto max-w-[1360px] px-6 py-20 lg:px-10">
          <SectionIntro
            eyebrow="Setup"
            title="Native app if you want it. Bare metal if you don&apos;t."
          />

          <div className="mt-14 grid gap-8 lg:grid-cols-2">
            {setupColumns.map((column) => (
              <SetupCard key={column.title} column={column} />
            ))}
          </div>
        </div>
      </section>

      <section id="ai" className="border-b border-[var(--border)]">
        <div className="mx-auto max-w-[1360px] px-6 py-20 lg:px-10">
          <div className="grid gap-10 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
            <SectionIntro
              eyebrow="AI integration"
              title="The message client is also the local tool surface."
              body="OpenMessage doesn&apos;t bolt automation on later. The same local runtime that powers the UI also powers MCP, search, diagnostics, and route-aware sends."
            />

            <div className="grid gap-6">
              <div className="grid gap-6 lg:grid-cols-2">
                {aiBlocks.map((block) => (
                  <AIBlockCard key={block.title} block={block} />
                ))}
              </div>

              <CommandBlock label="Claude Code">{claudeMcpCommand}</CommandBlock>
            </div>
          </div>
        </div>
      </section>

      <section className="mx-auto max-w-[1360px] px-6 py-20 lg:px-10">
        <div className="rounded-[2.25rem] border border-[var(--border)] bg-[linear-gradient(135deg,rgba(13,23,40,0.92),rgba(19,32,53,0.92))] px-8 py-10 shadow-[var(--panel-shadow)] lg:px-12 lg:py-14">
          <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
            <div>
              <span className="eyebrow">Ready now</span>
              <h2 className="mt-5 max-w-[28rem] text-[clamp(2.1rem,4vw,3.5rem)] font-semibold leading-[0.98] tracking-[-0.06em]">
                Ship the local workspace first. Add the rest of your messaging stack over time.
              </h2>
            </div>
            <div className="flex flex-col gap-4 sm:flex-row">
              <ActionLink href={downloadUrl} variant="primary">
                Download OpenMessage
              </ActionLink>
              <ActionLink href={repoUrl} openInNewTab>
                Read the code
              </ActionLink>
            </div>
          </div>
        </div>
      </section>

      <SiteFooter />
    </main>
  );
}
