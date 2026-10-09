import { useState, useCallback } from 'preact/hooks';

interface ApiOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
}

interface ApiState<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
}

/** Error thrown by apiFetch; carries the HTTP status for callers that care. */
export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

/** Fired when the server rejects the session so the app can show sign-in. */
export const SESSION_EXPIRED_EVENT = 'faucet:session-expired';

export function sessionToken(): string | null {
  return localStorage.getItem('faucet_session');
}

export function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};
  const session = sessionToken();
  if (session) headers['Authorization'] = `Bearer ${session}`;
  return headers;
}

export async function apiFetch<T = any>(path: string, options: ApiOptions = {}): Promise<T> {
  const { method = 'GET', body, headers = {}, signal } = options;

  let response: Response;
  try {
    response = await fetch(path, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...authHeaders(),
        ...headers,
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
  } catch (err) {
    if ((err as Error)?.name === 'AbortError') throw err;
    throw new ApiError('Faucet is not reachable. Check that the server is running.', 0);
  }

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`.trim();
    try {
      const errBody = await response.json();
      if (errBody?.error?.message) message = errBody.error.message;
    } catch {
      // keep the status text
    }
    if (response.status === 401 && sessionToken()) {
      localStorage.removeItem('faucet_session');
      window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
    }
    throw new ApiError(message, response.status);
  }

  if (response.status === 204) return undefined as T;
  return response.json();
}

export function errorMessage(err: unknown, fallback = 'Something went wrong'): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

export function useApi<T = any>(path: string, options: ApiOptions = {}) {
  const [state, setState] = useState<ApiState<T>>({ data: null, loading: false, error: null });

  const execute = useCallback(
    async (overrideOptions?: ApiOptions) => {
      setState({ data: null, loading: true, error: null });
      try {
        const data = await apiFetch<T>(path, { ...options, ...overrideOptions });
        setState({ data, loading: false, error: null });
        return data;
      } catch (err) {
        setState({ data: null, loading: false, error: errorMessage(err) });
        throw err;
      }
    },
    [path]
  );

  return { ...state, execute };
}
