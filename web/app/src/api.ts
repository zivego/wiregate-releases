export type Role = "admin" | "operator" | "readonly";
export type ThemePreference = "light" | "dark";

export interface ApiError {
  code: string;
  message: string;
  request_id?: string;
}

export interface HealthResponse {
  status: string;
  drift?: string;
}

export interface SessionResponse {
  session_id: string;
  user_id: string;
  email: string;
  role: Role;
  must_change_password: boolean;
  theme_preference: ThemePreference;
  auth_provider: "password" | "oidc" | string;
  last_seen_at: string;
  source_ip?: string;
  user_agent?: string;
  issued_at: string;
  expires_at: string;
}

export interface SessionInventoryItem {
  session_id: string;
  user_id: string;
  email: string;
  role: Role;
  auth_provider: "password" | "oidc" | string;
  current: boolean;
  issued_at: string;
  expires_at: string;
  last_seen_at: string;
  source_ip?: string;
  user_agent?: string;
}

export interface SessionInventoryPage {
  sessions: SessionInventoryItem[];
  next_cursor?: string;
}

export interface DNSConfig {
  enabled: boolean;
  servers: string[];
  search_domains: string[];
  updated_at?: string;
}

export type AnalyticsRange = "24h" | "7d" | "30d";
export type AnalyticsBucket = "hour" | "day";

export interface SecurityPolicy {
  required_admin_amr?: string;
  required_admin_acr?: string;
  dual_approval_enabled: boolean;
}

export interface SecurityApproval {
  id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  request_payload?: Record<string, unknown>;
  requested_by_user_id: string;
  approved_by_user_id?: string;
  rejected_by_user_id?: string;
  status: string;
  created_at: string;
  decided_at?: string;
}

export interface PendingSecurityApprovalResponse {
  approval: SecurityApproval;
}

export interface SecurityApprovalPage {
  approvals: SecurityApproval[];
  next_cursor?: string;
}

export interface AuthProviderInfo {
  id: string;
  type: "password" | "oidc" | string;
  enabled: boolean;
  display_name: string;
}

export interface AuthProvidersResponse {
  providers: AuthProviderInfo[];
}

export interface UpdateCurrentUserPreferencesInput {
  theme_preference: ThemePreference;
}

export interface CurrentUserPreferencesResponse {
  user_id: string;
  theme_preference: ThemePreference;
}

export interface UserRecord {
  id: string;
  email: string;
  role: Role;
  theme_preference?: ThemePreference;
  created_at: string;
}

export interface UserRecordPage {
  users: UserRecord[];
  next_cursor?: string;
}

export interface CreateUserInput {
  email: string;
  password: string;
  role: Role;
}

export interface UpdateUserInput {
  email: string;
  role: Role;
}

export interface PasswordActionResponse {
  user_id: string;
}

export interface PeerInventory {
  id: string;
  public_key: string;
  assigned_address?: string;
  allowed_ips?: string[];
  status: string;
  created_at: string;
}

export type TrafficMode = "standard" | "full_tunnel";
export type TrafficModeOverride = "inherit" | TrafficMode;
export type GatewayMode = "disabled" | "subnet_access" | "egress_gateway";
export type RouteProfile = TrafficMode | "subnet_access" | "egress_gateway";
export type PathMode = "direct" | "relay";

export interface AgentInventory {
  id: string;
  hostname: string;
  platform: string;
  status: string;
  is_online: boolean;
  traffic_mode: TrafficMode;
  traffic_mode_override?: TrafficMode;
  gateway_mode: GatewayMode;
  last_seen_at?: string;
  reported_version?: string;
  reported_config_fingerprint?: string;
  last_apply_status?: string;
  last_apply_error?: string;
  last_applied_at?: string;
  created_at: string;
  peer?: PeerInventory;
}

export type AgentStateAction = "disable" | "enable" | "revoke";

export interface ListAgentsParams {
  status?: string;
  platform?: string;
  q?: string;
  limit?: number;
  page_size?: number;
  cursor?: string;
}

export interface AgentInventoryPage {
  agents: AgentInventory[];
  next_cursor?: string;
}

