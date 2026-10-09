import { useEffect, useState } from 'preact/hooks';
import { CodeBlock, CodeSample } from '../components/CodeBlock';
import { DbLogo } from '../components/DbLogo';
import { Button, ButtonLink, CopyButton, EmptyState, Field, Input, Notice, PageHeader, Panel, Skeleton, Tag, useId } from '../components/ui';
import { apiFetch, errorMessage } from '../hooks/useApi';
import { engineForDriver } from '../lib/drivers';
import { serverOrigin, shellQuote } from '../lib/format';

/* Response of GET /api/v1/system/mcp (SystemHandler.MCPInfo). */
interface MCPTool {
  name: string;
  description: string;
  read_only: boolean;
}

interface MCPResource {
  uri: string;
  description: string;
}

interface MCPService {
  name: string;
  driver: string;
  read_only: boolean;
  raw_sql_allowed: boolean;
}

interface MCPTransport {
  type: string;
  description: string;
  command?: string;
  endpoint?: string;
}

interface MCPInfo {
  server_name: string;
  server_version: string;
  mcp_endpoint: string;
  transports: MCPTransport[];
  tools: MCPTool[];
  resources: MCPResource[];
  services: MCPService[];
}

const KEY_PLACEHOLDER = 'YOUR_API_KEY';

/** True when a URL points at this machine, so cloud services can't reach it. */
function isLocalUrl(url: string): boolean {
  try {
    const h = new URL(url).hostname;
    return h === 'localhost' || h === '127.0.0.1' || h === '::1' || h === '[::1]' || h.endsWith('.local');
  } catch {
    return false;
  }
}

