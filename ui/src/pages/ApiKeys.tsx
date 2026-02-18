import { useState, useEffect } from 'preact/hooks';
import { Modal } from '../components/Modal';
import { Table, Column } from '../components/Table';
import { StatusBadge } from '../components/StatusBadge';
import { CodeBlock } from '../components/CodeBlock';
import { apiFetch } from '../hooks/useApi';

interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  role_id: string;
  role_name: string;
  is_active: boolean;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
}

export function ApiKeys() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [roles, setRoles] = useState<{ id: string; name: string }[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [showKeyModal, setShowKeyModal] = useState(false);
  const [newKey, setNewKey] = useState('');
  const [form, setForm] = useState({
    name: '',
    role_id: '',
    expires_in_days: 0,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadKeys();
    loadRoles();
  }, []);

  async function loadKeys() {
    setLoading(true);
    try {
      const res = await apiFetch('/api/v1/system/api-keys');
      setKeys(res.resource || []);
    } catch {
      setKeys([]);
    } finally {
      setLoading(false);
    }
  }

  async function loadRoles() {
    try {
      const res = await apiFetch('/api/v1/system/roles');
      setRoles((res.resource || []).map((r: any) => ({ id: r.id, name: r.name })));
    } catch {
      setRoles([]);
    }
  }

  function openCreate() {
    setForm({ name: '', role_id: roles[0]?.id || '', expires_in_days: 0 });
    setError(null);
    setShowModal(true);
  }

  async function handleCreate() {
    setSaving(true);
    setError(null);
    try {
      const body: Record<string, any> = {
        name: form.name,
        role_id: form.role_id,
      };
      if (form.expires_in_days > 0) {
        body.expires_in_days = form.expires_in_days;
      }
      const res = await apiFetch('/api/v1/system/api-keys', { method: 'POST', body });
      setShowModal(false);
      setNewKey(res.key);
      setShowKeyModal(true);
      loadKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create API key');
    } finally {
      setSaving(false);
    }
  }

  async function handleRevoke(id: string, name: string) {
    if (!confirm(`Revoke API key "${name}"? This cannot be undone.`)) return;
    try {
      await apiFetch(`/api/v1/system/api-keys/${id}`, { method: 'DELETE' });
      loadKeys();
    } catch {
      // ignore
    }
  }

  function formatDate(d: string | null): string {
    if (!d) return '--';
    return new Date(d).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  }

  function timeAgo(d: string | null): string {
    if (!d) return 'Never';
    const now = Date.now();
    const then = new Date(d).getTime();
    const diff = now - then;
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'Just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d ago`;
    return formatDate(d);
  }

  const columns: Column<ApiKey>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (k) => (
        <div>
          <span class="font-semibold text-text-primary">{k.name}</span>
          <span class="text-xs text-text-muted ml-2 font-mono">{k.prefix}...</span>
        </div>
      ),
    },
    {
      key: 'role_name',
      header: 'Role',
      render: (k) => (
        <span class="badge bg-brand/10 text-brand">{k.role_name}</span>
      ),
    },
    {
      key: 'is_active',
      header: 'Status',
      render: (k) => <StatusBadge status={k.is_active ? 'active' : 'inactive'} />,
    },
    {
      key: 'last_used_at',
      header: 'Last Used',
      render: (k) => (
        <span class="text-sm text-text-secondary">{timeAgo(k.last_used_at)}</span>
      ),
    },
    {
      key: 'expires_at',
      header: 'Expires',
      render: (k) => (
        <span class="text-sm text-text-secondary">{k.expires_at ? formatDate(k.expires_at) : 'Never'}</span>
      ),
    },
    {
      key: 'actions',
      header: '',
      width: '80px',
      render: (k) => (
        <button
          onClick={(e) => { e.stopPropagation(); handleRevoke(k.id, k.name); }}
          class="btn-danger text-xs py-1 px-3"
        >
          Revoke
        </button>
      ),
    },
  ];

  return (
    <div class="space-y-6">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">API Keys</h1>
          <p class="text-sm text-text-secondary mt-1">Create and manage API access tokens</p>
        </div>
        <button onClick={openCreate} class="btn-primary flex items-center gap-2">
          <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 3a1 1 0 011 1v5h5a1 1 0 110 2h-5v5a1 1 0 11-2 0v-5H4a1 1 0 110-2h5V4a1 1 0 011-1z" clip-rule="evenodd" />
          </svg>
          Create API Key
        </button>
      </div>

      {/* Info banner */}
      <div class="p-4 rounded-xl border border-border-subtle bg-surface-raised flex items-start gap-3">
        <svg class="w-5 h-5 text-brand shrink-0 mt-0.5" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
        </svg>
        <div class="text-sm text-text-secondary">
          API keys authenticate requests to Faucet. Include the key in the
          <code class="px-1.5 py-0.5 rounded bg-surface-overlay font-mono text-xs text-cyan-accent mx-1">X-Faucet-Api-Key</code>
          header. Keys are hashed and cannot be retrieved after creation.
        </div>
      </div>

      {loading ? (
        <div class="space-y-3">
          {[1, 2, 3].map((i) => (
            <div key={i} class="h-16 bg-surface-raised rounded-xl border border-border-subtle animate-pulse" />
          ))}
        </div>
      ) : (
        <Table
          columns={columns}
          data={keys}
          keyField="id"
          emptyMessage="No API keys created yet. Create one to authenticate API requests."
        />
      )}

      {/* Create modal */}
      <Modal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        title="Create API Key"
      >
        <div class="space-y-4">
          {error && (
            <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error">
              {error}
            </div>
          )}

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">Key Name</label>
            <input
              type="text"
              class="input w-full"
              placeholder="production-backend"
              value={form.name}
              onInput={(e) => setForm({ ...form, name: (e.target as HTMLInputElement).value })}
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">Role</label>
            <select
              class="input w-full"
              value={form.role_id}
              onChange={(e) => setForm({ ...form, role_id: (e.target as HTMLSelectElement).value })}
            >
              <option value="">Select a role...</option>
              {roles.map((r) => (
                <option key={r.id} value={r.id}>{r.name}</option>
              ))}
            </select>
          </div>

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">
              Expires In <span class="text-text-muted font-normal">(0 = never)</span>
            </label>
            <div class="flex items-center gap-2">
              <input
                type="number"
                class="input w-24 font-mono"
                min="0"
                value={form.expires_in_days}
                onInput={(e) => setForm({ ...form, expires_in_days: parseInt((e.target as HTMLInputElement).value) || 0 })}
              />
              <span class="text-sm text-text-muted">days</span>
            </div>
          </div>

          <div class="flex items-center justify-end gap-2 pt-4 border-t border-border-subtle">
            <button onClick={() => setShowModal(false)} class="btn-ghost text-sm">Cancel</button>
            <button
              onClick={handleCreate}
              disabled={saving || !form.name || !form.role_id}
              class="btn-primary text-sm"
            >
              {saving ? 'Creating...' : 'Create Key'}
            </button>
          </div>
        </div>
      </Modal>

      {/* New key display modal */}
      <Modal
        isOpen={showKeyModal}
        onClose={() => setShowKeyModal(false)}
        title="API Key Created"
      >
        <div class="space-y-4">
          <div class="p-3 rounded-lg bg-warning/10 border border-warning/20 text-sm text-warning">
            Copy this key now. It will not be shown again.
          </div>
          <CodeBlock code={newKey} />
          <div class="flex justify-end pt-2">
            <button onClick={() => setShowKeyModal(false)} class="btn-primary text-sm">
              Done
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