export interface PeerView {
  id: string;
  agent_id: string;
  hostname?: string;
  public_key: string;
  assigned_address?: string;
  allowed_ips: string[];
  runtime_allowed_ips?: string[];
  status: string;
  drift: string;
}

export interface ListPeersParams {
  status?: string;
  agent_id?: string;
  q?: string;
  limit?: number;
  page_size?: number;
  cursor?: string;
}

export interface PeerViewPage {
  peers: PeerView[];
  next_cursor?: string;
}

export interface AccessPolicy {
  id: string;
  name: string;
  description?: string;
  destinations: string[];
  traffic_mode: TrafficMode;
  assigned_agent_ids: string[];
  created_at: string;
  updated_at: string;
}

export interface AccessPolicyPage {
  policies: AccessPolicy[];
  next_cursor?: string;
}

export interface EnrollmentToken {
  id: string;
  model: string;
  scope: string;
  status: string;
  bound_identity?: string;
  access_policy_ids?: string[];
  expires_at: string;
  used_at?: string;
  revoked_at?: string;
  created_by_user_id: string;
  created_at: string;
  token?: string;
  risk_warning?: string;
}

export interface EnrollmentTokenPage {
  tokens: EnrollmentToken[];
  next_cursor?: string;
}

export interface CreateEnrollmentTokenInput {
  model: "A" | "B";
  scope: string;
  bound_identity?: string;
  access_policy_ids?: string[];
  ttl_minutes?: number;
}

export interface CreateAccessPolicyInput {
  name: string;
  description?: string;
  destinations: string[];
  traffic_mode?: TrafficMode;
}

export interface AccessPolicyAssignment {
  id: string;
  agent_id: string;
  access_policy_id: string;
  status: string;
  created_at: string;
}

export interface PolicySimulationResult {
  agent_id?: string;
  policy_ids: string[];
  effective_traffic_mode: TrafficMode;
  allowed_ips: string[];
  destinations: string[];
  route_profile: TrafficMode;
}

export interface AuditEvent {
  id: string;
  actor_user_id?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  result: string;
  created_at: string;
  metadata?: Record<string, unknown>;
  hash_meta?: {
    prev_hash?: string;
    event_hash?: string;
  };
}

export interface DashboardSeriesPoint {
  bucket_start: string;
  count: number;
}

export interface EnrollmentFunnel {
  issued: number;
  used: number;
  revoked_or_expired: number;
}

export interface PolicyCoverage {
  policies_total: number;
  active_assignments: number;
  agents_with_policy: number;
  agents_without_policy: number;
  total_agents: number;
  coverage_percent: number;
}

export interface FailingAgent {
  agent_id: string;
  hostname: string;
  platform: string;
  status: string;
  last_apply_status?: string;
  last_apply_error?: string;
  runtime_drift_state?: string;
  failure_score: number;
  failure_categories?: string[];
}

export interface DashboardHealthCards {
  total_agents: number;
  recently_seen_agents: number;
  applied_agents: number;
  drifted_agents: number;
  failed_agents: number;
  pending_reconcile: number;
}

export interface DashboardAnalytics {
  range: AnalyticsRange;
  bucket: AnalyticsBucket;
  generated_at: string;
  auth_security_trend: DashboardSeriesPoint[];
  enrollment_funnel: EnrollmentFunnel;
  policy_coverage: PolicyCoverage;
  top_failing_agents: FailingAgent[];
  health_cards: DashboardHealthCards;
  log_delivery: unknown | null;
}

export interface ActionDistributionItem {
  category: string;
  count: number;
}

export interface AuditHeatmapCell {
  weekday: number;
  hour: number;
  count: number;
}

export interface AuditAnalytics {
  range: AnalyticsRange;
  bucket: AnalyticsBucket;
  generated_at: string;
  event_trend: DashboardSeriesPoint[];
  action_distribution: ActionDistributionItem[];
  activity_heatmap: AuditHeatmapCell[];
  export_issues: Record<string, unknown>[];
}

