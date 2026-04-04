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

const heroSignals = [
  "Google Messages + WhatsApp live now",
  "Native macOS app + local web runtime",
  "MCP endpoint built into the same local store"
];

function ActionLink({ children, href, external = false, primary = false }) {
  const className = primary
    ? "inline-flex items-center justify-center rounded-full bg-[var(--accent)] px-6 py-3.5 text-sm font-semibold text-[var(--bg-deep)] transition-transform hover:-translate-y-0.5"
    : "inline-flex items-center justify-center rounded-full border border-[var(--border)] bg-[color:rgba(8,13,24,0.72)] px-6 py-3.5 text-sm font-semibold text-[var(--text-primary)] transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--bg-hover)]";
  const props = external ? { rel: "noreferrer", target: "_blank" } : {};

  return (
    <a href={href} className={className} {...props}>
      {children}
    </a>
  );
}

function SectionHeading({ body, eyebrow, title }) {
  return (
    <div className="max-w-[34rem]">
      <div className="eyebrow">{eyebrow}</div>
      <h2 className="mt-5 text-[clamp(2.2rem,4vw,4rem)] font-semibold leading-[0.95] tracking-[-0.06em] text-[var(--text-primary)]">
        {title}
      </h2>
      {body ? (
        <p className="mt-5 text-base leading-7 text-[var(--text-secondary)]">{body}</p>
      ) : null}
    </div>
  );
}

function ProductSignal({ index, signal }) {
  return (
    <article className="grid gap-4 border-b border-[var(--border)] py-7 last:border-b-0 md:grid-cols-[72px_minmax(0,1fr)] md:gap-6">
      <div className="text-[0.75rem] font-semibold uppercase tracking-[0.24em] text-[var(--text-muted)]">
        {String(index + 1).padStart(2, "0")}
      </div>
      <div>
        <h3 className="text-[1.65rem] font-semibold tracking-[-0.05em] text-[var(--text-primary)]">
          {signal.title}
        </h3>
        <p className="mt-3 max-w-[34rem] text-base leading-7 text-[var(--text-secondary)]">
          {signal.body}
        </p>
      </div>
    </article>
  );
}

function WorkflowStep({ step }) {
  return (
    <div className="grid gap-4 border-b border-[var(--border)] py-6 last:border-b-0 md:grid-cols-[88px_minmax(0,1fr)]">
      <div className="text-[2.8rem] font-semibold tracking-[-0.08em] text-[var(--accent-strong)]">
        {step.number}
      </div>
      <div>
        <h3 className="text-[1.45rem] font-semibold tracking-[-0.045em] text-[var(--text-primary)]">
          {step.title}
        </h3>
        <p className="mt-3 max-w-[34rem] text-base leading-7 text-[var(--text-secondary)]">
          {step.body}
        </p>
      </div>
    </div>
  );
}

