const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type TraceContent = { product_id: string; internal_code: string; product_name: string; quantity: number };
export type TraceCustody = { event_id: string; employee_id: string | null; department_id: string | null; custodian_name: string; actor_user_id: string; transferred_at: string };
export type TraceEvent = { id: string; event_type: "box_created" | "content_added" | "content_removed" | "custody_transferred"; actor_user_id: string; notes: string | null; metadata: Record<string, unknown>; idempotency_key: string; created_at: string };
export type TraceBox = { id: string; opaque_identifier: string; label: string | null; created_by: string; created_at: string; contents: TraceContent[]; current_custody: TraceCustody | null; events: TraceEvent[] };
export type TraceOptions = { products: { id: string; internal_code: string; name: string }[]; employees: { id: string; name: string }[]; departments: { id: string; name: string }[] };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? `Request failed (${response.status})`); }
  return await response.json() as T;
}

const key = () => crypto.randomUUID();
const action = (path: string, body: Record<string, unknown>) => request<{ trace_box: TraceBox; idempotent_replay: boolean }>(path, { method: "POST", body: JSON.stringify({ ...body, idempotency_key: key() }) });

export const traceabilityAPI = {
  list: () => request<{ trace_boxes: TraceBox[] }>("/trace-boxes"),
  options: () => request<{ options: TraceOptions }>("/trace-box-options"),
  get: (id: string) => request<{ trace_box: TraceBox }>(`/trace-boxes/${id}`),
  resolve: (identifier: string) => request<{ trace_box: TraceBox }>(`/trace-box-resolutions/${encodeURIComponent(identifier.trim())}`),
  create: (label: string) => action("/trace-boxes", { label: label.trim() || null }),
  addContent: (boxID: string, productID: string, quantity: number, notes: string) => action(`/trace-boxes/${boxID}/contents`, { product_id: productID, quantity, notes: notes.trim() || null }),
  removeContent: (boxID: string, productID: string, quantity: number, notes: string) => action(`/trace-boxes/${boxID}/contents/remove`, { product_id: productID, quantity, notes: notes.trim() || null }),
  transferCustody: (boxID: string, target: { employee_id: string | null; department_id: string | null }, notes: string) => action(`/trace-boxes/${boxID}/custody`, { ...target, notes: notes.trim() || null }),
};
