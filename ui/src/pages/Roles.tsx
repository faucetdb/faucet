import { useState, useEffect } from 'preact/hooks';
import { Modal } from '../components/Modal';
import { Table, Column } from '../components/Table';
import { StatusBadge } from '../components/StatusBadge';
import { apiFetch } from '../hooks/useApi';

// Verb mask constants matching the Go backend.
const VerbGet = 1;
const VerbPost = 2;
const VerbPut = 4;
const VerbPatch = 8;
const VerbDelete = 16;

interface RoleAccess {
  service_name: string;
  component: string;
  verb_mask: number;
}

interface Role {
  id: string;
  name: string;
  description: string;
  is_active: boolean;
  access: RoleAccess[];
  created_at: string;
  updated_at: string;
}

// Form-level representation with individual verb booleans for the UI.
interface AccessForm {
  service_name: string;
  component: string;
  allow_get: boolean;
  allow_post: boolean;
  allow_put: boolean;
  allow_patch: boolean;
  allow_delete: boolean;
}

function accessToForm(a: RoleAccess): AccessForm {
  return {
    service_name: a.service_name,
    component: a.component,
    allow_get: (a.verb_mask & VerbGet) !== 0,
    allow_post: (a.verb_mask & VerbPost) !== 0,
    allow_put: (a.verb_mask & VerbPut) !== 0,
    allow_patch: (a.verb_mask & VerbPatch) !== 0,
    allow_delete: (a.verb_mask & VerbDelete) !== 0,
  };
}

function formToAccess(f: AccessForm): RoleAccess {
  let mask = 0;
  if (f.allow_get) mask |= VerbGet;
  if (f.allow_post) mask |= VerbPost;
  if (f.allow_put) mask |= VerbPut;
  if (f.allow_patch) mask |= VerbPatch;
  if (f.allow_delete) mask |= VerbDelete;
  return {
    service_name: f.service_name,
    component: f.component,
    verb_mask: mask,
  };
}

const emptyAccess: AccessForm = {
  service_name: '*',
  component: '*',
  allow_get: true,
  allow_post: false,
  allow_put: false,
  allow_patch: false,
  allow_delete: false,
};

