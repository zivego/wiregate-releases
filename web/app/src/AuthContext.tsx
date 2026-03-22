import React, { createContext, useContext, useEffect, useState } from "react";
import { getCurrentSession, SessionResponse } from "./api";

interface AuthContextValue {
  session: SessionResponse | null;
  setSession: (s: SessionResponse | null) => void;
  loading: boolean;
}

const AuthContext = createContext<AuthContextValue>({
  session: null,
  setSession: () => {},
  loading: true,
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [session, setSession] = useState<SessionResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getCurrentSession()
      .then(setSession)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  return (
    <AuthContext.Provider value={{ session, setSession, loading }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
