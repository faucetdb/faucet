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

export function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [currentPath, setCurrentPath] = useState(window.location.pathname);
  const [needsSetup, setNeedsSetup] = useState<boolean | null>(null);

  useEffect(() => {
    fetch('/api/v1/system/status')
      .then(r => r.json())
      .then(data => {
        if (data.needs_setup) {
          setNeedsSetup(true);
          route('/setup', true);
        } else {
          setNeedsSetup(false);
        }
      })
      .catch(() => {
        // API not available, show the UI anyway
        setNeedsSetup(false);
      });
  }, []);

  const handleRoute = (e: { url: string }) => {
    setCurrentPath(e.url);
    setSidebarOpen(false);
  };

  // Show setup page if needed
  if (needsSetup === true && currentPath !== '/setup') {
    route('/setup', true);
  }

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