export type LogCategory = "auth" | "session" | "user_mgmt" | "policy" | "agent" | "enrollment" | "reconcile" | "security" | "system";
export type LogSeverity = "debug" | "info" | "warn" | "error";
export type LogTransport = "udp" | "tcp" | "tls";
export type LogFormat = "rfc5424" | "json";

export interface LogSyslogConfig {
  transport: LogTransport;
  host: string;
  port: number;
  format: LogFormat;
  facility: number;
  app_name: string;
  hostname_override?: string;
  ca_cert_file?: string;
  client_cert_file?: string;
  client_key_file?: string;
}

export interface LogSink {
  id: string;
  name: string;
  type: "syslog" | string;
  enabled: boolean;
  syslog: LogSyslogConfig;
  created_at: string;
  updated_at: string;
}

export interface LogSinkInput {
  name: string;
  type: "syslog" | string;
  enabled: boolean;
  syslog: LogSyslogConfig;
}

export interface LogRouteRule {
  id?: string;
  sink_id: string;
  categories: LogCategory[];
  min_severity: LogSeverity;
  enabled: boolean;
}

export interface LogRoutesResponse {
  routes: LogRouteRule[];
  redacted_fields: string[];
}

export interface LogDeliveryStatus {
  sink_id: string;
  sink_name: string;
  sink_type: string;
  enabled: boolean;
  queue_depth: number;
  dropped_events: number;
  total_delivered: number;
  total_failed: number;
  consecutive_failures: number;
  last_attempted_at?: string;
  last_delivered_at?: string;
  last_error?: string;
  updated_at: string;
  dead_letter_count: number;
}

export interface LogDeliveryFailure {
  id: string;
  sink_id: string;
  sink_name: string;
  occurred_at: string;
  category: string;
  severity: LogSeverity;
  message: string;
  action?: string;
  error_message: string;
  test_delivery: boolean;
  metadata?: Record<string, unknown>;
}

export interface LoggingStatusSnapshot {
  queue_capacity: number;
  current_queued: number;
  redacted_fields: string[];
  sinks: LogDeliveryStatus[];
  recent_failures: LogDeliveryFailure[];
}

export interface TestDeliveryResult {
  accepted: boolean;
  sink_id?: string;
}

export interface DiagnosticsSummary {
  total_agents: number;
  direct_agents: number;
  relay_agents: number;
  gateway_agents: number;
  conflict_count: number;
}

export interface NetworkDiagnosticAgent {
  agent_id: string;
  hostname: string;
  platform: string;
  agent_status: string;
  peer_id?: string;
  peer_status?: string;
  traffic_mode: TrafficMode;
  gateway_mode: GatewayMode;
  route_profile: RouteProfile;
  path_mode: PathMode;
  gateway_assignment_status: string;
  drift_state?: string;
  allowed_destinations?: string[];
  route_conflicts?: string[];
  last_seen_at?: string;
}

export interface DiagnosticsSnapshot {
  generated_at: string;
  relay_available: boolean;
  relay_status: string;
  summary: DiagnosticsSummary;
  agents: NetworkDiagnosticAgent[];
}

export interface CapacityInventory {
  users: number;
  agents: number;
  peers: number;
  access_policies: number;
  policy_assignments: number;
  enrollment_tokens: number;
  pending_security_approvals: number;
  recently_seen_agents: number;
  failed_agents: number;
  drifted_peers: number;
}

export interface CapacitySessions {
  active_sessions: number;
  idle_timeout_minutes: number;
}

export interface CapacityAudit {
  total_events: number;
  oldest_event_at?: string;
  newest_event_at?: string;
}

export interface CapacityLogging {
  queue_capacity: number;
  current_queued: number;
  sinks_total: number;
  enabled_sinks: number;
  degraded_sinks: number;
  dropped_events: number;
  total_delivered: number;
  total_failed: number;
  consecutive_failures: number;
}

export interface CapacityStorage {
  analytics_rollups: number;
  log_dead_letters: number;
}

export interface CapacitySnapshot {
  generated_at: string;
  database_engine: "sqlite" | "postgres" | string;
  inventory: CapacityInventory;
  sessions: CapacitySessions;
  audit: CapacityAudit;
  logging: CapacityLogging;
  storage: CapacityStorage;
}

