import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { HeaderBrand } from "../Brand";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import {
  SystemVersion,
  UpdateCheckResult,
  UpdateStatus,
  getSystemVersion,
  checkForUpdate,
  applyUpdate,
  getUpdateStatus,
  logout,
} from "../api";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

type Phase = "idle" | "checking" | "checked" | "updating" | "reconnecting" | "done" | "failed";

export function SystemUpdatePage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();
  const isAdmin = session?.role === "admin";

  const [phase, setPhase] = useState<Phase>("idle");
  const [version, setVersion] = useState<SystemVersion | null>(null);
  const [checkResult, setCheckResult] = useState<UpdateCheckResult | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [error, setError] = useState("");
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!isAdmin) {
      navigate("/dashboard", { replace: true });
      return;
    }
    let active = true;
    async function loadVersion() {
      try {
        const v = await getSystemVersion();
        if (active) setVersion(v);
      } catch {
        // version endpoint may not be available yet
      }
    }
    void loadVersion();
    return () => { active = false; };
  }, [isAdmin, navigate]);

  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  async function handleCheck() {
    setPhase("checking");
    setError("");
    setCheckResult(null);
    try {
      const result = await checkForUpdate();
      setCheckResult(result);
      setPhase("checked");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "check failed");
      setPhase("failed");
    }
  }

  async function handleApply() {
    if (!checkResult?.latest_version) return;
    setPhase("updating");
    setError("");
    try {
      const status = await applyUpdate(checkResult.latest_version);
      setUpdateStatus(status);
      startStatusPolling();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "update failed");
      setPhase("failed");
    }
  }

  function startStatusPolling() {
    if (pollRef.current) clearInterval(pollRef.current);
    let failCount = 0;
    pollRef.current = setInterval(async () => {
      try {
        const status = await getUpdateStatus();
        setUpdateStatus(status);
        if (status.state === "restarting") {
          // Server is about to die — switch to reconnect polling
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          startReconnectPolling();
        } else if (status.state === "failed") {
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          setError(status.message || "update failed");
          setPhase("failed");
        } else if (status.state === "idle") {
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          setPhase("done");
        }
        failCount = 0;
      } catch {
        failCount++;
        if (failCount >= 3) {
          // Server likely restarting — switch to reconnect
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          startReconnectPolling();
        }
      }
    }, 2000);
  }

  function startReconnectPolling() {
    setPhase("reconnecting");
    const previousVersion = version?.version;
    let attempts = 0;
    const maxAttempts = 60; // 2min at 2s intervals
    pollRef.current = setInterval(async () => {
      attempts++;
      try {
        const v = await getSystemVersion();
        // Server is back — check if version changed
        if (v.version !== previousVersion || attempts > 5) {
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          setVersion(v);
          setPhase("done");
        }
      } catch {
        if (attempts >= maxAttempts) {
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          setError("Server did not come back within 2 minutes. Check manually.");
          setPhase("failed");
        }
      }
    }, 2000);
  }

  async function handleLogout() {
    try {
      await logout();
    } catch { /* ignore */ }
    setSession(null);
    navigate("/login", { replace: true });
  }

  const stateLabel: Record<string, string> = {
    checking: "Checking for updates...",
    pulling: "Pulling new images...",
    restarting: "Restarting containers...",
    backing_up: "Creating backup...",
  };

  return (
    <>
      <header style={s.header}>
        <HeaderBrand />
        <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
          <ThemeToggleButton />
          <button type="button" onClick={handleLogout} style={s.logoutBtn}>Logout</button>
        </div>
      </header>
      <HeaderNavMenu current="update" role={session?.role} />
      <main style={s.main}>
        <h2 style={s.title}>System Update</h2>

        {/* Current version */}
        <div style={s.card}>
          <div style={s.cardTitle}>Current Version</div>
          {version ? (
            <div style={s.versionGrid}>
              <span style={s.label}>Version</span>
              <span style={s.mono}>{version.version}</span>
              <span style={s.label}>Commit</span>
              <span style={s.mono}>{version.commit_sha}</span>
              <span style={s.label}>Built</span>
              <span style={s.mono}>{version.build_time || "unknown"}</span>
            </div>
          ) : (
            <div style={s.muted}>Loading...</div>
          )}
        </div>

        {/* Check / result */}
        <div style={s.card}>
          <div style={s.cardTitle}>Update Check</div>

          {phase === "idle" && (
            <button type="button" onClick={handleCheck} style={s.primaryBtn}>
              Check for Updates
            </button>
          )}

          {phase === "checking" && (
            <div style={s.muted}>Checking for updates...</div>
          )}

          {phase === "checked" && checkResult && (
            <>
              {checkResult.update_available ? (
                <div style={s.updateAvailable}>
                  <div>
                    <strong>Update available:</strong>{" "}
                    <span style={s.mono}>{checkResult.latest_version}</span>
                  </div>
                  {checkResult.released_at && (
                    <div style={s.muted}>Released: {checkResult.released_at}</div>
                  )}
                  {checkResult.changelog_url && (
                    <div>
                      <a href={checkResult.changelog_url} target="_blank" rel="noopener noreferrer" style={s.link}>
                        View changelog
                      </a>
                    </div>
                  )}
                  <button type="button" onClick={handleApply} style={{ ...s.primaryBtn, marginTop: "0.75rem" }}>
                    Update Now
                  </button>
                </div>
              ) : (
                <div>
                  <div style={{ color: "#1e8449", fontWeight: 600, marginBottom: "0.5rem" }}>
                    You are running the latest version.
                  </div>
                  <button type="button" onClick={handleCheck} style={s.secondaryBtn}>
                    Check Again
                  </button>
                </div>
              )}
            </>
          )}

          {(phase === "updating" || phase === "reconnecting") && (
            <div style={s.progressSection}>
              <div style={s.progressTitle}>
                {phase === "reconnecting"
                  ? "Waiting for server to restart..."
                  : stateLabel[updateStatus?.state ?? ""] ?? "Updating..."}
              </div>
              <div style={s.progressBar}>
                <div style={{
                  ...s.progressFill,
                  width: phase === "reconnecting" ? "85%" : updateStatus?.state === "pulling" ? "40%" : "60%",
                }} />
              </div>
              {updateStatus?.message && (
                <div style={s.muted}>{updateStatus.message}</div>
              )}
            </div>
          )}

          {phase === "done" && (
            <div>
              <div style={{ color: "#1e8449", fontWeight: 600, marginBottom: "0.5rem" }}>
                Update complete! Server is running version{" "}
                <span style={s.mono}>{version?.version ?? "unknown"}</span>.
              </div>
              <button type="button" onClick={() => { setPhase("idle"); setCheckResult(null); }} style={s.secondaryBtn}>
                Done
              </button>
            </div>
          )}

          {phase === "failed" && (
            <div>
              <div style={{ color: "#c0392b", fontWeight: 600, marginBottom: "0.5rem" }}>
                {error || "Update failed"}
              </div>
              <button type="button" onClick={() => { setPhase("idle"); setError(""); }} style={s.secondaryBtn}>
                Try Again
              </button>
            </div>
          )}
        </div>
      </main>
    </>
  );
}

