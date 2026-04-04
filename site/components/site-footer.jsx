import { repoUrl } from "../data/site-content";

export function SiteFooter() {
  return (
    <footer className="border-t border-[var(--border)]">
      <div className="mx-auto flex max-w-[1520px] flex-col gap-4 px-6 py-8 text-sm text-[var(--text-muted)] md:flex-row md:items-center md:justify-between lg:px-10">
        <p>Open source, local-first messaging for Google Messages, WhatsApp, and MCP.</p>
        <div className="flex flex-wrap items-center gap-4">
          <a className="transition-colors hover:text-[var(--text-primary)]" href="/privacy">
            Privacy
          </a>
          <a className="transition-colors hover:text-[var(--text-primary)]" href="/thesis">
            Thesis
          </a>
          <a
            className="transition-colors hover:text-[var(--text-primary)]"
            href={repoUrl}
            target="_blank"
            rel="noreferrer"
          >
            GitHub
          </a>
          <a
            className="transition-colors hover:text-[var(--text-primary)]"
            href="https://github.com/sponsors/MaxGhenis"
            target="_blank"
            rel="noreferrer"
          >
            Sponsor
          </a>
        </div>
      </div>
    </footer>
  );
}