export interface ListAuditEventsParams {
  action?: string;
  resource_type?: string;
  result?: string;
  actor_user_id?: string;
  limit?: number;
  page_size?: number;
  cursor?: string;
}

export interface AuditEventPage {
  events: AuditEvent[];
  next_cursor?: string;
}

export function clearSessionToken(): void {
  // Kept as compatibility no-op for existing page flows.
}

export function isPendingSecurityApprovalResponse(value: unknown): value is PendingSecurityApprovalResponse {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as { approval?: { id?: unknown; status?: unknown } };
  return typeof candidate.approval?.id === "string" && typeof candidate.approval?.status === "string";
}

function normalizeAgentInventory(agent: AgentInventory): AgentInventory {
  return {
    ...agent,
    traffic_mode: agent.traffic_mode === "full_tunnel" ? "full_tunnel" : "standard",
    traffic_mode_override: agent.traffic_mode_override === "full_tunnel" || agent.traffic_mode_override === "standard" ? agent.traffic_mode_override : undefined,
    gateway_mode: agent.gateway_mode === "subnet_access" || agent.gateway_mode === "egress_gateway" ? agent.gateway_mode : "disabled",
  };
}

function normalizeAccessPolicy(policy: AccessPolicy): AccessPolicy {
  return {
    ...policy,
    traffic_mode: policy.traffic_mode === "full_tunnel" ? "full_tunnel" : "standard",
    assigned_agent_ids: Array.isArray(policy.assigned_agent_ids) ? policy.assigned_agent_ids : [],
  };
}

async function authFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  return fetch(input, {
    ...init,
    credentials: "include",
    headers: init.headers,
  });
}

export async function fetchLiveHealth(baseUrl = ""): Promise<HealthResponse> {
  const response = await fetch(`${baseUrl}/api/v1/health/live`);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`health check failed: ${text}`);
  }
  return (await response.json()) as HealthResponse;
}

export async function fetchReconcileHealth(baseUrl = ""): Promise<HealthResponse> {
  const response = await authFetch(`${baseUrl}/api/v1/health/reconcile`);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`reconcile health failed: ${text}`);
  }
  return (await response.json()) as HealthResponse;
}

export async function getDashboardAnalytics(range: AnalyticsRange = "24h"): Promise<DashboardAnalytics> {
  const response = await authFetch(`/api/v1/analytics/dashboard?range=${encodeURIComponent(range)}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load dashboard analytics");
  }
  return (await response.json()) as DashboardAnalytics;
}

export async function getAuditAnalytics(range: AnalyticsRange = "24h", bucket?: AnalyticsBucket): Promise<AuditAnalytics> {
  const search = new URLSearchParams({ range });
  if (bucket) {
    search.set("bucket", bucket);
  }
  const response = await authFetch(`/api/v1/analytics/audit?${search.toString()}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load audit analytics");
  }
  return (await response.json()) as AuditAnalytics;
}

export async function listLoggingSinks(): Promise<LogSink[]> {
  const response = await authFetch("/api/v1/logging/sinks");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load log sinks");
  }
  const data = await response.json();
  return (data.sinks ?? []) as LogSink[];
}

export async function createLoggingSink(input: LogSinkInput): Promise<LogSink> {
  const response = await authFetch("/api/v1/logging/sinks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to create log sink");
  }
  return (await response.json()) as LogSink;
}

export async function updateLoggingSink(sinkID: string, input: LogSinkInput): Promise<LogSink> {
  const response = await authFetch(`/api/v1/logging/sinks/${sinkID}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update log sink");
  }
  return (await response.json()) as LogSink;
}

export async function deleteLoggingSink(sinkID: string): Promise<void> {
  const response = await authFetch(`/api/v1/logging/sinks/${sinkID}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to delete log sink");
  }
}

export async function getLoggingRoutes(): Promise<LogRoutesResponse> {
  const response = await authFetch("/api/v1/logging/routes");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load logging routes");
  }
  return (await response.json()) as LogRoutesResponse;
}

