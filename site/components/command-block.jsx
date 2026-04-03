export function CommandBlock({ children, label }) {
  return (
    <div className="rounded-3xl border border-[var(--border)] bg-[color:rgba(13,23,40,0.78)] p-4 shadow-[0_18px_60px_rgba(4,12,24,0.24)]">
      {label ? (
        <div className="mb-3 text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-[var(--text-muted)]">
          {label}
        </div>
      ) : null}
      <pre className="overflow-x-auto text-sm leading-7 text-[var(--text-primary)]">
        <code>{children}</code>
      </pre>
    </div>
  );
}
