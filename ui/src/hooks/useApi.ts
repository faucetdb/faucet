import { useState, useCallback } from 'preact/hooks';

interface ApiOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
}

interface ApiState<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
}

const BASE_URL = '';

function getAuthHeaders(): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  const apiKey = localStorage.getItem('faucet_api_key');
  if (apiKey) {
    headers['X-API-Key'] = apiKey;
  }

  const sessionToken = localStorage.getItem('faucet_session');
  if (sessionToken) {
    headers['Authorization'] = `Bearer ${sessionToken}`;
  }

  return headers;
}

export async function apiFetch<T = any>(
  path: string,
  options: ApiOptions = {}
): Promise<T> {
  const { method = 'GET', body, headers = {} } = options;

  const response = await fetch(`${BASE_URL}${path}`, {
    method,
    headers: {
      ...getAuthHeaders(),
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    let errorMsg = `HTTP ${response.status}`;
    try {
      const errBody = await response.json();
      if (errBody.error?.message) {
        errorMsg = errBody.error.message;
      }
    } catch {
      // Use status text fallback
      errorMsg = `${response.status} ${response.statusText}`;
    }
    throw new Error(errorMsg);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}

export function useApi<T = any>(path: string, options: ApiOptions = {}) {
  const [state, setState] = useState<ApiState<T>>({
    data: null,
    loading: false,
    error: null,
  });

  const execute = useCallback(
    async (overrideOptions?: ApiOptions) => {
      setState({ data: null, loading: true, error: null });
      try {
        const data = await apiFetch<T>(path, { ...options, ...overrideOptions });
        setState({ data, loading: false, error: null });
        return data;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Unknown error';
        setState({ data: null, loading: false, error: message });
        throw err;
      }
    },
    [path]
  );

  return { ...state, execute };
}

export function useApiMutation<T = any>() {
  const [state, setState] = useState<ApiState<T>>({
    data: null,
    loading: false,
    error: null,
  });

  const execute = useCallback(
    async (path: string, options: ApiOptions = {}) => {
      setState({ data: null, loading: true, error: null });
      try {
        const data = await apiFetch<T>(path, options);
        setState({ data, loading: false, error: null });
        return data;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Unknown error';
        setState({ data: null, loading: false, error: message });
        throw err;
      }
    },
    []
  );

  return { ...state, execute };
}