export async function updateLoggingRoutes(routes: LogRouteRule[]): Promise<LogRoutesResponse> {
  const response = await authFetch("/api/v1/logging/routes", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ routes }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update logging routes");
  }
  return (await response.json()) as LogRoutesResponse;
}

export async function getLoggingStatus(): Promise<LoggingStatusSnapshot> {
  const response = await authFetch("/api/v1/logging/status");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load logging status");
  }
  return (await response.json()) as LoggingStatusSnapshot;
}

export async function testLoggingDelivery(sinkID?: string): Promise<TestDeliveryResult> {
  const response = await authFetch("/api/v1/logging/test-delivery", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sink_id: sinkID ?? "" }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to test log delivery");
  }
  return (await response.json()) as TestDeliveryResult;
}

export async function getDNSConfig(): Promise<DNSConfig> {
  const response = await authFetch("/api/v1/dns/config");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load dns config");
  }
  return (await response.json()) as DNSConfig;
}

export async function updateDNSConfig(input: DNSConfig): Promise<DNSConfig> {
  const response = await authFetch("/api/v1/dns/config", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update dns config");
  }
  return (await response.json()) as DNSConfig;
}

export async function getNetworkDiagnostics(): Promise<DiagnosticsSnapshot> {
  const response = await authFetch("/api/v1/network/diagnostics");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load network diagnostics");
  }
  return (await response.json()) as DiagnosticsSnapshot;
}

export async function getSystemCapacity(): Promise<CapacitySnapshot> {
  const response = await authFetch("/api/v1/system/capacity");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load system capacity");
  }
  return (await response.json()) as CapacitySnapshot;
}

export async function login(email: string, password: string): Promise<SessionResponse> {
  const response = await fetch("/api/v1/sessions", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "login failed");
  }
  return (await response.json()) as SessionResponse;
}

