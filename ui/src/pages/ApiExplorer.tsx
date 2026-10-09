import { ComponentChildren } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { CodeBlock, CodeSample } from '../components/CodeBlock';
import { ServiceRecord } from '../components/ConnectionForm';
import { Icon } from '../components/Icon';
import { JsonView } from '../components/JsonView';
import { confirm, toast } from '../components/Overlay';
import {
  Button, ButtonLink, CopyButton, Field, IconButton, Input, Notice, PageHeader, Panel, PasswordInput,
  Segmented, Select, Skeleton, Spinner, Switch, Tabs, Tag, Textarea, useId, INPUT_CLASS,
} from '../components/ui';
import { SESSION_EXPIRED_EVENT, apiFetch, errorMessage, sessionToken } from '../hooks/useApi';
import { plural, serverOrigin, shellQuote, timeAgo } from '../lib/format';
import { DOCS_URL } from '../lib/server';

/* ------------------------------------------------------------------ Types */

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
const METHODS: Method[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];
const BODY_METHODS = new Set<Method>(['POST', 'PUT', 'PATCH']);

type AuthMode = 'session' | 'key';

interface Column {
  name: string;
  db_type: string;
  json_type: string;
  nullable: boolean;
  is_primary_key: boolean;
  is_auto_increment: boolean;
}

interface HeaderRow {
  id: number;
  name: string;
  value: string;
}

interface HistoryEntry {
  method: Method;
  url: string;
  at: string;
}

type Result =
  | { kind: 'network'; method: Method; url: string; ms: number }
  | {
      kind: 'http';
      method: Method;
      url: string;
      ms: number;
      status: number;
      statusText: string;
      size: number;
      headers: [string, string][];
      json: unknown;
      isJson: boolean;
      text: string;
      customAuth: boolean;
    };

/* ------------------------------------------------------------ URL helpers
   The URL input is the single source of truth. The database/table selects
   and the query-parameter fields read from it and write back into it, so
   anything typed by hand (other endpoints, extra params) is preserved. */

function splitUrl(url: string): { path: string; query: string } {
  const i = url.indexOf('?');
  return i < 0 ? { path: url, query: '' } : { path: url.slice(0, i), query: url.slice(i + 1) };
}

function readParams(url: string): [string, string][] {
  const out: [string, string][] = [];
  new URLSearchParams(splitUrl(url).query).forEach((v, k) => out.push([k, v]));
  return out;
}

/** Percent-encode a query value, leaving a few safe characters readable. */
function encodeValue(v: string): string {
  return encodeURIComponent(v).replace(/%2C/gi, ',').replace(/%3A/gi, ':').replace(/%2F/gi, '/');
}

function buildUrl(path: string, params: [string, string][]): string {
  const q = params.map(([k, v]) => `${encodeURIComponent(k)}=${encodeValue(v)}`).join('&');
  return q ? `${path}?${q}` : path;
}

function getParam(url: string, name: string): string {
  return new URLSearchParams(splitUrl(url).query).get(name) ?? '';
}

function setParam(url: string, name: string, value: string): string {
  const out: [string, string][] = [];
  let placed = false;
  for (const [k, v] of readParams(url)) {
    if (k !== name) out.push([k, v]);
    else if (!placed && value !== '') {
      out.push([name, value]);
      placed = true;
    }
  }
  if (!placed && value !== '') out.push([name, value]);
  return buildUrl(splitUrl(url).path, out);
}

function removeParams(url: string, names: string[]): string {
  const params = readParams(url);
  const kept = params.filter(([k]) => !names.includes(k));
  return kept.length === params.length ? url : buildUrl(splitUrl(url).path, kept);
}

