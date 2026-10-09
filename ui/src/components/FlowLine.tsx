import { DbLogo } from './DbLogo';

export type FlowState = 'idle' | 'testing' | 'live' | 'error';

/** A pipe segment between two nodes. Dashes travel when water is flowing. */
function Pipe({ state }: { state: FlowState }) {
  const color =
    state === 'live' ? 'var(--live)' : state === 'error' ? 'var(--bad)' : state === 'testing' ? 'var(--brand)' : 'var(--line-strong)';
  return (
    <div class="flex-1 min-w-6 h-6 self-center" aria-hidden="true">
      <svg width="100%" height="24" preserveAspectRatio="none" viewBox="0 0 100 24">
        <line x1="0" y1="12" x2="100" y2="12" stroke="var(--line)" stroke-width="6" stroke-linecap="round" vector-effect="non-scaling-stroke" />
        <line
          x1="0" y1="12" x2="100" y2="12"
          stroke={color}
          stroke-width="2"
          stroke-dasharray={state === 'error' ? '3 9' : '8 4'}
          vector-effect="non-scaling-stroke"
          class={state === 'live' || state === 'testing' ? 'flow-on' : ''}
          style={state === 'testing' ? { animationDuration: '1.6s' } : undefined}
        />
      </svg>
    </div>
  );
}

/**
 * The signature element: database → Faucet → REST + MCP. It shows what is
 * about to happen while a connection is being set up, and "turns on" when the
 * connection test succeeds.
 */
export function FlowLine({ engine, source, apiName, state }: { engine: string; source: string; apiName: string; state: FlowState }) {
  const name = apiName || 'your-db';
  const statusText = {
    idle: 'Not tested yet',
    testing: 'Testing connection',
    live: 'Connected',
    error: 'Connection failed',
  }[state];

  return (
    <figure class="rounded-[10px] border border-line bg-panel-2/60 px-4 py-3.5" aria-label={`${source || 'Database'} to ${name} API. ${statusText}.`}>
      <div class="flex items-stretch gap-2">
        <div class="flex items-center gap-2.5 min-w-0 max-w-[38%]">
          <DbLogo engine={engine} size={30} />
          <div class="min-w-0">
            <p class="text-xs text-fg-muted">Database</p>
            <p class="text-sm font-mono text-fg truncate" title={source}>{source || 'not set'}</p>
          </div>
        </div>
        <Pipe state={state} />
        <div class="flex items-center self-center">
          <span class={`inline-flex items-center justify-center w-8 h-8 rounded-full border ${state === 'live' ? 'border-live/50 bg-live/10' : 'border-line-strong bg-panel'}`}>
            <img src="/faucet-icon.svg" alt="Faucet" width="16" height="16" />
          </span>
        </div>
        <Pipe state={state} />
        <div class="min-w-0 max-w-[38%] self-center">
          <p class="text-sm font-mono text-fg truncate">/api/v1/{name}</p>
          <p class="text-xs text-fg-muted">REST API and MCP tools</p>
        </div>
      </div>
      <figcaption class={`mt-2.5 text-xs ${state === 'live' ? 'text-live' : state === 'error' ? 'text-bad' : 'text-fg-faint'}`} aria-live="polite">
        {statusText}
      </figcaption>
    </figure>
  );
}
