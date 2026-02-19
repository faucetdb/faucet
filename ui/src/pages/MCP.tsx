import { useState, useEffect } from 'preact/hooks';
import { apiFetch } from '../hooks/useApi';
import { CodeBlock } from '../components/CodeBlock';

interface MCPTool {
  name: string;
  description: string;
  read_only: boolean;
}

interface MCPTransport {
  type: string;
  description: string;
  command?: string;
  endpoint?: string;
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

interface MCPInfoResponse {
  server_name: string;
  server_version: string;
  mcp_endpoint: string;
  transports: MCPTransport[];
  tools: MCPTool[];
  resources: MCPResource[];
  services: MCPService[];
}

export function MCP() {
  const [info, setInfo] = useState<MCPInfoResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeSection, setActiveSection] = useState('overview');

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    setLoading(true);
    try {
      const data = await apiFetch<MCPInfoResponse>('/api/v1/system/mcp');
      setInfo(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load MCP info');
    } finally {
      setLoading(false);
    }
  }

  const sections = [
    { id: 'overview', label: 'Overview', icon: '~' },
    { id: 'connect', label: 'Connect', icon: '>' },
    { id: 'tools', label: 'Tools', icon: '#' },
    { id: 'services', label: 'Services', icon: '@' },
  ];

  if (loading) {
    return (
      <div class="space-y-6">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">MCP Server</h1>
          <p class="text-sm text-text-secondary mt-1">Model Context Protocol integration</p>
        </div>
        <div class="card animate-pulse">
          <div class="space-y-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} class="h-10 bg-surface-overlay rounded" />
            ))}
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div class="space-y-6">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">MCP Server</h1>
          <p class="text-sm text-text-secondary mt-1">Model Context Protocol integration</p>
        </div>
        <div class="card">
          <div class="p-4 rounded-lg bg-error/10 border border-error/20">
            <p class="text-sm text-error">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  // Derive endpoint from API response or current origin
  const mcpEndpoint = info?.mcp_endpoint || `${window.location.origin}/mcp`;

  // --- Connection configs ---

  // Remote HTTP: Claude Desktop
  const claudeDesktopRemoteConfig = JSON.stringify({
    mcpServers: {
      faucet: {
        url: mcpEndpoint,
        headers: {
          'X-API-Key': 'YOUR_API_KEY',
        },
      },
    },
  }, null, 2);

  // Remote HTTP: Claude Code CLI
  const claudeCodeRemoteCommand = `claude mcp add faucet --transport http "${mcpEndpoint}" --header "X-API-Key: YOUR_API_KEY"`;

  // Remote HTTP: Cursor
  const cursorRemoteConfig = JSON.stringify({
    mcpServers: {
      faucet: {
        url: mcpEndpoint,
        headers: {
          'X-API-Key': 'YOUR_API_KEY',
        },
      },
    },
  }, null, 2);

  // Remote HTTP: ChatGPT / OpenAI
  const openaiConfig = `{
  "type": "mcp",
  "server_label": "faucet",
  "server_url": "${mcpEndpoint}",
  "headers": {
    "X-API-Key": "YOUR_API_KEY"
  }
}`;

  // Remote HTTP: generic curl test
  const curlTestCommand = `curl -X POST ${mcpEndpoint} \\
  -H "Content-Type: application/json" \\
  -H "Accept: application/json, text/event-stream" \\
  -H "X-API-Key: YOUR_API_KEY" \\
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'`;

  // Local stdio: Claude Desktop
  const claudeDesktopStdioConfig = JSON.stringify({
    mcpServers: {
      faucet: {
        command: 'faucet',
        args: ['mcp'],
      },
    },
  }, null, 2);

  // Local stdio: Claude Code CLI
  const claudeCodeStdioCommand = `claude mcp add faucet -- faucet mcp`;

  return (
    <div class="space-y-6">
      {/* Header */}
      <div>
        <h1 class="text-2xl font-semibold text-text-primary">MCP Server</h1>
        <p class="text-sm text-text-secondary mt-1">
          Connect AI assistants to your databases via the Model Context Protocol
        </p>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* Section nav */}
        <div class="lg:col-span-1">
          <nav class="space-y-1">
            {sections.map((s) => (
              <button
                key={s.id}
                onClick={() => setActiveSection(s.id)}
                class={`
                  w-full text-left px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2
                  transition-colors duration-150
                  ${activeSection === s.id
                    ? 'bg-brand/10 text-brand'
                    : 'text-text-secondary hover:text-text-primary hover:bg-surface-overlay'
                  }
                `}
              >
                <span class="w-6 text-center font-mono text-text-muted">{s.icon}</span>
                {s.label}
              </button>
            ))}
          </nav>
        </div>

        {/* Content */}
        <div class="lg:col-span-3">
          {activeSection === 'overview' && (
            <div class="space-y-6">
              {/* Endpoint banner */}
              <div class="card border-brand/30 bg-brand/5 space-y-3">
                <div class="flex items-center gap-2">
                  <div class="w-2.5 h-2.5 rounded-full bg-success animate-pulse" />
                  <h2 class="text-base font-semibold text-text-primary">MCP Endpoint Active</h2>
                </div>
                <p class="text-sm text-text-secondary">
                  AI tools can connect to your Faucet instance remotely via Streamable HTTP:
                </p>
                <CodeBlock code={mcpEndpoint} language="text" />
                <p class="text-xs text-text-muted">
                  Authenticate with an API key via the <code class="px-1 py-0.5 rounded bg-surface-overlay text-text-primary font-mono">X-API-Key</code> header or <code class="px-1 py-0.5 rounded bg-surface-overlay text-text-primary font-mono">Authorization: Bearer</code> token.
                </p>
              </div>

              {/* Status card */}
              <div class="card space-y-6">
                <div>
                  <h2 class="text-base font-semibold text-text-primary mb-1">Server Status</h2>
                  <p class="text-sm text-text-muted">
                    Faucet includes a built-in MCP server that exposes your databases as tools for AI agents.
                  </p>
                </div>

                <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Server</p>
                    <p class="text-sm font-medium text-text-primary">{info?.server_name}</p>
                  </div>
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Version</p>
                    <p class="text-sm font-mono text-text-primary">{info?.server_version}</p>
                  </div>
                  <div class="p-4 rounded-lg bg-surface border border-border-subtle">
                    <p class="text-xs text-text-muted mb-1">Exposed Databases</p>
                    <p class="text-sm font-medium text-text-primary">{info?.services?.length || 0}</p>
                  </div>
                </div>
              </div>

              {/* What is MCP */}
              <div class="card space-y-4">
                <h2 class="text-base font-semibold text-text-primary">What is MCP?</h2>
                <p class="text-sm text-text-secondary leading-relaxed">
                  The <span class="font-medium text-text-primary">Model Context Protocol</span> (MCP) is an open standard that lets AI assistants like Claude, ChatGPT, Cursor, and Windsurf directly interact with external tools and data sources. Faucet's MCP server exposes your configured database services so AI agents can discover schemas, query data, and perform CRUD operations — all through a standardized interface.
                </p>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div class="flex items-start gap-3 p-3 rounded-lg bg-surface border border-border-subtle">
                    <div class="w-8 h-8 rounded-lg bg-brand/10 flex items-center justify-center shrink-0 mt-0.5">
                      <svg class="w-4 h-4 text-brand" viewBox="0 0 20 20" fill="currentColor">
                        <path fill-rule="evenodd" d="M4.083 9h1.946c.089-1.546.383-2.97.837-4.118A6.004 6.004 0 004.083 9zM10 2a8 8 0 100 16 8 8 0 000-16zm0 2c-.076 0-.232.032-.465.262-.238.234-.497.623-.737 1.182-.389.907-.673 2.142-.766 3.556h3.936c-.093-1.414-.377-2.649-.766-3.556-.24-.56-.5-.948-.737-1.182C10.232 4.032 10.076 4 10 4zm3.971 5c-.089-1.546-.383-2.97-.837-4.118A6.004 6.004 0 0115.917 9h-1.946zm-2.003 2H8.032c.093 1.414.377 2.649.766 3.556.24.56.5.948.737 1.182.233.23.389.262.465.262.076 0 .232-.032.465-.262.238-.234.497-.623.737-1.182.389-.907.673-2.142.766-3.556zm1.166 4.118c.454-1.147.748-2.572.837-4.118h1.946a6.004 6.004 0 01-2.783 4.118zm-6.268 0C6.412 13.97 6.118 12.546 6.03 11H4.083a6.004 6.004 0 002.783 4.118z" clip-rule="evenodd" />
                      </svg>
                    </div>
                    <div>
                      <p class="text-sm font-medium text-text-primary">Remote HTTP Access</p>
                      <p class="text-xs text-text-muted">Connect any AI tool over the network — no CLI needed</p>
                    </div>
                  </div>
                  <div class="flex items-start gap-3 p-3 rounded-lg bg-surface border border-border-subtle">
                    <div class="w-8 h-8 rounded-lg bg-success/10 flex items-center justify-center shrink-0 mt-0.5">
                      <svg class="w-4 h-4 text-success" viewBox="0 0 20 20" fill="currentColor">
                        <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                      </svg>
                    </div>
                    <div>
                      <p class="text-sm font-medium text-text-primary">Schema Discovery</p>
                      <p class="text-xs text-text-muted">AI can explore tables, columns, and relationships</p>
                    </div>
                  </div>
                  <div class="flex items-start gap-3 p-3 rounded-lg bg-surface border border-border-subtle">
                    <div class="w-8 h-8 rounded-lg bg-success/10 flex items-center justify-center shrink-0 mt-0.5">
                      <svg class="w-4 h-4 text-success" viewBox="0 0 20 20" fill="currentColor">
                        <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                      </svg>
                    </div>
                    <div>
                      <p class="text-sm font-medium text-text-primary">Safe Queries</p>
                      <p class="text-xs text-text-muted">Parameterized queries prevent SQL injection</p>
                    </div>
                  </div>
                  <div class="flex items-start gap-3 p-3 rounded-lg bg-surface border border-border-subtle">
                    <div class="w-8 h-8 rounded-lg bg-success/10 flex items-center justify-center shrink-0 mt-0.5">
                      <svg class="w-4 h-4 text-success" viewBox="0 0 20 20" fill="currentColor">
                        <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                      </svg>
                    </div>
                    <div>
                      <p class="text-sm font-medium text-text-primary">Multi-Database</p>
                      <p class="text-xs text-text-muted">Expose PostgreSQL, MySQL, MSSQL, and Snowflake</p>
                    </div>
                  </div>
                </div>
              </div>

              {/* Transport modes */}
              <div class="card space-y-4">
                <h2 class="text-base font-semibold text-text-primary">Transport Modes</h2>
                <div class="space-y-3">
                  {info?.transports?.map((t) => (
                    <div key={t.type} class="p-4 rounded-lg bg-surface border border-border-subtle">
                      <div class="flex items-center gap-2 mb-2">
                        <span class={`px-2 py-0.5 rounded text-xs font-mono font-medium ${t.type === 'http' ? 'bg-brand/10 text-brand' : 'bg-surface-overlay text-text-secondary'}`}>
                          {t.type}
                        </span>
                        {t.type === 'http' && (
                          <span class="px-2 py-0.5 rounded text-xs font-medium bg-success/10 text-success">
                            Recommended
                          </span>
                        )}
                        <span class="text-sm text-text-secondary">{t.description}</span>
                      </div>
                      {t.endpoint && <CodeBlock code={t.endpoint} language="text" />}
                      {t.command && <CodeBlock code={t.command} language="bash" />}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {activeSection === 'connect' && (
            <div class="space-y-6">
              {/* Quick start banner */}
              <div class="card border-brand/30 bg-brand/5 space-y-2">
                <h2 class="text-base font-semibold text-text-primary">Remote HTTP Endpoint</h2>
                <p class="text-sm text-text-secondary">
                  Faucet's MCP server is built into the main server — no separate process needed. Any MCP-compatible AI tool can connect over HTTP.
                </p>
                <CodeBlock code={mcpEndpoint} language="text" />
              </div>

              {/* Claude Desktop - Remote */}
              <div class="card space-y-4">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-[#D97757]/10 flex items-center justify-center shrink-0">
                    <svg class="w-5 h-5 text-[#D97757]" viewBox="0 0 24 24" fill="currentColor">
                      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z" />
                    </svg>
                  </div>
                  <div>
                    <h2 class="text-base font-semibold text-text-primary">Claude Desktop</h2>
                    <p class="text-sm text-text-muted">Connect via remote HTTP endpoint</p>
                  </div>
                </div>
                <p class="text-sm text-text-secondary">
                  Add the following to your Claude Desktop config file. On macOS:{' '}
                  <code class="px-1.5 py-0.5 rounded bg-surface-overlay text-text-primary text-xs font-mono">
                    ~/Library/Application Support/Claude/claude_desktop_config.json
                  </code>
                </p>
                <CodeBlock code={claudeDesktopRemoteConfig} language="json" />
                <p class="text-xs text-text-muted">
                  Replace <code class="font-mono">YOUR_API_KEY</code> with a Faucet API key from the{' '}
                  <a href="/api-keys" class="text-brand hover:text-brand-light">API Keys</a> page.
                </p>
              </div>

              {/* Claude Code CLI */}
              <div class="card space-y-4">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-[#D97757]/10 flex items-center justify-center shrink-0">
                    <svg class="w-5 h-5 text-[#D97757]" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M12.316 3.051a1 1 0 01.633 1.265l-4 12a1 1 0 11-1.898-.632l4-12a1 1 0 011.265-.633zM5.707 6.293a1 1 0 010 1.414L3.414 10l2.293 2.293a1 1 0 11-1.414 1.414l-3-3a1 1 0 010-1.414l3-3a1 1 0 011.414 0zm8.586 0a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 11-1.414-1.414L16.586 10l-2.293-2.293a1 1 0 010-1.414z" clip-rule="evenodd" />
                    </svg>
                  </div>
                  <div>
                    <h2 class="text-base font-semibold text-text-primary">Claude Code (CLI)</h2>
                    <p class="text-sm text-text-muted">Register via command line</p>
                  </div>
                </div>
                <p class="text-sm text-text-secondary">
                  Register Faucet as a remote MCP server:
                </p>
                <CodeBlock code={claudeCodeRemoteCommand} language="bash" />
              </div>

              {/* ChatGPT / OpenAI */}
              <div class="card space-y-4">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-[#10A37F]/10 flex items-center justify-center shrink-0">
                    <svg class="w-5 h-5 text-[#10A37F]" viewBox="0 0 20 20" fill="currentColor">
                      <path d="M10 2a8 8 0 100 16 8 8 0 000-16zM8.5 7.5a1.5 1.5 0 113 0v4a1.5 1.5 0 01-3 0v-4zm1.5 8a1 1 0 100-2 1 1 0 000 2z" />
                    </svg>
                  </div>
                  <div>
                    <h2 class="text-base font-semibold text-text-primary">ChatGPT / OpenAI</h2>
                    <p class="text-sm text-text-muted">Use with the Responses API</p>
                  </div>
                </div>
                <p class="text-sm text-text-secondary">
                  Add Faucet as an MCP tool in your OpenAI Responses API calls:
                </p>
                <CodeBlock code={openaiConfig} language="json" />
              </div>

              {/* Cursor */}
              <div class="card space-y-4">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-brand/10 flex items-center justify-center shrink-0">
                    <svg class="w-5 h-5 text-brand" viewBox="0 0 20 20" fill="currentColor">
                      <path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z" />
                    </svg>
                  </div>
                  <div>
                    <h2 class="text-base font-semibold text-text-primary">Cursor</h2>
                    <p class="text-sm text-text-muted">Add to Cursor's MCP configuration</p>
                  </div>
                </div>
                <p class="text-sm text-text-secondary">
                  Add the following to{' '}
                  <code class="px-1.5 py-0.5 rounded bg-surface-overlay text-text-primary text-xs font-mono">
                    ~/.cursor/mcp.json
                  </code>
                </p>
                <CodeBlock code={cursorRemoteConfig} language="json" />
              </div>

              {/* Test with curl */}
              <div class="card space-y-4">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-surface-overlay flex items-center justify-center shrink-0">
                    <svg class="w-5 h-5 text-text-secondary" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M2 5a2 2 0 012-2h12a2 2 0 012 2v10a2 2 0 01-2 2H4a2 2 0 01-2-2V5zm3.293 1.293a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 01-1.414-1.414L7.586 10 5.293 7.707a1 1 0 010-1.414zM11 12a1 1 0 100 2h3a1 1 0 100-2h-3z" clip-rule="evenodd" />
                    </svg>
                  </div>
                  <div>
                    <h2 class="text-base font-semibold text-text-primary">Test with curl</h2>
                    <p class="text-sm text-text-muted">Verify the endpoint is working</p>
                  </div>
                </div>
                <p class="text-sm text-text-secondary">
                  Send an MCP initialize request to verify connectivity:
                </p>
                <CodeBlock code={curlTestCommand} language="bash" />
              </div>

              {/* Local stdio (collapsed / secondary) */}
              <div class="card space-y-4 opacity-80">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-surface-overlay flex items-center justify-center shrink-0">
                    <svg class="w-5 h-5 text-text-muted" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M2 5a2 2 0 012-2h12a2 2 0 012 2v10a2 2 0 01-2 2H4a2 2 0 01-2-2V5zm3.293 1.293a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 01-1.414-1.414L7.586 10 5.293 7.707a1 1 0 010-1.414zM11 12a1 1 0 100 2h3a1 1 0 100-2h-3z" clip-rule="evenodd" />
                    </svg>
                  </div>
                  <div>
                    <h2 class="text-base font-semibold text-text-primary">Local Stdio Mode</h2>
                    <p class="text-sm text-text-muted">For development or local-only setups</p>
                  </div>
                </div>
                <p class="text-sm text-text-secondary">
                  If you prefer to run Faucet as a local subprocess (no network access needed):
                </p>
                <div class="space-y-3">
                  <div>
                    <p class="text-xs font-medium text-text-secondary mb-1">Claude Desktop (stdio)</p>
                    <CodeBlock code={claudeDesktopStdioConfig} language="json" />
                  </div>
                  <div>
                    <p class="text-xs font-medium text-text-secondary mb-1">Claude Code CLI (stdio)</p>
                    <CodeBlock code={claudeCodeStdioCommand} language="bash" />
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'tools' && (
            <div class="space-y-6">
              <div class="card space-y-4">
                <div>
                  <h2 class="text-base font-semibold text-text-primary mb-1">Available Tools</h2>
                  <p class="text-sm text-text-muted">
                    These tools are available to AI agents connected via MCP
                  </p>
                </div>
                <div class="space-y-2">
                  {info?.tools?.map((tool) => (
                    <div key={tool.name} class="flex items-start gap-3 p-3 rounded-lg bg-surface border border-border-subtle">
                      <span class={`mt-0.5 px-2 py-0.5 rounded text-xs font-mono font-medium shrink-0 ${tool.read_only ? 'bg-success/10 text-success' : 'bg-warning/10 text-warning'}`}>
                        {tool.read_only ? 'READ' : 'WRITE'}
                      </span>
                      <div class="min-w-0">
                        <p class="text-sm font-mono font-medium text-text-primary">{tool.name}</p>
                        <p class="text-xs text-text-muted mt-0.5">{tool.description}</p>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div class="card space-y-4">
                <div>
                  <h2 class="text-base font-semibold text-text-primary mb-1">Resources</h2>
                  <p class="text-sm text-text-muted">
                    MCP resources provide contextual data to AI agents
                  </p>
                </div>
                <div class="space-y-2">
                  {info?.resources?.map((resource) => (
                    <div key={resource.uri} class="flex items-start gap-3 p-3 rounded-lg bg-surface border border-border-subtle">
                      <span class="mt-0.5 px-2 py-0.5 rounded text-xs font-mono font-medium shrink-0 bg-brand/10 text-brand">
                        URI
                      </span>
                      <div class="min-w-0">
                        <p class="text-sm font-mono font-medium text-text-primary">{resource.uri}</p>
                        <p class="text-xs text-text-muted mt-0.5">{resource.description}</p>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {activeSection === 'services' && (
            <div class="card space-y-4">
              <div>
                <h2 class="text-base font-semibold text-text-primary mb-1">Exposed Databases</h2>
                <p class="text-sm text-text-muted">
                  These active database services are accessible through the MCP server
                </p>
              </div>

              {info?.services?.length === 0 ? (
                <div class="p-6 text-center rounded-lg bg-surface border border-border-subtle">
                  <p class="text-sm text-text-muted">No active services configured.</p>
                  <a href="/services" class="text-sm text-brand hover:text-brand-light mt-1 inline-block">
                    Add a database service
                  </a>
                </div>
              ) : (
                <div class="overflow-x-auto rounded-lg border border-border-subtle">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b border-border-subtle bg-surface">
                        <th class="text-left px-4 py-3 font-medium text-text-secondary">Service</th>
                        <th class="text-left px-4 py-3 font-medium text-text-secondary">Driver</th>
                        <th class="text-left px-4 py-3 font-medium text-text-secondary">Access</th>
                        <th class="text-left px-4 py-3 font-medium text-text-secondary">Raw SQL</th>
                      </tr>
                    </thead>
                    <tbody>
                      {info?.services?.map((svc) => (
                        <tr key={svc.name} class="border-b border-border-subtle last:border-0">
                          <td class="px-4 py-3 font-mono font-medium text-text-primary">{svc.name}</td>
                          <td class="px-4 py-3 text-text-secondary">{svc.driver}</td>
                          <td class="px-4 py-3">
                            <span class={`px-2 py-0.5 rounded text-xs font-medium ${svc.read_only ? 'bg-success/10 text-success' : 'bg-warning/10 text-warning'}`}>
                              {svc.read_only ? 'Read-only' : 'Read/Write'}
                            </span>
                          </td>
                          <td class="px-4 py-3">
                            <span class={`px-2 py-0.5 rounded text-xs font-medium ${svc.raw_sql_allowed ? 'bg-warning/10 text-warning' : 'bg-surface-overlay text-text-muted'}`}>
                              {svc.raw_sql_allowed ? 'Enabled' : 'Disabled'}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
