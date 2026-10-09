import { useState } from 'preact/hooks';
import { CopyButton } from './ui';

export interface CodeSample {
  label: string;
  code: string;
  /** Optional note shown above the code (e.g. where a config file lives). */
  note?: preact.ComponentChildren;
}

/**
 * Copyable code. Pass `code` for one snippet, or `samples` for a tabbed set
 * (curl / JavaScript / Python, or one tab per MCP client).
 */
export function CodeBlock({ code, samples, title, wrap = false }: { code?: string; samples?: CodeSample[]; title?: string; wrap?: boolean }) {
  const list = samples || [{ label: title || '', code: code || '' }];
  const [active, setActive] = useState(0);
  const current = list[Math.min(active, list.length - 1)];
  const tabbed = list.length > 1;

  return (
    <div class="rounded-[8px] border border-line bg-panel-2 overflow-hidden">
      {(tabbed || title) && (
        <div class="flex items-center justify-between gap-2 pl-1.5 pr-1.5 border-b border-line">
          {tabbed ? (
            <div role="tablist" class="flex gap-0.5 overflow-x-auto">
              {list.map((s, i) => (
                <button
                  key={s.label}
                  type="button"
                  role="tab"
                  aria-selected={i === active}
                  onClick={() => setActive(i)}
                  class={`relative h-9 px-2.5 text-sm font-medium whitespace-nowrap transition-colors ${
                    i === active ? 'text-fg' : 'text-fg-muted hover:text-fg'
                  }`}
                >
                  {s.label}
                  {i === active && <span class="absolute left-1.5 right-1.5 -bottom-px h-0.5 rounded-full bg-brand" />}
                </button>
              ))}
            </div>
          ) : (
            <span class="pl-2 text-sm text-fg-muted">{title}</span>
          )}
          <CopyButton text={current.code} />
        </div>
      )}
      {current.note && <div class="px-4 pt-3 text-sm text-fg-muted">{current.note}</div>}
      <div class="relative group">
        <pre class={`px-4 py-3 text-[12.5px] leading-[20px] font-mono text-fg overflow-x-auto ${wrap ? 'whitespace-pre-wrap break-all' : ''}`}>
          <code>{current.code}</code>
        </pre>
        {!tabbed && !title && (
          <div class="absolute top-1.5 right-1.5 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
            <CopyButton text={current.code} variant="secondary" />
          </div>
        )}
      </div>
    </div>
  );
}
