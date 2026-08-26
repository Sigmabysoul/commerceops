const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type Principal = { user_id: string; company_id: string; email: string };
export type Company = { id: string; name: string; status: string };
export type Employee = { id: string; display_name: string; status: string; user_id: string | null };
export type Role = { id: string; name: string; status: string; permissions: string[] };
export type Permission = { key: string; description: string };
export type Entitlement = { module_key: string; enabled: boolean };
export type AuditEntry = { id: string; action: string; target_type: string; target_id: string; occurred_at: string };

type APIError = { error?: { message?: string } };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as APIError;
    throw new Error(body.error?.message ?? `Request failed (${response.status})`);
  }
  return response.status === 204 ? (undefined as T) : ((await response.json()) as T);
}

export const coreAPI = {
  session: () => request<{ user: Principal }>("/auth/session"),
  login: (email: string, password: string, companyID: string) => request<{ user: Principal }>("/auth/login", { method: "POST", body: JSON.stringify({ email, password, company_id: companyID }) }),
  logout: () => request<void>("/auth/logout", { method: "POST" }),
  company: () => request<{ company: Company }>("/company"),
  employees: () => request<{ employees: Employee[] }>("/employees"),
  createEmployee: (displayName: string) => request<{ employee: Employee }>("/employees", { method: "POST", body: JSON.stringify({ display_name: displayName }) }),
  roles: () => request<{ roles: Role[] }>("/roles"),
  createRole: (name: string) => request<{ role: Role }>("/roles", { method: "POST", body: JSON.stringify({ name }) }),
  permissions: () => request<{ permissions: Permission[] }>("/permissions"),
  setRolePermissions: (roleID: string, permissions: string[]) => request<void>(`/roles/${roleID}/permissions`, { method: "PUT", body: JSON.stringify({ permissions }) }),
  entitlements: () => request<{ module_entitlements: Entitlement[] }>("/module-entitlements"),
  setEntitlement: (moduleKey: string, enabled: boolean) => request<void>(`/module-entitlements/${moduleKey}`, { method: "PUT", body: JSON.stringify({ enabled }) }),
  audit: () => request<{ audit_logs: AuditEntry[] }>("/audit-logs"),
};