function SetupColumn({ column, index }) {
  return (
    <section
      className={`py-8 ${index === 0 ? "border-b border-[var(--border)] lg:border-b-0 lg:border-r lg:pr-8" : "lg:pl-8"}`}
    >
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

function AIBlock({ block }) {
  return (
    <div className="border-b border-[var(--border)] py-6 last:border-b-0">
      <div className="text-[1.35rem] font-semibold tracking-[-0.045em] text-[var(--text-primary)]">
        {block.title}
      </div>
      <p className="mt-3 max-w-[38rem] text-base leading-7 text-[var(--text-secondary)]">
        {block.body}
      </p>
      <div className="mt-4 font-mono text-sm text-[var(--accent-strong)]">{block.command}</div>
    </div>
  );
}

export default function HomePage() {
  return (
    <main className="min-h-screen">
      <SiteHeader overlay />

      <section className="relative overflow-hidden border-b border-[var(--border)]">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(118,137,255,0.18),transparent_34%),radial-gradient(circle_at_75%_30%,rgba(118,137,255,0.1),transparent_24%)]" />
        <div className="relative mx-auto max-w-[1520px] px-6 pb-16 pt-28 lg:px-10 lg:pb-24 lg:pt-34">
          <div className="grid gap-12 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)] lg:items-end">
            <div className="max-w-[32rem]">
              <div className="eyebrow">Live now for Google Messages + WhatsApp</div>
              <div className="animate-fade-up mt-7 text-[clamp(4.2rem,10vw,8rem)] font-semibold leading-[0.88] tracking-[-0.09em] text-[var(--text-primary)]">
                OpenMessage
              </div>
              <h1 className="animate-fade-up mt-6 text-[clamp(2rem,4.5vw,4rem)] font-semibold leading-[0.94] tracking-[-0.06em] text-[var(--text-primary)] [animation-delay:120ms]">
                The local desktop for SMS, RCS, and WhatsApp.
              </h1>
              <p className="animate-fade-up mt-6 max-w-[30rem] text-lg leading-8 text-[var(--text-secondary)] [animation-delay:220ms]">
                Pair the routes people already use, keep the message store on your machine, and
                give Claude the same context through a local MCP endpoint.
              </p>

              <div className="animate-fade-up mt-9 flex flex-col gap-4 sm:flex-row [animation-delay:320ms]">
                <ActionLink href={downloadUrl} primary>
                  Download for macOS
                </ActionLink>
                <ActionLink href={repoUrl} external>
                  View the repo
                </ActionLink>
              </div>

              <div className="animate-fade-up mt-9 flex flex-wrap gap-3 [animation-delay:420ms]">
                {heroSignals.map((signal) => (
                  <div
                    key={signal}
                    className="rounded-full border border-[var(--border)] bg-[color:rgba(8,13,24,0.6)] px-4 py-2 text-[0.78rem] font-medium text-[var(--text-secondary)]"
                  >
                    {signal}
                  </div>
                ))}
              </div>
            </div>

            <div className="relative animate-fade-up [animation-delay:180ms]">
              <div className="absolute right-[6%] top-[8%] hidden h-52 w-52 rounded-full bg-[var(--accent-glow)] blur-3xl lg:block" />
              <div className="relative overflow-hidden rounded-[2.6rem] border border-[var(--border)] bg-[color:rgba(8,13,24,0.78)] shadow-[var(--panel-shadow)]">
                <Image
                  src="/hero-product-dark.png"
                  alt="OpenMessage workspace showing grouped SMS and WhatsApp threads"
                  width={1600}
                  height={1100}
                  priority
                  className="h-auto w-full"
                />
              </div>
              <div className="mt-4 flex flex-col gap-3 text-sm text-[var(--text-muted)] md:flex-row md:items-center md:justify-between">
                <div>Grouped routes, media, desktop notifications, local diagnostics.</div>
                <div className="text-[0.72rem] font-semibold uppercase tracking-[0.24em] text-[var(--accent-strong)]">
                  Local-first by default
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section id="features" className="border-b border-[var(--border)]">
        <div className="mx-auto grid max-w-[1520px] gap-12 px-6 py-20 lg:grid-cols-[minmax(0,360px)_minmax(0,1fr)] lg:px-10">
          <SectionHeading
            eyebrow="What ships now"
            title="A real client first, then an AI surface on top."
            body="The point is not another mirror tab. OpenMessage already handles the actual workflows: route-aware replies, WhatsApp media, grouped people, native notifications, and a shared local store."
          />

          <div className="border-y border-[var(--border)]">
            {productSignals.map((signal, index) => (
              <ProductSignal key={signal.title} index={index} signal={signal} />
            ))}
          </div>
        </div>
      </section>

      <section className="border-b border-[var(--border)]">
        <div className="mx-auto grid max-w-[1520px] gap-14 px-6 py-20 lg:grid-cols-[minmax(0,1.08fr)_minmax(0,360px)] lg:px-10">
          <div className="overflow-hidden rounded-[2.3rem] border border-[var(--border)] bg-[color:rgba(8,13,24,0.74)] shadow-[var(--panel-shadow)]">
            <Image
              src="/hero-command-surface.png"
              alt="OpenMessage app beside a terminal using MCP commands against it"
              width={3232}
              height={1800}
              className="h-auto w-full"
            />
          </div>

          <div className="flex flex-col justify-between">
            <SectionHeading
              eyebrow="Workflow"
              title="One thread surface for the messages and the commands around them."
              body="The same local runtime powers the app, search, diagnostics, and MCP. That means your assistant can inspect or draft against the same conversation state you are already looking at."
            />

            <div className="mt-8 border-y border-[var(--border)]">
              {workflowSteps.map((step) => (
                <WorkflowStep key={step.number} step={step} />
              ))}
            </div>
          </div>
        </div>
      </section>

      <section id="setup" className="border-b border-[var(--border)]">
        <div className="mx-auto max-w-[1520px] px-6 py-20 lg:px-10">
          <SectionHeading
            eyebrow="Setup"
            title="Start with the native app, or run the same local stack directly."
            body="The fast path is the macOS app with notifications, contacts, and an embedded backend. The same product also runs as a plain Go binary with a local web UI and the same MCP surface."
          />

          <div className="mt-14 grid border-y border-[var(--border)] lg:grid-cols-2">
            {setupColumns.map((column, index) => (
              <SetupColumn key={column.title} column={column} index={index} />
            ))}
          </div>
        </div>
      </section>

      <section id="ai" className="border-b border-[var(--border)]">
        <div className="mx-auto grid max-w-[1520px] gap-12 px-6 py-20 lg:grid-cols-[minmax(0,360px)_minmax(0,1fr)] lg:px-10">
          <SectionHeading
            eyebrow="MCP"
            title="Your assistant sees the same routes you do."
            body="OpenMessage exposes one local endpoint for search, drafts, sends, and thread inspection. No cloud relay, no separate sync layer, and no fake demo surface disconnected from the actual inbox."
          />

          <div className="border-y border-[var(--border)]">
            {aiBlocks.map((block) => (
              <AIBlock key={block.title} block={block} />
            ))}
            <div className="py-8">
              <CommandBlock label="Claude Code">{claudeMcpCommand}</CommandBlock>
            </div>
          </div>
        </div>
      </section>

      <section className="mx-auto max-w-[1520px] px-6 py-20 lg:px-10">
        <div className="border-y border-[var(--border)] py-12">
          <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
            <div className="max-w-[36rem]">
              <div className="eyebrow">Ready now</div>
              <h2 className="mt-5 text-[clamp(2.2rem,4vw,4rem)] font-semibold leading-[0.95] tracking-[-0.06em] text-[var(--text-primary)]">
                Start with Google Messages and WhatsApp. Add the rest of the stack later.
              </h2>
              <p className="mt-5 text-base leading-7 text-[var(--text-secondary)]">
                OpenMessage already ships the routes people actually need, with local search,
                route-aware replies, media, diagnostics, notifications, and MCP in one product.
              </p>
            </div>

            <div className="flex flex-col gap-4 sm:flex-row">
              <ActionLink href={downloadUrl} primary>
                Download OpenMessage
              </ActionLink>
              <ActionLink href={repoUrl} external>
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
