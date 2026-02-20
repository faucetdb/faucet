import { useState } from 'preact/hooks';
import { apiFetch } from '../hooks/useApi';

interface LoginProps {
  onLogin: () => void;
}

export function Login({ onLogin }: LoginProps) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const res = await apiFetch('/api/v1/system/admin/session', {
        method: 'POST',
        body: { email, password },
      });
      if (res.session_token) {
        localStorage.setItem('faucet_session', res.session_token);
      }
      onLogin();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div class="min-h-screen flex items-center justify-center bg-surface p-4">
      <div class="w-full max-w-sm">
        {/* Logo */}
        <div class="text-center mb-8">
          <img src="/faucet-icon.svg" alt="Faucet" width="56" height="56" class="mx-auto mb-4" />
          <h1 class="text-xl font-bold text-text-primary">Faucet Admin</h1>
          <p class="text-sm text-text-muted mt-1">Sign in to manage your instance</p>
        </div>

        {/* Login form */}
        <form onSubmit={handleSubmit} class="card space-y-4">
          {error && (
            <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error">
              {error}
            </div>
          )}

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">Email</label>
            <input
              type="email"
              class="input w-full"
              placeholder="admin@example.com"
              value={email}
              autoFocus
              onInput={(e) => setEmail((e.target as HTMLInputElement).value)}
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">Password</label>
            <input
              type="password"
              class="input w-full"
              placeholder="Enter password"
              value={password}
              onInput={(e) => setPassword((e.target as HTMLInputElement).value)}
            />
          </div>

          <button
            type="submit"
            disabled={loading || !email || !password}
            class="btn-primary w-full py-2.5"
          >
            {loading ? (
              <span class="flex items-center justify-center gap-2">
                <svg class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Signing in...
              </span>
            ) : (
              'Sign In'
            )}
          </button>
        </form>
      </div>
    </div>
  );
}
