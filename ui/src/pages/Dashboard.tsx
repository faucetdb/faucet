import { useState, useEffect } from 'preact/hooks';
import { StatusBadge } from '../components/StatusBadge';
import { apiFetch } from '../hooks/useApi';

interface ServiceSummary {
  name: string;
  driver: string;
  active: boolean;
}

export function Dashboard() {
  const [services, setServices] = useState<ServiceSummary[]>([]);
  const [apiKeyCount, setApiKeyCount] = useState(0);
  const [roleCount, setRoleCount] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadDashboard();
  }, []);

  async function loadDashboard() {
    setLoading(true);
    try {
      const [servicesRes, keysRes, rolesRes] = await Promise.allSettled([
        apiFetch('/api/v1/system/service'),
        apiFetch('/api/v1/system/api-key'),
        apiFetch('/api/v1/system/role'),
      ]);

      if (servicesRes.status === 'fulfilled') {
        setServices(servicesRes.value.resource || []);
      }
      if (keysRes.status === 'fulfilled') {
        setApiKeyCount((keysRes.value.resource || []).length);
      }
      if (rolesRes.status === 'fulfilled') {
        setRoleCount((rolesRes.value.resource || []).length);
      }
    } catch {
      // Dashboard is best-effort
    } finally {
      setLoading(false);
    }
  }

  function StatCard({ label, value, icon, accent }: { label: string; value: string | number; icon: any; accent?: string }) {
    return (
      <div class="card group hover:border-border-default transition-colors duration-200">
        <div class="flex items-start justify-between">
          <div>
            <p class="text-sm text-text-secondary mb-1">{label}</p>
            <p class={`text-2xl font-semibold font-mono ${accent || 'text-text-primary'}`}>{value}</p>
          </div>
          <div class="p-2 rounded-lg bg-surface-overlay text-text-muted group-hover:text-brand transition-colors">
            {icon}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div class="space-y-6">
      {/* Page header */}
      <div>
        <h1 class="text-2xl font-semibold text-text-primary">Dashboard</h1>
        <p class="text-sm text-text-secondary mt-1">Overview of your Faucet instance</p>
      </div>

      {/* Stats grid */}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        <StatCard
          label="Connected Databases"
          value={services.length}
          accent="text-brand"
          icon={
            <svg class="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M3 12v3c0 1.657 3.134 3 7 3s7-1.343 7-3v-3c0 1.657-3.134 3-7 3s-7-1.343-7-3z" />
              <path d="M3 7v3c0 1.657 3.134 3 7 3s7-1.343 7-3V7c0 1.657-3.134 3-7 3S3 8.657 3 7z" />
              <path d="M17 5c0 1.657-3.134 3-7 3S3 6.657 3 5s3.134-3 7-3 7 1.343 7 3z" />
            </svg>
          }
        />
        <StatCard
          label="API Keys"
          value={apiKeyCount}
          icon={
            <svg class="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd" />
            </svg>
          }
        />
        <StatCard
          label="Roles"
          value={roleCount}
          icon={
            <svg class="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z" />
            </svg>
          }
        />
      </div>

      {/* Services status + Quick actions */}
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Connected services */}
        <div class="lg:col-span-2 card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-base font-semibold text-text-primary">Connected Databases</h2>
            <a href="/services" class="text-xs text-brand hover:text-brand-light transition-colors">
              View all &rarr;
            </a>
          </div>
          {loading ? (
            <div class="space-y-3">
              {[1, 2, 3].map((i) => (
                <div key={i} class="h-14 bg-surface-overlay rounded-lg animate-pulse" />
              ))}
            </div>
          ) : services.length === 0 ? (
            <div class="text-center py-8">
              <div class="text-text-muted mb-3">
                <svg class="w-10 h-10 mx-auto" viewBox="0 0 20 20" fill="currentColor" opacity="0.3">
                  <path d="M3 12v3c0 1.657 3.134 3 7 3s7-1.343 7-3v-3c0 1.657-3.134 3-7 3s-7-1.343-7-3z" />
                  <path d="M3 7v3c0 1.657 3.134 3 7 3s7-1.343 7-3V7c0 1.657-3.134 3-7 3S3 8.657 3 7z" />
                  <path d="M17 5c0 1.657-3.134 3-7 3S3 6.657 3 5s3.134-3 7-3 7 1.343 7 3z" />
                </svg>
              </div>
              <p class="text-sm text-text-muted mb-3">No services connected yet</p>
              <a href="/services" class="btn-primary text-sm inline-block">Add Service</a>
            </div>
          ) : (
            <div class="space-y-2">
              {services.map((svc) => (
                <div
                  key={svc.name}
                  class="flex items-center justify-between p-3 rounded-lg bg-surface hover:bg-surface-overlay transition-colors"
                >
                  <div class="flex items-center gap-3">
                    <div class="w-8 h-8 rounded-lg bg-brand/10 flex items-center justify-center">
                      <span class="text-xs font-bold text-brand uppercase">{svc.driver.slice(0, 2)}</span>
                    </div>
                    <div>
                      <p class="text-sm font-medium text-text-primary">{svc.name}</p>
                      <p class="text-xs text-text-muted">{svc.driver}</p>
                    </div>
                  </div>
                  <StatusBadge status={svc.active ? 'active' : 'inactive'} />
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Quick actions */}
        <div class="card">
          <h2 class="text-base font-semibold text-text-primary mb-4">Quick Actions</h2>
          <div class="space-y-2">
            {[
              { label: 'Add Database', href: '/services', icon: '+' },
              { label: 'Explore Schema', href: '/schema', icon: '#' },
              { label: 'Test API', href: '/api-explorer', icon: '>' },
              { label: 'Create API Key', href: '/api-keys', icon: '*' },
              { label: 'Manage Roles', href: '/roles', icon: '@' },
            ].map((action) => (
              <a
                key={action.label}
                href={action.href}
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-surface-overlay text-text-secondary hover:text-text-primary transition-colors group"
              >
                <span class="w-8 h-8 rounded-lg bg-surface-overlay flex items-center justify-center font-mono text-sm text-text-muted group-hover:text-brand group-hover:bg-brand/10 transition-colors">
                  {action.icon}
                </span>
                <span class="text-sm font-medium">{action.label}</span>
                <svg class="w-4 h-4 ml-auto text-text-muted group-hover:text-text-secondary transition-colors" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd" />
                </svg>
              </a>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
