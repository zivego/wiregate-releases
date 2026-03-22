import { useNavigate } from "react-router-dom";
import { logout } from "../api";
import { useAuth } from "../AuthContext";
import { HeaderNavMenu } from "../HeaderNavMenu";
import { HeaderBrand } from "../Brand";
import { ThemeToggleButton } from "../ThemeToggleButton";
import { uiTheme } from "../uiTheme";

type GuideRole = "admin" | "operator" | "readonly";

interface PageGuideSection {
  id: string;
  title: string;
  route?: string;
  access: string;
  visibleTo: GuideRole[];
  purpose: string;
  mainData: string[];
  actions: string[];
  workflows: string[];
  notes: string[];
  related: string[];
}

interface WorkflowSection {
  id: string;
  title: string;
  summary: string;
  steps: string[];
  notes: string[];
}

interface OpsSection {
  id: string;
  title: string;
  summary: string;
  bullets: string[];
}

interface TroubleSection {
  id: string;
  symptom: string;
  whereToLook: string[];
  resolution: string[];
}

const allRoles: GuideRole[] = ["admin", "operator", "readonly"];
const adminOperator: GuideRole[] = ["admin", "operator"];
const adminOnly: GuideRole[] = ["admin"];

const pageSections: PageGuideSection[] = [
  {
    id: "dashboard-page",
    title: "Dashboard",
    route: "/dashboard",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Start here for control-plane health, analytics, inventory summaries, and log-delivery visibility.",
    mainData: [
      "Live health, reconcile health, and high-level inventory counts",
      "Auth and security event trends over 24h, 7d, or 30d",
      "Enrollment funnel, policy coverage, and top failing agents",
      "SIEM delivery health widgets for admin and operator",
    ],
    actions: [
      "Change analytics range and auto-refresh interval",
      "Use charts and summaries to decide which operational page to open next",
    ],
    workflows: [
      "Confirm whether a rollout increased drift, apply failures, or export failures",
      "Spot enrollment problems before operators report them manually",
    ],
    notes: [
      "Dashboard charts are operational summaries, not the authoritative security record.",
      "Audit remains the source of truth for who did what and when.",
    ],
    related: ["Agents", "Peers", "Audit", "Logging & SIEM"],
  },
  {
    id: "agents-page",
    title: "Agents",
    route: "/agents",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Manage enrolled endpoints, lifecycle state, policy overrides, gateway mode, and replacement flows.",
    mainData: [
      "Hostname, platform, status, and creation time",
      "Online state, last seen timestamp, version, and last apply fingerprint",
      "Traffic mode override, gateway mode, and linked peer summary",
    ],
    actions: [
      "Filter by status and platform, search by hostname or ID",
      "Disable or enable an agent",
      "Revoke an enrolled identity",
      "Reissue enrollment for a revoked agent",
      "Rotate the agent's WireGuard key pair",
      "Change traffic mode override or gateway mode",
    ],
    workflows: [
      "Use Disable for temporary access cutoff and Enable to restore it later",
      "Use Revoke when the endpoint is retired, lost, or no longer trusted",
      "Use Reissue after Revoke to generate a replacement enrollment token",
      "Use Rotate key when the same logical agent should stay in place but its key material must change",
    ],
    notes: [
      "Reissue does not reactivate the revoked agent. It creates a fresh enrollment token for replacement enrollment.",
      "Rotate key keeps the same logical agent and replaces the WireGuard key pair.",
      "If dual approval is enabled, sensitive actions may return a pending approval instead of changing state immediately.",
      "Readonly users can inspect agents but cannot run lifecycle or mode-changing actions.",
    ],
    related: ["Enrollment Tokens", "Access Policies", "Peers", "Network", "Audit"],
  },
  {
    id: "peers-page",
    title: "Peers",
    route: "/peers",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Inspect policy-rendered WireGuard intent, runtime state, and drift across enrolled peers.",
    mainData: [
      "Peer public key, assigned address, and rendered AllowedIPs",
      "Runtime AllowedIPs, status, and drift summary",
      "Linked agent ID and hostname",
    ],
    actions: [
      "Search and filter peers by status or owning agent",
      "Select one or more peers for reconcile",
      "Open Peer Detail for a single peer investigation",
    ],
    workflows: [
      "Compare intended routes to runtime routes before forcing changes",
      "Bulk reconcile peers after policy or network changes",
    ],
    notes: [
      "AllowedIPs are rendered server-side from policy intent. The client does not choose its own access rights.",
      "Only admin and operator can reconcile peers.",
    ],
    related: ["Peer Detail", "Access Policies", "Agents", "Audit"],
  },
  {
    id: "peer-detail-page",
    title: "Peer Detail",
    route: "/peers/:peerId",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Drill down into a single peer when the inventory page shows drift, planning state, or a runtime mismatch.",
    mainData: [
      "Peer identity and linked agent",
      "Desired AllowedIPs versus runtime AllowedIPs",
      "Status and drift state in one view",
    ],
    actions: [
      "Reconcile the individual peer if you have operator or admin access",
    ],
    workflows: [
      "Validate whether the issue is a server-side intent mismatch or an agent-side apply mismatch",
      "Confirm whether a reconcile attempt actually corrected the runtime state",
    ],
    notes: [
      "Reconcile is blocked for disabled or revoked peers.",
      "Persistent drift usually means the native agent path or WireGuard runtime needs investigation.",
    ],
    related: ["Peers", "Agents", "Audit"],
  },
  {
    id: "tokens-page",
    title: "Enrollment Tokens",
    route: "/enrollment-tokens",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Issue, inspect, and revoke bootstrap tokens for new or replacement agent enrollment.",
    mainData: [
      "Token model, scope, TTL, and bound identity",
      "Attached access policies and lifecycle status",
      "Issue, use, and revoke timestamps",
    ],
    actions: [
      "Create a new Model A or Model B token",
      "Bind Model B tokens to a future identity",
      "Attach policies during issuance",
      "Copy generated Linux and Windows install commands",
      "Revoke leaked or unused tokens",
    ],
    workflows: [
      "Prefer Model B for known future hosts and pre-authorized access",
      "Use Model A only when the future host cannot be bound in advance",
      "Issue a replacement token after Reissue on a revoked agent",
    ],
    notes: [
      "Raw token material is shown only once at creation time.",
      "Model B is the preferred path; Model A remains available with higher misuse risk.",
      "Readonly users can inspect token history but cannot create or revoke tokens.",
    ],
    related: ["Agents", "Access Policies", "Audit"],
  },
  {
    id: "policies-page",
    title: "Access Policies",
    route: "/access-policies",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Define destination networks, assign policy intent to agents, and simulate rendered peer access before rollout.",
    mainData: [
      "Policy name, description, traffic mode, and destination CIDRs",
      "Assigned agents and recent assignment activity",
      "Simulation results for effective traffic mode and rendered routes",
    ],
    actions: [
      "Create or edit a policy",
      "Assign or unassign policies to agents",
      "Simulate policy output before changing live endpoints",
    ],
    workflows: [
      "Build standard routed access around explicit CIDRs",
      "Use full_tunnel only when all traffic should route through the WireGuard path",
      "Preview policy intent with the simulator before applying a change to a live agent",
    ],
    notes: [
      "Policy rendering is authoritative and server-side.",
      "Readonly users can inspect policy state but cannot change it.",
      "Policy changes can move peers back into a planned state until reconcile completes.",
    ],
    related: ["Agents", "Peers", "Audit"],
  },
  {
    id: "audit-page",
    title: "Audit",
    route: "/audit-events",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Review the authoritative security trail and supporting analytics for authentication, lifecycle, and configuration changes.",
    mainData: [
      "Event list with actor, action, resource, result, and metadata",
      "Audit trend, action distribution, and hourly activity heatmap",
      "Delivery-failure overlay for admin and operator",
    ],
    actions: [
      "Filter by action, resource type, result, or actor",
      "Use quick presets to pivot by sessions, users, enrollment, or policies",
      "Expand metadata for deeper event context",
    ],
    workflows: [
      "Trace who revoked, reissued, rotated, or reassigned something",
      "Confirm whether a failure happened in the control plane or only in log export",
    ],
    notes: [
      "Audit is the authoritative security history. Exported runtime logs do not replace it.",
      "Delivery issues describe problems leaving the process, not missing audit writes.",
    ],
    related: ["Sessions", "Dashboard", "Logging & SIEM", "Agents", "Access Policies"],
  },
  {
    id: "sessions-page",
    title: "Sessions",
    route: "/sessions",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Inspect active browser and API sessions, review device metadata, and force logout when needed.",
    mainData: [
      "Session ID, auth provider, and current-session marker",
      "Issued, last-seen, and expiry timestamps",
      "Source IP and user agent metadata",
    ],
    actions: [
      "Revoke a session",
      "Load older session history",
    ],
    workflows: [
      "Force logout after a suspected session leak",
      "Confirm whether a session came from password login or OIDC login",
    ],
    notes: [
      "Admin can inspect and revoke all sessions.",
      "Operator and readonly users are limited to their own sessions.",
      "Session expiry and idle timeout are enforced even if a browser tab remains open.",
    ],
    related: ["Account", "Audit"],
  },
  {
    id: "logging-page",
    title: "Logging & SIEM",
    route: "/logging",
    access: "admin full access, operator read-only, readonly hidden",
    visibleTo: adminOperator,
    purpose: "Configure and monitor the runtime log export pipeline, sink health, redaction policy, and delivery status.",
    mainData: [
      "Configured sinks, transports, and formats",
      "Route rules by category and minimum severity",
      "Queue capacity, current backlog, recent failures, and dead letters",
      "Redacted field summary and last error details",
    ],
    actions: [
      "Create, edit, enable, disable, or delete sinks",
      "Change route rules",
      "Run test delivery",
      "Inspect retries, drops, and delivery failures",
    ],
    workflows: [
      "Configure a syslog sink for a collector or SIEM",
      "Validate route and transport settings with a synthetic event",
      "Investigate queue pressure before export failures become persistent",
    ],
    notes: [
      "Admin is the only role that can edit sinks or routes.",
      "Operator access is read-only and focuses on health and diagnostics.",
      "Readonly users do not see this page.",
      "The export pipeline redacts secret material before queueing and delivery.",
      "Runtime/export logs are not the same as the authoritative audit trail.",
    ],
    related: ["Dashboard", "Audit"],
  },
  {
    id: "dns-page",
    title: "DNS",
    route: "/dns",
    access: "admin edit, operator read-only, readonly hidden",
    visibleTo: adminOperator,
    purpose: "Manage the DNS settings that become part of desired WireGuard config for enrolled agents.",
    mainData: [
      "Managed DNS enablement state",
      "Resolver IPs and search domains",
      "Rendered preview of the DNS line in the future agent config",
    ],
    actions: [
      "Enable or disable managed DNS",
      "Add or remove resolver IPs",
      "Add or remove search domains",
    ],
    workflows: [
      "Publish internal resolvers and search domains for tunnel-connected hosts",
      "Preview desired-state rendering before affecting live endpoints",
    ],
    notes: [
      "Admin can edit DNS settings; operator can review them only.",
      "Readonly users do not see this page.",
      "DNS changes affect desired state and may require a later agent reconfigure.",
    ],
    related: ["Agents", "Network"],
  },
  {
    id: "network-page",
    title: "Network",
    route: "/network",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Inspect route profile, path mode, gateway role readiness, and route conflicts across the fleet.",
    mainData: [
      "Route profile per agent",
      "Path mode and relay visibility",
      "Gateway assignment status and conflict details",
      "Network summary counts",
    ],
    actions: [
      "Read diagnostics and use them to decide whether to change traffic or gateway settings elsewhere",
    ],
    workflows: [
      "Validate gateway readiness before changing a routing role",
      "Spot route conflicts before changing policies or gateway mode",
    ],
    notes: [
      "This page is diagnostic only. Gateway mode changes happen from the Agents page.",
      "Current diagnostics are control-plane-first and may still reflect direct-only behavior where relay runtime is not active.",
    ],
    related: ["Agents", "Peers", "DNS"],
  },
  {
    id: "users-page",
    title: "Users",
    route: "/users",
    access: "admin only",
    visibleTo: adminOnly,
    purpose: "Manage local control-plane users and their role assignments.",
    mainData: [
      "User email and assigned role",
      "Create, edit, reset-password, and delete controls",
    ],
    actions: [
      "Create a new user",
      "Edit a user's email or role",
      "Reset another user's password",
      "Delete a user",
    ],
    workflows: [
      "Onboard a new admin, operator, or readonly user",
      "Promote or demote user access based on operational responsibility",
      "Reset credentials after lockout or suspected password compromise",
    ],
    notes: [
      "Password reset revokes the target user's active sessions.",
      "Deleting a user does not remove historical audit evidence.",
      "This page is intentionally hidden from non-admin roles.",
    ],
    related: ["Sessions", "Audit", "Account"],
  },
  {
    id: "account-page",
    title: "Account",
    route: "/account",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Handle self-service identity tasks for the currently signed-in user.",
    mainData: [
      "Current identity context",
      "Password change form",
    ],
    actions: [
      "Change your own password",
      "Log out",
    ],
    workflows: [
      "Complete the first-login password change",
      "Rotate your password after a suspected account compromise",
    ],
    notes: [
      "Changing your password revokes your active sessions.",
      "Expect to sign in again from other browsers or devices after password change.",
    ],
    related: ["Sessions", "Audit"],
  },
  {
    id: "guide-page",
    title: "Guide",
    route: "/guide",
    access: "admin, operator, readonly",
    visibleTo: allRoles,
    purpose: "Use the in-app reader for a condensed version of the operator manual without leaving the admin panel.",
    mainData: [
      "Platform overview and core terms",
      "Page-by-page guidance",
      "Cross-page workflows",
      "Operations references and troubleshooting index",
    ],
    actions: [
      "Jump between anchors",
      "Use role notes to understand what your current account can and cannot do",
    ],
    workflows: [
      "Orient a new operator quickly",
      "Confirm the meaning of lifecycle actions before using them",
    ],
    notes: [
      "The canonical source of truth lives in docs at docs/implementation/SYSTEM_GUIDE.md.",
      "Dedicated runbooks still own host deployment, release operations, and native agent installation details.",
    ],
    related: ["All pages"],
  },
];

