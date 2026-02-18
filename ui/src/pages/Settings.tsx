import { useState, useEffect } from 'preact/hooks';
import { apiFetch } from '../hooks/useApi';

interface AdminInfo {
  id: number;
  username: string;
  email?: string;
  created_at: string;
}

export function Settings() {
  const [admins, setAdmins] = useState<AdminInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeSection, setActiveSection] = useState('info');

  // Password change form
  const [passwordForm, setPasswordForm] = useState({
    current: '',
    newPass: '',
    confirm: '',
  });
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [passwordSuccess, setPasswordSuccess] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    setLoading(true);
    try {
      const res = await apiFetch('/api/v1/system/admin');
      setAdmins(res.resource || []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }

  const sections = [
    { id: 'info', label: 'Server Info', icon: '~' },
    { id: 'admins', label: 'Admins', icon: '#' },
    { id: 'about', label: 'About', icon: '?' },
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
      <div>
        <h1 class="text-2xl font-semibold text-text-primary">Settings</h1>
        <p class="text-sm text-text-secondary mt-1">Server configuration and information</p>
      </div>

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

        {/* Settings content */}
        <div class="lg:col-span-3">
          {activeSection === 'info' && (
            <div class="card space-y-6">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">Server Information</h2>
                <p class="text-sm text-text-muted">Current configuration and runtime details</p>
              </div>

              <div class="space-y-4">
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">API Base</p>
                    <p class="text-sm font-mono text-text-primary">/api/v1</p>
                  </div>
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Health Endpoint</p>
                    <p class="text-sm font-mono text-text-primary">/healthz</p>
                  </div>
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">OpenAPI Spec</p>
                    <a href="/openapi.json" target="_blank" class="text-sm font-mono text-brand hover:text-brand-light">/openapi.json</a>
                  </div>
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Authentication</p>
                    <p class="text-sm font-mono text-text-primary">X-API-Key / Bearer JWT</p>
                  </div>
                </div>

                <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                  <p class="text-xs text-text-muted mb-2">API Endpoints</p>
                  <div class="space-y-1.5 font-mono text-xs">
                    <div class="flex items-center gap-2">
                      <span class="text-success font-semibold w-12">GET</span>
                      <span class="text-text-secondary">/api/v1/{'{service}'}/_schema</span>
                      <span class="text-text-muted ml-auto">List tables</span>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-success font-semibold w-12">GET</span>
                      <span class="text-text-secondary">/api/v1/{'{service}'}/_table/{'{table}'}</span>
                      <span class="text-text-muted ml-auto">Query records</span>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-brand font-semibold w-12">POST</span>
                      <span class="text-text-secondary">/api/v1/{'{service}'}/_table/{'{table}'}</span>
                      <span class="text-text-muted ml-auto">Create records</span>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-warning font-semibold w-12">PUT</span>
                      <span class="text-text-secondary">/api/v1/{'{service}'}/_table/{'{table}'}</span>
                      <span class="text-text-muted ml-auto">Replace records</span>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-error font-semibold w-12">DEL</span>
                      <span class="text-text-secondary">/api/v1/{'{service}'}/_table/{'{table}'}</span>
                      <span class="text-text-muted ml-auto">Delete records</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'admins' && (
            <div class="card space-y-6">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">Admin Accounts</h2>
                <p class="text-sm text-text-muted">Manage administrator accounts</p>
              </div>

              <div class="overflow-x-auto rounded-lg border border-border-subtle">
                <table class="w-full text-sm">
                  <thead>
                    <tr class="border-b border-border-subtle bg-surface">
                      <th class="text-left px-4 py-3 font-medium text-text-secondary">Username</th>
                      <th class="text-left px-4 py-3 font-medium text-text-secondary">Email</th>
                      <th class="text-left px-4 py-3 font-medium text-text-secondary">Created</th>
                    </tr>
                  </thead>
                  <tbody>
                    {admins.length === 0 ? (
                      <tr>
                        <td colSpan={3} class="px-4 py-8 text-center text-text-muted">No admins found</td>
                      </tr>
                    ) : (
                      admins.map((admin) => (
                        <tr key={admin.id} class="border-b border-border-subtle last:border-0">
                          <td class="px-4 py-3 font-medium text-text-primary">{admin.username}</td>
                          <td class="px-4 py-3 text-text-secondary">{admin.email || '--'}</td>
                          <td class="px-4 py-3 text-text-secondary text-xs">
                            {admin.created_at ? new Date(admin.created_at).toLocaleDateString() : '--'}
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {activeSection === 'about' && (
            <div class="card space-y-6">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">About Faucet</h2>
                <p class="text-sm text-text-muted">Open-source database-to-REST API generator</p>
              </div>

              <div class="space-y-4">
                <div class="flex items-center gap-4 p-4 rounded-lg bg-surface border border-border-subtle">
                  <div class="w-12 h-12 rounded-xl bg-gradient-to-br from-brand to-cyan-accent flex items-center justify-center shrink-0">
                    <svg class="w-6 h-6 text-white" viewBox="0 0 20 20" fill="currentColor">
                      <path d="M5 3a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2V5a2 2 0 00-2-2H5zM5 11a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2H5zM11 5a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V5zM14 11a1 1 0 011 1v1h1a1 1 0 110 2h-1v1a1 1 0 11-2 0v-1h-1a1 1 0 110-2h1v-1a1 1 0 011-1z" />
                    </svg>
                  </div>
                  <div>
                    <p class="text-sm font-semibold text-text-primary">Faucet</p>
                    <p class="text-xs text-text-muted">Turn any SQL database into a secure REST API. Single binary, zero configuration.</p>
                  </div>
                </div>

                <div class="grid grid-cols-2 gap-4">
                  <div class="p-3 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Built With</p>
                    <p class="text-sm text-text-primary">Go + Chi + sqlx</p>
                  </div>
                  <div class="p-3 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Frontend</p>
                    <p class="text-sm text-text-primary">Preact + Tailwind</p>
                  </div>
                  <div class="p-3 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Supported DBs</p>
                    <p class="text-sm text-text-primary">PostgreSQL, MySQL, MSSQL, Snowflake</p>
                  </div>
                  <div class="p-3 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Features</p>
                    <p class="text-sm text-text-primary">RBAC, MCP Server, OpenAPI</p>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
