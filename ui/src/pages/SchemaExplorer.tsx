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

export function SchemaExplorer() {
  const [services, setServices] = useState<{ name: string; driver: string }[]>([]);
  const [selectedService, setSelectedService] = useState('');
  const [tables, setTables] = useState<TableInfo[]>([]);
  const [selectedTable, setSelectedTable] = useState('');
  const [columns, setColumns] = useState<ColumnInfo[]>([]);
  const [loadingTables, setLoadingTables] = useState(false);
  const [loadingColumns, setLoadingColumns] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  useEffect(() => {
    apiFetch('/api/v1/system/service')
      .then((res) => {
        const svcs = (res.resource || []).map((s: any) => ({ name: s.name, driver: s.driver }));
        setServices(svcs);
        // Check URL params for pre-selected service
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

  useEffect(() => {
    if (!selectedService) return;
    setLoadingTables(true);
    setSelectedTable('');
    setColumns([]);
    apiFetch(`/api/v1/${selectedService}/_schema`)
      .then((res) => {
        // The schema endpoint returns table info - could be resource array or table_names
        const tableData = res.resource || res.tables || [];
        const parsed: TableInfo[] = tableData.map((t: any) =>
          typeof t === 'string' ? { name: t } : { name: t.name, type: t.type }
        );
        setTables(parsed);
      })
      .catch(() => setTables([]))
      .finally(() => setLoadingTables(false));
  }, [selectedService]);

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

  return (
    <div class="space-y-6">
      {/* Header */}
      <div>
        <h1 class="text-2xl font-semibold text-text-primary">Schema Explorer</h1>
        <p class="text-sm text-text-secondary mt-1">Browse tables, columns, and relationships</p>
      </div>

      {/* Service selector */}
      <div class="flex items-center gap-4">
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
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Table count */}
            {tables.length > 0 && (
              <div class="px-4 py-2 border-t border-border-subtle text-xs text-text-muted">
                {filteredTables.length} of {tables.length} tables
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
                    <h2 class="text-base font-semibold text-text-primary font-mono">{selectedTable}</h2>
                    <p class="text-xs text-text-muted mt-0.5">
                      {columns.length} columns
                      {columns.some((c) => c.is_primary_key) && (
                        <span> &middot; PK: {columns.filter((c) => c.is_primary_key).map((c) => c.name).join(', ')}</span>
                      )}
                    </p>
                  </div>
                  <div class="flex items-center gap-2">
                    <a
                      href={`/api-explorer?service=${selectedService}&table=${selectedTable}`}
                      class="btn-secondary text-xs py-1.5"
                    >
                      Query &rarr;
                    </a>
                  </div>
                </div>

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
