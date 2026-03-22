import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { uiTheme } from "./uiTheme";

type NavKey = "dashboard" | "agents" | "peers" | "network" | "tokens" | "policies" | "audit" | "logging" | "dns" | "sessions" | "account" | "guide" | "users" | "update";

interface HeaderNavMenuProps {
  current: NavKey;
  role?: string;
  restricted?: boolean;
}

interface NavItem {
  key: NavKey;
  to: string;
  label: string;
  short: string;
}

export function HeaderNavMenu({ current, role, restricted = false }: HeaderNavMenuProps) {
  const items: NavItem[] = restricted
    ? [{ key: "account", to: "/account", label: "Account", short: "AC" }]
    : [
        { key: "dashboard", to: "/dashboard", label: "Dashboard", short: "DB" },
        { key: "agents", to: "/agents", label: "Agents", short: "AG" },
        { key: "peers", to: "/peers", label: "Peers", short: "PR" },
        { key: "network", to: "/network", label: "Network", short: "NW" },
        { key: "tokens", to: "/enrollment-tokens", label: "Tokens", short: "TK" },
        { key: "policies", to: "/access-policies", label: "Policies", short: "PL" },
        { key: "audit", to: "/audit-events", label: "Audit", short: "AU" },
        ...(role !== "readonly" ? [{ key: "dns" as const, to: "/dns", label: "DNS", short: "DN" }] : []),
        ...(role !== "readonly" ? [{ key: "logging" as const, to: "/logging", label: "Logging", short: "LG" }] : []),
        { key: "sessions", to: "/sessions", label: "Sessions", short: "SS" },
        { key: "account", to: "/account", label: "Account", short: "AC" },
        { key: "guide", to: "/guide", label: "Guide", short: "GD" },
        ...(role === "admin" ? [{ key: "users" as const, to: "/users", label: "Users", short: "US" }] : []),
        ...(role === "admin" ? [{ key: "update" as const, to: "/system/update", label: "Update", short: "UP" }] : []),
      ];
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.localStorage.getItem("wiregate:sidebar-collapsed") === "true";
  });
  const [moreOpen, setMoreOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.matchMedia("(max-width: 960px)").matches;
  });

  useEffect(() => {
    if (typeof window === "undefined") return;
    const media = window.matchMedia("(max-width: 960px)");
    const update = () => setIsMobile(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem("wiregate:sidebar-collapsed", String(collapsed));
  }, [collapsed]);

  useEffect(() => {
    setMoreOpen(false);
  }, [current]);

  useEffect(() => {
    if (typeof document === "undefined") return;
    const body = document.body;
    body.classList.add("wg-shell-nav");
    body.style.setProperty("--wg-sidebar-offset", isMobile ? "0px" : collapsed ? "88px" : "224px");
    body.style.setProperty("--wg-nav-bottom-space", isMobile ? "74px" : "0px");
    if (!isMobile && collapsed) {
      body.classList.add("wg-shell-nav-collapsed");
    } else {
      body.classList.remove("wg-shell-nav-collapsed");
    }
    return () => {
      body.classList.remove("wg-shell-nav");
      body.classList.remove("wg-shell-nav-collapsed");
      body.style.removeProperty("--wg-sidebar-offset");
      body.style.removeProperty("--wg-nav-bottom-space");
    };
  }, [collapsed, isMobile]);

  const primaryMobileKeys: NavKey[] = restricted ? ["account"] : ["dashboard", "agents", "peers", "policies"];
  const primaryMobile = items.filter((item) => primaryMobileKeys.includes(item.key));
  const overflowMobile = items.filter((item) => !primaryMobileKeys.includes(item.key));
  const currentInOverflow = overflowMobile.some((item) => item.key === current);

  const desktopNav = (
    <aside style={collapsed ? { ...s.sidebar, ...s.sidebarCollapsed } : s.sidebar} aria-label="Primary navigation">
      <div style={s.sidebarTop}>
        <button
          type="button"
          onClick={() => setCollapsed((value) => !value)}
          style={s.collapseBtn}
          title={collapsed ? "Expand navigation" : "Collapse navigation"}
        >
          {collapsed ? ">>" : "<<"}
        </button>
      </div>
      <nav style={s.sidebarNav}>
        {items.map((item) => (
          <Link
            key={item.key}
            to={item.to}
            title={item.label}
            style={item.key === current ? { ...s.sidebarLink, ...s.sidebarLinkActive } : s.sidebarLink}
          >
            <span style={s.sidebarShort}>{item.short}</span>
            {!collapsed && <span>{item.label}</span>}
          </Link>
        ))}
      </nav>
    </aside>
  );

  const mobileNav = (
    <>
      {moreOpen && (
        <div style={s.moreSheet}>
          <div style={s.moreTitle}>More</div>
          <div style={s.moreList}>
            {overflowMobile.map((item) => (
              <Link
                key={item.key}
                to={item.to}
                style={item.key === current ? { ...s.moreLink, ...s.moreLinkActive } : s.moreLink}
              >
                {item.label}
              </Link>
            ))}
          </div>
        </div>
      )}
      <nav style={s.mobileBar} aria-label="Mobile navigation">
        {primaryMobile.map((item) => (
          <Link key={item.key} to={item.to} style={item.key === current ? { ...s.mobileLink, ...s.mobileLinkActive } : s.mobileLink}>
            {item.label}
          </Link>
        ))}
        {overflowMobile.length > 0 && (
          <button type="button" onClick={() => setMoreOpen((value) => !value)} style={currentInOverflow || moreOpen ? { ...s.mobileLinkBtn, ...s.mobileLinkActive } : s.mobileLinkBtn}>
            More
          </button>
        )}
      </nav>
    </>
  );

  return isMobile ? mobileNav : desktopNav;
}

