import { useState, useEffect } from 'preact/hooks';
import { apiFetch } from '../hooks/useApi';

interface TableInfo {
  name: string;
  type?: string;
}

interface ColumnInfo {
  name: string;
  db_type: string;
  nullable: boolean;
  default?: string | null;
  is_primary_key: boolean;
  is_foreign_key?: boolean;
  references?: {
    table: string;
    column: string;
  };
}

interface DriftItem {
  type: 'additive' | 'breaking';
  category: string;
  table_name: string;
  column_name?: string;
  old_value?: string;
  new_value?: string;
  description: string;
}

interface TableDrift {
  table_name: string;
  has_drift: boolean;
  has_breaking: boolean;
  items: DriftItem[];
}

interface ServiceDriftReport {
  service_name: string;
  tables: TableDrift[];
  has_drift: boolean;
  has_breaking: boolean;
  total_additive: number;
  total_breaking: number;
}

export function SchemaExplorer() {
  const [services, setServices] = useState<{ name: string; driver: string; schema_lock?: string }[]>([]);
  const [selectedService, setSelectedService] = useState('');
  const [tables, setTables] = useState<TableInfo[]>([]);
  const [selectedTable, setSelectedTable] = useState('');
  const [columns, setColumns] = useState<ColumnInfo[]>([]);
  const [loadingTables, setLoadingTables] = useState(false);
  const [loadingColumns, setLoadingColumns] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  // Contract locking state
  const [lockMode, setLockMode] = useState<string>('none');
  const [lockedTables, setLockedTables] = useState<Set<string>>(new Set());
  const [driftReport, setDriftReport] = useState<ServiceDriftReport | null>(null);
  const [lockLoading, setLockLoading] = useState<string | null>(null); // table name being locked/unlocked
  const [promoteLoading, setPromoteLoading] = useState(false);
  const [modeLoading, setModeLoading] = useState(false);

  useEffect(() => {
    apiFetch('/api/v1/system/service')
      .then((res) => {
        const svcs = (res.resource || []).map((s: any) => ({
          name: s.name,
          driver: s.driver,
          schema_lock: s.schema_lock || 'none',
        }));
        setServices(svcs);
        const params = new URLSearchParams(window.location.search);
        const svcParam = params.get('service');
        if (svcParam && svcs.find((s: any) => s.name === svcParam)) {
          setSelectedService(svcParam);
        } else if (svcs.length > 0) {
          setSelectedService(svcs[0].name);
        }
      })
      .catch(() => setServices([]));
  }, []);

  // Load tables when service changes
  useEffect(() => {
    if (!selectedService) return;
    setLoadingTables(true);
    setSelectedTable('');
    setColumns([]);
    apiFetch(`/api/v1/${selectedService}/_schema`)
      .then((res) => {
        const tableData = res.resource || res.tables || [];
        const parsed: TableInfo[] = tableData.map((t: any) =>
          typeof t === 'string' ? { name: t } : { name: t.name, type: t.type }
        );
        setTables(parsed);
      })
      .catch(() => setTables([]))
      .finally(() => setLoadingTables(false));
  }, [selectedService]);

  // Load contracts + drift when service changes
  useEffect(() => {
    if (!selectedService) return;
    const svc = services.find((s) => s.name === selectedService);
    setLockMode(svc?.schema_lock || 'none');
    loadContracts();
  }, [selectedService, services]);

  function loadContracts() {
    if (!selectedService) return;
    // Load locked tables
    apiFetch(`/api/v1/system/contract/${selectedService}`)
      .then((res) => {
        const contracts = res.contracts || [];
        setLockedTables(new Set(contracts.map((c: any) => c.table_name)));
      })
      .catch(() => setLockedTables(new Set()));

    // Load drift report
    apiFetch(`/api/v1/system/contract/${selectedService}/diff`)
      .then((res) => setDriftReport(res))
      .catch(() => setDriftReport(null));
  }

  // Load columns when table changes
  useEffect(() => {
    if (!selectedService || !selectedTable) return;
    setLoadingColumns(true);
    apiFetch(`/api/v1/${selectedService}/_schema/${selectedTable}`)
      .then((res) => {
        const cols = res.resource || res.columns || res.fields || [];
        setColumns(cols);
      })
      .catch(() => setColumns([]))
      .finally(() => setLoadingColumns(false));
  }, [selectedService, selectedTable]);

  const filteredTables = tables.filter((t) =>
    t.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  function getDriftForTable(tableName: string): TableDrift | undefined {
    return driftReport?.tables?.find((t) => t.table_name === tableName);
  }

  async function handleLockToggle(tableName: string) {
    setLockLoading(tableName);
    try {
      if (lockedTables.has(tableName)) {
        await apiFetch(`/api/v1/system/contract/${selectedService}/${tableName}`, { method: 'DELETE' });
      } else {
        await apiFetch(`/api/v1/system/contract/${selectedService}/${tableName}`, { method: 'POST' });
      }
      loadContracts();
    } catch {
      // ignore
    } finally {
      setLockLoading(null);
    }
  }

  async function handlePromote(tableName: string) {
    setPromoteLoading(true);
    try {
      await apiFetch(`/api/v1/system/contract/${selectedService}/${tableName}/promote`, { method: 'POST' });
      loadContracts();
    } catch {
      // ignore
    } finally {
      setPromoteLoading(false);
    }
  }

  async function handleModeChange(mode: string) {
    setModeLoading(true);
    try {
      await apiFetch(`/api/v1/system/contract/${selectedService}/mode`, {
        method: 'PUT',
        body: { mode },
      });
      setLockMode(mode);
      // Update services state to reflect the change
      setServices((prev) =>
        prev.map((s) => (s.name === selectedService ? { ...s, schema_lock: mode } : s))
      );
      loadContracts();
    } catch {
      // ignore
    } finally {
      setModeLoading(false);
    }
  }

  function driftBadge(tableName: string) {
    const isLocked = lockedTables.has(tableName);
    if (!isLocked) return null;

    const drift = getDriftForTable(tableName);
    if (!drift || !drift.has_drift) {
      return (
        <span class="w-2 h-2 rounded-full bg-success shrink-0" title="Locked - No drift" />
      );
    }
    if (drift.has_breaking) {
      return (
        <span class="w-2 h-2 rounded-full bg-error shrink-0 animate-pulse" title="Breaking drift detected" />
      );
    }
    return (
      <span class="w-2 h-2 rounded-full bg-warning shrink-0" title="Additive drift detected" />
    );
  }

  function typeColor(type: string | undefined): string {
    if (!type) return 'text-text-secondary';
    const t = type.toLowerCase();
    if (t.includes('int') || t.includes('serial') || t.includes('numeric') || t.includes('decimal') || t.includes('float') || t.includes('double')) return 'text-cyan-accent';
    if (t.includes('varchar') || t.includes('text') || t.includes('char')) return 'text-success';
    if (t.includes('bool')) return 'text-warning';
    if (t.includes('timestamp') || t.includes('date') || t.includes('time')) return 'text-purple-400';
    if (t.includes('json') || t.includes('jsonb')) return 'text-orange-400';
    if (t.includes('uuid')) return 'text-pink-400';
    return 'text-text-secondary';
  }

  const selectedTableDrift = selectedTable ? getDriftForTable(selectedTable) : undefined;
  const isSelectedLocked = selectedTable ? lockedTables.has(selectedTable) : false;

  return (
    <div class="space-y-6">
      {/* Header */}
      <div>
        <h1 class="text-2xl font-semibold text-text-primary">Schema Explorer</h1>
        <p class="text-sm text-text-secondary mt-1">Browse tables, columns, and relationships</p>
      </div>

      {/* Service selector + lock mode */}
      <div class="flex items-center gap-4 flex-wrap">
        <select
          class="input w-64"
          value={selectedService}
          onChange={(e) => setSelectedService((e.target as HTMLSelectElement).value)}
        >
          <option value="">Select a service...</option>
          {services.map((s) => (
            <option key={s.name} value={s.name}>
              {s.name} ({s.driver})
            </option>
          ))}
        </select>

        {selectedService && (
          <div class="flex items-center gap-2">
            <span class="text-xs text-text-muted">Schema Lock:</span>
            <select
              class="input text-xs py-1.5 w-28"
              value={lockMode}
              disabled={modeLoading}
              onChange={(e) => handleModeChange((e.target as HTMLSelectElement).value)}
            >
              <option value="none">None</option>
              <option value="auto">Auto</option>
              <option value="strict">Strict</option>
            </select>
            {lockMode !== 'none' && (
              <span class={`text-xs px-2 py-0.5 rounded-full ${
                lockMode === 'strict'
                  ? 'bg-error/10 text-error'
                  : 'bg-warning/10 text-warning'
              }`}>
                {lockMode === 'strict' ? 'All changes blocked' : 'Breaking changes blocked'}
              </span>
            )}
          </div>
        )}

        {/* Drift summary */}
        {driftReport && driftReport.has_drift && (
          <div class={`text-xs px-2 py-0.5 rounded-full ${
            driftReport.has_breaking ? 'bg-error/10 text-error' : 'bg-warning/10 text-warning'
          }`}>
            {driftReport.total_breaking > 0 && `${driftReport.total_breaking} breaking`}
            {driftReport.total_breaking > 0 && driftReport.total_additive > 0 && ', '}
            {driftReport.total_additive > 0 && `${driftReport.total_additive} additive`}
            {' '}change{(driftReport.total_breaking + driftReport.total_additive) !== 1 ? 's' : ''}
          </div>
        )}
      </div>

      {/* Two-panel layout */}
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 min-h-[60vh]">
        {/* Tables list */}
        <div class="lg:col-span-4 xl:col-span-3">
          <div class="card p-0 h-full flex flex-col">
            {/* Search */}
            <div class="p-3 border-b border-border-subtle">
              <input
                type="text"
                class="input w-full text-sm"
                placeholder="Filter tables..."
                value={searchQuery}
                onInput={(e) => setSearchQuery((e.target as HTMLInputElement).value)}
              />
            </div>

            {/* Table list */}
            <div class="flex-1 overflow-y-auto">
              {loadingTables ? (
                <div class="p-4 space-y-2">
                  {[1, 2, 3, 4, 5].map((i) => (
                    <div key={i} class="h-8 bg-surface-overlay rounded animate-pulse" />
                  ))}
                </div>
              ) : filteredTables.length === 0 ? (
                <div class="p-4 text-center text-sm text-text-muted">
                  {selectedService ? 'No tables found' : 'Select a service'}
                </div>
              ) : (
                <div class="p-1.5">
                  {filteredTables.map((table) => (
                    <button
                      key={table.name}
                      onClick={() => setSelectedTable(table.name)}
                      class={`
                        w-full text-left px-3 py-2 rounded-lg text-sm flex items-center justify-between
                        transition-colors duration-100
                        ${selectedTable === table.name
                          ? 'bg-brand/10 text-brand'
                          : 'text-text-secondary hover:bg-surface-overlay hover:text-text-primary'
                        }
                      `}
                    >
                      <span class="flex items-center gap-2 min-w-0">
                        <svg class="w-4 h-4 shrink-0 text-text-muted" viewBox="0 0 20 20" fill="currentColor">
                          <path fill-rule="evenodd" d="M5 4a3 3 0 00-3 3v6a3 3 0 003 3h10a3 3 0 003-3V7a3 3 0 00-3-3H5zm-1 9v-1h5v2H5a1 1 0 01-1-1zm7 1h4a1 1 0 001-1v-1h-5v2zm0-4h5V8h-5v2zM9 8H4v2h5V8z" clip-rule="evenodd" />
                        </svg>
                        <span class="truncate font-mono text-xs">{table.name}</span>
                      </span>
                      <span class="flex items-center gap-1.5">
                        {lockedTables.has(table.name) && (
                          <svg class="w-3 h-3 shrink-0 text-text-muted" viewBox="0 0 20 20" fill="currentColor" title="Locked">
                            <path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd" />
                          </svg>
                        )}
                        {driftBadge(table.name)}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Table count */}
            {tables.length > 0 && (
              <div class="px-4 py-2 border-t border-border-subtle text-xs text-text-muted flex items-center justify-between">
                <span>{filteredTables.length} of {tables.length} tables</span>
                {lockedTables.size > 0 && (
                  <span>{lockedTables.size} locked</span>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Column details */}
        <div class="lg:col-span-8 xl:col-span-9">
          <div class="card p-0 h-full">
            {selectedTable ? (
              <>
                {/* Table header */}
                <div class="px-6 py-4 border-b border-border-subtle flex items-center justify-between">
                  <div>
                    <h2 class="text-base font-semibold text-text-primary font-mono flex items-center gap-2">
                      {selectedTable}
                      {isSelectedLocked && (
                        <svg class="w-4 h-4 text-text-muted" viewBox="0 0 20 20" fill="currentColor" title="Locked">
                          <path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd" />
                        </svg>
                      )}
                    </h2>
                    <p class="text-xs text-text-muted mt-0.5">
                      {columns.length} columns
                      {columns.some((c) => c.is_primary_key) && (
                        <span> &middot; PK: {columns.filter((c) => c.is_primary_key).map((c) => c.name).join(', ')}</span>
                      )}
                    </p>
                  </div>
                  <div class="flex items-center gap-2">
                    {/* Lock/Unlock button */}
                    <button
                      onClick={() => handleLockToggle(selectedTable)}
                      disabled={lockLoading === selectedTable}
                      class={`text-xs py-1.5 px-3 rounded-lg transition-colors ${
                        isSelectedLocked
                          ? 'bg-surface-overlay text-text-secondary hover:bg-error/10 hover:text-error'
                          : 'bg-surface-overlay text-text-secondary hover:bg-brand/10 hover:text-brand'
                      }`}
                    >
                      {lockLoading === selectedTable ? 'Loading...' : isSelectedLocked ? 'Unlock' : 'Lock'}
                    </button>

                    {/* Promote button (only when locked and has drift) */}
                    {isSelectedLocked && selectedTableDrift?.has_drift && (
                      <button
                        onClick={() => handlePromote(selectedTable)}
                        disabled={promoteLoading}
                        class="text-xs py-1.5 px-3 rounded-lg bg-brand/10 text-brand hover:bg-brand/20 transition-colors"
                      >
                        {promoteLoading ? 'Promoting...' : 'Promote'}
                      </button>
                    )}

                    <a
                      href={`/api-explorer?service=${selectedService}&table=${selectedTable}`}
                      class="btn-secondary text-xs py-1.5"
                    >
                      Query &rarr;
                    </a>
                  </div>
                </div>

                {/* Drift alert */}
                {isSelectedLocked && selectedTableDrift?.has_drift && (
                  <div class={`px-6 py-3 border-b border-border-subtle ${
                    selectedTableDrift.has_breaking ? 'bg-error/5' : 'bg-warning/5'
                  }`}>
                    <div class="flex items-center gap-2 mb-2">
                      <span class={`w-2 h-2 rounded-full ${
                        selectedTableDrift.has_breaking ? 'bg-error' : 'bg-warning'
                      }`} />
                      <span class={`text-xs font-medium ${
                        selectedTableDrift.has_breaking ? 'text-error' : 'text-warning'
                      }`}>
                        Schema Drift Detected
                      </span>
                      <span class="text-xs text-text-muted">
                        The live database schema differs from the locked API contract.
                      </span>
                    </div>
                    <div class="space-y-1">
                      {selectedTableDrift.items?.map((item, i) => (
                        <div key={i} class="flex items-center gap-2 text-xs">
                          <span class={`px-1.5 py-0.5 rounded text-[10px] font-medium uppercase ${
                            item.type === 'breaking'
                              ? 'bg-error/10 text-error'
                              : 'bg-warning/10 text-warning'
                          }`}>
                            {item.type}
                          </span>
                          <span class="text-text-secondary">{item.description}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Columns */}
                {loadingColumns ? (
                  <div class="p-6 space-y-3">
                    {[1, 2, 3, 4].map((i) => (
                      <div key={i} class="h-12 bg-surface-overlay rounded animate-pulse" />
                    ))}
                  </div>
                ) : (
                  <div class="overflow-x-auto">
                    <table class="w-full text-sm">
                      <thead>
                        <tr class="border-b border-border-subtle">
                          <th class="text-left px-6 py-3 font-medium text-text-secondary">Column</th>
                          <th class="text-left px-6 py-3 font-medium text-text-secondary">Type</th>
                          <th class="text-left px-6 py-3 font-medium text-text-secondary">Nullable</th>
                          <th class="text-left px-6 py-3 font-medium text-text-secondary">Default</th>
                          <th class="text-left px-6 py-3 font-medium text-text-secondary">Constraints</th>
                        </tr>
                      </thead>
                      <tbody>
                        {columns.map((col) => (
                          <tr key={col.name} class="border-b border-border-subtle last:border-0 hover:bg-surface-overlay/50 transition-colors">
                            <td class="px-6 py-3">
                              <span class="font-mono text-sm text-text-primary flex items-center gap-2">
                                {col.is_primary_key && (
                                  <svg class="w-3.5 h-3.5 text-warning shrink-0" viewBox="0 0 20 20" fill="currentColor" title="Primary Key">
                                    <path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd" />
                                  </svg>
                                )}
                                {col.is_foreign_key && (
                                  <svg class="w-3.5 h-3.5 text-brand shrink-0" viewBox="0 0 20 20" fill="currentColor" title="Foreign Key">
                                    <path fill-rule="evenodd" d="M12.586 4.586a2 2 0 112.828 2.828l-3 3a2 2 0 01-2.828 0 1 1 0 00-1.414 1.414 4 4 0 005.656 0l3-3a4 4 0 00-5.656-5.656l-1.5 1.5a1 1 0 101.414 1.414l1.5-1.5zm-5 5a2 2 0 012.828 0 1 1 0 101.414-1.414 4 4 0 00-5.656 0l-3 3a4 4 0 105.656 5.656l1.5-1.5a1 1 0 10-1.414-1.414l-1.5 1.5a2 2 0 11-2.828-2.828l3-3z" clip-rule="evenodd" />
                                  </svg>
                                )}
                                {col.name}
                              </span>
                            </td>
                            <td class="px-6 py-3">
                              <span class={`font-mono text-xs ${typeColor(col.db_type)}`}>
                                {col.db_type}
                              </span>
                            </td>
                            <td class="px-6 py-3">
                              <span class={`text-xs ${col.nullable ? 'text-text-muted' : 'text-warning'}`}>
                                {col.nullable ? 'YES' : 'NOT NULL'}
                              </span>
                            </td>
                            <td class="px-6 py-3">
                              <span class="font-mono text-xs text-text-muted">
                                {col.default || '--'}
                              </span>
                            </td>
                            <td class="px-6 py-3">
                              <div class="flex items-center gap-1.5">
                                {col.is_primary_key && (
                                  <span class="badge bg-warning/10 text-warning">PK</span>
                                )}
                                {col.is_foreign_key && col.references && (
                                  <button
                                    onClick={() => setSelectedTable(col.references!.table)}
                                    class="badge bg-brand/10 text-brand hover:bg-brand/20 transition-colors cursor-pointer"
                                  >
                                    FK &rarr; {col.references.table}.{col.references.column}
                                  </button>
                                )}
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            ) : (
              <div class="flex items-center justify-center h-full min-h-[300px] text-text-muted text-sm">
                <div class="text-center">
                  <svg class="w-10 h-10 mx-auto mb-3" viewBox="0 0 20 20" fill="currentColor" opacity="0.3">
                    <path fill-rule="evenodd" d="M5 4a3 3 0 00-3 3v6a3 3 0 003 3h10a3 3 0 003-3V7a3 3 0 00-3-3H5zm-1 9v-1h5v2H5a1 1 0 01-1-1zm7 1h4a1 1 0 001-1v-1h-5v2zm0-4h5V8h-5v2zM9 8H4v2h5V8z" clip-rule="evenodd" />
                  </svg>
                  Select a table to view its schema
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