function safeDecode(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

const RESERVED_SEGMENTS = new Set(['system', 'setup']);

/** Which database and table (if any) a path points at. */
function parsePath(path: string): { service: string; table: string; records: boolean } {
  const m = path.match(/^\/api\/v1\/([^/?#]+)(?:\/(_table|_schema)\/([^/?#]+)\/?$)?/);
  if (!m || RESERVED_SEGMENTS.has(m[1])) return { service: '', table: '', records: false };
  return { service: safeDecode(m[1]), table: m[3] ? safeDecode(m[3]) : '', records: m[2] === '_table' && !!m[3] };
}

function servicePath(service: string, suffix: string): string {
  return `/api/v1/${encodeURIComponent(service)}/${suffix}`;
}

function tablePath(service: string, table: string): string {
  return servicePath(service, table ? `_table/${encodeURIComponent(table)}` : '_table');
}

/** Params that only mean something for a given set of methods. */
const PARAM_METHODS: Record<string, Method[]> = {
  fields: ['GET'],
  order: ['GET'],
  limit: ['GET'],
  offset: ['GET'],
  include_count: ['GET'],
  group: ['GET'],
  filter: ['GET', 'PUT', 'PATCH', 'DELETE'],
  ids: ['GET', 'PATCH', 'DELETE'],
  rollback: ['POST', 'PUT', 'PATCH', 'DELETE'],
  continue: ['POST', 'PUT'],
};

/** Drop record-endpoint params the new method would ignore. */
function paramsForMethod(url: string, method: Method): string {
  const drop = Object.keys(PARAM_METHODS).filter((p) => !PARAM_METHODS[p].includes(method));
  return removeParams(url, drop);
}

const AUTH_HEADER_NAMES = new Set(['x-api-key', 'authorization']);

/** Whether the user added their own credential header, which then decides who the request runs as. */
function hasUserAuthHeader(headers: HeaderRow[]): boolean {
  return headers.some((h) => AUTH_HEADER_NAMES.has(h.name.trim().toLowerCase()));
}

/** System endpoints (except sign-in) need an admin session token, not an API key. */
function isSystemPath(path: string): boolean {
  return /^\/api\/v1\/system\//.test(path) && !/^\/api\/v1\/system\/admin\/session\/?$/.test(path);
}

interface DeletePrompt {
  title: string;
  body: ComponentChildren;
  confirmLabel: string;
}

/** Confirmation wording for a DELETE, matched to what the endpoint removes. */
function deletePrompt(pathname: string, scope: () => string): DeletePrompt {
  const path = pathname.replace(/\/+$/, '');
  const mono = (t: string) => <span class="font-mono text-fg">{t}</span>;
  let m = path.match(/^\/api\/v1\/([^/]+)\/_table\/([^/]+)$/);
  if (m && !RESERVED_SEGMENTS.has(m[1])) {
    const [svc, table] = [safeDecode(m[1]), safeDecode(m[2])];
    return {
      title: `Delete rows from ${table}?`,
      body: <>This permanently deletes {scope()} in {mono(svc)}. It cannot be undone.</>,
      confirmLabel: 'Delete rows',
    };
  }
  m = path.match(/^\/api\/v1\/([^/]+)\/_schema\/([^/]+)$/);
  if (m && !RESERVED_SEGMENTS.has(m[1])) {
    const [svc, table] = [safeDecode(m[1]), safeDecode(m[2])];
    return {
      title: `Drop table ${table}?`,
      body: <>This drops {mono(table)} from {mono(svc)}, with every row in it. It cannot be undone.</>,
      confirmLabel: 'Drop table',
    };
  }
  m = path.match(/^\/api\/v1\/system\/service\/([^/]+)$/);
  if (m) {
    const name = safeDecode(m[1]);
    return {
      title: `Remove database connection ${name}?`,
      body: <>Faucet stops serving {mono(`/api/v1/${name}`)} and its MCP tools, and apps that use it start getting errors. The database itself is not changed.</>,
      confirmLabel: 'Remove database',
    };
  }
  m = path.match(/^\/api\/v1\/system\/role\/([^/]+)$/);
  if (m) {
    return {
      title: `Delete role ${safeDecode(m[1])}?`,
      body: 'API keys that use this role lose the access it gave them. It cannot be undone.',
      confirmLabel: 'Delete role',
    };
  }
  m = path.match(/^\/api\/v1\/system\/api-key\/([^/]+)$/);
  if (m) {
    return {
      title: `Revoke API key ${safeDecode(m[1])}?`,
      body: 'Apps and agents using this key stop working right away. It cannot be undone.',
      confirmLabel: 'Revoke key',
    };
  }
  return {
    title: 'Send this DELETE request?',
    body: <>{mono(`DELETE ${safeDecode(path)}`)} can permanently remove data or settings.</>,
    confirmLabel: 'Send DELETE',
  };
}

/* ------------------------------------------------------- History storage */

const HISTORY_KEY = 'faucet_explorer_history';
const HISTORY_MAX = 10;
const SECRET_PARAM = /key|token|secret|password|passwd|auth/i;

function loadHistory(): HistoryEntry[] {
  try {
    const raw = JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]');
    if (!Array.isArray(raw)) return [];
    return raw
      .filter((e) => e && typeof e.url === 'string' && METHODS.includes(e.method))
      .slice(0, HISTORY_MAX);
  } catch {
    return [];
  }
}

function saveHistory(list: HistoryEntry[]) {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(list));
  } catch {
    // Storage full or blocked: history is a convenience only.
  }
}

/** Strip anything that looks like a credential before a URL is stored. */
function scrubUrl(url: string): string {
  const params = readParams(url);
  const kept = params.filter(([k]) => !SECRET_PARAM.test(k));
  return kept.length === params.length ? url : buildUrl(splitUrl(url).path, kept);
}

/* ----------------------------------------------------- Example bodies */

function sampleValue(c: Column): unknown {
  const t = (c.db_type || '').toLowerCase();
  switch (c.json_type) {
    case 'integer':
      return 1;
    case 'number':
      return 9.99;
    case 'boolean':
      return true;
    case 'object':
      return {};
    case 'array':
      return [];
  }
  if (t.includes('timestamp') || t.includes('datetime')) return new Date().toISOString().slice(0, 19) + 'Z';
  if (t === 'date') return new Date().toISOString().slice(0, 10);
  if (t.includes('time')) return '12:00:00';
  if (t.includes('uuid')) return '00000000-0000-0000-0000-000000000000';
  return 'example';
}

function exampleBody(method: Method, columns: Column[] | null): string {
  const cols = columns || [];
  const writable = cols.filter((c) => !c.is_auto_increment).slice(0, 6);
  const record: Record<string, unknown> = {};
  for (const c of writable) record[c.name] = sampleValue(c);
  if (!writable.length) record.name = 'example';

  if (method === 'POST') return JSON.stringify({ resource: [record] }, null, 2);
  if (method === 'PUT') {
    const { id: _id, ...rest } = record;
    return JSON.stringify({ resource: [{ id: 1, ...rest }] }, null, 2);
  }
  // PATCH: one or two fields that are not the key.
  const changes: Record<string, unknown> = {};
  const candidates = cols.filter((c) => !c.is_primary_key && !c.is_auto_increment).slice(0, 2);
  for (const c of candidates) changes[c.name] = sampleValue(c);
  if (!candidates.length) changes.name = 'updated';
  return JSON.stringify(changes, null, 2);
}

/* --------------------------------------------------------- Code samples */

interface BuiltRequest {
  method: Method;
  href: string;
  headers: [string, string][];
  body: string | null;
  parsed: unknown;
  bodyIsJson: boolean;
  ndjson: boolean;
  /** Add `Authorization: Bearer $FAUCET_ADMIN_TOKEN`, read from the environment. */
  adminToken: boolean;
}

const ADMIN_TOKEN_ENV = 'FAUCET_ADMIN_TOKEN';

function indentAfterFirst(text: string, pad: string): string {
  return text.split('\n').map((l, i) => (i ? pad + l : l)).join('\n');
}

function curlCode(r: BuiltRequest): string {
  const lines = [`curl${r.method === 'GET' ? '' : ` -X ${r.method}`} ${shellQuote(r.href)}`];
  if (r.adminToken) lines.push(`-H "Authorization: Bearer $${ADMIN_TOKEN_ENV}"`);
  for (const [k, v] of r.headers) lines.push(`-H ${shellQuote(`${k}: ${v}`)}`);
  if (r.body !== null) lines.push(`${/^\s*[[{]/.test(r.body) ? '-d' : '--data-raw'} ${shellQuote(r.body)}`);
  return lines.join(' \\\n  ');
}

function jsCode(r: BuiltRequest): string {
  const opts: string[] = [];
  if (r.method !== 'GET') opts.push(`  method: ${JSON.stringify(r.method)},`);
  const jsHeaders = r.headers.map(([k, v]) => `    ${JSON.stringify(k)}: ${JSON.stringify(v)},`);
  if (r.adminToken) jsHeaders.unshift('    "Authorization": `Bearer ${process.env.' + ADMIN_TOKEN_ENV + '}`,');
  if (jsHeaders.length) {
    opts.push(`  headers: {\n${jsHeaders.join('\n')}\n  },`);
  }
  if (r.body !== null) {
    opts.push(
      r.bodyIsJson
        ? `  body: JSON.stringify(${indentAfterFirst(JSON.stringify(r.parsed, null, 2), '  ')}),`
        : `  body: ${JSON.stringify(r.body)},`
    );
  }
  const call = opts.length ? `fetch(${JSON.stringify(r.href)}, {\n${opts.join('\n')}\n})` : `fetch(${JSON.stringify(r.href)})`;
  return `const res = await ${call};\nconst data = await res.${r.ndjson ? 'text' : 'json'}();\nconsole.log(res.status, data);`;
}

function toPython(v: unknown, ind: string): string {
  if (v === null || v === undefined) return 'None';
  if (v === true) return 'True';
  if (v === false) return 'False';
  if (typeof v === 'number') return String(v);
  if (typeof v === 'string') return JSON.stringify(v);
  const next = ind + '    ';
  if (Array.isArray(v)) return v.length ? `[\n${v.map((x) => next + toPython(x, next)).join(',\n')},\n${ind}]` : '[]';
  const entries = Object.entries(v as Record<string, unknown>);
  return entries.length ? `{\n${entries.map(([k, x]) => `${next}${JSON.stringify(k)}: ${toPython(x, next)}`).join(',\n')},\n${ind}}` : '{}';
}

function pythonCode(r: BuiltRequest): string {
  let base = r.href;
  const params: [string, string][] = [];
  try {
    const u = new URL(r.href);
    base = u.origin + u.pathname;
    u.searchParams.forEach((v, k) => params.push([k, v]));
  } catch {
    // keep the raw URL
  }
  const args = [`    ${JSON.stringify(base)},`];
  // requests sets Content-Type itself when json= is used.
  const headers = r.bodyIsJson ? r.headers.filter(([k]) => k.toLowerCase() !== 'content-type') : r.headers;
  const pyHeaders = headers.map(([k, v]) => `        ${JSON.stringify(k)}: ${JSON.stringify(v)},`);
  if (r.adminToken) pyHeaders.unshift(`        "Authorization": f"Bearer {os.environ['${ADMIN_TOKEN_ENV}']}",`);
  if (pyHeaders.length) args.push(`    headers={\n${pyHeaders.join('\n')}\n    },`);
  if (params.length) {
    const dup = new Set(params.map(([k]) => k)).size !== params.length;
    args.push(
      dup
        ? `    params=[\n${params.map(([k, v]) => `        (${JSON.stringify(k)}, ${JSON.stringify(v)}),`).join('\n')}\n    ],`
        : `    params={\n${params.map(([k, v]) => `        ${JSON.stringify(k)}: ${JSON.stringify(v)},`).join('\n')}\n    },`
    );
  }
  if (r.body !== null) args.push(r.bodyIsJson ? `    json=${toPython(r.parsed, '    ')},` : `    data=${JSON.stringify(r.body)},`);
  const read = r.ndjson ? 'print(response.text)' : 'print(response.json())';
  return `${r.adminToken ? 'import os\n' : ''}import requests\n\nresponse = requests.${r.method.toLowerCase()}(\n${args.join('\n')}\n)\nprint(response.status_code)\n${read}`;
}

/* ------------------------------------------------------- Response helpers */

const STATUS_TEXT: Record<number, string> = {
  200: 'OK', 201: 'Created', 204: 'No Content', 400: 'Bad Request', 401: 'Unauthorized', 403: 'Forbidden',
  404: 'Not Found', 405: 'Method Not Allowed', 409: 'Conflict', 413: 'Payload Too Large', 422: 'Unprocessable Entity',
  429: 'Too Many Requests', 500: 'Internal Server Error', 502: 'Bad Gateway', 503: 'Service Unavailable',
};

function statusTone(status: number): 'ok' | 'brand' | 'warn' | 'bad' {
  if (status >= 200 && status < 300) return 'ok';
  if (status >= 300 && status < 400) return 'brand';
  if (status >= 400 && status < 500) return 'warn';
  return 'bad';
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function statusHint(status: number, auth: AuthMode, method: Method, customAuth: boolean): ComponentChildren {
  switch (status) {
    case 400:
      return 'Faucet could not use part of the request. The message says which part; check the query parameters and the body.';
    case 401:
      if (customAuth) return 'Faucet did not accept the X-API-Key or Authorization header you added. Check its value, or remove it to send as your admin session.';
      return auth === 'key'
        ? <>Faucet did not accept this API key. Check that you pasted all of it and that it has not been revoked on <a class="text-brand-fg hover:underline" href="/api-keys">API keys</a>.</>
        : 'Your admin session has expired. Sign in again to keep exploring.';
    case 403:
      return auth === 'key' || customAuth
        ? <>The key's role does not allow {method} here. Give the role access on <a class="text-brand-fg hover:underline" href="/roles">Roles</a>, or use a different key.</>
        : 'This database does not accept the request. If it is read-only, only GET requests are allowed.';
    case 404:
      return 'Check the database API name and the table name. Table names are case-sensitive on some databases.';
    case 405:
      return `This endpoint does not accept ${method}. Try another method.`;
    case 409:
      return 'A row with the same unique value already exists. Change the value or update the existing row instead.';
    case 429:
      return 'Too many requests in a short time. Wait a moment and send again.';
    default:
      return status >= 500
        ? 'The database or server returned an error. The message has the details from the database.'
        : null;
  }
}

function errorMessageFrom(json: unknown): string | null {
  const e = (json as { error?: { message?: unknown } } | null)?.error;
  return e && typeof e.message === 'string' ? e.message : null;
}

const IS_MAC = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
const MOD_KEY = IS_MAC ? '⌘' : 'Ctrl';

const FILTER_EXAMPLES = ["status = 'active'", '(age >= 21) AND (country IN (\'US\', \'CA\'))', "name LIKE 'A%'", 'deleted_at IS NULL'];

/* ================================================================== Page */

export function ApiExplorer() {
  const [services, setServices] = useState<ServiceRecord[] | null>(null);
  const [servicesError, setServicesError] = useState<string | null>(null);

  const [method, setMethodState] = useState<Method>('GET');
  const [url, setUrl] = useState('');
  const [urlError, setUrlError] = useState<string | null>(null);
  const [body, setBody] = useState('');

  const [auth, setAuth] = useState<AuthMode>('session');
  const [apiKey, setApiKey] = useState(''); // component state only, never persisted
  const [keyError, setKeyError] = useState<string | null>(null);
  const [extraHeaders, setExtraHeaders] = useState<HeaderRow[]>([]);
  const headerSeq = useRef(0);

  const [tables, setTables] = useState<Record<string, { names: string[] | null; error?: string }>>({});
  const [columns, setColumns] = useState<Column[] | null>(null);

  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<Result | null>(null);
  const [resultTab, setResultTab] = useState<'body' | 'headers'>('body');
  const [history, setHistory] = useState<HistoryEntry[]>(() => loadHistory());

  const abortRef = useRef<AbortController | null>(null);
  const lastExample = useRef('');

  const ids = {
    method: useId('method'), url: useId('url'), db: useId('db'), table: useId('table'), filter: useId('filter'),
    fields: useId('fields'), order: useId('order'), limit: useId('limit'), offset: useId('offset'), idsParam: useId('ids'),
    batch: useId('batch'), body: useId('body'), key: useId('key'),
  };

  const { path } = splitUrl(url);
  const target = useMemo(() => parsePath(path), [path]);
  const service = services?.find((s) => s.name === target.service) || null;
  const knownService = !!service;
  const hasBody = BODY_METHODS.has(method);

  /* ---- Load databases, then prefill from ?service=&table= */
  useEffect(() => {
    (async () => {
      let list: ServiceRecord[] = [];
      try {
        const res = await apiFetch('/api/v1/system/service');
        list = res.resource || [];
        setServicesError(null);
      } catch (err) {
        setServicesError(errorMessage(err));
      }
      setServices(list);
      const q = new URLSearchParams(window.location.search);
      const svc = q.get('service');
      const table = q.get('table') || '';
      if (svc) setUrl(tablePath(svc, table));
      else if (list.length) setUrl((u) => u || tablePath(list[0].name, ''));
      else setUrl((u) => u || '/api/v1/system/service');
    })();
  }, []);

  /* ---- Table list for the selected database (GET /api/v1/{svc}/_table) */
  useEffect(() => {
    const svc = target.service;
    if (!knownService || tables[svc]) return;
    setTables((t) => ({ ...t, [svc]: { names: null } }));
    apiFetch(`/api/v1/${encodeURIComponent(svc)}/_table`)
      .then((res) => {
        const names = ((res.resource || []) as { name: string }[]).map((r) => r.name);
        setTables((t) => ({ ...t, [svc]: { names } }));
      })
      .catch((err) => setTables((t) => ({ ...t, [svc]: { names: [], error: errorMessage(err) } })));
  }, [target.service, knownService]);

  /* ---- Columns of the selected table, for examples and field picking */
  useEffect(() => {
    setColumns(null);
    if (!knownService || !target.table) return;
    let alive = true;
    apiFetch(`/api/v1/${encodeURIComponent(target.service)}/_schema/${encodeURIComponent(target.table)}`)
      .then((res) => alive && setColumns(Array.isArray(res?.columns) ? res.columns : []))
      .catch(() => alive && setColumns([]));
    return () => {
      alive = false;
    };
  }, [target.service, target.table, knownService]);

  /* ---- Keep the example body in step with method and table, unless edited */
  useEffect(() => {
    if (!hasBody) return;
    if (body.trim() === '' || body === lastExample.current) {
      const ex = exampleBody(method, columns);
      lastExample.current = ex;
      setBody(ex);
    }
  }, [method, columns, hasBody]);

  /* ---- Ctrl/Cmd+Enter sends from anywhere on the page */
  const sendRef = useRef<() => void>(() => {});
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !document.querySelector('[role="dialog"]')) {
        e.preventDefault();
        sendRef.current();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      abortRef.current?.abort();
    };
  }, []);

  /* ---- Body validity */
  const bodyCheck = useMemo(() => {
    if (!hasBody || body.trim() === '') return { empty: true, ok: false, parsed: undefined as unknown, error: null as string | null };
    try {
      return { empty: false, ok: true, parsed: JSON.parse(body) as unknown, error: null };
    } catch (err) {
      return { empty: false, ok: false, parsed: undefined, error: (err as Error).message };
    }
  }, [body, hasBody]);

  /* ---- Request as the code samples see it */
  const built: BuiltRequest = useMemo(() => {
    let href = serverOrigin() + (url.trim() || '/');
    let pathname = splitUrl(url.trim()).path;
    try {
      const u = new URL(url.trim() || '/', serverOrigin());
      href = u.href;
      pathname = u.pathname;
    } catch {
      // keep the raw text
    }
    // A credential header the user added replaces the default one.
    const ownAuth = hasUserAuthHeader(extraHeaders);
    const adminToken = !ownAuth && isSystemPath(pathname);
    const headers: [string, string][] = [];
    if (!ownAuth && !adminToken) headers.push(['X-API-Key', auth === 'key' && apiKey.trim() ? apiKey.trim() : 'YOUR_API_KEY']);
    const sendsBody = hasBody && !bodyCheck.empty;
    if (sendsBody) headers.push(['Content-Type', 'application/json']);
    for (const h of extraHeaders) if (h.name.trim()) headers.push([h.name.trim(), h.value]);
    return {
      adminToken,
      method,
      href,
      headers,
      body: sendsBody ? body.trim() : null,
      parsed: bodyCheck.parsed,
      bodyIsJson: bodyCheck.ok,
      ndjson: extraHeaders.some((h) => h.name.trim().toLowerCase() === 'accept' && h.value.includes('ndjson')),
    };
  }, [method, url, body, auth, apiKey, extraHeaders, bodyCheck, hasBody]);

  const samples: CodeSample[] = [
    { label: 'curl', code: curlCode(built) },
    { label: 'JavaScript', code: jsCode(built) },
    { label: 'Python', code: pythonCode(built) },
  ];

  /* ---- Mutators */

  function setMethod(m: Method) {
    setMethodState(m);
    if (target.records) setUrl((u) => paramsForMethod(u, m));
  }

  function onDatabase(svc: string) {
    if (!svc) return;
    setUrl(tablePath(svc, ''));
    setUrlError(null);
  }

  function onTable(table: string) {
    if (!target.service) return;
    setUrl(paramsForMethod(tablePath(target.service, table), method));
    setUrlError(null);
  }

  function param(name: string) {
    return getParam(url, name);
  }

  function updateParam(name: string, value: string) {
    setUrl((u) => setParam(u, name, value));
  }

  function toggleField(col: string) {
    const current = param('fields').split(',').map((s) => s.trim()).filter(Boolean);
    const next = current.includes(col) ? current.filter((c) => c !== col) : [...current, col];
    updateParam('fields', next.join(','));
  }

  function goTo(nextPath: string) {
    setMethodState('GET');
    setUrl(nextPath);
    setUrlError(null);
  }

  function restore(h: HistoryEntry) {
    setMethodState(h.method);
    setUrl(h.url);
    setUrlError(null);
    document.getElementById(ids.url)?.focus();
  }

  function remember(m: Method, u: string) {
    const entry: HistoryEntry = { method: m, url: scrubUrl(u), at: new Date().toISOString() };
    setHistory((list) => {
      const next = [entry, ...list.filter((e) => !(e.method === entry.method && e.url === entry.url))].slice(0, HISTORY_MAX);
      saveHistory(next);
      return next;
    });
  }

  function clearHistory() {
    setHistory([]);
    saveHistory([]);
    toast('History cleared');
  }

  /* ---- Send */

  async function send() {
    if (sending) return;
    const raw = url.trim();
    if (!raw) {
      setUrlError('Enter a path, for example /api/v1/system/service.');
      return;
    }
    let resolved: URL;
    try {
      resolved = new URL(raw, serverOrigin());
    } catch {
      setUrlError('This is not a valid URL. Start the path with /api/v1/.');
      return;
    }
    if (resolved.origin !== serverOrigin()) {
      setUrlError('The explorer only sends requests to this Faucet server. Copy the curl command to call another host.');
      return;
    }
    setUrlError(null);
    if (auth === 'key' && !apiKey.trim()) {
      setKeyError('Paste an API key, or switch to your admin session.');
      document.getElementById(ids.key)?.focus();
      return;
    }
    if (hasBody && bodyCheck.error) {
      document.getElementById(ids.body)?.focus();
      return;
    }
    if (method === 'DELETE') {
      const scope = () => (param('filter') ? 'every row that matches the filter' : param('ids') ? `the rows with ids ${param('ids')}` : 'the matching rows');
      const ok = await confirm({ ...deletePrompt(resolved.pathname, scope), danger: true });
      if (!ok) return;
    }

    const sendPath = resolved.pathname + resolved.search;
    // With their own X-API-Key or Authorization header, a 401 is about that
    // header, not the admin session.
    const customAuth = hasUserAuthHeader(extraHeaders);
    const headers: Record<string, string> = {};
    if (auth === 'key') headers['X-API-Key'] = apiKey.trim();
    else if (!customAuth) {
      const token = sessionToken();
      if (token) headers['Authorization'] = `Bearer ${token}`;
    }
    const sendsBody = hasBody && !bodyCheck.empty;
    if (sendsBody) headers['Content-Type'] = 'application/json';
    for (const h of extraHeaders) if (h.name.trim()) headers[h.name.trim()] = h.value;

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setSending(true);
    const started = performance.now();
    try {
      let res: Response;
      try {
        res = await fetch(sendPath, { method, headers, body: sendsBody ? body : undefined, signal: controller.signal });
      } catch (err) {
        if ((err as Error)?.name === 'AbortError') return;
        setResult({ kind: 'network', method, url: raw, ms: Math.round(performance.now() - started) });
        return;
      }
      const text = await res.text();
      const ms = Math.round(performance.now() - started);
      const contentType = res.headers.get('content-type') || '';
      let json: unknown = undefined;
      let isJson = false;
      if (contentType.includes('json') && !contentType.includes('ndjson') && text) {
        try {
          json = JSON.parse(text);
          isJson = true;
        } catch {
          // show as text
        }
      }
      const resHeaders: [string, string][] = [];
      res.headers.forEach((v, k) => resHeaders.push([k, v]));
      resHeaders.sort((a, b) => a[0].localeCompare(b[0]));
      setResult({
        kind: 'http',
        method,
        url: raw,
        ms,
        status: res.status,
        statusText: res.statusText || STATUS_TEXT[res.status] || '',
        size: new TextEncoder().encode(text).length,
        headers: resHeaders,
        json,
        isJson,
        text,
        customAuth,
      });
      setResultTab('body');
      remember(method, raw);
      if (res.status === 401 && auth === 'session' && !customAuth && sessionToken()) {
        localStorage.removeItem('faucet_session');
        window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
        setSending(false);
      }
    }
  }
  sendRef.current = send;

  /* ---- Derived view bits */

  const tableState = target.service ? tables[target.service] : undefined;
  const tableNames = tableState?.names || [];
  const tableOptions = [
    { value: '', label: tableState && tableState.names === null ? 'Loading tables' : 'All tables (list names)' },
    ...(target.table && !tableNames.includes(target.table) ? [{ value: target.table, label: target.table }] : []),
    ...tableNames.map((t) => ({ value: t, label: t })),
  ];
  const dbOptions = [
    ...(target.service && !knownService ? [{ value: target.service, label: target.service }] : []),
    ...(!target.service ? [{ value: '', label: services === null ? 'Loading databases' : 'Choose a database' }] : []),
    ...(services || []).map((s) => ({ value: s.name, label: s.is_active ? s.name : `${s.name} (paused)` })),
  ];
  const needsScope = (method === 'PATCH' || method === 'DELETE') && target.records && !param('filter') && !param('ids');
  const fieldList = param('fields').split(',').map((s) => s.trim()).filter(Boolean);

  return (
    <div>
      <PageHeader
        title="API explorer"
        description="Build a request against any connected database, send it, and copy the same request as code for your app."
        actions={<ButtonLink href="/openapi.json" external icon="book" variant="secondary">OpenAPI spec</ButtonLink>}
      />

      {servicesError && (
        <div class="mb-4">
          <Notice tone="bad" title="Could not load your databases">{servicesError}. You can still type a path below.</Notice>
        </div>
      )}

      <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_280px] items-start">
        <div class="flex flex-col gap-5 min-w-0">
          {/* ------------------------------------------------ Request */}
          <Panel bodyClass="divide-y divide-line">
            <div class="px-4 sm:px-5 py-4 flex flex-col gap-4">
              <div>
                <label for={ids.url} class="text-sm font-medium text-fg">Request</label>
                <div class="mt-1.5 flex flex-col sm:flex-row gap-2">
                  <div class="flex gap-2 sm:contents">
                    <div class="w-[112px] shrink-0">
                      <Select
                        id={ids.method}
                        aria-label="Method"
                        value={method}
                        onValue={(v) => setMethod(v as Method)}
                        options={METHODS.map((m) => ({ value: m, label: m }))}
                        class="font-mono"
                      />
                    </div>
                    <div class="flex-1 sm:order-last sm:flex-none">
                      <Button variant="primary" icon="play" loading={sending} onClick={send} class="w-full sm:w-auto">
                        Send
                      </Button>
                    </div>
                  </div>
                  <input
                    id={ids.url}
                    class={`${INPUT_CLASS} font-mono text-[13px] flex-1 min-w-0`}
                    value={url}
                    placeholder="/api/v1/{database}/_table/{table}"
                    spellcheck={false}
                    autocomplete="off"
                    aria-invalid={urlError ? true : undefined}
                    aria-describedby={`${ids.url}-hint`}
                    onInput={(e) => {
                      setUrl((e.target as HTMLInputElement).value);
                      setUrlError(null);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
                        e.preventDefault();
                        send();
                      }
                    }}
                  />
                </div>
                {urlError ? (
                  <p id={`${ids.url}-hint`} class="mt-1.5 text-xs text-bad" role="alert">{urlError}</p>
                ) : (
                  <p id={`${ids.url}-hint`} class="mt-1.5 text-xs text-fg-muted">
                    Edit the path directly for any endpoint. Press <Kbd>Enter</Kbd> in the path, or <Kbd>{MOD_KEY}</Kbd> <Kbd>Enter</Kbd> anywhere, to send.
                  </p>
                )}
              </div>

              {services && services.length === 0 && !servicesError ? (
                <div class="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3 rounded-[8px] bg-panel-2 border border-line">
                  <p class="text-sm text-fg-muted">Connect a database and its tables show up here, ready to query.</p>
                  <ButtonLink size="sm" variant="primary" icon="plus" href="/services?add=1">Add database</ButtonLink>
                </div>
              ) : (
                <div class="grid gap-3 sm:grid-cols-2">
                  <Field label="Database" htmlFor={ids.db}>
                    <Select id={ids.db} value={target.service} onValue={onDatabase} options={dbOptions} disabled={services === null} />
                  </Field>
                  <Field
                    label="Table"
                    htmlFor={ids.table}
                    error={tableState?.error ? `Could not list tables: ${tableState.error}` : null}
                  >
                    <Select id={ids.table} value={target.table} onValue={onTable} options={tableOptions} disabled={!target.service} />
                  </Field>
                </div>
              )}

              {service && (
                <div class="flex flex-wrap items-center gap-x-1 gap-y-1 -ml-1">
                  <span class="text-sm text-fg-muted ml-1 mr-1">Other endpoints</span>
                  {target.table && (
                    <Button size="sm" variant="ghost" onClick={() => goTo(servicePath(service.name, `_schema/${encodeURIComponent(target.table)}`))}>Table schema</Button>
                  )}
                  <Button size="sm" variant="ghost" onClick={() => goTo(servicePath(service.name, '_schema'))}>Full schema</Button>
                  <Button size="sm" variant="ghost" onClick={() => goTo(servicePath(service.name, '_proc'))}>Procedures</Button>
                  <Button size="sm" variant="ghost" onClick={() => goTo(servicePath(service.name, '_doc'))}>OpenAPI for {service.name}</Button>
                  {service.read_only && <span class="ml-1"><Tag>Read-only</Tag></span>}
                </div>
              )}

              {service?.read_only && method !== 'GET' && (
                <Notice tone="warn" title="This database is read-only">
                  Faucet rejects {method} requests to {service.name}. Turn off read-only on the <a class="text-brand-fg hover:underline" href="/services">Databases</a> page to allow writes.
                </Notice>
              )}
            </div>

            {/* -------------------------------------- Query parameters */}
            {target.records && (
              <div class="px-4 sm:px-5 py-4 flex flex-col gap-4">
                <SectionTitle
                  title="Query parameters"
                  hint={method === 'GET' ? 'Each field adds a parameter to the path above.' : undefined}
                />

                {(method === 'GET' || method === 'PUT' || method === 'PATCH' || method === 'DELETE') && (
                  <Field
                    label="Filter"
                    htmlFor={ids.filter}
                    optional={method === 'GET' || method === 'PUT'}
                    hint={
                      <>
                        {method === 'PUT' && 'Used only for records in the body that have no id field. '}
                        Like a SQL WHERE clause; values are always sent as bound parameters.{' '}
                        <a class="text-brand-fg hover:underline" href={`${DOCS_URL}/filter-syntax`} target="_blank" rel="noopener noreferrer">Filter syntax</a>
                      </>
                    }
                  >
                    <Input id={ids.filter} mono value={param('filter')} onValue={(v) => updateParam('filter', v)} placeholder="status = 'active'" spellcheck={false} autocomplete="off" />
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span class="text-xs text-fg-faint">Examples</span>
                      {FILTER_EXAMPLES.map((ex) => (
                        <button
                          key={ex}
                          type="button"
                          onClick={() => updateParam('filter', ex)}
                          class="h-6 px-1.5 rounded-[6px] border border-line bg-panel-2 text-fg-muted hover:text-fg hover:border-line-strong font-mono text-[11.5px]"
                        >
                          {ex}
                        </button>
                      ))}
                    </div>
                  </Field>
                )}

                {method === 'GET' && (
                  <>
                    <div class="grid gap-3 sm:grid-cols-2">
                      <Field label="Fields" htmlFor={ids.fields} optional hint="Comma-separated columns. Aggregates such as COUNT(*) work too.">
                        <Input id={ids.fields} mono value={param('fields')} onValue={(v) => updateParam('fields', v)} placeholder="id,name" spellcheck={false} autocomplete="off" />
                      </Field>
                      <Field label="Order" htmlFor={ids.order} optional hint="Column, then ASC or DESC. Separate several with commas.">
                        <Input id={ids.order} mono value={param('order')} onValue={(v) => updateParam('order', v)} placeholder="created_at DESC" spellcheck={false} autocomplete="off" />
                      </Field>
                      <Field label="Limit" htmlFor={ids.limit} optional hint="Rows per page. Default 25, maximum 1000.">
                        <Input id={ids.limit} type="number" min={0} max={1000} value={param('limit')} onValue={(v) => updateParam('limit', v)} placeholder="25" />
                      </Field>
                      <Field label="Offset" htmlFor={ids.offset} optional hint="Rows to skip, for the next page.">
                        <Input id={ids.offset} type="number" min={0} value={param('offset')} onValue={(v) => updateParam('offset', v)} placeholder="0" />
                      </Field>
                    </div>
                    {columns && columns.length > 0 && (
                      <div class="flex flex-wrap items-center gap-1.5" role="group" aria-label="Pick columns for fields">
                        <span class="text-xs text-fg-faint mr-0.5">Columns</span>
                        {columns.slice(0, 40).map((c) => {
                          const on = fieldList.includes(c.name);
                          return (
                            <button
                              key={c.name}
                              type="button"
                              aria-pressed={on}
                              onClick={() => toggleField(c.name)}
                              title={`${c.name} (${c.db_type})`}
                              class={`h-6 px-1.5 rounded-[6px] border font-mono text-[11.5px] transition-colors ${
                                on ? 'bg-brand/10 border-brand/30 text-brand-fg' : 'bg-panel-2 border-line text-fg-muted hover:text-fg hover:border-line-strong'
                              }`}
                            >
                              {c.name}
                            </button>
                          );
                        })}
                      </div>
                    )}
                    <Switch
                      checked={param('include_count') === 'true'}
                      onChange={(v) => updateParam('include_count', v ? 'true' : '')}
                      label="Include total count"
                      description="Adds meta.total, the number of rows that match the filter, for pagination."
                    />
                  </>
                )}

                {(method === 'PATCH' || method === 'DELETE') && (
                  <Field label="IDs" htmlFor={ids.idsParam} optional hint={'Comma-separated values of the "id" column. Use this or a filter.'}>
                    <Input id={ids.idsParam} mono value={param('ids')} onValue={(v) => updateParam('ids', v)} placeholder="1,2,3" spellcheck={false} autocomplete="off" />
                  </Field>
                )}

                {needsScope && (
                  <Notice tone="warn" title={`Add a filter or IDs`}>
                    Faucet refuses to {method === 'PATCH' ? 'update' : 'delete'} every row in a table, so this request returns 400 until you choose which rows it affects.
                  </Notice>
                )}

                {(method === 'POST' || method === 'PUT') && (
                  <Field
                    label="When a row fails"
                    htmlFor={ids.batch}
                    hint={
                      param('rollback') === 'true'
                        ? 'Runs in one transaction. If any row fails, nothing is saved.'
                        : param('continue') === 'true'
                          ? 'Tries every row and reports which ones failed. Rows that worked are saved.'
                          : 'Stops at the first failure. Rows before it stay saved.'
                    }
                  >
                    <Select
                      id={ids.batch}
                      value={param('rollback') === 'true' ? 'rollback' : param('continue') === 'true' ? 'continue' : ''}
                      onValue={(v) => setUrl((u) => {
                        const cleared = removeParams(u, ['rollback', 'continue']);
                        return v ? setParam(cleared, v, 'true') : cleared;
                      })}
                      options={[
                        { value: '', label: 'Stop at the first error' },
                        { value: 'rollback', label: 'Save all rows or none' },
                        { value: 'continue', label: 'Keep going and report each row' },
                      ]}
                    />
                  </Field>
                )}
              </div>
            )}

            {/* ------------------------------------------------- Body */}
            {hasBody && (
              <div class="px-4 sm:px-5 py-4 flex flex-col gap-2">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <label for={ids.body} class="text-sm font-medium text-fg">Body</label>
                  <div class="flex items-center gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={!bodyCheck.ok}
                      onClick={() => setBody(JSON.stringify(bodyCheck.parsed, null, 2))}
                    >
                      Format
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      icon="refresh"
                      onClick={() => {
                        const ex = exampleBody(method, columns);
                        lastExample.current = ex;
                        setBody(ex);
                      }}
                    >
                      Reset to example
                    </Button>
                  </div>
                </div>
                <Textarea
                  id={ids.body}
                  mono
                  rows={9}
                  value={body}
                  onValue={setBody}
                  spellcheck={false}
                  aria-invalid={bodyCheck.error ? true : undefined}
                  aria-describedby={`${ids.body}-hint`}
                  class="resize-y min-h-[140px]"
                />
                <div id={`${ids.body}-hint`} class="flex flex-col gap-1">
                  {bodyCheck.error ? (
                    <p class="text-xs text-bad" role="alert">
                      <Icon name="alert" size={12} class="inline -mt-0.5 mr-1" />
                      Not valid JSON: {bodyCheck.error}. Fix it to send.
                    </p>
                  ) : bodyCheck.ok ? (
                    <p class="text-xs text-ok">
                      <Icon name="check" size={12} class="inline -mt-0.5 mr-1" />
                      Valid JSON
                    </p>
                  ) : (
                    <p class="text-xs text-fg-muted">Empty body. Nothing is sent.</p>
                  )}
                  <p class="text-xs text-fg-muted">
                    {method === 'POST' && <>Send one object, an array of objects, or <code class="font-mono">{'{"resource": [...]}'}</code> to insert several rows at once.</>}
                    {method === 'PUT' && <>Each record replaces the row whose <code class="font-mono">id</code> it carries. Records without an <code class="font-mono">id</code> update the rows the filter selects.</>}
                    {method === 'PATCH' && <>One object with only the fields to change. It applies to every row the filter or IDs select.</>}
                    {columns === null && target.records ? ' Loading columns for a better example.' : ''}
                  </p>
                </div>
              </div>
            )}

            {/* ------------------------------------------ Auth + headers */}
            <div class="px-4 sm:px-5 py-4 flex flex-col gap-3">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <span class="text-sm font-medium text-fg">Send as</span>
                <Segmented<AuthMode>
                  label="Send as"
                  value={auth}
                  onChange={(v) => {
                    setAuth(v);
                    setKeyError(null);
                  }}
                  options={[
                    { value: 'session', label: 'My admin session' },
                    { value: 'key', label: 'API key' },
                  ]}
                />
              </div>
              {auth === 'session' ? (
                <p class="text-sm text-fg-muted">
                  Your admin session can reach every database and table. Apps and agents should send an API key in the{' '}
                  <code class="font-mono text-fg">X-API-Key</code> header so its role limits what they can do. Switch to API key to test exactly what a key can see.
                </p>
              ) : (
                <Field
                  label="API key"
                  htmlFor={ids.key}
                  error={keyError}
                  hint={<>Kept in this tab only and never saved. Don't have one? <a class="text-brand-fg hover:underline" href="/api-keys">Create a key</a>.</>}
                >
                  <PasswordInput
                    id={ids.key}
                    mono
                    value={apiKey}
                    onValue={(v) => {
                      setApiKey(v);
                      setKeyError(null);
                    }}
                    placeholder="faucet_..."
                    autocomplete="off"
                    spellcheck={false}
                    invalid={!!keyError}
                  />
                </Field>
              )}

              <div class="flex flex-col gap-2 pt-1">
                <div class="flex items-center justify-between gap-2">
                  <span class="text-sm font-medium text-fg">
                    Extra headers
                    {extraHeaders.length === 0 && <span class="font-normal text-fg-faint"> (optional)</span>}
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    icon="plus"
                    onClick={() => setExtraHeaders((h) => [...h, { id: ++headerSeq.current, name: '', value: '' }])}
                  >
                    Add header
                  </Button>
                </div>
                {extraHeaders.length === 0 ? (
                  <p class="text-xs text-fg-muted">
                    For example <code class="font-mono">Accept: application/x-ndjson</code> streams GET results one row per line.
                  </p>
                ) : (
                  extraHeaders.map((h, i) => (
                    <div key={h.id} class="flex items-center gap-2">
                      <input
                        class={`${INPUT_CLASS} font-mono text-[13px] flex-1 min-w-0`}
                        aria-label={`Header ${i + 1} name`}
                        placeholder="Accept"
                        value={h.name}
                        spellcheck={false}
                        onInput={(e) => {
                          const v = (e.target as HTMLInputElement).value;
                          setExtraHeaders((list) => list.map((x) => (x.id === h.id ? { ...x, name: v } : x)));
                        }}
                      />
                      <input
                        class={`${INPUT_CLASS} font-mono text-[13px] flex-[1.4] min-w-0`}
                        aria-label={`Header ${i + 1} value`}
                        placeholder="application/x-ndjson"
                        value={h.value}
                        spellcheck={false}
                        onInput={(e) => {
                          const v = (e.target as HTMLInputElement).value;
                          setExtraHeaders((list) => list.map((x) => (x.id === h.id ? { ...x, value: v } : x)));
                        }}
                      />
                      <IconButton icon="x" label={`Remove header ${i + 1}`} onClick={() => setExtraHeaders((list) => list.filter((x) => x.id !== h.id))} />
                    </div>
                  ))
                )}
              </div>
            </div>
          </Panel>

          {/* ------------------------------------------------ Response */}
          <ResponsePanel
            result={result}
            sending={sending}
            tab={resultTab}
            onTab={setResultTab}
            auth={auth}
          />

          {/* -------------------------------------------- Copy as code */}
          <Panel
            title="Copy as code"
            description={
              built.adminToken
                ? <>These endpoints need an admin session token, not an API key. Set <code class="font-mono">{ADMIN_TOKEN_ENV}</code> to the token that signing in with <code class="font-mono">POST /api/v1/system/admin/session</code> returns. Tokens expire, so apps should not rely on them.</>
                : auth === 'key' && apiKey.trim() && !hasUserAuthHeader(extraHeaders)
                ? 'Includes the API key you entered. Treat the copied code like a password.'
                : <>Replace <code class="font-mono">YOUR_API_KEY</code> with a key from <a class="text-brand-fg hover:underline" href="/api-keys">API keys</a>. Admin session tokens expire, so apps should not use them.</>
            }
            bodyClass="p-4 sm:p-5"
          >
            <CodeBlock samples={samples} />
          </Panel>
        </div>

        {/* -------------------------------------------- Recent requests */}
        <Panel
          title="Recent requests"
          actions={history.length > 0 ? <Button size="sm" variant="ghost" onClick={clearHistory}>Clear history</Button> : undefined}
          class="lg:sticky lg:top-6"
        >
          {history.length === 0 ? (
            <p class="px-5 py-4 text-sm text-fg-muted">
              Requests you send show up here so you can run them again. Only the method and path are kept, never keys or bodies.
            </p>
          ) : (
            <ul class="divide-y divide-line">
              {history.map((h) => (
                <li key={`${h.method} ${h.url}`}>
                  <button
                    type="button"
                    onClick={() => restore(h)}
                    title={`${h.method} ${h.url}`}
                    class="w-full text-left flex items-start gap-2.5 px-4 py-2.5 hover:bg-panel-2 focus-visible:bg-panel-2 outline-none transition-colors"
                  >
                    <span class={`shrink-0 w-[52px] font-mono text-xs font-medium pt-px ${h.method === 'GET' ? 'text-ok' : h.method === 'DELETE' ? 'text-bad' : 'text-brand-fg'}`}>
                      {h.method}
                    </span>
                    <span class="min-w-0 flex-1">
                      <span class="block font-mono text-xs text-fg truncate">{h.url}</span>
                      <span class="block text-xs text-fg-faint mt-0.5">{timeAgo(h.at)}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </div>
  );
}

/* ---------------------------------------------------------- Subcomponents */

function Kbd({ children }: { children: ComponentChildren }) {
  return <kbd class="inline-flex items-center h-[18px] px-1 rounded border border-line-strong bg-panel-2 text-[11px] text-fg-muted font-sans">{children}</kbd>;
}

function SectionTitle({ title, hint }: { title: string; hint?: string }) {
  return (
    <div>
      <h3 class="text-sm font-semibold text-fg">{title}</h3>
      {hint && <p class="text-xs text-fg-muted mt-0.5">{hint}</p>}
    </div>
  );
}

function ResponsePanel({ result, sending, tab, onTab, auth }: {
  result: Result | null; sending: boolean; tab: 'body' | 'headers'; onTab: (t: 'body' | 'headers') => void; auth: AuthMode;
}) {
  if (!result) {
    return (
      <Panel title="Response">
        {sending ? (
          <div class="px-5 py-6 flex flex-col gap-2.5">
            <Skeleton class="h-4 w-40" />
            <Skeleton class="h-3 w-full" />
            <Skeleton class="h-3 w-2/3" />
          </div>
        ) : (
          <div class="px-5 py-8 flex items-start gap-3">
            <span class="inline-flex items-center justify-center w-9 h-9 shrink-0 rounded-[10px] bg-panel-2 border border-line text-fg-muted">
              <Icon name="terminal" size={18} />
            </span>
            <div>
              <p class="text-base font-medium text-fg">No response yet</p>
              <p class="text-sm text-fg-muted mt-0.5">Press Send to run the request. The status, timing, body and headers appear here.</p>
            </div>
          </div>
        )}
      </Panel>
    );
  }

  if (result.kind === 'network') {
    return (
      <Panel title="Response" actions={sending ? <Spinner /> : undefined}>
        <div class="p-4 sm:p-5">
          <Notice tone="bad" title="Faucet did not respond">
            The request to <span class="font-mono text-fg">{result.url}</span> never reached a response after {result.ms} ms. Check that the Faucet server is still running and that this browser can reach it, then send again.
          </Notice>
        </div>
      </Panel>
    );
  }

  const tone = statusTone(result.status);
  const message = errorMessageFrom(result.json);
  const hint = result.status >= 400 ? statusHint(result.status, auth, result.method, result.customAuth) : null;
  const meta = (result.json as { meta?: { count?: number; total?: number } } | null)?.meta;
  const rows = result.isJson && Array.isArray((result.json as { resource?: unknown })?.resource)
    ? ((result.json as { resource: unknown[] }).resource.length)
    : null;
  const copyText = result.isJson ? JSON.stringify(result.json, null, 2) : result.text;

  return (
    <Panel
      title="Response"
      actions={
        <>
          {sending && <Spinner />}
          {result.text && <CopyButton text={copyText} label="Copy body" />}
        </>
      }
    >
      <div class={`transition-opacity ${sending ? 'opacity-60' : ''}`} aria-live="polite">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 px-4 sm:px-5 pt-3.5">
          <Tag tone={tone}>
            <span class="font-mono">{result.status}</span>&nbsp;{result.statusText}
          </Tag>
          <span class="text-sm text-fg-muted">{result.ms} ms</span>
          <span class="text-sm text-fg-muted">{formatBytes(result.size)}</span>
          {rows !== null && result.status < 300 && (
            <span class="text-sm text-fg-muted">
              {plural(rows, 'row')}
              {typeof meta?.total === 'number' && <> of {meta.total.toLocaleString()}</>}
            </span>
          )}
          <span class="text-sm text-fg-faint font-mono truncate min-w-0 basis-full sm:basis-auto sm:flex-1 sm:text-right">
            {result.method} {result.url}
          </span>
        </div>

        {(message || hint) && (
          <div class="px-4 sm:px-5 pt-3">
            <Notice tone={tone === 'bad' ? 'bad' : 'warn'} title={message || `${result.status} ${result.statusText}`}>
              {hint}
            </Notice>
          </div>
        )}

        <Tabs
          class="px-3 sm:px-4 mt-2"
          value={tab}
          onChange={onTab}
          tabs={[
            { value: 'body', label: 'Body' },
            { value: 'headers', label: <>Headers <span class="text-fg-faint font-normal">{result.headers.length}</span></> },
          ]}
        />

        <div class="max-h-[60vh] overflow-auto">
          {tab === 'body' ? (
            result.isJson ? (
              <JsonView data={result.json} class="px-4 sm:px-5 py-3.5" />
            ) : result.text ? (
              <pre class="px-4 sm:px-5 py-3.5 font-mono text-[12.5px] leading-[20px] text-fg whitespace-pre-wrap break-words">{result.text}</pre>
            ) : (
              <p class="px-4 sm:px-5 py-4 text-sm text-fg-muted">The response has no body.</p>
            )
          ) : (
            <dl class="divide-y divide-line">
              {result.headers.map(([k, v]) => (
                <div key={k} class="px-4 sm:px-5 py-2 flex flex-col sm:flex-row sm:gap-4">
                  <dt class="sm:w-[200px] shrink-0 font-mono text-xs text-fg-muted break-all">{k}</dt>
                  <dd class="font-mono text-xs text-fg break-all min-w-0">{v}</dd>
                </div>
              ))}
            </dl>
          )}
        </div>
      </div>
    </Panel>
  );
}