const s: Record<string, React.CSSProperties> = {
  header: {
    position: "fixed",
    top: 0,
    left: 0,
    right: 0,
    height: 56,
    zIndex: 200,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    padding: "0 1.2rem",
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    borderBottom: `1px solid ${uiTheme.border}`,
    boxShadow: uiTheme.shadow,
  },
  logoutBtn: {
    background: "transparent",
    color: uiTheme.headerText,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 6,
    padding: "0.35rem 0.75rem",
    cursor: "pointer",
    fontSize: "0.82rem",
    fontWeight: 600,
  },
  main: {
    padding: "5rem 1.5rem 2rem calc(var(--wg-sidebar-offset, 224px) + 1.5rem)",
    maxWidth: 700,
  },
  title: {
    fontSize: "1.35rem",
    fontWeight: 700,
    marginBottom: "1.2rem",
    color: uiTheme.text,
  },
  card: {
    background: uiTheme.surface,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 10,
    padding: "1.2rem",
    marginBottom: "1rem",
    boxShadow: uiTheme.shadow,
  },
  cardTitle: {
    fontSize: "0.95rem",
    fontWeight: 700,
    marginBottom: "0.75rem",
    color: uiTheme.text,
  },
  versionGrid: {
    display: "grid",
    gridTemplateColumns: "90px 1fr",
    gap: "0.35rem 0.75rem",
    fontSize: "0.88rem",
  },
  label: {
    color: uiTheme.textMuted,
    fontWeight: 600,
  },
  mono: {
    fontFamily: "monospace",
    fontSize: "0.86rem",
  },
  muted: {
    color: uiTheme.textMuted,
    fontSize: "0.86rem",
  },
  primaryBtn: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 8,
    padding: "0.55rem 1.2rem",
    cursor: "pointer",
    fontWeight: 600,
    fontSize: "0.88rem",
  },
  secondaryBtn: {
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 8,
    padding: "0.55rem 1.2rem",
    cursor: "pointer",
    fontWeight: 600,
    fontSize: "0.88rem",
  },
  link: {
    color: "#1f6feb",
    textDecoration: "underline",
    fontSize: "0.86rem",
  },
  updateAvailable: {
    display: "grid",
    gap: "0.35rem",
  },
  progressSection: {
    display: "grid",
    gap: "0.5rem",
  },
  progressTitle: {
    fontWeight: 600,
    fontSize: "0.92rem",
    color: uiTheme.text,
  },
  progressBar: {
    height: 8,
    borderRadius: 4,
    background: uiTheme.surfaceAlt,
    overflow: "hidden",
  },
  progressFill: {
    height: "100%",
    borderRadius: 4,
    background: "#1f6feb",
    transition: "width 0.5s ease",
  },
};
