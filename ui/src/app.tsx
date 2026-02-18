import { useState, useEffect } from 'preact/hooks';
import Router, { Route, route } from 'preact-router';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { Dashboard } from './pages/Dashboard';
import { Services } from './pages/Services';
import { SchemaExplorer } from './pages/SchemaExplorer';
import { ApiExplorer } from './pages/ApiExplorer';
import { Roles } from './pages/Roles';
import { ApiKeys } from './pages/ApiKeys';
import { Settings } from './pages/Settings';
import { Setup } from './pages/Setup';
import { Login } from './pages/Login';

type AuthState = 'loading' | 'setup' | 'login' | 'authenticated';

export function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [currentPath, setCurrentPath] = useState(window.location.pathname);
  const [authState, setAuthState] = useState<AuthState>('loading');

  useEffect(() => {
    checkAuth();
  }, []);

  async function checkAuth() {
    const session = localStorage.getItem('faucet_session');

    if (!session) {
      // No session -- check if any admins exist by trying to login with empty creds
      // If we get a specific error, the system is set up. If server returns something else, might need setup.
      try {
        const res = await fetch('/api/v1/system/admin/session', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username: '', password: '' }),
        });
        if (res.status === 404 || res.status === 503) {
          // Server might not be running or endpoint doesn't exist
          setAuthState('login');
        } else {
          // Server is up, login endpoint exists -- system is set up, need to login
          const body = await res.json().catch(() => ({}));
          // Check if the error indicates no admins exist
          if (body.error?.message?.toLowerCase().includes('no admin') ||
              body.error?.message?.toLowerCase().includes('not found')) {
            setAuthState('setup');
            route('/setup', true);
          } else {
            setAuthState('login');
          }
        }
      } catch {
        // Can't reach server -- show login anyway, they'll see errors
        setAuthState('login');
      }
      return;
    }

    // We have a session token -- validate it
    try {
      const res = await fetch('/api/v1/system/service', {
        headers: {
          'Authorization': `Bearer ${session}`,
          'Content-Type': 'application/json',
        },
      });
      if (res.ok) {
        setAuthState('authenticated');
      } else if (res.status === 401) {
        localStorage.removeItem('faucet_session');
        setAuthState('login');
      } else {
        // Some other error, try authenticated anyway
        setAuthState('authenticated');
      }
    } catch {
      // Can't reach server, show authenticated layout (errors will show per-page)
      setAuthState('authenticated');
    }
  }

  function handleLogin() {
    setAuthState('authenticated');
    route('/', true);
  }

  function handleLogout() {
    localStorage.removeItem('faucet_session');
    setAuthState('login');
    route('/', true);
  }

  const handleRoute = (e: { url: string }) => {
    setCurrentPath(e.url);
    setSidebarOpen(false);
  };

  // Loading state
  if (authState === 'loading') {
    return (
      <div class="min-h-screen flex items-center justify-center bg-surface">
        <div class="flex items-center gap-3">
          <svg class="w-5 h-5 animate-spin text-brand" viewBox="0 0 24 24" fill="none">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <span class="text-text-muted text-sm">Loading...</span>
        </div>
      </div>
    );
  }

  // Setup flow
  if (authState === 'setup') {
    return (
      <div class="min-h-screen bg-surface">
        <Setup />
      </div>
    );
  }

  // Login
  if (authState === 'login') {
    return <Login onLogin={handleLogin} />;
  }

  // Authenticated layout
  return (
    <div class="flex h-screen overflow-hidden bg-surface">
      <Sidebar
        currentPath={currentPath}
        isOpen={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
      />

      <div class="flex flex-col flex-1 min-w-0">
        <Header
          onMenuClick={() => setSidebarOpen(!sidebarOpen)}
          onLogout={handleLogout}
        />

        <main class="flex-1 overflow-y-auto p-6">
          <div class="max-w-7xl mx-auto animate-fade-in">
            <Router onChange={handleRoute}>
              <Route path="/" component={Dashboard} />
              <Route path="/services" component={Services} />
              <Route path="/schema" component={SchemaExplorer} />
              <Route path="/api-explorer" component={ApiExplorer} />
              <Route path="/roles" component={Roles} />
              <Route path="/api-keys" component={ApiKeys} />
              <Route path="/settings" component={Settings} />
              <Route path="/setup" component={Setup} />
            </Router>
          </div>
        </main>
      </div>

      {/* Mobile sidebar backdrop */}
      {sidebarOpen && (
        <div
          class="fixed inset-0 bg-black/60 z-30 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}
    </div>
  );
}
