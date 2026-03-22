import { Navigate, Route, Routes } from "react-router-dom";
import { LoginPage } from "./pages/LoginPage";
import { Dashboard } from "./pages/Dashboard";
import { AgentsPage } from "./pages/AgentsPage";
import { PeersPage } from "./pages/PeersPage";
import { PeerDetailPage } from "./pages/PeerDetailPage";
import { EnrollmentTokensPage } from "./pages/EnrollmentTokensPage";
import { AccessPoliciesPage } from "./pages/AccessPoliciesPage";
import { AuditEventsPage } from "./pages/AuditEventsPage";
import { SessionsPage } from "./pages/SessionsPage";
import { LoggingPage } from "./pages/LoggingPage";
import { DNSPage } from "./pages/DNSPage";
import { NetworkPage } from "./pages/NetworkPage";
import { UsersPage } from "./pages/UsersPage";
import { AccountPage } from "./pages/AccountPage";
import { GuidePage } from "./pages/GuidePage";
import { SystemUpdatePage } from "./pages/SystemUpdatePage";
import { ProtectedRoute } from "./ProtectedRoute";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/dashboard" replace />} />
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute>
            <Dashboard />
          </ProtectedRoute>
        }
      />
      <Route
        path="/agents"
        element={
          <ProtectedRoute>
            <AgentsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/peers"
        element={
          <ProtectedRoute>
            <PeersPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/peers/:peerId"
        element={
          <ProtectedRoute>
            <PeerDetailPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/enrollment-tokens"
        element={
          <ProtectedRoute>
            <EnrollmentTokensPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/access-policies"
        element={
          <ProtectedRoute>
            <AccessPoliciesPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/audit-events"
        element={
          <ProtectedRoute>
            <AuditEventsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/sessions"
        element={
          <ProtectedRoute>
            <SessionsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/logging"
        element={
          <ProtectedRoute>
            <LoggingPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/dns"
        element={
          <ProtectedRoute>
            <DNSPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/network"
        element={
          <ProtectedRoute>
            <NetworkPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/users"
        element={
          <ProtectedRoute>
            <UsersPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/account"
        element={
          <ProtectedRoute>
            <AccountPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/guide"
        element={
          <ProtectedRoute>
            <GuidePage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/system/update"
        element={
          <ProtectedRoute>
            <SystemUpdatePage />
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