const workflowSections: WorkflowSection[] = [
  {
    id: "first-login",
    title: "First Login and Password Change",
    summary: "Bootstrap or newly created users may need to complete a password change before they can operate the system normally.",
    steps: [
      "Sign in through /login with your assigned credentials.",
      "If password change is required, complete it before trying to use other pages.",
      "Re-authenticate after the password update if your session is invalidated.",
      "Return to Dashboard or Guide after the account is in a normal state.",
    ],
    notes: [
      "Password change revokes the active session set for that user.",
      "While must-change-password is active, most control-plane actions remain blocked on purpose.",
    ],
  },
  {
    id: "enrollment-models",
    title: "Enrollment Flow: Model B Preferred, Model A Available",
    summary: "Enrollment tokens bootstrap new or replacement agents. Model B is the safer and preferred path because it binds the token to a future identity.",
    steps: [
      "Create a Model B token when you already know the future host identity.",
      "Attach access policies during issuance whenever possible.",
      "Copy the generated Linux or Windows installation command and run it on the endpoint.",
      "Confirm the new agent in Agents and its peer in Peers after successful enrollment.",
      "Use Model A only when pre-binding is not practical, and keep TTL short.",
    ],
    notes: [
      "Raw token material is shown only once.",
      "If a token leaks, revoke it immediately from Enrollment Tokens.",
    ],
  },
  {
    id: "lifecycle-actions",
    title: "Disable, Revoke, Reissue, and Rotate Key",
    summary: "These actions look similar in the UI, but they solve different operational problems.",
    steps: [
      "Use Disable when you need a temporary cutoff without ending the enrolled identity.",
      "Use Revoke when the enrolled identity must no longer be trusted.",
      "Use Reissue after Revoke when you need a fresh enrollment token for a replacement host.",
      "Use Rotate key when the same logical agent stays in place but its WireGuard key pair must change.",
    ],
    notes: [
      "Reissue does not revive a revoked agent.",
      "Rotate key does not create a new agent.",
      "If dual approval is enabled, another admin may need to approve sensitive actions before they take effect.",
    ],
  },
  {
    id: "reconcile-drift",
    title: "Reconcile and Drift Handling",
    summary: "Use reconcile to push desired peer intent into runtime when the two states diverge.",
    steps: [
      "Detect drift in Dashboard, Peers, or Peer Detail.",
      "Compare desired and runtime AllowedIPs before forcing changes.",
      "Reconcile one peer or a selected group of peers if the drift should be corrected immediately.",
      "If drift returns, inspect the native agent apply path or runtime adapter rather than repeatedly forcing reconcile.",
    ],
    notes: [
      "Desired state is server-authoritative.",
      "Runtime state is what the platform most recently observed, not what it intended.",
    ],
  },
  {
    id: "security-ops",
    title: "Session and Security Operations",
    summary: "Session review, audit investigation, and approval-driven lifecycle changes belong together during incident response.",
    steps: [
      "Use Sessions to inspect and revoke active sessions.",
      "Use Audit to trace the actor, resource, and result around a security event.",
      "If a sensitive action stays pending, wait for the second-admin approval path to complete it.",
      "Use Logging & SIEM to confirm whether export health was affected, but keep audit as the security source of truth.",
    ],
    notes: [
      "Exported runtime logs are supportive evidence, not a replacement for audit history.",
      "Operator and readonly users have narrower visibility into session and logging controls.",
    ],
  },
];

