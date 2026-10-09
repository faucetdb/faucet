import { useEffect, useState } from 'preact/hooks';
import { apiFetch } from '../hooks/useApi';

export interface ServerInfo {
  version: string;
  drivers: string[];
}

let infoCache: Promise<ServerInfo> | null = null;

/** Version and drivers of the running server (fetched once per page load). */
export function useServerInfo(): ServerInfo | null {
  const [info, setInfo] = useState<ServerInfo | null>(null);
  useEffect(() => {
    if (!infoCache) {
      infoCache = apiFetch<ServerInfo>('/api/v1/system/info').catch(() => {
        infoCache = null;
        return { version: '', drivers: [] };
      });
    }
    let alive = true;
    infoCache.then((i) => alive && setInfo(i));
    return () => {
      alive = false;
    };
  }, []);
  return info;
}

export type Health = 'ok' | 'degraded' | 'down' | 'unknown';

/**
 * Polls /readyz, which pings every connected database. "degraded" means at
 * least one database is unreachable.
 */
export function useHealth(intervalMs = 30_000): { health: Health; checks: Record<string, string>; refresh: () => void } {
  const [state, setState] = useState<{ health: Health; checks: Record<string, string> }>({ health: 'unknown', checks: {} });
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let alive = true;
    const check = async () => {
      try {
        const res = await fetch('/readyz');
        const body = await res.json().catch(() => ({}));
        if (alive) setState({ health: res.ok ? 'ok' : 'degraded', checks: body.checks || {} });
      } catch {
        if (alive) setState({ health: 'down', checks: {} });
      }
    };
    check();
    const id = window.setInterval(check, intervalMs);
    return () => {
      alive = false;
      window.clearInterval(id);
    };
  }, [intervalMs, tick]);
  return { ...state, refresh: () => setTick((t) => t + 1) };
}

export const DOCS_URL = 'https://wiki.faucetdb.ai';
export const GITHUB_URL = 'https://github.com/faucetdb/faucet';
