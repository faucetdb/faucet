import { useState } from 'preact/hooks';
import { Icon, IconName } from './Icon';
import { DOCS_URL, GITHUB_URL, useHealth, useServerInfo } from '../lib/server';
import { ThemeChoice, getTheme, setTheme } from '../lib/theme';

interface NavItem {
  path: string;
  label: string;
  icon: IconName;
}

const NAV: { heading?: string; items: NavItem[] }[] = [
  { items: [{ path: '/', label: 'Overview', icon: 'home' }] },
  {
    heading: 'Data',
    items: [
      { path: '/services', label: 'Databases', icon: 'database' },
      { path: '/schema', label: 'Schema', icon: 'table' },
      { path: '/api-explorer', label: 'API explorer', icon: 'terminal' },
    ],
  },
  {
    heading: 'Access',
    items: [
      { path: '/roles', label: 'Roles', icon: 'shield' },
      { path: '/api-keys', label: 'API keys', icon: 'key' },
    ],
  },
  {
    heading: 'Integrations',
    items: [{ path: '/mcp', label: 'AI agents (MCP)', icon: 'sparkle' }],
  },
];

const THEME_NEXT: Record<ThemeChoice, ThemeChoice> = { system: 'dark', dark: 'light', light: 'system' };
const THEME_ICON: Record<ThemeChoice, IconName> = { system: 'monitor', dark: 'moon', light: 'sun' };
const THEME_LABEL: Record<ThemeChoice, string> = { system: 'Theme: system', dark: 'Theme: dark', light: 'Theme: light' };

function isActive(current: string, path: string) {
  const p = current.split('?')[0];
  return path === '/' ? p === '/' : p === path || p.startsWith(path + '/');
}

export function Sidebar({ currentPath, isOpen, onClose, onLogout }: {
  currentPath: string; isOpen: boolean; onClose: () => void; onLogout: () => void;
}) {
  const info = useServerInfo();
  const { health, checks } = useHealth();
  const [theme, setThemeState] = useState<ThemeChoice>(getTheme());
  const email = localStorage.getItem('faucet_admin_email');

  const down = Object.entries(checks).filter(([, v]) => v !== 'ok').map(([k]) => k);
  const healthText =
    health === 'down' ? 'Server unreachable'
      : health === 'degraded' ? `${down.length === 1 ? `${down[0]} is` : `${down.length} databases are`} unreachable`
      : health === 'ok' ? 'All systems normal'
      : 'Checking';
  const healthColor = health === 'ok' ? 'bg-ok' : health === 'unknown' ? 'bg-fg-faint' : health === 'degraded' ? 'bg-warn' : 'bg-bad';

  return (
    <aside
      class={`fixed lg:sticky top-0 inset-y-0 left-0 z-40 w-[240px] h-screen shrink-0 flex flex-col bg-panel border-r border-line
        transition-transform duration-200 ease-out ${isOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
      aria-label="Main navigation"
    >
      <div class="flex items-center gap-2.5 px-4 h-14 shrink-0">
        <img src="/faucet-icon.svg" alt="" width="24" height="24" />
        <span class="text-lg font-semibold tracking-[-0.01em] text-fg">Faucet</span>
        <button type="button" onClick={onClose} class="ml-auto lg:hidden p-1.5 rounded text-fg-muted hover:text-fg" aria-label="Close menu">
          <Icon name="x" size={18} />
        </button>
      </div>

      <nav class="flex-1 overflow-y-auto px-2.5 pb-4">
        {NAV.map((group, gi) => (
          <div key={gi} class={gi > 0 ? 'mt-5' : 'mt-1'}>
            {group.heading && <p class="px-2.5 mb-1 text-xs font-medium text-fg-faint">{group.heading}</p>}
            <ul class="flex flex-col gap-0.5">
              {group.items.map((item) => {
                const active = isActive(currentPath, item.path);
                return (
                  <li key={item.path}>
                    <a
                      href={item.path}
                      aria-current={active ? 'page' : undefined}
                      class={`flex items-center gap-2.5 h-8 px-2.5 rounded-[6px] text-[13.5px] transition-colors ${
                        active ? 'bg-brand/10 text-fg font-medium' : 'text-fg-muted hover:text-fg hover:bg-panel-2'
                      }`}
                    >
                      <Icon name={item.icon} size={16} class={active ? 'text-brand-fg' : ''} />
                      {item.label}
                    </a>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      <div class="border-t border-line px-2.5 py-2.5 flex flex-col gap-0.5">
        <a href="/services" class="flex items-center gap-2.5 h-8 px-2.5 rounded-[6px] text-sm text-fg-muted hover:text-fg hover:bg-panel-2" title={down.length ? `Unreachable: ${down.join(', ')}` : undefined}>
          <span class={`w-2 h-2 rounded-full ${healthColor}`} aria-hidden="true" />
          <span class="truncate">{healthText}</span>
        </a>
        <a href="/settings" aria-current={isActive(currentPath, '/settings') ? 'page' : undefined}
          class={`flex items-center gap-2.5 h-8 px-2.5 rounded-[6px] text-sm ${isActive(currentPath, '/settings') ? 'bg-brand/10 text-fg font-medium' : 'text-fg-muted hover:text-fg hover:bg-panel-2'}`}>
          <Icon name="settings" size={16} />
          Settings
        </a>
        <a href={DOCS_URL} target="_blank" rel="noopener noreferrer" class="flex items-center gap-2.5 h-8 px-2.5 rounded-[6px] text-sm text-fg-muted hover:text-fg hover:bg-panel-2">
          <Icon name="book" size={16} />
          Documentation
          <Icon name="external" size={12} class="ml-auto opacity-60" />
        </a>
        <div class="flex items-center gap-1 mt-1.5 pt-2.5 border-t border-line px-1">
          <div class="min-w-0 flex-1 px-1.5">
            <p class="text-xs text-fg truncate" title={email || undefined}>{email || 'Admin'}</p>
            <a href={`${GITHUB_URL}/releases`} target="_blank" rel="noopener noreferrer" class="text-xs text-fg-faint hover:text-fg-muted font-mono">
              {info?.version || ''}
            </a>
          </div>
          <a href={GITHUB_URL} target="_blank" rel="noopener noreferrer" class="inline-flex items-center justify-center w-8 h-8 rounded-[6px] text-fg-muted hover:text-fg hover:bg-panel-2" title="Faucet on GitHub" aria-label="Faucet on GitHub">
            <Icon name="github" size={16} />
          </a>
          <button
            type="button"
            class="inline-flex items-center justify-center w-8 h-8 rounded-[6px] text-fg-muted hover:text-fg hover:bg-panel-2"
            title={THEME_LABEL[theme]}
            aria-label={THEME_LABEL[theme]}
            onClick={() => {
              const next = THEME_NEXT[theme];
              setTheme(next);
              setThemeState(next);
            }}
          >
            <Icon name={THEME_ICON[theme]} size={16} />
          </button>
          <button type="button" onClick={onLogout} class="inline-flex items-center justify-center w-8 h-8 rounded-[6px] text-fg-muted hover:text-fg hover:bg-panel-2" title="Sign out" aria-label="Sign out">
            <Icon name="logout" size={16} />
          </button>
        </div>
      </div>
    </aside>
  );
}
