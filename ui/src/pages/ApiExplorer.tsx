import { useState, useEffect } from 'preact/hooks';
import { JsonView } from '../components/JsonView';
import { CodeBlock } from '../components/CodeBlock';

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const;
type Method = typeof HTTP_METHODS[number];

const METHOD_COLORS: Record<Method, string> = {
  GET: 'bg-success/10 text-success border-success/20',
  POST: 'bg-brand/10 text-brand border-brand/20',
  PUT: 'bg-warning/10 text-warning border-warning/20',
  PATCH: 'bg-orange-500/10 text-orange-400 border-orange-500/20',
  DELETE: 'bg-error/10 text-error border-error/20',
};

interface HeaderEntry {
  key: string;
  value: string;
  enabled: boolean;
}

export function ApiExplorer() {
  const [method, setMethod] = useState<Method>('GET');
  const [url, setUrl] = useState('/api/v1/');
  const [headers, setHeaders] = useState<HeaderEntry[]>([
    { key: 'Content-Type', value: 'application/json', enabled: true },
    { key: 'X-Faucet-Api-Key', value: '', enabled: false },
  ]);
  const [body, setBody] = useState('');
  const [response, setResponse] = useState<{
    status: number;
    statusText: string;
    headers: Record<string, string>;
    body: unknown;
    time: number;
    size: string;
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState<'body' | 'headers' | 'curl'>('body');
  const [responseTab, setResponseTab] = useState<'body' | 'headers'>('body');

  // Pre-fill from URL params
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const service = params.get('service');
    const table = params.get('table');
    if (service && table) {
      setUrl(`/api/v1/${service}/_table/${table}`);
    } else if (service) {
      setUrl(`/api/v1/${service}/_table/`);
    }
  }, []);

  async function sendRequest() {
    setLoading(true);
    setResponse(null);
    const start = performance.now();

    try {
      const reqHeaders: Record<string, string> = {};
      headers.forEach((h) => {
        if (h.enabled && h.key && h.value) {
          reqHeaders[h.key] = h.value;
        }
      });

      // Add auth from localStorage if available
      const apiKey = localStorage.getItem('faucet_api_key');
      if (apiKey && !reqHeaders['X-Faucet-Api-Key']) {
        reqHeaders['X-Faucet-Api-Key'] = apiKey;
      }
      const session = localStorage.getItem('faucet_session');
      if (session && !reqHeaders['Authorization']) {
        reqHeaders['Authorization'] = `Bearer ${session}`;
      }

      const fetchOpts: RequestInit = {
        method,
        headers: reqHeaders,
      };
      if (['POST', 'PUT', 'PATCH'].includes(method) && body) {
        fetchOpts.body = body;
      }

      const res = await fetch(url, fetchOpts);
      const elapsed = Math.round(performance.now() - start);

      let resBody: unknown;
      const contentType = res.headers.get('content-type') || '';
      if (contentType.includes('json')) {
        resBody = await res.json();
      } else {
        resBody = await res.text();
      }

      const resHeaders: Record<string, string> = {};
      res.headers.forEach((v, k) => {
        resHeaders[k] = v;
      });

      const bodyStr = typeof resBody === 'string' ? resBody : JSON.stringify(resBody);
      const sizeBytes = new Blob([bodyStr]).size;
      const size = sizeBytes > 1024
        ? `${(sizeBytes / 1024).toFixed(1)} KB`
        : `${sizeBytes} B`;

      setResponse({
        status: res.status,
        statusText: res.statusText,
        headers: resHeaders,
        body: resBody,
        time: elapsed,
        size,
      });
    } catch (err) {
      const elapsed = Math.round(performance.now() - start);
      setResponse({
        status: 0,
        statusText: 'Network Error',
        headers: {},
        body: { error: err instanceof Error ? err.message : 'Request failed' },
        time: elapsed,
        size: '0 B',
      });
    } finally {
      setLoading(false);
    }
  }

  function generateCurl(): string {
    const parts = ['curl'];
    if (method !== 'GET') parts.push(`-X ${method}`);
    headers.forEach((h) => {
      if (h.enabled && h.key && h.value) {
        parts.push(`-H '${h.key}: ${h.value}'`);
      }
    });
    if (['POST', 'PUT', 'PATCH'].includes(method) && body) {
      parts.push(`-d '${body}'`);
    }
    parts.push(`'${window.location.origin}${url}'`);
    return parts.join(' \\\n  ');
  }

  function addHeader() {
    setHeaders([...headers, { key: '', value: '', enabled: true }]);
  }

  function removeHeader(idx: number) {
    setHeaders(headers.filter((_, i) => i !== idx));
  }

  function updateHeader(idx: number, field: keyof HeaderEntry, value: string | boolean) {
    const updated = [...headers];
    (updated[idx] as any)[field] = value;
    setHeaders(updated);
  }

  function statusColor(status: number): string {
    if (status >= 200 && status < 300) return 'text-success';
    if (status >= 300 && status < 400) return 'text-brand';
    if (status >= 400 && status < 500) return 'text-warning';
    return 'text-error';
  }

  return (
    <div class="space-y-6">
      {/* Header */}
      <div>
        <h1 class="text-2xl font-semibold text-text-primary">API Explorer</h1>
        <p class="text-sm text-text-secondary mt-1">Test API endpoints interactively</p>
      </div>

      {/* Request bar */}
      <div class="card p-0">
        <div class="flex items-center gap-0 border-b border-border-subtle">
          {/* Method selector */}
          <select
            class="h-12 px-4 bg-transparent border-r border-border-subtle text-sm font-semibold focus:outline-none cursor-pointer"
            value={method}
            onChange={(e) => setMethod((e.target as HTMLSelectElement).value as Method)}
            style={{ color: method === 'GET' ? '#00c853' : method === 'POST' ? '#0066FF' : method === 'DELETE' ? '#ff1744' : method === 'PUT' ? '#ffab00' : '#fb923c' }}
          >
            {HTTP_METHODS.map((m) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>

          {/* URL input */}
          <input
            type="text"
            class="flex-1 h-12 px-4 bg-transparent text-sm font-mono text-text-primary placeholder-text-muted focus:outline-none"
            placeholder="/api/v1/myservice/_table/users"
            value={url}
            onInput={(e) => setUrl((e.target as HTMLInputElement).value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') sendRequest();
            }}
          />

          {/* Send button */}
          <button
            onClick={sendRequest}
            disabled={loading}
            class="h-12 px-6 bg-brand text-white text-sm font-semibold hover:bg-brand-light transition-colors disabled:opacity-50 rounded-none rounded-tr-xl"
          >
            {loading ? (
              <span class="flex items-center gap-2">
                <svg class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Sending
              </span>
            ) : (
              'Send'
            )}
          </button>
        </div>

        {/* Request options tabs */}
        <div class="border-b border-border-subtle">
          <div class="flex">
            {(['body', 'headers', 'curl'] as const).map((tab) => (
              <button
                key={tab}
                onClick={() => setActiveTab(tab)}
                class={`px-4 py-2.5 text-xs font-medium capitalize transition-colors border-b-2 -mb-px ${
                  activeTab === tab
                    ? 'border-brand text-brand'
                    : 'border-transparent text-text-muted hover:text-text-secondary'
                }`}
              >
                {tab}
                {tab === 'headers' && (
                  <span class="ml-1.5 px-1.5 py-0.5 rounded-full text-[10px] bg-surface-overlay">
                    {headers.filter((h) => h.enabled).length}
                  </span>
                )}
              </button>
            ))}
          </div>
        </div>

        {/* Tab content */}
        <div class="p-4">
          {activeTab === 'body' && (
            <textarea
              class="input w-full h-32 font-mono text-sm resize-y"
              placeholder={`{\n  "field": "value"\n}`}
              value={body}
              onInput={(e) => setBody((e.target as HTMLTextAreaElement).value)}
            />
          )}

          {activeTab === 'headers' && (
            <div class="space-y-2">
              {headers.map((h, idx) => (
                <div key={idx} class="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={h.enabled}
                    onChange={(e) => updateHeader(idx, 'enabled', (e.target as HTMLInputElement).checked)}
                    class="rounded border-border-default bg-surface text-brand focus:ring-brand/50"
                  />
                  <input
                    type="text"
                    class="input flex-1 text-sm font-mono"
                    placeholder="Header name"
                    value={h.key}
                    onInput={(e) => updateHeader(idx, 'key', (e.target as HTMLInputElement).value)}
                  />
                  <input
                    type="text"
                    class="input flex-1 text-sm font-mono"
                    placeholder="Value"
                    value={h.value}
                    onInput={(e) => updateHeader(idx, 'value', (e.target as HTMLInputElement).value)}
                  />
                  <button
                    onClick={() => removeHeader(idx)}
                    class="p-2 text-text-muted hover:text-error transition-colors"
                  >
                    <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                    </svg>
                  </button>
                </div>
              ))}
              <button onClick={addHeader} class="btn-ghost text-xs">
                + Add Header
              </button>
            </div>
          )}

          {activeTab === 'curl' && (
            <CodeBlock code={generateCurl()} language="bash" />
          )}
        </div>
      </div>

      {/* Response */}
      {response && (
        <div class="card p-0 animate-fade-in">
          {/* Response status bar */}
          <div class="flex items-center justify-between px-4 py-3 border-b border-border-subtle">
            <div class="flex items-center gap-4">
              <span class={`font-mono font-bold text-sm ${statusColor(response.status)}`}>
                {response.status} {response.statusText}
              </span>
              <span class={`badge ${
                response.status >= 200 && response.status < 300
                  ? 'bg-success/10 text-success'
                  : response.status >= 400
                  ? 'bg-error/10 text-error'
                  : 'bg-warning/10 text-warning'
              }`}>
                {response.status >= 200 && response.status < 300 ? 'Success' : response.status >= 400 ? 'Error' : 'Redirect'}
              </span>
            </div>
            <div class="flex items-center gap-4 text-xs text-text-muted font-mono">
              <span>{response.time}ms</span>
              <span>{response.size}</span>
            </div>
          </div>

          {/* Response tabs */}
          <div class="border-b border-border-subtle">
            <div class="flex">
              {(['body', 'headers'] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setResponseTab(tab)}
                  class={`px-4 py-2.5 text-xs font-medium capitalize transition-colors border-b-2 -mb-px ${
                    responseTab === tab
                      ? 'border-brand text-brand'
                      : 'border-transparent text-text-muted hover:text-text-secondary'
                  }`}
                >
                  {tab}
                </button>
              ))}
            </div>
          </div>

          {/* Response content */}
          <div class="p-4 max-h-[60vh] overflow-y-auto">
            {responseTab === 'body' ? (
              typeof response.body === 'object' ? (
                <JsonView data={response.body} />
              ) : (
                <pre class="font-mono text-sm text-text-primary whitespace-pre-wrap">
                  {String(response.body)}
                </pre>
              )
            ) : (
              <div class="space-y-1">
                {Object.entries(response.headers).map(([key, value]) => (
                  <div key={key} class="flex gap-2 text-sm font-mono">
                    <span class="text-brand-light">{key}:</span>
                    <span class="text-text-secondary">{value}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
