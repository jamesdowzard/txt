export function SiteFooter() {
  return (
    <footer className="border-t border-[var(--border)]">
      <div className="mx-auto flex max-w-[1360px] flex-col gap-4 px-6 py-8 text-sm text-[var(--text-muted)] md:flex-row md:items-center md:justify-between lg:px-10">
        <p>Created by Max Ghenis. Open source under Apache 2.0.</p>
        <div className="flex flex-wrap items-center gap-4">
          <a className="transition-colors hover:text-[var(--text-primary)]" href="/privacy">
            Privacy
          </a>
          <a className="transition-colors hover:text-[var(--text-primary)]" href="/thesis">
            Thesis
          </a>
          <a
            className="transition-colors hover:text-[var(--text-primary)]"
            href="https://github.com/MaxGhenis/openmessage"
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