export async function listAuthProviders(): Promise<AuthProvidersResponse> {
  const response = await fetch("/api/v1/auth/providers", {
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error("failed to load auth providers");
  }
  return (await response.json()) as AuthProvidersResponse;
}

export async function logout(): Promise<void> {
  await authFetch("/api/v1/sessions/current", {
    method: "DELETE",
  });
}

export async function getCurrentSession(): Promise<SessionResponse | null> {
  const response = await authFetch("/api/v1/sessions/current");
  if (response.status === 401) {
    return null;
  }
  if (!response.ok) {
    return null;
  }
  return (await response.json()) as SessionResponse;
}

export async function listSessions(cursor?: string, pageSize = 50): Promise<SessionInventoryPage> {
  const search = new URLSearchParams();
  search.set("page_size", String(pageSize));
  if (cursor) {
    search.set("cursor", cursor);
  }
  const response = await authFetch(`/api/v1/sessions?${search.toString()}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load sessions");
  }
  const data = await response.json();
  return {
    sessions: (data.sessions ?? []) as SessionInventoryItem[],
    next_cursor: data.next_cursor as string | undefined,
  };
}

export async function revokeSession(sessionID: string): Promise<void> {
  const response = await authFetch(`/api/v1/sessions/${sessionID}/revoke`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to revoke session");
  }
}

export async function getSecurityPolicy(): Promise<SecurityPolicy> {
  const response = await authFetch("/api/v1/security/policies");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load security policy");
  }
  return (await response.json()) as SecurityPolicy;
}

export async function updateSecurityPolicy(input: SecurityPolicy): Promise<SecurityPolicy> {
  const response = await authFetch("/api/v1/security/policies", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update security policy");
  }
  return (await response.json()) as SecurityPolicy;
}

export async function listSecurityApprovals(status?: string, limit?: number, cursor?: string): Promise<SecurityApprovalPage> {
  const search = new URLSearchParams();
  if (status) search.set("status", status);
  if (limit) search.set("page_size", String(limit));
  if (cursor) search.set("cursor", cursor);
  const query = search.toString();

  const response = await authFetch(`/api/v1/security/approvals${query ? `?${query}` : ""}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load security approvals");
  }
  const data = await response.json();
  return {
    approvals: (data.approvals ?? []) as SecurityApproval[],
    next_cursor: data.next_cursor as string | undefined,
  };
}

export async function approveSecurityApproval(approvalID: string): Promise<{ approval: SecurityApproval; result?: unknown }> {
  const response = await authFetch(`/api/v1/security/approvals/${approvalID}/approve`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to approve security action");
  }
  return (await response.json()) as { approval: SecurityApproval; result?: unknown };
}

export async function rejectSecurityApproval(approvalID: string): Promise<{ approval: SecurityApproval }> {
  const response = await authFetch(`/api/v1/security/approvals/${approvalID}/reject`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to reject security action");
  }
  return (await response.json()) as { approval: SecurityApproval };
}

export async function updateCurrentUserPreferences(input: UpdateCurrentUserPreferencesInput): Promise<CurrentUserPreferencesResponse> {
  const response = await authFetch("/api/v1/users/me/preferences", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update preferences");
  }
  return (await response.json()) as CurrentUserPreferencesResponse;
}

export async function listUsers(cursor?: string, pageSize = 50): Promise<UserRecordPage> {
  const search = new URLSearchParams();
  search.set("page_size", String(pageSize));
  if (cursor) {
    search.set("cursor", cursor);
  }
  const response = await authFetch(`/api/v1/users?${search.toString()}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load users");
  }
  const data = await response.json();
  return {
    users: (data.users ?? []) as UserRecord[],
    next_cursor: typeof data.next_cursor === "string" ? data.next_cursor : undefined,
  };
}

export async function createUser(input: CreateUserInput): Promise<UserRecord> {
  const response = await authFetch("/api/v1/users", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to create user");
  }
  return (await response.json()) as UserRecord;
}

export async function updateUser(userID: string, input: UpdateUserInput): Promise<UserRecord> {
  const response = await authFetch(`/api/v1/users/${userID}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update user");
  }
  return (await response.json()) as UserRecord;
}

export async function deleteUser(userID: string): Promise<void> {
  const response = await authFetch(`/api/v1/users/${userID}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to delete user");
  }
}

export async function resetUserPassword(userID: string, newPassword: string): Promise<PasswordActionResponse> {
  const response = await authFetch(`/api/v1/users/${userID}/password-reset`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ new_password: newPassword }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to reset user password");
  }
  return (await response.json()) as PasswordActionResponse;
}

export async function changeOwnPassword(currentPassword: string, newPassword: string): Promise<PasswordActionResponse> {
  const response = await authFetch("/api/v1/users/me/password", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to change password");
  }
  return (await response.json()) as PasswordActionResponse;
}

export async function listAgents(params: ListAgentsParams = {}): Promise<AgentInventoryPage> {
  const search = new URLSearchParams();
  if (params.status) search.set("status", params.status);
  if (params.platform) search.set("platform", params.platform);
  if (params.q) search.set("q", params.q);
  if (params.page_size) search.set("page_size", String(params.page_size));
  else if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  const query = search.toString();

  const response = await authFetch(`/api/v1/agents${query ? `?${query}` : ""}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load agents");
  }
  const data = await response.json();
  const agents = (data.agents ?? []) as AgentInventory[];
  return {
    agents: agents.map(normalizeAgentInventory),
    next_cursor: typeof data.next_cursor === "string" ? data.next_cursor : undefined,
  };
}

export async function listAllAgents(params: Omit<ListAgentsParams, "limit" | "page_size" | "cursor"> = {}): Promise<AgentInventory[]> {
  const agents: AgentInventory[] = [];
  let cursor: string | undefined;

  do {
    const page = await listAgents({
      ...params,
      page_size: 200,
      cursor,
    });
    agents.push(...page.agents);
    cursor = page.next_cursor;
  } while (cursor);

  return agents;
}

export async function patchAgentState(agentID: string, action: AgentStateAction): Promise<AgentInventory | PendingSecurityApprovalResponse> {
  const response = await authFetch(`/api/v1/agents/${agentID}/state`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update agent state");
  }
  const data = await response.json();
  if (isPendingSecurityApprovalResponse(data)) {
    return data;
  }
  return normalizeAgentInventory(data as AgentInventory);
}

export async function patchAgentTrafficMode(agentID: string, mode: TrafficModeOverride): Promise<AgentInventory> {
  const response = await authFetch(`/api/v1/agents/${agentID}/traffic-mode`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update agent traffic mode");
  }
  return normalizeAgentInventory((await response.json()) as AgentInventory);
}

export async function postAgentGatewayMode(agentID: string, mode: GatewayMode): Promise<AgentInventory> {
  const response = await authFetch(`/api/v1/agents/${agentID}/gateway-mode`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update agent gateway mode");
  }
  return normalizeAgentInventory((await response.json()) as AgentInventory);
}

export async function reissueAgentEnrollment(agentID: string): Promise<EnrollmentToken | PendingSecurityApprovalResponse> {
  const response = await authFetch(`/api/v1/agents/${agentID}/reissue`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to reissue enrollment");
  }
  const data = await response.json();
  if (isPendingSecurityApprovalResponse(data)) {
    return data;
  }
  return data as EnrollmentToken;
}

export async function rotateAgent(agentID: string): Promise<AgentInventory | PendingSecurityApprovalResponse> {
  const response = await authFetch(`/api/v1/agents/${agentID}/rotate`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to request key rotation");
  }
  const data = await response.json();
  if (isPendingSecurityApprovalResponse(data)) {
    return data;
  }
  return normalizeAgentInventory(data as AgentInventory);
}

export async function listPeers(params: ListPeersParams = {}): Promise<PeerViewPage> {
  const search = new URLSearchParams();
  if (params.status) search.set("status", params.status);
  if (params.agent_id) search.set("agent_id", params.agent_id);
  if (params.q) search.set("q", params.q);
  if (params.page_size) search.set("page_size", String(params.page_size));
  else if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  const query = search.toString();

  const response = await authFetch(`/api/v1/peers${query ? `?${query}` : ""}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load peers");
  }
  const data = await response.json();
  return {
    peers: (data.peers ?? []) as PeerView[],
    next_cursor: typeof data.next_cursor === "string" ? data.next_cursor : undefined,
  };
}

export async function reconcilePeer(peerID: string): Promise<PeerView> {
  const response = await authFetch(`/api/v1/peers/${peerID}/reconcile`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to reconcile peer");
  }
  return (await response.json()) as PeerView;
}

export async function getPeer(peerID: string): Promise<PeerView> {
  const response = await authFetch(`/api/v1/peers/${peerID}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load peer");
  }
  return (await response.json()) as PeerView;
}

export async function listAccessPolicies(cursor?: string, pageSize = 50): Promise<AccessPolicyPage> {
  const search = new URLSearchParams();
  search.set("page_size", String(pageSize));
  if (cursor) {
    search.set("cursor", cursor);
  }
  const response = await authFetch(`/api/v1/access-policies?${search.toString()}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load access policies");
  }
  const data = await response.json();
  const policies = (data.policies ?? []) as AccessPolicy[];
  return {
    policies: policies.map(normalizeAccessPolicy),
    next_cursor: typeof data.next_cursor === "string" ? data.next_cursor : undefined,
  };
}

export async function listAllAccessPolicies(): Promise<AccessPolicy[]> {
  const policies: AccessPolicy[] = [];
  let cursor: string | undefined;

  do {
    const page = await listAccessPolicies(cursor, 200);
    policies.push(...page.policies);
    cursor = page.next_cursor;
  } while (cursor);

  return policies;
}

export async function createAccessPolicy(input: CreateAccessPolicyInput): Promise<AccessPolicy> {
  const response = await authFetch("/api/v1/access-policies", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to create access policy");
  }
  return normalizeAccessPolicy((await response.json()) as AccessPolicy);
}

export async function updateAccessPolicy(policyID: string, input: CreateAccessPolicyInput): Promise<AccessPolicy> {
  const response = await authFetch(`/api/v1/access-policies/${policyID}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to update access policy");
  }
  return normalizeAccessPolicy((await response.json()) as AccessPolicy);
}

export async function assignAccessPolicy(policyID: string, agentID: string): Promise<AccessPolicyAssignment> {
  const response = await authFetch(`/api/v1/access-policies/${policyID}/assign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ agent_id: agentID }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to assign access policy");
  }
  return (await response.json()) as AccessPolicyAssignment;
}

export async function unassignAccessPolicy(policyID: string, agentID: string): Promise<AccessPolicyAssignment> {
  const response = await authFetch(`/api/v1/access-policies/${policyID}/unassign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ agent_id: agentID }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to remove access policy assignment");
  }
  return (await response.json()) as AccessPolicyAssignment;
}

