// App.tsx — thin shell: mounts ProjectProvider, renders AppShell with route switching.
// No page-local state lives here. Target: under 100 lines.

import { useEffect, useState } from "react";
import { ProjectProvider, useProject } from "./state/ProjectContext";
import { isRouteEnabled } from "./app/capability";
import { DEFAULT_ROUTE, type AppRoute } from "./app/routes";
import AppShell from "./components/AppShell";
import ToastContainer from "./components/Toast";
import SourcesPage from "./pages/SourcesPage";
import ScanJobsPage from "./pages/ScanJobsPage";
import DuplicateResultsPage from "./pages/DuplicateResultsPage";
import GovernanceReviewPage from "./pages/GovernanceReviewPage";
import ExecutionCenterPage from "./pages/ExecutionCenterPage";
import AuditRecoveryPage from "./pages/AuditRecoveryPage";
import SettingsPage from "./pages/SettingsPage";

function renderPage(route: AppRoute) {
  switch (route) {
    case "sources": return <SourcesPage />;
    case "scan-jobs": return <ScanJobsPage />;
    case "duplicate-results": return <DuplicateResultsPage />;
    case "governance-review": return <GovernanceReviewPage />;
    case "execution-center": return <ExecutionCenterPage />;
    case "audit-recovery": return <AuditRecoveryPage />;
    case "settings": return <SettingsPage />;
    default: return <SourcesPage />;
  }
}

function AppContent() {
  const { capabilities } = useProject();
  const [activeRoute, setActiveRoute] = useState<AppRoute>(DEFAULT_ROUTE);

  // Reset to default route if current route becomes disabled (e.g. project closed)
  useEffect(() => {
    if (!isRouteEnabled(activeRoute, capabilities)) {
      setActiveRoute(DEFAULT_ROUTE);
    }
  }, [activeRoute, capabilities]);

  return (
    <AppShell activeRoute={activeRoute} onRouteChange={setActiveRoute}>
      {renderPage(activeRoute)}
    </AppShell>
  );
}

export default function App() {
  return (
    <ProjectProvider>
      <AppContent />
      <ToastBridge />
    </ProjectProvider>
  );
}

// ToastBridge: renders toasts from context at the app root level.
function ToastBridge() {
  const { toasts, dismissToast } = useProject();
  return <ToastContainer toasts={toasts} onDismiss={dismissToast} />;
}
