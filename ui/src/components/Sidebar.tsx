import { JSX } from 'preact';

interface NavItem {
  path: string;
  label: string;
  icon: (props: { class?: string }) => JSX.Element;
}

const navItems: NavItem[] = [
  {
    path: '/',
    label: 'Dashboard',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z" />
      </svg>
    ),
  },
  {
    path: '/services',
    label: 'Services',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path d="M3 12v3c0 1.657 3.134 3 7 3s7-1.343 7-3v-3c0 1.657-3.134 3-7 3s-7-1.343-7-3z" />
        <path d="M3 7v3c0 1.657 3.134 3 7 3s7-1.343 7-3V7c0 1.657-3.134 3-7 3S3 8.657 3 7z" />
        <path d="M17 5c0 1.657-3.134 3-7 3S3 6.657 3 5s3.134-3 7-3 7 1.343 7 3z" />
      </svg>
    ),
  },
  {
    path: '/schema',
    label: 'Schema',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M3 4a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm0 4a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm0 4a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm0 4a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1z" clip-rule="evenodd" />
      </svg>
    ),
  },
  {
    path: '/api-explorer',
    label: 'API Explorer',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M12.316 3.051a1 1 0 01.633 1.265l-4 12a1 1 0 11-1.898-.632l4-12a1 1 0 011.265-.633zM5.707 6.293a1 1 0 010 1.414L3.414 10l2.293 2.293a1 1 0 11-1.414 1.414l-3-3a1 1 0 010-1.414l3-3a1 1 0 011.414 0zm8.586 0a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 11-1.414-1.414L16.586 10l-2.293-2.293a1 1 0 010-1.414z" clip-rule="evenodd" />
      </svg>
    ),
  },
  {
    path: '/roles',
    label: 'Roles',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z" />
      </svg>
    ),
  },
  {
    path: '/api-keys',
    label: 'API Keys',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd" />
      </svg>
    ),
  },
  {
    path: '/mcp',
    label: 'MCP Server',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path d="M13 7H7v6h6V7z" />
        <path fill-rule="evenodd" d="M7 2a1 1 0 012 0v1h2V2a1 1 0 112 0v1h2a2 2 0 012 2v2h1a1 1 0 110 2h-1v2h1a1 1 0 110 2h-1v2a2 2 0 01-2 2h-2v1a1 1 0 11-2 0v-1H9v1a1 1 0 11-2 0v-1H5a2 2 0 01-2-2v-2H2a1 1 0 110-2h1V9H2a1 1 0 010-2h1V5a2 2 0 012-2h2V2zM5 5h10v10H5V5z" clip-rule="evenodd" />
      </svg>
    ),
  },
  {
    path: '/settings',
    label: 'Settings',
    icon: (p) => (
      <svg class={p.class} viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z" clip-rule="evenodd" />
      </svg>
    ),
  },
];

interface SidebarProps {
  currentPath: string;
  isOpen: boolean;
  onClose: () => void;
}

export function Sidebar({ currentPath, isOpen, onClose }: SidebarProps) {
  return (
    <aside
      class={`
        fixed lg:static inset-y-0 left-0 z-40
        w-64 bg-surface-raised border-r border-border-subtle
        flex flex-col
        transition-transform duration-200 ease-out
        ${isOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
      `}
    >
      {/* Logo */}
      <div class="flex items-center gap-3 px-5 h-16 border-b border-border-subtle shrink-0">
        <img src="/faucet-icon.svg" alt="Faucet" width="28" height="28" class="shrink-0" />
        <div>
          <span class="text-base font-semibold text-text-primary tracking-tight">Faucet</span>
          <span class="text-xs text-text-muted ml-2">Admin</span>
        </div>
      </div>

      {/* Navigation */}
      <nav class="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {navItems.map((item) => {
          const isActive = currentPath === item.path ||
            (item.path !== '/' && currentPath.startsWith(item.path));
          return (
            <a
              key={item.path}
              href={item.path}
              onClick={(e) => {
                // Let preact-router handle navigation
              }}
              class={`
                flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium
                transition-colors duration-150 group
                ${isActive
                  ? 'bg-brand/10 text-brand'
                  : 'text-text-secondary hover:text-text-primary hover:bg-surface-overlay'
                }
              `}
            >
              <item.icon class={`w-5 h-5 shrink-0 ${isActive ? 'text-brand' : 'text-text-muted group-hover:text-text-secondary'}`} />
              {item.label}
              {isActive && (
                <div class="ml-auto w-1.5 h-1.5 rounded-full bg-brand" />
              )}
            </a>
          );
        })}
      </nav>

      {/* Footer */}
      <div class="px-5 py-4 border-t border-border-subtle">
        <div class="flex items-center gap-2 text-xs text-text-muted">
          <div class="w-2 h-2 rounded-full bg-success animate-pulse" />
          <span>Faucet v0.1.0</span>
        </div>
      </div>
    </aside>
  );
}
