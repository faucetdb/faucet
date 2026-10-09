import { useEffect, useState } from 'preact/hooks';
import Router, { Route, route } from 'preact-router';
import { Sidebar } from './components/Sidebar';
import { Icon } from './components/Icon';
import { OverlayHost } from './components/Overlay';
import { Spinner } from './components/ui';
import { SESSION_EXPIRED_EVENT, apiFetch } from './hooks/useApi';
import { Dashboard } from './pages/Dashboard';
import { Services } from './pages/Services';
import { SchemaExplorer } from './pages/SchemaExplorer';
import { ApiExplorer } from './pages/ApiExplorer';
import { Roles } from './pages/Roles';
import { ApiKeys } from './pages/ApiKeys';
import { Settings } from './pages/Settings';
import { MCP } from './pages/MCP';
import { Setup } from './pages/Setup';
import { Login } from './pages/Login';

type AuthState = 'loading' | 'setup' | 'login' | 'authenticated';

const TITLES: Record<string, string> = {
  '/': 'Overview',
  '/services': 'Databases',
  '/schema': 'Schema',
  '/api-explorer': 'API explorer',
  '/roles': 'Roles',
  '/api-keys': 'API keys',
  '/mcp': 'AI agents',
  '/settings': 'Settings',
};

/** Older docs and `faucet serve` linked to /admin; send those to the app root. */
function normalizeLegacyPath() {
  const { pathname, search } = window.location;
  if (pathname === '/admin' || pathname.startsWith('/admin/')) {
    const rest = pathname.slice('/admin'.length) || '/';
    window.history.replaceState(null, '', rest + search);
  }
}

export function App() {
  normalizeLegacyPath();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [currentPath, setCurrentPath] = useState(window.location.pathname + window.location.search);
  const [authState, setAuthState] = useState<AuthState>('loading');

  useEffect(() => {
    checkAuth();
    const onExpired = () => setAuthState('login');
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  }, []);

  useEffect(() => {
    const base = currentPath.split('?')[0];
    document.title = authState === 'authenticated' && TITLES[base] ? `${TITLES[base]} | Faucet` : 'Faucet';
  }, [currentPath, authState]);

  async function checkAuth() {
    try {
      const res = await fetch('/api/v1/setup');
      if (res.ok && (await res.json()).needs_setup) {
        localStorage.removeItem('faucet_session');
        setAuthState('setup');
        return;
      }
    } catch {
      // fall through to the session check; pages show their own errors
    }
    if (!localStorage.getItem('faucet_session')) {
      setAuthState('login');
      return;
    }
    try {
      await apiFetch('/api/v1/system/info');
      setAuthState('authenticated');
    } catch (err: any) {
      setAuthState(err?.status === 401 || err?.status === 403 ? 'login' : 'authenticated');
    }
  }

  function handleLogout() {
    localStorage.removeItem('faucet_session');
    apiFetch('/api/v1/system/admin/session', { method: 'DELETE' }).catch(() => {});
    setAuthState('login');
  }

  if (authState === 'loading') {
    return (
      <div class="min-h-screen flex items-center justify-center text-fg-muted" role="status">
        <Spinner size={18} />
        <span class="sr-only">Loading</span>
      </div>
    );
  }

  if (authState === 'setup') {
    return (
      <>
        <Setup
          onComplete={(next) => {
            setAuthState('authenticated');
            route(next || '/', true);
            setCurrentPath(next || '/');
          }}
        />
        <OverlayHost />
      </>
    );
  }

  if (authState === 'login') {
    return (
      <>
        <Login
          onLogin={() => {
            setAuthState('authenticated');
            const here = window.location.pathname;
            if (here === '/setup') route('/', true);
          }}
        />
        <OverlayHost />
      </>
    );
  }

  return (
    <div class="flex min-h-screen">
      <a href="#main" class="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:px-3 focus:py-2 focus:rounded focus:bg-panel focus:text-fg">
        Skip to content
      </a>
      <Sidebar currentPath={currentPath} isOpen={sidebarOpen} onClose={() => setSidebarOpen(false)} onLogout={handleLogout} />
      {sidebarOpen && <div class="fixed inset-0 z-30 bg-[var(--overlay)] lg:hidden" onClick={() => setSidebarOpen(false)} aria-hidden="true" />}

      <div class="flex-1 min-w-0 flex flex-col">
        <div class="lg:hidden sticky top-0 z-20 flex items-center gap-2 h-12 px-3 bg-panel/90 backdrop-blur border-b border-line">
          <button type="button" class="p-2 rounded text-fg-muted hover:text-fg" onClick={() => setSidebarOpen(true)} aria-label="Open menu">
            <Icon name="menu" size={18} />
          </button>
          <img src="/faucet-icon.svg" alt="" width="20" height="20" />
          <span class="font-semibold text-fg">Faucet</span>
        </div>
        <main id="main" class="flex-1 px-4 py-6 sm:px-8 sm:py-8">
          <div class="max-w-[1120px] mx-auto">
            <Router
              onChange={(e: { url: string }) => {
                setCurrentPath(e.url);
                setSidebarOpen(false);
                window.scrollTo(0, 0);
              }}
            >
              <Route path="/" component={Dashboard} />
              <Route path="/services" component={Services} />
              <Route path="/schema" component={SchemaExplorer} />
              <Route path="/api-explorer" component={ApiExplorer} />
              <Route path="/roles" component={Roles} />
              <Route path="/api-keys" component={ApiKeys} />
              <Route path="/settings" component={Settings} />
              <Route path="/mcp" component={MCP} />
              <Route path="/setup" component={GoHome} />
              <Route default component={NotFound} />
            </Router>
          </div>
        </main>
      </div>
      <OverlayHost />
    </div>
  );
}

function GoHome() {
  useEffect(() => {
    route('/', true);
  }, []);
  return null;
}

function NotFound() {
  return (
    <div class="py-16">
      <h1 class="text-xl font-semibold text-fg">This page doesn't exist</h1>
      <p class="text-fg-muted mt-1">
        Go back to the <a href="/" class="link">overview</a>.
      </p>
    </div>
  );
}