export function Roles() {
  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState({
    name: '',
    description: '',
    is_active: true,
    access: [{ ...emptyAccess }] as AccessForm[],
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadRoles();
  }, []);

  async function loadRoles() {
    setLoading(true);
    try {
      const res = await apiFetch('/api/v1/system/role');
      setRoles(res.resource || []);
    } catch {
      setRoles([]);
    } finally {
      setLoading(false);
    }
  }

  function openNew() {
    setForm({
      name: '',
      description: '',
      is_active: true,
      access: [{ ...emptyAccess }],
    });
    setEditId(null);
    setError(null);
    setShowModal(true);
  }

  function openEdit(role: Role) {
    const accessForms = role.access && role.access.length > 0
      ? role.access.map(accessToForm)
      : [{ ...emptyAccess }];
    setForm({
      name: role.name,
      description: role.description,
      is_active: role.is_active,
      access: accessForms,
    });
    setEditId(role.id);
    setError(null);
    setShowModal(true);
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      const body = {
        name: form.name,
        description: form.description,
        is_active: form.is_active,
        access: form.access.map(formToAccess),
      };
      if (editId) {
        await apiFetch(`/api/v1/system/role/${editId}`, { method: 'PUT', body });
      } else {
        await apiFetch('/api/v1/system/role', { method: 'POST', body });
      }
      setShowModal(false);
      loadRoles();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save role');
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: string, name: string) {
    if (!confirm(`Delete role "${name}"?`)) return;
    try {
      await apiFetch(`/api/v1/system/role/${id}`, { method: 'DELETE' });
      loadRoles();
    } catch {
      // ignore
    }
  }

  function addAccess() {
    setForm({
      ...form,
      access: [...form.access, { ...emptyAccess }],
    });
  }

  function removeAccess(idx: number) {
    setForm({
      ...form,
      access: form.access.filter((_, i) => i !== idx),
    });
  }

  function updateAccess(idx: number, field: keyof AccessForm, value: any) {
    const updated = [...form.access];
    (updated[idx] as any)[field] = value;
    setForm({ ...form, access: updated });
  }

  const columns: Column<Role>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (role) => (
        <span class="font-semibold text-text-primary">{role.name}</span>
      ),
    },
    {
      key: 'description',
      header: 'Description',
      render: (role) => (
        <span class="text-text-secondary text-sm">{role.description || '--'}</span>
      ),
    },
    {
      key: 'access',
      header: 'Access Rules',
      render: (role) => (
        <span class="font-mono text-xs text-text-secondary">
          {role.access?.length || 0} rule{role.access?.length !== 1 ? 's' : ''}
        </span>
      ),
    },
    {
      key: 'is_active',
      header: 'Status',
      render: (role) => (
        <StatusBadge status={role.is_active ? 'active' : 'inactive'} />
      ),
    },
    {
      key: 'actions',
      header: '',
      width: '120px',
      render: (role) => (
        <div class="flex items-center gap-2 justify-end">
          <button
            onClick={(e) => { e.stopPropagation(); openEdit(role); }}
            class="btn-ghost text-xs py-1 px-2"
          >
            Edit
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); handleDelete(role.id, role.name); }}
            class="btn-ghost text-xs py-1 px-2 text-error"
          >
            Delete
          </button>
        </div>
      ),
    },
  ];

  return (
    <div class="space-y-6">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">Roles</h1>
          <p class="text-sm text-text-secondary mt-1">Manage RBAC roles and permissions</p>
        </div>
        <button onClick={openNew} class="btn-primary flex items-center gap-2">
          <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 3a1 1 0 011 1v5h5a1 1 0 110 2h-5v5a1 1 0 11-2 0v-5H4a1 1 0 110-2h5V4a1 1 0 011-1z" clip-rule="evenodd" />
          </svg>
          Create Role
        </button>
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
          data={roles}
          keyField="id"
          emptyMessage="No roles defined. Create a role to control API access."
          onRowClick={openEdit}
        />
      )}

      {/* Role editor modal */}
      <Modal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        title={editId ? 'Edit Role' : 'Create Role'}
        width="max-w-2xl"
      >
        <div class="space-y-6">
          {error && (
            <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error">
              {error}
            </div>
          )}

          {/* Basic info */}
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Role Name</label>
              <input
                type="text"
                class="input w-full"
                placeholder="viewer"
                value={form.name}
                onInput={(e) => setForm({ ...form, name: (e.target as HTMLInputElement).value })}
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Status</label>
              <select
                class="input w-full"
                value={form.is_active ? 'active' : 'inactive'}
                onChange={(e) => setForm({ ...form, is_active: (e.target as HTMLSelectElement).value === 'active' })}
              >
                <option value="active">Active</option>
                <option value="inactive">Inactive</option>
              </select>
            </div>
            <div class="col-span-2">
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Description</label>
              <input
                type="text"
                class="input w-full"
                placeholder="Read-only access to all resources"
                value={form.description}
                onInput={(e) => setForm({ ...form, description: (e.target as HTMLInputElement).value })}
              />
            </div>
          </div>

          {/* Access Rules */}
          <div>
            <div class="flex items-center justify-between mb-3">
              <label class="text-sm font-medium text-text-secondary">Access Rules</label>
              <button onClick={addAccess} class="btn-ghost text-xs py-1">
                + Add Rule
              </button>
            </div>

            <div class="space-y-3">
              {form.access.map((rule, idx) => (
                <div key={idx} class="p-4 rounded-lg bg-surface border border-border-subtle">
                  <div class="flex items-center justify-between mb-3">
                    <span class="text-xs font-medium text-text-muted">Rule {idx + 1}</span>
                    {form.access.length > 1 && (
                      <button
                        onClick={() => removeAccess(idx)}
                        class="text-xs text-error hover:text-error/80"
                      >
                        Remove
                      </button>
                    )}
                  </div>

                  <div class="grid grid-cols-2 gap-3 mb-3">
                    <div>
                      <label class="block text-xs text-text-muted mb-1">Service</label>
                      <input
                        type="text"
                        class="input w-full text-sm font-mono"
                        placeholder="* (all)"
                        value={rule.service_name}
                        onInput={(e) => updateAccess(idx, 'service_name', (e.target as HTMLInputElement).value)}
                      />
                    </div>
                    <div>
                      <label class="block text-xs text-text-muted mb-1">Component</label>
                      <input
                        type="text"
                        class="input w-full text-sm font-mono"
                        placeholder="* (all)"
                        value={rule.component}
                        onInput={(e) => updateAccess(idx, 'component', (e.target as HTMLInputElement).value)}
                      />
                    </div>
                  </div>

                  <div class="flex flex-wrap gap-3">
                    {(['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const).map((m) => {
                      const key = `allow_${m.toLowerCase()}` as keyof AccessForm;
                      return (
                        <label key={m} class="flex items-center gap-1.5 text-xs cursor-pointer">
                          <input
                            type="checkbox"
                            checked={rule[key] as boolean}
                            onChange={(e) => updateAccess(idx, key, (e.target as HTMLInputElement).checked)}
                            class="rounded border-border-default bg-surface text-brand focus:ring-brand/50"
                          />
                          <span class="text-text-secondary font-mono">{m}</span>
                        </label>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Actions */}
          <div class="flex items-center justify-end gap-2 pt-4 border-t border-border-subtle">
            <button onClick={() => setShowModal(false)} class="btn-ghost text-sm">Cancel</button>
            <button
              onClick={handleSave}
              disabled={saving || !form.name}
              class="btn-primary text-sm"
            >
              {saving ? 'Saving...' : editId ? 'Update Role' : 'Create Role'}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
