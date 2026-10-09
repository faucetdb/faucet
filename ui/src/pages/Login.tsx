import { useState } from 'preact/hooks';
import { Button, Field, Input, Notice, PasswordInput, useId } from '../components/ui';
import { apiFetch, errorMessage } from '../hooks/useApi';

export function Login({ onLogin }: { onLogin: () => void }) {
  const [email, setEmail] = useState(localStorage.getItem('faucet_admin_email') || '');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const emailId = useId('email');
  const passId = useId('pass');

  async function handleSubmit(e: Event) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const res = await apiFetch('/api/v1/system/admin/session', { method: 'POST', body: { email, password } });
      if (res.session_token) localStorage.setItem('faucet_session', res.session_token);
      localStorage.setItem('faucet_admin_email', res.email || email);
      onLogin();
    } catch (err) {
      setError(errorMessage(err, 'Sign-in failed'));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div class="min-h-screen flex items-center justify-center px-4 py-12">
      <div class="w-full max-w-[360px]">
        <img src="/faucet-icon.svg" alt="" width="36" height="36" class="mb-5" />
        <h1 class="text-xl font-semibold tracking-[-0.01em] text-fg">Sign in to Faucet</h1>
        <p class="text-base text-fg-muted mt-1 mb-6">Use the admin account created during setup.</p>

        <form onSubmit={handleSubmit} class="flex flex-col gap-4" noValidate>
          {error && <Notice tone="bad" title={error} />}
          <Field label="Email" htmlFor={emailId}>
            <Input id={emailId} type="email" value={email} onValue={setEmail} autocomplete="username" autoFocus={!email} required />
          </Field>
          <Field label="Password" htmlFor={passId}>
            <PasswordInput id={passId} value={password} onValue={setPassword} autocomplete="current-password" autoFocus={!!email} required />
          </Field>
          <Button type="submit" variant="primary" loading={loading} disabled={!email || !password} class="mt-1 w-full">
            Sign in
          </Button>
        </form>

        <p class="text-sm text-fg-muted mt-8">
          Locked out? On the server, create another admin with{' '}
          <span class="code-chip">faucet admin create</span>.
        </p>
      </div>
    </div>
  );
}
