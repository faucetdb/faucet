import { JSX } from 'preact';

export interface Column<T> {
  key: string;
  header: string;
  width?: string;
  render?: (item: T) => JSX.Element | string;
}

interface TableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyField: string;
  emptyMessage?: string;
  onRowClick?: (item: T) => void;
}

export function Table<T extends Record<string, any>>({
  columns,
  data,
  keyField,
  emptyMessage = 'No data to display',
  onRowClick,
}: TableProps<T>) {
  if (data.length === 0) {
    return (
      <div class="card text-center py-12">
        <div class="text-text-muted text-sm">{emptyMessage}</div>
      </div>
    );
  }

  return (
    <div class="overflow-x-auto rounded-xl border border-border-subtle">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-border-subtle bg-surface-raised">
            {columns.map((col) => (
              <th
                key={col.key}
                class="text-left px-4 py-3 font-medium text-text-secondary"
                style={col.width ? { width: col.width } : undefined}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((item) => (
            <tr
              key={item[keyField]}
              class={`
                border-b border-border-subtle last:border-0
                bg-surface-raised
                ${onRowClick ? 'cursor-pointer hover:bg-surface-overlay' : ''}
                transition-colors duration-100
              `}
              onClick={() => onRowClick?.(item)}
            >
              {columns.map((col) => (
                <td key={col.key} class="px-4 py-3 text-text-primary">
                  {col.render ? col.render(item) : String(item[col.key] ?? '')}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
