import { useEffect, useState } from 'preact/hooks';
import { ServiceRecord } from '../components/ConnectionForm';
import { CodeBlock } from '../components/CodeBlock';
import { DbLogo } from '../components/DbLogo';
import { Icon } from '../components/Icon';
import { ButtonLink, CopyButton, PageHeader, Panel, Skeleton, StatusDot } from '../components/ui';
import { apiFetch } from '../hooks/useApi';
import { describeConnection, engineForDriver } from '../lib/drivers';
import { serverOrigin } from '../lib/format';
import { useHealth } from '../lib/server';

interface Counts {
  services: ServiceRecord[];
  roles: number;
  keys: { last_used?: string | null }[];
}

export function Dashboard() {
  const [data, setData] = useState<Counts | null>(null);
  const { checks } = useHealth(15_000);

  useEffect(() => {
    (async () => {
      const [s, r, k] = await Promise.allSettled([
        apiFetch('/api/v1/system/service'),
        apiFetch('/api/v1/system/role'),
        apiFetch('/api/v1/system/api-key'),
      ]);
      setData({
        services: s.status === 'fulfilled' ? s.value.resource || [] : [],
        roles: r.status === 'fulfilled' ? (r.value.resource || []).length : 0,
        keys: k.status === 'fulfilled' ? k.value.resource || [] : [],
      });
    })();
  }, []);

  const origin = serverOrigin();
  const first = data?.services[0];

  const steps = data
    ? [
        {
          done: data.services.length > 0,
          title: 'Connect a database',
          body: 'Host, port, user and password. Faucet builds the API from your schema.',
          action: <ButtonLink size="sm" variant="primary" href="/services?add=1" icon="plus">Add database</ButtonLink>,
        },
        {
          done: data.roles > 0,
          title: 'Create a role',
          body: 'Decide which databases and tables a key can read or write.',
          action: <ButtonLink size="sm" href="/roles">Create role</ButtonLink>,
        },
        {
          done: data.keys.length > 0,
          title: 'Create an API key',
          body: 'Keys carry a role. Apps and AI agents send them in the X-API-Key header.',
          action: <ButtonLink size="sm" href="/api-keys">Create key</ButtonLink>,
        },
        {
          done: data.keys.some((k) => !!k.last_used),
          title: 'Make your first request',
          body: 'Call the REST API with your key, or connect Claude, Cursor or ChatGPT over MCP.',
          action: (
            <div class="flex flex-wrap gap-1.5">
              <ButtonLink size="sm" href={first ? `/api-explorer?service=${encodeURIComponent(first.name)}` : '/api-explorer'}>Open API explorer</ButtonLink>
              <ButtonLink size="sm" variant="ghost" href="/mcp">Connect an AI agent</ButtonLink>
            </div>
          ),
        },
      ]
    : [];
  const doneCount = steps.filter((s) => s.done).length;
  const nextIdx = steps.findIndex((s) => !s.done);

  return (
    <div>
      <PageHeader title="Overview" description="Your databases, their endpoints and what to do next." />

      {data === null ? (
        <div class="flex flex-col gap-3">
          <Skeleton class="h-40 w-full rounded-[10px]" />
          <Skeleton class="h-28 w-full rounded-[10px]" />
        </div>
      ) : (
        <div class="flex flex-col gap-6">
          {nextIdx !== -1 && (
            <Panel title="Get started" description={`${doneCount} of ${steps.length} done`}>
              <ol class="divide-y divide-line">
                {steps.map((s, i) => {
                  const isNext = i === nextIdx;
                  return (
                    <li key={s.title} class={`flex flex-wrap items-start gap-x-4 gap-y-2 px-5 py-4 ${s.done ? '' : isNext ? '' : 'opacity-70'}`}>
                      <span
                        class={`mt-0.5 inline-flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold shrink-0 ${
                          s.done ? 'bg-ok/15 text-ok' : isNext ? 'bg-brand text-white' : 'border border-line-strong text-fg-faint'
                        }`}
                        aria-label={s.done ? 'Done' : `Step ${i + 1}`}
                      >
                        {s.done ? <Icon name="check" size={13} /> : i + 1}
                      </span>
                      <div class="flex-1 min-w-[220px]">
                        <p class={`text-base font-medium ${s.done ? 'text-fg-muted line-through decoration-fg-faint' : 'text-fg'}`}>{s.title}</p>
                        {!s.done && <p class="text-sm text-fg-muted mt-0.5">{s.body}</p>}
                      </div>
                      {!s.done && isNext && <div class="shrink-0">{s.action}</div>}
                    </li>
                  );
                })}
              </ol>
            </Panel>
          )}

          <div class="grid gap-6 lg:grid-cols-[1fr_360px] items-start">
            <Panel
              class="min-w-0"
              title="Databases"
              actions={data.services.length > 0 ? <ButtonLink size="sm" variant="ghost" href="/services">Manage</ButtonLink> : undefined}
            >
              {data.services.length === 0 ? (
                <p class="px-5 py-6 text-sm text-fg-muted">
                  Nothing connected yet. <a href="/services?add=1" class="link">Add a database</a> to get an API.
                </p>
              ) : (
                <ul class="divide-y divide-line">
                  {data.services.map((svc) => {
                    const check = checks[svc.name];
                    return (
                      <li key={svc.name} class="flex items-center gap-3 px-5 py-3">
                        <DbLogo driver={svc.driver} size={28} />
                        <div class="min-w-0 flex-1">
                          <a href={`/schema?service=${encodeURIComponent(svc.name)}`} class="text-sm font-medium font-mono text-fg hover:underline">{svc.name}</a>
                          <p class="text-xs text-fg-muted truncate">
                            {engineForDriver(svc.driver).label}
                            {describeConnection(svc.driver, svc.connection) && ` on ${describeConnection(svc.driver, svc.connection)}`}
                          </p>
                        </div>
                        {!svc.is_active ? (
                          <StatusDot status="idle" label="Paused" />
                        ) : check === 'ok' ? (
                          <StatusDot status="ok" label="Connected" />
                        ) : check ? (
                          <StatusDot status="bad" label="Unreachable" />
                        ) : (
                          <StatusDot status="warn" label="Not connected" />
                        )}
                      </li>
                    );
                  })}
                </ul>
              )}
            </Panel>

            <Panel class="min-w-0" title="Endpoints" description="Base URLs for apps and AI tools.">
              <dl class="divide-y divide-line">
                {[
                  { label: 'REST API', value: `${origin}/api/v1/${first?.name || '{database}'}` },
                  { label: 'MCP server', value: `${origin}/mcp` },
                  { label: 'OpenAPI spec', value: `${origin}/openapi.json` },
                ].map((e) => (
                  <div key={e.label} class="px-5 py-3">
                    <dt class="text-xs text-fg-muted">{e.label}</dt>
                    <dd class="flex items-center gap-1 mt-0.5">
                      <code class="flex-1 min-w-0 truncate text-[12.5px] text-fg" title={e.value}>{e.value}</code>
                      <CopyButton text={e.value} label="Copy" />
                    </dd>
                  </div>
                ))}
              </dl>
            </Panel>
          </div>

          {first && (
            <Panel title="Try it from a terminal" description={`Lists the tables in ${first.name}. Replace YOUR_API_KEY with a key from the API keys page.`} bodyClass="p-4">
              <CodeBlock
                samples={[
                  { label: 'curl', code: `curl ${origin}/api/v1/${first.name}/_table \\\n  -H "X-API-Key: YOUR_API_KEY"` },
                  {
                    label: 'JavaScript',
                    code: `const res = await fetch("${origin}/api/v1/${first.name}/_table", {\n  headers: { "X-API-Key": process.env.FAUCET_API_KEY },\n});\nconsole.log(await res.json());`,
                  },
                  {
                    label: 'Python',
                    code: `import os, requests\n\nres = requests.get(\n    "${origin}/api/v1/${first.name}/_table",\n    headers={"X-API-Key": os.environ["FAUCET_API_KEY"]},\n)\nprint(res.json())`,
                  },
                ]}
              />
            </Panel>
          )}
        </div>
      )}
    </div>
  );
}