const opsSections: OpsSection[] = [
  {
    id: "agent-runtime",
    title: "Agent and Runtime Model",
    summary: "Wiregate separates enrollment, desired state, and runtime observation so operators can see drift instead of assuming the endpoint applied correctly.",
    bullets: [
      "Linux and Windows agents enroll with a token, store local state, and check in for desired config changes.",
      "Peer intent is rendered server-side from assigned policy and routing mode.",
      "Runtime state can drift from desired state, which is why Peers and Network exist as separate diagnostics surfaces.",
    ],
  },
  {
    id: "deployment-entrypoints",
    title: "Compose Deployment Entry Points",
    summary: "Use the release commands for daily operations on the single-node control plane.",
    bullets: [
      "make release deploys the current tagged version.",
      "make release-upgrade performs mandatory backup plus deploy plus verify.",
      "make release-rollback deploys WIREGATE_PREVIOUS_VERSION and reruns runtime checks.",
      "Use scripts/release.sh directly when you need lower-level control.",
    ],
  },
  {
    id: "backup-upgrade",
    title: "Backup, Restore, and Upgrade References",
    summary: "Operational procedures live in the dedicated runbooks and remain the authority for host-level steps.",
    bullets: [
      "See COMPOSE_RELEASE_RUNBOOK.md for env, deploy, verify, and release gates.",
      "See BACKUP_RESTORE_RUNBOOK.md for backup and restore expectations.",
      "See UPGRADE_ROLLBACK_RUNBOOK.md for release sequencing and rollback behavior.",
      "See WINDOWS_AGENT_RUNBOOK.md for Windows-native agent commands and recovery.",
    ],
  },
];