export async function simulateAccessPolicy(input: { agent_id?: string; policy_ids?: string[]; traffic_mode_override?: TrafficModeOverride }): Promise<PolicySimulationResult> {
  const response = await authFetch("/api/v1/access-policies/simulate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to simulate access policy");
  }
  return (await response.json()) as PolicySimulationResult;
}

export async function listAuditEvents(params: ListAuditEventsParams = {}): Promise<AuditEventPage> {
  const search = new URLSearchParams();
  if (params.action) search.set("action", params.action);
  if (params.resource_type) search.set("resource_type", params.resource_type);
  if (params.result) search.set("result", params.result);
  if (params.actor_user_id) search.set("actor_user_id", params.actor_user_id);
  if (params.cursor) search.set("cursor", params.cursor);
  if (params.page_size) search.set("page_size", String(params.page_size));
  if (params.limit) search.set("limit", String(params.limit));
  const query = search.toString();

  const response = await authFetch(`/api/v1/audit-events${query ? `?${query}` : ""}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load audit events");
  }
  const data = await response.json();
  return {
    events: (data.events ?? []) as AuditEvent[],
    next_cursor: typeof data.next_cursor === "string" && data.next_cursor ? data.next_cursor : undefined,
  };
}

export async function listEnrollmentTokens(cursor?: string, pageSize = 50): Promise<EnrollmentTokenPage> {
  const search = new URLSearchParams();
  search.set("page_size", String(pageSize));
  if (cursor) {
    search.set("cursor", cursor);
  }
  const response = await authFetch(`/api/v1/enrollment-tokens?${search.toString()}`);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to load enrollment tokens");
  }
  const data = await response.json();
  return {
    tokens: (data.tokens ?? []) as EnrollmentToken[],
    next_cursor: typeof data.next_cursor === "string" ? data.next_cursor : undefined,
  };
}

export async function createEnrollmentToken(input: CreateEnrollmentTokenInput): Promise<EnrollmentToken> {
  const response = await authFetch("/api/v1/enrollment-tokens", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to create enrollment token");
  }
  return (await response.json()) as EnrollmentToken;
}

export async function revokeEnrollmentToken(tokenID: string): Promise<EnrollmentToken> {
  const response = await authFetch(`/api/v1/enrollment-tokens/${tokenID}/revoke`, {
    method: "POST",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to revoke enrollment token");
  }
  return (await response.json()) as EnrollmentToken;
}

// --- Server Update ---

export interface SystemVersion {
  version: string;
  commit_sha: string;
  build_time: string;
}

export interface UpdateCheckResult {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  released_at?: string;
  changelog_url?: string;
  checked_at: string;
}

export interface UpdateStatus {
  state: string;
  message?: string;
  started_at?: string;
}

export async function getSystemVersion(): Promise<SystemVersion> {
  const response = await fetch("/api/v1/system/version");
  if (!response.ok) {
    throw new Error("failed to fetch system version");
  }
  return (await response.json()) as SystemVersion;
}

export async function checkForUpdate(): Promise<UpdateCheckResult> {
  const response = await authFetch("/api/v1/system/update/check");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to check for updates");
  }
  return (await response.json()) as UpdateCheckResult;
}

export async function applyUpdate(targetVersion: string): Promise<UpdateStatus> {
  const response = await authFetch("/api/v1/system/update/apply", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ target_version: targetVersion }),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to start update");
  }
  return (await response.json()) as UpdateStatus;
}

export async function getUpdateStatus(): Promise<UpdateStatus> {
  const response = await authFetch("/api/v1/system/update/status");
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data?.error?.message ?? "failed to get update status");
  }
  return (await response.json()) as UpdateStatus;
}