function json(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

/** Every client snippet, built from the endpoint and the key to show. */
function clientSamples(url: string, origin: string, key: string): CodeSample[] {
  const header = `X-API-Key: ${key}`;
  // mcp-remote refuses plain http to anything but localhost unless told otherwise.
  const remoteArgs = ['-y', 'mcp-remote', url, '--header', `X-API-Key:${key}`];
  if (url.startsWith('http://') && !isLocalUrl(url)) remoteArgs.push('--allow-http');
  const local = isLocalUrl(url);

  return [
    {
      label: 'Claude Code',
      note: (
        <>
          Run this in your project. Add <code class="code-chip">--scope user</code> to use Faucet in every project, or{' '}
          <code class="code-chip">--scope project</code> to share it with your team through <code class="code-chip">.mcp.json</code>.
        </>
      ),
      code: `claude mcp add --transport http faucet ${url} \\\n  --header ${shellQuote(header)}`,
    },
    {
      label: 'Cursor',
      note: (
        <>
          Add to <code class="code-chip">~/.cursor/mcp.json</code> for every project, or <code class="code-chip">.cursor/mcp.json</code> in one project.
        </>
      ),
      code: json({ mcpServers: { faucet: { url, headers: { 'X-API-Key': key } } } }),
    },
    {
      label: 'VS Code',
      note: (
        <>
          Add to <code class="code-chip">.vscode/mcp.json</code> in your workspace, then use the tools from Copilot Chat in agent mode.
        </>
      ),
      code: json({ servers: { faucet: { type: 'http', url, headers: { 'X-API-Key': key } } } }),
    },
    {
      label: 'Windsurf',
      note: (
        <>
          Add to <code class="code-chip">~/.codeium/windsurf/mcp_config.json</code>, then refresh the MCP servers list in Cascade.
        </>
      ),
      code: json({ mcpServers: { faucet: { serverUrl: url, headers: { 'X-API-Key': key } } } }),
    },
    {
      label: 'Claude Desktop',
      note: (
        <>
          <p>
            Claude Desktop's config file only launches local commands, so the <code class="code-chip">mcp-remote</code> bridge (needs Node.js) forwards
            requests to Faucet. Open Settings, then Developer, then Edit config, or edit the file directly:
          </p>
          <ul class="mt-1.5 flex flex-col gap-0.5">
            <li>macOS: <code class="code-chip">~/Library/Application Support/Claude/claude_desktop_config.json</code></li>
            <li>Windows: <code class="code-chip">%APPDATA%\Claude\claude_desktop_config.json</code></li>
          </ul>
          <p class="mt-1.5">Restart Claude Desktop after saving. If Faucet runs on the same machine, the "Same machine" tab needs no bridge.</p>
        </>
      ),
      code: json({ mcpServers: { faucet: { command: 'npx', args: remoteArgs } } }),
    },
    {
      label: 'Same machine',
      note: (
        <>
          <p>
            When the <code class="code-chip">faucet</code> binary is installed where the agent runs, the client can start{' '}
            <code class="code-chip">faucet mcp</code> itself and talk over stdio. It reads the same config as this server
            (<code class="code-chip">~/.faucet</code> by default; add <code class="code-chip">"--data-dir", "/path"</code> to args otherwise).
          </p>
          <p class="mt-1.5">
            Stdio runs with full admin rights and ignores roles, so use it only on your own machine. Use the full path to{' '}
            <code class="code-chip">faucet</code> if the client can't find it. For Claude Code:{' '}
            <code class="code-chip">claude mcp add faucet -- faucet mcp</code>
          </p>
        </>
      ),
      code: json({ mcpServers: { faucet: { command: 'faucet', args: ['mcp'] } } }),
    },
    {
      label: 'ChatGPT',
      note: (
        <>
          <p>
            ChatGPT connectors sign in with OAuth, which Faucet does not offer yet. Two routes work today:
          </p>
          <ul class="mt-1.5 flex flex-col gap-1 list-disc pl-5">
            <li>
              Custom GPT: add an Action, import from <code class="code-chip">{origin}/openapi.json</code>, and set authentication to API key, custom
              header <code class="code-chip">X-API-Key</code>.
            </li>
            <li>Developers: pass this tool to the OpenAI Responses API in the <code class="code-chip">tools</code> array.</li>
          </ul>
          {local ? (
            <p class="mt-1.5 text-warn">
              OpenAI calls Faucet from its own servers, and this address is only reachable from your machine. Put Faucet behind a public URL first.
            </p>
          ) : (
            <p class="mt-1.5">OpenAI calls Faucet from its own servers, so this URL must be reachable from the internet.</p>
          )}
        </>
      ),
      code: json({ type: 'mcp', server_label: 'faucet', server_url: url, headers: { 'X-API-Key': key }, require_approval: 'never' }),
    },
    {
      label: 'Test with curl',
      note: 'Sends the MCP handshake. A reply with serverInfo means the URL and key work.',
      code:
        `curl -sS -X POST ${url} \\\n` +
        `  -H 'Content-Type: application/json' \\\n` +
        `  -H 'Accept: application/json, text/event-stream' \\\n` +
        `  -H ${shellQuote(header)} \\\n` +
        `  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1.0"}}}'`,
    },
  ];
}

export function MCP() {
  const [info, setInfo] = useState<MCPInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [apiKey, setApiKey] = useState('');
  const keyId = useId('mcp-key');

  useEffect(() => {
    load();
  }, []);

  async function load() {
    setLoading(true);
    try {
      setInfo(await apiFetch<MCPInfo>('/api/v1/system/mcp'));
      setError(null);
    } catch (err) {
      setError(errorMessage(err, 'Could not load the MCP settings.'));
    } finally {
      setLoading(false);
    }
  }

  const origin = serverOrigin();
  const url = info?.mcp_endpoint || `${origin}/mcp`;
  const key = apiKey.trim() || KEY_PLACEHOLDER;
  const samples = clientSamples(url, origin, key);

  return (
    <div>
      <PageHeader
        title="AI agents"
        description="Faucet is an MCP server, so AI agents can list your tables and query them with the permissions of an API key."
      />

      {error && (
        <div class="mb-6">
          <Notice tone="bad" title="Could not load the tool list">
            <p>{error}</p>
            <Button size="sm" class="mt-2" icon="refresh" onClick={load} loading={loading}>
              Try again
            </Button>
          </Notice>
        </div>
      )}

      <div class="flex flex-col gap-6">
        <Panel title="Endpoint" description="Streamable HTTP on the same port as the REST API. No extra process to run.">
          <div class="px-5 py-4 flex flex-col gap-4">
            <div class="flex flex-wrap items-center gap-2 rounded-[8px] border border-line bg-panel-2 pl-3 pr-1.5 py-1.5">
              <code class="flex-1 min-w-0 break-all text-[13px] text-fg">{url}</code>
              <CopyButton text={url} label="Copy URL" variant="secondary" />
            </div>
            <div class="text-sm text-fg-muted flex flex-col gap-1.5 max-w-[72ch]">
              <p>
                Agents send an API key in the <code class="code-chip">X-API-Key</code> header. The key's role decides which databases and tables the
                agent sees and what it may change, exactly as for the REST API. Read-only databases refuse writes.
              </p>
              <p>
                An admin session token (<code class="code-chip">Authorization: Bearer</code>) also works but skips roles, so give agents a key
                instead.
              </p>
            </div>
          </div>
          <div class="px-5 py-4 border-t border-line">
            <Field
              label="Use this API key in the snippets"
              htmlFor={keyId}
              optional
              hint={
                <>
                  Only used to fill in the snippets below. It is not saved or sent anywhere. Keys are shown once, when you{' '}
                  <a href="/api-keys" class="link">create one on the API keys page</a>.
                </>
              }
            >
              <Input
                id={keyId}
                mono
                value={apiKey}
                onValue={setApiKey}
                placeholder={KEY_PLACEHOLDER}
                autoComplete="off"
                spellcheck={false}
                class="max-w-md"
              />
            </Field>
          </div>
        </Panel>

        <Panel title="Connect a client" description="Pick your AI tool and copy the setup." bodyClass="p-4">
          <CodeBlock samples={samples} />
        </Panel>

        <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,420px)] items-start">
          <Panel
            class="min-w-0"
            title="Tools"
            description="What an agent can call. Each call is checked against the key's role."
          >
            {loading && !info ? (
              <RowsSkeleton rows={4} />
            ) : (
              <ul class="divide-y divide-line">
                {(info?.tools || []).map((t) => (
                  <li key={t.name} class="flex flex-wrap items-start gap-x-3 gap-y-1 px-5 py-3">
                    <div class="min-w-0 flex-1 basis-56">
                      <p class="font-mono text-[13px] text-fg break-all">{t.name}</p>
                      <p class="text-sm text-fg-muted mt-0.5">{t.description}</p>
                    </div>
                    {t.read_only ? <Tag>Read</Tag> : <Tag tone="warn">Writes data</Tag>}
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <div class="flex flex-col gap-6 min-w-0">
            <Panel
              title="Databases exposed"
              description="Active databases, filtered further by each key's role."
              actions={info && info.services.length > 0 ? <ButtonLink size="sm" variant="ghost" href="/services">Change</ButtonLink> : undefined}
            >
              {loading && !info ? (
                <RowsSkeleton rows={2} />
              ) : !info || info.services.length === 0 ? (
                <EmptyState
                  icon="database"
                  title="No databases to expose"
                  action={<ButtonLink variant="primary" icon="plus" href="/services?add=1">Add database</ButtonLink>}
                >
                  Connect a database and its tables become available to agents right away.
                </EmptyState>
              ) : (
                <ul class="divide-y divide-line">
                  {info.services.map((s) => (
                    <li key={s.name} class="flex items-center gap-3 px-5 py-3">
                      <DbLogo driver={s.driver} size={28} />
                      <div class="min-w-0 flex-1">
                        <p class="font-mono text-[13px] font-medium text-fg truncate">{s.name}</p>
                        <p class="text-xs text-fg-muted">
                          {engineForDriver(s.driver).label}, raw SQL {s.raw_sql_allowed ? 'on' : 'off'}
                        </p>
                      </div>
                      {s.read_only ? <Tag>Read-only</Tag> : <Tag tone="brand">Read and write</Tag>}
                    </li>
                  ))}
                </ul>
              )}
            </Panel>

            <Panel title="Resources" description="Context an agent can load without calling a tool.">
              {loading && !info ? (
                <RowsSkeleton rows={2} />
              ) : (
                <ul class="divide-y divide-line">
                  {(info?.resources || []).map((r) => (
                    <li key={r.uri} class="px-5 py-3">
                      <p class="font-mono text-[13px] text-fg break-all">{r.uri}</p>
                      <p class="text-sm text-fg-muted mt-0.5">{r.description}</p>
                    </li>
                  ))}
                </ul>
              )}
            </Panel>

            {info?.server_version && (
              <p class="text-xs text-fg-faint px-1">
                {info.server_name}, version <span class="font-mono">{info.server_version}</span>
              </p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function RowsSkeleton({ rows }: { rows: number }) {
  return (
    <div class="divide-y divide-line">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} class="px-5 py-3.5 flex flex-col gap-2">
          <Skeleton class="h-3.5 w-40" />
          <Skeleton class="h-3 w-64 max-w-full" />
        </div>
      ))}
    </div>
  );
}