const troubleshooting: TroubleSection[] = [
  {
    id: "trouble-pending-approval",
    symptom: "A sensitive lifecycle action returns a pending approval notice instead of changing state immediately.",
    whereToLook: [
      "Agents notices after Revoke, Reissue, or Rotate key",
      "Audit history for related request events",
    ],
    resolution: [
      "Dual approval is enabled.",
      "Have a second admin approve the action before expecting the state change to complete.",
    ],
  },
  {
    id: "trouble-drift",
    symptom: "An agent looks enrolled, but the peer remains planned, drifted, or out of sync.",
    whereToLook: [
      "Peers and Peer Detail",
      "Dashboard top failing agents panel",
      "Native agent runtime or apply path",
    ],
    resolution: [
      "Compare desired versus runtime routes first.",
      "Reconcile if the desired state is correct.",
      "If drift persists, inspect the agent-side apply path or WireGuard adapter rather than repeatedly forcing reconcile.",
    ],
  },
  {
    id: "trouble-export",
    symptom: "Logs do not appear in the external collector or SIEM.",
    whereToLook: [
      "Logging & SIEM queue depth, last error, recent failures, and dead letters",
      "Dashboard delivery health widgets",
      "Audit page delivery issue overlay",
    ],
    resolution: [
      "Validate sink config and route rules.",
      "Run test delivery and watch the failure counters.",
      "Remember that audit writes can still be healthy even when export delivery fails.",
    ],
  },
  {
    id: "trouble-role",
    symptom: "A user cannot find DNS, Logging, or Users in the navigation.",
    whereToLook: [
      "Current role badge in the header",
      "Guide role notes for each page",
    ],
    resolution: [
      "Readonly users do not see DNS or Logging.",
      "Only admin sees Users.",
      "Operator sees DNS and Logging but only admin can edit those admin-only surfaces.",
    ],
  },
  {
    id: "trouble-replacement",
    symptom: "A revoked host still needs to be replaced and the operator expects the old identity to come back.",
    whereToLook: [
      "Agents page",
      "Enrollment Tokens page",
    ],
    resolution: [
      "Use Reissue to create a fresh enrollment token for replacement enrollment.",
      "Do not expect the revoked agent itself to become active again.",
    ],
  },
];