const s: Record<string, React.CSSProperties> = {
  sidebar: {
    position: "fixed",
    top: 56,
    left: 0,
    bottom: 0,
    width: 224,
    zIndex: 110,
    borderRight: `1px solid ${uiTheme.border}`,
    background: uiTheme.surface,
    boxShadow: uiTheme.shadow,
    padding: "0.8rem 0.65rem",
    display: "grid",
    gridTemplateRows: "auto 1fr",
    gap: "0.8rem",
  },
  sidebarCollapsed: {
    width: 88,
  },
  sidebarTop: {
    display: "flex",
    justifyContent: "flex-end",
  },
  collapseBtn: {
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 6,
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
    cursor: "pointer",
    padding: "0.25rem 0.5rem",
    fontSize: "0.78rem",
    fontWeight: 600,
  },
  sidebarNav: {
    display: "grid",
    gap: "0.4rem",
    alignContent: "start",
  },
  sidebarLink: {
    display: "grid",
    gridTemplateColumns: "30px 1fr",
    alignItems: "center",
    gap: "0.45rem",
    color: uiTheme.text,
    textDecoration: "none",
    fontSize: "0.9rem",
    padding: "0.45rem 0.5rem",
    borderRadius: 8,
    border: `1px solid transparent`,
  },
  sidebarLinkActive: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    borderColor: uiTheme.border,
    fontWeight: 600,
  },
  sidebarShort: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    width: 30,
    height: 24,
    borderRadius: 6,
    background: uiTheme.surfaceAlt,
    fontSize: "0.72rem",
    fontWeight: 700,
    letterSpacing: 0.4,
  },
  mobileBar: {
    position: "fixed",
    left: 0,
    right: 0,
    bottom: 0,
    zIndex: 210,
    borderTop: `1px solid ${uiTheme.border}`,
    background: uiTheme.surface,
    boxShadow: uiTheme.shadow,
    display: "grid",
    gridTemplateColumns: "repeat(5, minmax(0, 1fr))",
    gap: "0.3rem",
    padding: "0.45rem 0.55rem max(0.45rem, env(safe-area-inset-bottom))",
  },
  mobileLink: {
    textDecoration: "none",
    color: uiTheme.textMuted,
    background: "transparent",
    border: `1px solid transparent`,
    borderRadius: 8,
    padding: "0.5rem 0.35rem",
    fontSize: "0.75rem",
    textAlign: "center",
    fontWeight: 600,
  },
  mobileLinkBtn: {
    color: uiTheme.textMuted,
    background: "transparent",
    border: `1px solid transparent`,
    borderRadius: 8,
    padding: "0.5rem 0.35rem",
    fontSize: "0.75rem",
    textAlign: "center",
    fontWeight: 600,
    cursor: "pointer",
  },
  mobileLinkActive: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    borderColor: uiTheme.border,
  },
  moreSheet: {
    position: "fixed",
    left: 0,
    right: 0,
    bottom: "74px",
    zIndex: 220,
    background: uiTheme.surface,
    borderTop: `1px solid ${uiTheme.border}`,
    boxShadow: uiTheme.shadow,
    padding: "0.75rem",
    display: "grid",
    gap: "0.6rem",
  },
  moreTitle: {
    fontSize: "0.78rem",
    textTransform: "uppercase",
    letterSpacing: 0.6,
    color: uiTheme.textMuted,
    fontWeight: 700,
  },
  moreList: {
    display: "grid",
    gap: "0.45rem",
  },
  moreLink: {
    color: uiTheme.text,
    textDecoration: "none",
    border: `1px solid ${uiTheme.border}`,
    borderRadius: 8,
    padding: "0.6rem 0.7rem",
    fontSize: "0.86rem",
    fontWeight: 600,
  },
  moreLinkActive: {
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
  },
};
