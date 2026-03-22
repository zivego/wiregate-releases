import { useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";

import {
  IconLayoutDashboard, IconRobot, IconWorld, IconTopologyStarRing3,
  IconKey, IconShieldCheck, IconCompass,
  IconFileText, IconTerminal2, IconDeviceDesktop,
  IconUser, IconBook2, IconUsers, IconRefresh,
} from "@tabler/icons-react";

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
  icon: ReactNode;
  accent: string;
  section?: string;
}

const SECTIONS = {
  core: "Overview",
  manage: "Management",
  security: "Security",
  system: "System",
} as const;

const ICON_SIZE = 18;
const ICON_STROKE = 1.8;

function buildItems(role?: string, restricted?: boolean): NavItem[] {
  if (restricted) {
    return [{ key: "account", to: "/account", label: "Account", icon: <IconUser size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#a78bfa", section: "system" }];
  }
  return [
    // Overview
    { key: "dashboard", to: "/dashboard", label: "Dashboard", icon: <IconLayoutDashboard size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#60a5fa", section: "core" },
    { key: "agents", to: "/agents", label: "Agents", icon: <IconRobot size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#34d399", section: "core" },
    { key: "peers", to: "/peers", label: "Peers", icon: <IconWorld size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#38bdf8", section: "core" },
    { key: "network", to: "/network", label: "Network", icon: <IconTopologyStarRing3 size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#818cf8", section: "core" },

    // Management
    { key: "tokens", to: "/enrollment-tokens", label: "Tokens", icon: <IconKey size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#fbbf24", section: "manage" },
    { key: "policies", to: "/access-policies", label: "Policies", icon: <IconShieldCheck size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#f472b6", section: "manage" },
    ...(role !== "readonly" ? [{ key: "dns" as const, to: "/dns", label: "DNS", icon: <IconCompass size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#2dd4bf", section: "manage" }] : []),

    // Security
    { key: "audit", to: "/audit-events", label: "Audit Log", icon: <IconFileText size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#fb923c", section: "security" },
    ...(role !== "readonly" ? [{ key: "logging" as const, to: "/logging", label: "Logging", icon: <IconTerminal2 size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#a3e635", section: "security" }] : []),
    { key: "sessions", to: "/sessions", label: "Sessions", icon: <IconDeviceDesktop size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#e879f9", section: "security" },

    // System
    { key: "account", to: "/account", label: "Account", icon: <IconUser size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#a78bfa", section: "system" },
    { key: "guide", to: "/guide", label: "Guide", icon: <IconBook2 size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#67e8f9", section: "system" },
    ...(role === "admin" ? [{ key: "users" as const, to: "/users", label: "Users", icon: <IconUsers size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#6ee7b7", section: "system" }] : []),
    ...(role === "admin" ? [{ key: "update" as const, to: "/system/update", label: "Update", icon: <IconRefresh size={ICON_SIZE} stroke={ICON_STROKE} />, accent: "#fca5a1", section: "system" }] : []),
  ];
}

export function HeaderNavMenu({ current, role, restricted = false }: HeaderNavMenuProps) {
  const items = buildItems(role, restricted);

  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.localStorage.getItem("wiregate:sidebar-collapsed") === "true";
  });
  const [moreOpen, setMoreOpen] = useState(false);
  const [hoveredKey, setHoveredKey] = useState<NavKey | null>(null);
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

  useEffect(() => { setMoreOpen(false); }, [current]);

  useEffect(() => {
    if (typeof document === "undefined") return;
    const body = document.body;
    body.classList.add("wg-shell-nav");
    body.style.setProperty("--wg-sidebar-offset", isMobile ? "0px" : collapsed ? "68px" : "240px");
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

  const sections = !restricted ? Object.entries(SECTIONS) : [["system", "System"] as const];
  const grouped = sections.map(([sectionKey, sectionLabel]) => ({
    label: sectionLabel,
    items: items.filter((item) => item.section === sectionKey),
  })).filter((g) => g.items.length > 0);

  const renderLink = (item: NavItem) => {
    const isActive = item.key === current;
    const isHovered = item.key === hoveredKey;
    const linkStyle: React.CSSProperties = {
      ...s.sidebarLink,
      ...(isActive ? { background: `${item.accent}18`, borderColor: `${item.accent}40` } : {}),
      ...(isHovered && !isActive ? { background: "var(--wg-surface-alt)" } : {}),
    };
    const iconStyle: React.CSSProperties = {
      ...s.sidebarIcon,
      color: isActive ? item.accent : "var(--wg-text-muted)",
      ...(isActive ? { background: `${item.accent}20`, boxShadow: `0 0 10px ${item.accent}20` } : {}),
    };
    const labelStyle: React.CSSProperties = {
      ...s.sidebarLabel,
      ...(isActive ? { color: item.accent, fontWeight: 600 } : {}),
    };
    return (
      <Link
        key={item.key}
        to={item.to}
        title={item.label}
        style={linkStyle}
        onMouseEnter={() => setHoveredKey(item.key)}
        onMouseLeave={() => setHoveredKey(null)}
      >
        <span style={iconStyle}>{item.icon}</span>
        {!collapsed && <span style={labelStyle}>{item.label}</span>}
      </Link>
    );
  };

  const desktopNav = (
    <aside style={collapsed ? { ...s.sidebar, ...s.sidebarCollapsed } : s.sidebar} aria-label="Primary navigation">
      <div style={s.sidebarTop}>
        <button
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          style={s.collapseBtn}
          title={collapsed ? "Expand" : "Collapse"}
        >
          {collapsed ? "\u276F" : "\u276E"}
        </button>
      </div>
      <nav style={s.sidebarNav}>
        {grouped.map((group, i) => (
          <div key={i}>
            {!collapsed && <div style={s.sectionLabel}>{group.label}</div>}
            <div style={s.sectionItems}>
              {group.items.map(renderLink)}
            </div>
          </div>
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
                style={item.key === current ? { ...s.moreLink, background: `${item.accent}18`, borderColor: `${item.accent}40`, color: item.accent } : s.moreLink}
              >
                <span style={{ marginRight: "0.5rem", color: item.key === current ? item.accent : "var(--wg-text-muted)" }}>{item.icon}</span>
                {item.label}
              </Link>
            ))}
          </div>
        </div>
      )}
      <nav style={s.mobileBar} aria-label="Mobile navigation">
        {primaryMobile.map((item) => (
          <Link
            key={item.key}
            to={item.to}
            style={item.key === current ? { ...s.mobileLink, color: item.accent, borderColor: `${item.accent}40` } : s.mobileLink}
          >
            <span style={{ color: item.key === current ? item.accent : "var(--wg-text-muted)" }}>{item.icon}</span>
            <span style={{ fontSize: "0.68rem" }}>{item.label}</span>
          </Link>
        ))}
        {overflowMobile.length > 0 && (
          <button
            type="button"
            onClick={() => setMoreOpen((v) => !v)}
            style={currentInOverflow || moreOpen ? { ...s.mobileLinkBtn, color: "var(--wg-text)" } : s.mobileLinkBtn}
          >
            <span style={{ fontSize: "1.1rem", letterSpacing: "2px" }}>{"\u2022\u2022\u2022"}</span>
            <span style={{ fontSize: "0.68rem" }}>More</span>
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
    width: 240,
    zIndex: 110,
    borderRight: "1px solid var(--wg-border-subtle)",
    background: "var(--wg-surface)",
    padding: "0.6rem 0.5rem",
    display: "grid",
    gridTemplateRows: "auto 1fr",
    gap: "0.5rem",
    overflowY: "auto",
  },
  sidebarCollapsed: { width: 68 },
  sidebarTop: {
    display: "flex",
    justifyContent: "flex-end",
    padding: "0.15rem 0.25rem",
  },
  collapseBtn: {
    border: "none",
    borderRadius: 8,
    background: "transparent",
    color: "var(--wg-text-muted)",
    cursor: "pointer",
    padding: "0.35rem 0.55rem",
    fontSize: "0.85rem",
    fontWeight: 600,
    transition: "background 150ms ease",
  },
  sidebarNav: {
    display: "grid",
    gap: "0.75rem",
    alignContent: "start",
  },
  sectionLabel: {
    fontSize: "0.65rem",
    fontWeight: 700,
    textTransform: "uppercase",
    letterSpacing: "0.08em",
    color: "var(--wg-text-muted)",
    padding: "0.4rem 0.6rem 0.2rem",
    fontFamily: "'Space Grotesk', sans-serif",
  },
  sectionItems: { display: "grid", gap: "2px" },
  sidebarLink: {
    display: "grid",
    gridTemplateColumns: "34px 1fr",
    alignItems: "center",
    gap: "0.5rem",
    color: "var(--wg-text)",
    textDecoration: "none",
    fontSize: "0.88rem",
    padding: "0.42rem 0.45rem",
    borderRadius: 10,
    border: "1px solid transparent",
    transition: "all 150ms ease",
    cursor: "pointer",
    fontFamily: "'Inter', sans-serif",
  },
  sidebarIcon: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    width: 34,
    height: 30,
    borderRadius: 8,
    transition: "all 150ms ease",
  },
  sidebarLabel: { transition: "color 150ms ease" },
  mobileBar: {
    position: "fixed",
    left: 0, right: 0, bottom: 0,
    zIndex: 210,
    borderTop: "1px solid var(--wg-border-subtle)",
    background: "var(--wg-surface)",
    display: "grid",
    gridTemplateColumns: "repeat(5, minmax(0, 1fr))",
    gap: "0.25rem",
    padding: "0.35rem 0.55rem max(0.35rem, env(safe-area-inset-bottom))",
  },
  mobileLink: {
    textDecoration: "none",
    color: "var(--wg-text-muted)",
    background: "transparent",
    border: "1px solid transparent",
    borderRadius: 10,
    padding: "0.35rem 0.25rem",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: "0.15rem",
    fontWeight: 600,
    transition: "all 150ms ease",
  },
  mobileLinkBtn: {
    color: "var(--wg-text-muted)",
    background: "transparent",
    border: "1px solid transparent",
    borderRadius: 10,
    padding: "0.35rem 0.25rem",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: "0.15rem",
    fontSize: "0.75rem",
    fontWeight: 600,
    cursor: "pointer",
    transition: "all 150ms ease",
  },
  moreSheet: {
    position: "fixed",
    left: 0, right: 0, bottom: "74px",
    zIndex: 220,
    background: "var(--wg-surface)",
    borderTop: "1px solid var(--wg-border-subtle)",
    boxShadow: "var(--wg-shadow)",
    padding: "0.75rem",
    display: "grid",
    gap: "0.5rem",
  },
  moreTitle: {
    fontSize: "0.72rem",
    textTransform: "uppercase",
    letterSpacing: "0.08em",
    color: "var(--wg-text-muted)",
    fontWeight: 700,
    padding: "0 0.15rem",
    fontFamily: "'Space Grotesk', sans-serif",
  },
  moreList: { display: "grid", gap: "0.35rem" },
  moreLink: {
    color: "var(--wg-text)",
    textDecoration: "none",
    border: "1px solid var(--wg-border-subtle)",
    borderRadius: 10,
    padding: "0.55rem 0.65rem",
    fontSize: "0.86rem",
    fontWeight: 600,
    display: "flex",
    alignItems: "center",
    transition: "all 150ms ease",
  },
};