function canAccess(role: string | undefined, visibleTo: GuideRole[]): boolean {
  if (role !== "admin" && role !== "operator" && role !== "readonly") {
    return false;
  }
  return visibleTo.includes(role);
}

export function GuidePage() {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();

  async function handleLogout() {
    await logout();
    setSession(null);
    navigate("/login", { replace: true });
  }

  return (
    <div style={s.shell}>
      <header style={s.header}>
        <div style={s.headerLeft}>
          <HeaderBrand />
          <HeaderNavMenu current="guide" role={session?.role} />
        </div>
        <div style={s.headerRight}>
          <ThemeToggleButton />
          <span style={s.userInfo}>
            {session?.email} <span style={s.roleBadge}>{session?.role}</span>
          </span>
          <button onClick={handleLogout} style={s.logoutBtn}>Logout</button>
        </div>
      </header>

      <main style={s.page}>
        <section id="overview" style={s.heroCard}>
          <div style={s.eyebrow}>Operator Manual</div>
          <h2 style={s.heading}>Wiregate System Guide</h2>
          <p style={s.subtitle}>
            A page-by-page guide for the admin UI, plus the workflows and operations context you need to run the current control plane safely.
          </p>
          <div style={s.heroMeta}>
            <div style={s.metaCard}>
              <div style={s.metaLabel}>Canonical source</div>
              <div style={s.metaValue}>docs/implementation/SYSTEM_GUIDE.md</div>
            </div>
            <div style={s.metaCard}>
              <div style={s.metaLabel}>Current audience</div>
              <div style={s.metaValue}>admin, operator, readonly</div>
            </div>
            <div style={s.metaCard}>
              <div style={s.metaLabel}>Current role</div>
              <div style={s.metaValue}>{session?.role ?? "unknown"}</div>
            </div>
          </div>
        </section>

        <section style={s.quickNavCard}>
          <div style={s.sectionHeader}>
            <h3 style={s.sectionHeading}>Quick Navigation</h3>
            <p style={s.sectionCopy}>Jump by topic instead of scrolling through the full manual.</p>
          </div>
          <div style={s.quickNavGrid}>
            <a href="#overview" style={s.navChip}>Overview</a>
            <a href="#pages" style={s.navChip}>Page Sections</a>
            <a href="#workflows" style={s.navChip}>Workflows</a>
            <a href="#operations" style={s.navChip}>Operations</a>
            <a href="#troubleshooting" style={s.navChip}>Troubleshooting</a>
          </div>
        </section>

        <section style={s.card}>
          <div style={s.sectionHeader}>
            <h3 style={s.sectionHeading}>Core Terms and Reading Order</h3>
            <p style={s.sectionCopy}>Use the same vocabulary across the UI, API, audit trail, and operational docs.</p>
          </div>
          <div style={s.overviewGrid}>
            <div style={s.infoCard}>
              <div style={s.infoTitle}>Agent</div>
              <p style={s.infoText}>The installed Windows or Linux software on an endpoint.</p>
            </div>
            <div style={s.infoCard}>
              <div style={s.infoTitle}>Peer</div>
              <p style={s.infoText}>The WireGuard identity and route object associated with an enrolled agent.</p>
            </div>
            <div style={s.infoCard}>
              <div style={s.infoTitle}>Enrollment token</div>
              <p style={s.infoText}>The bootstrap secret used for first enrollment or replacement enrollment.</p>
            </div>
            <div style={s.infoCard}>
              <div style={s.infoTitle}>Access policy</div>
              <p style={s.infoText}>The server-side definition of allowed destination CIDRs and routing intent.</p>
            </div>
          </div>
          <div style={s.readOrder}>
            <div style={s.readOrderTitle}>Best reading order</div>
            <ol style={s.orderedList}>
              <li>Start with the page you are currently using.</li>
              <li>Use the workflow section when a task crosses multiple pages.</li>
              <li>Use the operations and troubleshooting appendices when a UI symptom points to agent or deployment work.</li>
            </ol>
          </div>
        </section>

        <section id="pages" style={s.card}>
          <div style={s.sectionHeader}>
            <h3 style={s.sectionHeading}>Page-by-Page Guide</h3>
            <p style={s.sectionCopy}>Every routed page is covered below, including pages hidden from some roles.</p>
          </div>
          <div style={s.pageIndex}>
            {pageSections.map((section) => (
              <a key={section.id} href={`#${section.id}`} style={s.indexLink}>
                {section.title}
              </a>
            ))}
          </div>
          <div style={s.sectionStack}>
            {pageSections.map((section) => {
              const accessible = canAccess(session?.role, section.visibleTo);
              return (
                <article key={section.id} id={section.id} style={accessible ? s.guideCard : { ...s.guideCard, ...s.guideCardMuted }}>
                  <div style={s.guideHeader}>
                    <div>
                      <div style={s.guideTitleRow}>
                        <h4 style={s.guideTitle}>{section.title}</h4>
                        {section.route && <span style={s.routeBadge}>{section.route}</span>}
                      </div>
                      <p style={s.guidePurpose}>{section.purpose}</p>
                    </div>
                    <span style={s.accessBadge}>{section.access}</span>
                  </div>
                  {!accessible && (
                    <div style={s.roleNotice}>
                      This page is not available in the current navigation for the <strong>{session?.role ?? "unknown"}</strong> role.
                      The section stays here so you can understand the full system surface and escalation path.
                    </div>
                  )}
                  <div style={s.guideGrid}>
                    <GuideListCard title="Main data shown" items={section.mainData} />
                    <GuideListCard title="Primary actions" items={section.actions} />
                    <GuideListCard title="Common operator workflows" items={section.workflows} />
                    <GuideListCard title="Warnings and security notes" items={section.notes} tone="warning" />
                  </div>
                  <div style={s.relatedRow}>
                    <span style={s.relatedLabel}>Related pages and next steps</span>
                    <div style={s.relatedTags}>
                      {section.related.map((item) => (
                        <span key={item} style={s.relatedTag}>{item}</span>
                      ))}
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        </section>

        <section id="workflows" style={s.card}>
          <div style={s.sectionHeader}>
            <h3 style={s.sectionHeading}>System Workflows</h3>
            <p style={s.sectionCopy}>These tasks span multiple pages and are where most operational mistakes happen.</p>
          </div>
          <div style={s.sectionStack}>
            {workflowSections.map((workflow) => (
              <article key={workflow.id} id={workflow.id} style={s.guideCard}>
                <div style={s.guideHeaderCompact}>
                  <h4 style={s.guideTitle}>{workflow.title}</h4>
                </div>
                <p style={s.guidePurpose}>{workflow.summary}</p>
                <div style={s.guideGrid}>
                  <GuideListCard title="Recommended path" items={workflow.steps} ordered />
                  <GuideListCard title="Important notes" items={workflow.notes} tone="warning" />
                </div>
              </article>
            ))}
          </div>
        </section>

        <section id="operations" style={s.card}>
          <div style={s.sectionHeader}>
            <h3 style={s.sectionHeading}>Operations and Runtime Appendices</h3>
            <p style={s.sectionCopy}>Keep the system model and release entry points close at hand when you move from UI work to host or agent work.</p>
          </div>
          <div style={s.opsGrid}>
            {opsSections.map((entry) => (
              <article key={entry.id} id={entry.id} style={s.infoCardWide}>
                <h4 style={s.infoWideTitle}>{entry.title}</h4>
                <p style={s.infoWideText}>{entry.summary}</p>
                <ul style={s.list}>
                  {entry.bullets.map((bullet) => (
                    <li key={bullet}>{bullet}</li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
        </section>

        <section id="troubleshooting" style={s.card}>
          <div style={s.sectionHeader}>
            <h3 style={s.sectionHeading}>Troubleshooting Index</h3>
            <p style={s.sectionCopy}>Map the UI symptom to the right page or operational follow-up without guessing.</p>
          </div>
          <div style={s.sectionStack}>
            {troubleshooting.map((entry) => (
              <article key={entry.id} id={entry.id} style={s.guideCard}>
                <h4 style={s.guideTitle}>{entry.symptom}</h4>
                <div style={s.guideGrid}>
                  <GuideListCard title="Where to look" items={entry.whereToLook} />
                  <GuideListCard title="Likely resolution" items={entry.resolution} />
                </div>
              </article>
            ))}
          </div>
        </section>
      </main>
    </div>
  );
}

function GuideListCard({
  title,
  items,
  ordered = false,
  tone = "default",
}: {
  title: string;
  items: string[];
  ordered?: boolean;
  tone?: "default" | "warning";
}) {
  const cardStyle = tone === "warning" ? { ...s.listCard, ...s.listCardWarning } : s.listCard;
  return (
    <section style={cardStyle}>
      <div style={s.listCardTitle}>{title}</div>
      {ordered ? (
        <ol style={s.orderedList}>
          {items.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ol>
      ) : (
        <ul style={s.list}>
          {items.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      )}
    </section>
  );
}

const s: Record<string, React.CSSProperties> = {
  shell: { minHeight: "100vh", background: uiTheme.pageBg, fontFamily: "system-ui, sans-serif" },
  header: { background: uiTheme.headerBg, color: uiTheme.headerText, padding: "0 2rem", height: 56, display: "flex", alignItems: "center", justifyContent: "space-between" },
  headerLeft: { display: "flex", alignItems: "center", gap: "2rem" },
  headerRight: { display: "flex", alignItems: "center", gap: "1rem" },
  userInfo: { fontSize: "0.9rem", color: uiTheme.headerLink },
  roleBadge: { background: uiTheme.headerChipBg, padding: "2px 8px", borderRadius: 4, fontSize: "0.8rem", color: uiTheme.headerChipText },
  logoutBtn: { background: "transparent", border: `1px solid ${uiTheme.border}`, color: uiTheme.headerText, padding: "5px 14px", borderRadius: 5, cursor: "pointer", fontSize: "0.875rem" },
  page: { padding: "2rem", maxWidth: 1200, margin: "0 auto", display: "grid", gap: "1rem" },
  heroCard: {
    background: `linear-gradient(135deg, ${uiTheme.surface} 0%, ${uiTheme.surfaceAlt} 100%)`,
    borderRadius: 16,
    padding: "1.6rem 1.8rem",
    boxShadow: uiTheme.shadow,
    border: `1px solid ${uiTheme.border}`,
  },
  eyebrow: {
    display: "inline-flex",
    alignItems: "center",
    padding: "0.2rem 0.55rem",
    borderRadius: 999,
    background: uiTheme.headerBg,
    color: uiTheme.headerText,
    fontSize: "0.72rem",
    letterSpacing: 0.5,
    textTransform: "uppercase",
    fontWeight: 700,
  },
  heading: { margin: "0.8rem 0 0", color: uiTheme.text, fontSize: "1.85rem", lineHeight: 1.15 },
  subtitle: { margin: "0.55rem 0 0", color: uiTheme.textMuted, fontSize: "1rem", maxWidth: 820, lineHeight: 1.6 },
  heroMeta: { marginTop: "1.2rem", display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: "0.8rem" },
  metaCard: { background: uiTheme.surface, borderRadius: 12, border: `1px solid ${uiTheme.border}`, padding: "0.95rem 1rem" },
  metaLabel: { color: uiTheme.textMuted, fontSize: "0.78rem", textTransform: "uppercase", letterSpacing: 0.4, marginBottom: "0.35rem" },
  metaValue: { color: uiTheme.text, fontWeight: 600, wordBreak: "break-word" },
  quickNavCard: { background: uiTheme.surface, borderRadius: 12, padding: "1.2rem 1.4rem", boxShadow: uiTheme.shadow, border: `1px solid ${uiTheme.border}` },
  quickNavGrid: { display: "flex", flexWrap: "wrap", gap: "0.7rem" },
  navChip: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    padding: "0.55rem 0.9rem",
    borderRadius: 999,
    textDecoration: "none",
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
    border: `1px solid ${uiTheme.border}`,
    fontSize: "0.9rem",
    fontWeight: 600,
  },
  card: { background: uiTheme.surface, borderRadius: 12, padding: "1.3rem 1.5rem", boxShadow: uiTheme.shadow, border: `1px solid ${uiTheme.border}` },
  sectionHeader: { marginBottom: "1rem" },
  sectionHeading: { margin: 0, color: uiTheme.text, fontSize: "1.2rem" },
  sectionCopy: { margin: "0.4rem 0 0", color: uiTheme.textMuted, lineHeight: 1.6 },
  overviewGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: "0.8rem" },
  infoCard: { background: uiTheme.surfaceAlt, borderRadius: 12, border: `1px solid ${uiTheme.border}`, padding: "1rem" },
  infoTitle: { color: uiTheme.text, fontWeight: 700, marginBottom: "0.4rem" },
  infoText: { margin: 0, color: uiTheme.textMuted, lineHeight: 1.55 },
  readOrder: { marginTop: "1rem", paddingTop: "1rem", borderTop: `1px solid ${uiTheme.border}` },
  readOrderTitle: { color: uiTheme.text, fontWeight: 700, marginBottom: "0.4rem" },
  pageIndex: { display: "flex", flexWrap: "wrap", gap: "0.55rem", marginBottom: "1rem" },
  indexLink: {
    display: "inline-flex",
    alignItems: "center",
    padding: "0.45rem 0.75rem",
    borderRadius: 999,
    textDecoration: "none",
    background: uiTheme.surfaceAlt,
    color: uiTheme.text,
    border: `1px solid ${uiTheme.border}`,
    fontSize: "0.86rem",
  },
  sectionStack: { display: "grid", gap: "1rem" },
  guideCard: { background: uiTheme.pageBg, borderRadius: 14, border: `1px solid ${uiTheme.border}`, padding: "1.2rem 1.2rem 1rem" },
  guideCardMuted: { opacity: 0.82 },
  guideHeader: { display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "1rem", marginBottom: "0.9rem", flexWrap: "wrap" },
  guideHeaderCompact: { marginBottom: "0.55rem" },
  guideTitleRow: { display: "flex", alignItems: "center", gap: "0.6rem", flexWrap: "wrap", marginBottom: "0.35rem" },
  guideTitle: { margin: 0, color: uiTheme.text, fontSize: "1.05rem" },
  routeBadge: { display: "inline-flex", alignItems: "center", borderRadius: 999, border: `1px solid ${uiTheme.border}`, padding: "0.18rem 0.55rem", fontSize: "0.78rem", color: uiTheme.textMuted, background: uiTheme.surface },
  accessBadge: { display: "inline-flex", alignItems: "center", borderRadius: 999, padding: "0.3rem 0.65rem", background: uiTheme.headerBg, color: uiTheme.headerText, fontSize: "0.78rem", fontWeight: 600 },
  guidePurpose: { margin: 0, color: uiTheme.textMuted, lineHeight: 1.6, maxWidth: 860 },
  roleNotice: { marginBottom: "0.9rem", borderRadius: 10, padding: "0.85rem 0.95rem", background: uiTheme.surface, border: `1px solid ${uiTheme.border}`, color: uiTheme.textMuted, lineHeight: 1.5 },
  guideGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(230px, 1fr))", gap: "0.8rem" },
  listCard: { background: uiTheme.surface, borderRadius: 12, border: `1px solid ${uiTheme.border}`, padding: "0.95rem 1rem" },
  listCardWarning: { background: uiTheme.surfaceAlt },
  listCardTitle: { color: uiTheme.text, fontWeight: 700, marginBottom: "0.55rem" },
  list: { margin: 0, paddingLeft: "1.15rem", color: uiTheme.text, lineHeight: 1.6 },
  orderedList: { margin: 0, paddingLeft: "1.15rem", color: uiTheme.text, lineHeight: 1.7 },
  relatedRow: { marginTop: "0.95rem", paddingTop: "0.85rem", borderTop: `1px solid ${uiTheme.border}` },
  relatedLabel: { display: "block", color: uiTheme.textMuted, fontSize: "0.82rem", marginBottom: "0.45rem" },
  relatedTags: { display: "flex", flexWrap: "wrap", gap: "0.45rem" },
  relatedTag: { display: "inline-flex", alignItems: "center", padding: "0.3rem 0.6rem", borderRadius: 999, background: uiTheme.surfaceAlt, border: `1px solid ${uiTheme.border}`, color: uiTheme.text, fontSize: "0.8rem" },
  opsGrid: { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: "0.85rem" },
  infoCardWide: { background: uiTheme.pageBg, borderRadius: 12, border: `1px solid ${uiTheme.border}`, padding: "1rem 1rem 0.95rem" },
  infoWideTitle: { margin: 0, color: uiTheme.text, fontSize: "1rem" },
  infoWideText: { margin: "0.45rem 0 0.7rem", color: uiTheme.textMuted, lineHeight: 1.6 },
};
