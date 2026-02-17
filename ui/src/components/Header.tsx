interface HeaderProps {
  onMenuClick: () => void;
}

export function Header({ onMenuClick }: HeaderProps) {
  return (
    <header class="h-16 border-b border-border-subtle bg-surface-raised/80 backdrop-blur-sm flex items-center justify-between px-4 lg:px-6 shrink-0 z-20">
      {/* Left side */}
      <div class="flex items-center gap-3">
        <button
          onClick={onMenuClick}
          class="lg:hidden p-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-overlay transition-colors"
          aria-label="Toggle menu"
        >
          <svg class="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M3 5a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM3 10a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM3 15a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1z" clip-rule="evenodd" />
          </svg>
        </button>

        {/* Breadcrumb / Search placeholder */}
        <div class="hidden sm:flex items-center gap-2 text-sm text-text-muted">
          <kbd class="px-2 py-0.5 rounded bg-surface-overlay border border-border-default text-xs font-mono">/</kbd>
          <span>Quick search...</span>
        </div>
      </div>

      {/* Right side */}
      <div class="flex items-center gap-3">
        {/* Health indicator */}
        <a
          href="/api/v1/system/health"
          target="_blank"
          class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium text-text-secondary hover:text-text-primary hover:bg-surface-overlay transition-colors"
        >
          <div class="w-2 h-2 rounded-full bg-success" />
          <span class="hidden sm:inline">Healthy</span>
        </a>

        {/* Docs link */}
        <a
          href="/openapi.json"
          target="_blank"
          class="p-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-overlay transition-colors"
          title="OpenAPI Spec"
        >
          <svg class="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4z" clip-rule="evenodd" />
          </svg>
        </a>

        {/* User avatar */}
        <div class="w-8 h-8 rounded-full bg-gradient-to-br from-brand to-cyan-accent flex items-center justify-center text-xs font-bold text-white">
          A
        </div>
      </div>
    </header>
  );
}
