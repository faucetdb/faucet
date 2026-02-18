import { useState, useEffect } from 'preact/hooks';
import { apiFetch } from '../hooks/useApi';

interface ServerConfig {
  listen_addr: string;
  base_path: string;
  cors_origins: string;
  rate_limit: number;
  log_level: string;
  mcp_enabled: boolean;
  mcp_transport: string;
}

export function Settings() {
  const [config, setConfig] = useState<ServerConfig>({
    listen_addr: ':8080',
    base_path: '/api/v1',
    cors_origins: '*',
    rate_limit: 100,
    log_level: 'info',
    mcp_enabled: true,
    mcp_transport: 'stdio',
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeSection, setActiveSection] = useState('server');

  useEffect(() => {
    loadConfig();
  }, []);

  async function loadConfig() {
    setLoading(true);
    try {
      const res = await apiFetch('/api/v1/system/config');
      setConfig(res);
    } catch {
      // Use defaults
    } finally {
      setLoading(false);
    }
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      await apiFetch('/api/v1/system/config', { method: 'PUT', body: config });
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save configuration');
    } finally {
      setSaving(false);
    }
  }

  const sections = [
    { id: 'server', label: 'Server', icon: '~' },
    { id: 'mcp', label: 'MCP', icon: '*' },
    { id: 'admin', label: 'Admin', icon: '#' },
  ];

  if (loading) {
    return (
      <div class="space-y-6">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">Settings</h1>
          <p class="text-sm text-text-secondary mt-1">Server configuration</p>
        </div>
        <div class="card animate-pulse">
          <div class="space-y-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} class="h-10 bg-surface-overlay rounded" />
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div class="space-y-6">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">Settings</h1>
          <p class="text-sm text-text-secondary mt-1">Server configuration and preferences</p>
        </div>
        <div class="flex items-center gap-3">
          {saved && (
            <span class="text-sm text-success flex items-center gap-1 animate-fade-in">
              <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
              </svg>
              Saved
            </span>
          )}
          <button
            onClick={handleSave}
            disabled={saving}
            class="btn-primary"
          >
            {saving ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      </div>

      {error && (
        <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error">
          {error}
        </div>
      )}

      <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* Section nav */}
        <div class="lg:col-span-1">
          <nav class="space-y-1">
            {sections.map((s) => (
              <button
                key={s.id}
                onClick={() => setActiveSection(s.id)}
                class={`
                  w-full text-left px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2
                  transition-colors duration-150
                  ${activeSection === s.id
                    ? 'bg-brand/10 text-brand'
                    : 'text-text-secondary hover:text-text-primary hover:bg-surface-overlay'
                  }
                `}
              >
                <span class="w-6 text-center font-mono text-text-muted">{s.icon}</span>
                {s.label}
              </button>
            ))}
          </nav>
        </div>

        {/* Settings form */}
        <div class="lg:col-span-3">
          {activeSection === 'server' && (
            <div class="card space-y-6">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">Server Configuration</h2>
                <p class="text-sm text-text-muted">Core HTTP server settings</p>
              </div>

              <div class="space-y-5">
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Listen Address</label>
                  <input
                    type="text"
                    class="input w-full max-w-sm font-mono text-sm"
                    value={config.listen_addr}
                    onInput={(e) => setConfig({ ...config, listen_addr: (e.target as HTMLInputElement).value })}
                  />
                  <p class="text-xs text-text-muted mt-1">Host and port for the HTTP server (e.g. :8080, 0.0.0.0:8080)</p>
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">API Base Path</label>
                  <input
                    type="text"
                    class="input w-full max-w-sm font-mono text-sm"
                    value={config.base_path}
                    onInput={(e) => setConfig({ ...config, base_path: (e.target as HTMLInputElement).value })}
                  />
                  <p class="text-xs text-text-muted mt-1">Prefix for all API routes</p>
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">CORS Origins</label>
                  <input
                    type="text"
                    class="input w-full font-mono text-sm"
                    placeholder="* or https://example.com,https://app.example.com"
                    value={config.cors_origins}
                    onInput={(e) => setConfig({ ...config, cors_origins: (e.target as HTMLInputElement).value })}
                  />
                  <p class="text-xs text-text-muted mt-1">Comma-separated allowed origins, or * for all</p>
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Rate Limit</label>
                  <div class="flex items-center gap-2">
                    <input
                      type="number"
                      class="input w-32 font-mono text-sm"
                      min="0"
                      value={config.rate_limit}
                      onInput={(e) => setConfig({ ...config, rate_limit: parseInt((e.target as HTMLInputElement).value) || 0 })}
                    />
                    <span class="text-sm text-text-muted">requests/minute per key</span>
                  </div>
                  <p class="text-xs text-text-muted mt-1">0 to disable rate limiting</p>
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Log Level</label>
                  <select
                    class="input w-48"
                    value={config.log_level}
                    onChange={(e) => setConfig({ ...config, log_level: (e.target as HTMLSelectElement).value })}
                  >
                    <option value="debug">Debug</option>
                    <option value="info">Info</option>
                    <option value="warn">Warn</option>
                    <option value="error">Error</option>
                  </select>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'mcp' && (
            <div class="card space-y-6">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">MCP Configuration</h2>
                <p class="text-sm text-text-muted">Model Context Protocol server settings for AI agent access</p>
              </div>

              <div class="space-y-5">
                <div class="flex items-center justify-between p-4 rounded-lg bg-surface border border-border-subtle">
                  <div>
                    <p class="text-sm font-medium text-text-primary">Enable MCP Server</p>
                    <p class="text-xs text-text-muted mt-0.5">Expose database tools via Model Context Protocol</p>
                  </div>
                  <button
                    onClick={() => setConfig({ ...config, mcp_enabled: !config.mcp_enabled })}
                    class={`
                      relative inline-flex h-6 w-11 items-center rounded-full transition-colors
                      ${config.mcp_enabled ? 'bg-brand' : 'bg-surface-overlay border border-border-default'}
                    `}
                  >
                    <span
                      class={`
                        inline-block h-4 w-4 rounded-full bg-white transition-transform
                        ${config.mcp_enabled ? 'translate-x-6' : 'translate-x-1'}
                      `}
                    />
                  </button>
                </div>

                {config.mcp_enabled && (
                  <div>
                    <label class="block text-sm font-medium text-text-secondary mb-1.5">Transport</label>
                    <select
                      class="input w-48"
                      value={config.mcp_transport}
                      onChange={(e) => setConfig({ ...config, mcp_transport: (e.target as HTMLSelectElement).value })}
                    >
                      <option value="stdio">stdio</option>
                      <option value="sse">SSE (HTTP)</option>
                    </select>
                    <p class="text-xs text-text-muted mt-1">
                      stdio for local tools (e.g. Claude Code), SSE for remote access
                    </p>
                  </div>
                )}
              </div>
            </div>
          )}

          {activeSection === 'admin' && (
            <div class="card space-y-6">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">Admin Account</h2>
                <p class="text-sm text-text-muted">Change the admin password</p>
              </div>

              <div class="space-y-4">
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Current Password</label>
                  <input type="password" class="input w-full max-w-sm" placeholder="Enter current password" />
                </div>
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">New Password</label>
                  <input type="password" class="input w-full max-w-sm" placeholder="Enter new password" />
                </div>
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Confirm New Password</label>
                  <input type="password" class="input w-full max-w-sm" placeholder="Confirm new password" />
                </div>
                <div class="pt-2">
                  <button class="btn-primary text-sm">Update Password</button>
                </div>
              </div>

              <div class="border-t border-border-subtle pt-6">
                <h3 class="text-base font-semibold text-error mb-1">Danger Zone</h3>
                <p class="text-sm text-text-muted mb-4">Irreversible actions</p>
                <div class="flex items-center gap-4">
                  <button class="btn-danger text-sm">
                    Reset All Configuration
                  </button>
                  <button class="btn-danger text-sm">
                    Purge All Data
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
