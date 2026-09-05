const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
export type Trigger = "scheduled" | "ecommerce_batch_ready" | "consignment_packing" | "consignment_packed";
export type Schedule = { mode?: "daily" | "weekdays" | "selected_weekdays"; times?: string[]; weekdays?: number[]; start_date?: string; end_date?: string };
export type RuleInput = { name: string; enabled: boolean; paused: boolean; trigger_type: Trigger; schedule: Schedule; asset_id: string; printer_id: string; copies: number; daily_limit: number | null; failure_threshold: number; backoff_seconds: number; version: number };
export type Rule = RuleInput & { id: string; timezone: string; created_by: string; asset_name: string; printer_name: string; consecutive_failures: number; backoff_until: string | null; next_run_at: string | null; created_at: string; updated_at: string };
export type Execution = { id: string; rule_id: string; rule_version: number; occurrence_key: string; event_id: string | null; scheduled_at: string | null; test_run: boolean; snapshot: Rule; status: string; attempt_count: number; available_at: string; error: string | null; printer_job_id: string | null; job_status: string | null; created_at: string };
export type Metric = { origin: "automatic" | "manual"; printer_id: string; printer_name: string; jobs: number; copies: number; completed: number; failed: number; cancelled: number; pending: number; failure_events: number };
export type AssetOption = { id: string; name: string; active: boolean };
export type PrinterOption = { id: string; friendly_name: string; status: string; enabled: boolean };
export type History = { id: string; action: string; actor_user_id: string; occurred_at: string; metadata: Record<string, unknown> };
async function request<T>(path: string, init?: RequestInit): Promise<T> {
 const response = await fetch(`${API_BASE_URL}/api/v1/automations${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...init?.headers } });
 if (!response.ok) { const body = await response.json() as { error?: { message?: string } }; throw new Error(body.error?.message ?? `Request failed (${response.status})`); }
 return response.status === 204 ? undefined as T : await response.json() as T;
}
const body = (method: string, value: unknown): RequestInit => ({ method, body: JSON.stringify(value) });
export const automationAPI = {
 options: () => request<{ assets: AssetOption[]; printers: PrinterOption[] }>("/options"),
 rules: () => request<{ rules: Rule[] }>("/rules"),
 save: (id: string | null, input: RuleInput) => request<{ rule: Rule }>(id ? `/rules/${id}` : "/rules", body(id ? "PUT" : "POST", input)),
 pause: (rule: Rule) => request<{ rule: Rule }>(`/rules/${rule.id}/pause`, body("POST", { version: rule.version, paused: !rule.paused })),
 test: (id: string, key: string) => request<{ execution_id: string }>(`/rules/${id}/test`, body("POST", { idempotency_key: key })),
 timezone: () => request<{ timezone: string }>("/timezone"),
 setTimezone: (timezone: string) => request<{ timezone: string }>("/timezone", body("PUT", { timezone })),
 preview: (schedule: Schedule) => request<{ occurrences: string[] }>("/preview", body("POST", schedule)),
 runs: (rule = "", failures = false) => request<{ runs: Execution[] }>(`/runs?rule_id=${encodeURIComponent(rule)}&failures=${failures}`),
 retry: (id: string) => request<void>(`/runs/${id}/retry`, { method: "POST" }),
 upcoming: () => request<{ upcoming: { rule: Rule; at: string }[] }>("/upcoming"),
 report: () => request<{ metrics: Metric[] }>("/report"),
 history: (id: string) => request<{ history: History[] }>(`/rules/${id}/history`),
};
