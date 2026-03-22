export type Role = "admin" | "operator" | "readonly";

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
  issued_at: string;
  expires_at: string;
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
  const response = await fetch(`${baseUrl}/api/v1/health/reconcile`, {
    credentials: "include",
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`reconcile health failed: ${text}`);
  }
  return (await response.json()) as HealthResponse;
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

export async function logout(): Promise<void> {
  await fetch("/api/v1/sessions/current", {
    method: "DELETE",
    credentials: "include",
  });
}

export async function getCurrentSession(): Promise<SessionResponse | null> {
  const response = await fetch("/api/v1/sessions/current", {
    credentials: "include",
  });
  if (response.status === 401) {
    return null;
  }
  if (!response.ok) {
    return null;
  }
  return (await response.json()) as SessionResponse;
}
