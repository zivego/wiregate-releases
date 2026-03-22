import { Navigate, useLocation } from "react-router-dom";
import { useAuth } from "./AuthContext";

export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { session, loading } = useAuth();
  const location = useLocation();

  if (loading) {
    return <div style={{ padding: "2rem", fontFamily: "sans-serif" }}>Loading...</div>;
  }

  if (!session) {
    return <Navigate to="/login" replace />;
  }

  if (session.must_change_password && location.pathname !== "/account") {
    return <Navigate to="/account" replace />;
  }

  return <>{children}</>;
}
