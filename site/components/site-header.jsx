const navItems = [
  { href: "/#features", label: "Features" },
  { href: "/#setup", label: "Setup" },
  { href: "/#ai", label: "AI" }
];

export function SiteHeader({ compact = false }) {
  return (
    <header className="sticky top-0 z-50 border-b border-[var(--border)] bg-[color:rgba(9,17,28,0.82)] backdrop-blur-xl">
      <div className="mx-auto flex max-w-[1360px] items-center justify-between gap-6 px-6 py-4 lg:px-10">
        <a
          href="/"
          className="text-[1.45rem] font-semibold tracking-[-0.04em] text-[var(--text-primary)] transition-colors hover:text-[var(--accent-strong)]"
        >
          OpenMessage
        </a>
        <div className="flex items-center gap-3 md:gap-5">
          <nav className="hidden items-center gap-5 text-sm text-[var(--text-secondary)] md:flex">
            {navItems.map((item) => (
              <a
                key={item.href}
                href={item.href}
                className="transition-colors hover:text-[var(--text-primary)]"
              >
                {item.label}
              </a>
            ))}
          </nav>
          <a
            href="https://github.com/MaxGhenis/openmessage/releases/latest/download/OpenMessage.dmg"
            className={`inline-flex items-center justify-center rounded-full border border-[var(--border-strong)] bg-[var(--accent)] px-4 py-2 text-sm font-medium text-[var(--bg-deep)] transition-transform hover:-translate-y-0.5 ${
              compact ? "min-w-[112px]" : "min-w-[132px]"
            }`}
          >
            Download
          </a>
        </div>
      </div>
    </header>
  );
}
