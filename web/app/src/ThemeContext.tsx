import React, { createContext, useContext, useEffect, useRef, useState } from "react";
import { updateCurrentUserPreferences } from "./api";
import { useAuth } from "./AuthContext";
import { ThemePreference } from "./uiTheme";

interface ThemeContextValue {
  theme: ThemePreference;
  saving: boolean;
  toggleTheme: () => Promise<void>;
}

const ThemeContext = createContext<ThemeContextValue>({
  theme: "light",
  saving: false,
  toggleTheme: async () => {},
});

const THEME_STORAGE_KEY = "wiregate:theme-preference";

function readStoredTheme(): ThemePreference | null {
  if (typeof window === "undefined") {
    return null;
  }
  const value = window.localStorage.getItem(THEME_STORAGE_KEY);
  return value === "light" || value === "dark" ? value : null;
}

function writeStoredTheme(theme: ThemePreference) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(THEME_STORAGE_KEY, theme);
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const { session, setSession } = useAuth();
  const [saving, setSaving] = useState(false);
  const hasStoredThemeRef = useRef(readStoredTheme() !== null);
  const [theme, setTheme] = useState<ThemePreference>(() => readStoredTheme() ?? "dark");

  useEffect(() => {
    if (!hasStoredThemeRef.current && session?.theme_preference) {
      setTheme(session.theme_preference);
    }
  }, [session?.theme_preference]);

  useEffect(() => {
    writeStoredTheme(theme);
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  async function toggleTheme() {
    if (saving) {
      return;
    }

    const previousSession = session;
    const previousTheme = theme;
    const nextTheme: ThemePreference = theme === "light" ? "dark" : "light";

    setSaving(true);
    setTheme(nextTheme);
    if (!session) {
      setSaving(false);
      return;
    }
    setSession({ ...session, theme_preference: nextTheme });
    try {
      const updated = await updateCurrentUserPreferences({ theme_preference: nextTheme });
      setSession({ ...session, theme_preference: updated.theme_preference });
      setTheme(updated.theme_preference);
    } catch {
      if (previousSession) {
        setSession(previousSession);
      }
      setTheme(previousTheme);
      throw new Error("failed to update theme preference");
    } finally {
      setSaving(false);
    }
  }

  return (
    <ThemeContext.Provider value={{ theme, saving, toggleTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}
