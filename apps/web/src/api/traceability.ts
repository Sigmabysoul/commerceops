const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type TraceContent = { product_id: string; internal_code: string; product_name: string; quantity: number };
export type TraceCustody = { event_id: string; employee_id: string | null; department_id: string | null; custodian_name: string; actor_user_id: string; transferred_at: string };
export type TraceEvent = { id: string; event_type: string; actor_user_id: string; notes: string | null; metadata: Record<string, unknown>; idempotency_key: string; created_at: string };
export type TraceWorkRequirement = { id: string; product_id: string; internal_code: string; product_name: string; quantity: number; work_type: string; rejection_reason: string; created_at: string; completed_at: string | null; completed_by_employee_id: string | null };
export type TraceWorkflow = { status: string; latest_qc: { event_id: string; checked_by_employee_id: string; checked_at: string; passed_quantity: number; rejected_quantity: number } | null; work_requirements: TraceWorkRequirement[]; pending_handover: { id: string; target_employee_id: string | null; target_department_id: string | null; target_name: string; sent_at: string } | null; packed_at: string | null; final_check: { event_id: string; passed: boolean; checked_at: string } | null; ready_at: string | null };
export type TraceBox = { id: string; opaque_identifier: string; label: string | null; created_by: string; created_at: string; contents: TraceContent[]; current_custody: TraceCustody | null; workflow: TraceWorkflow; events: TraceEvent[] };
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
  recordQC: (boxID: string, lines: { product_id: string; checked_quantity: number; passed_quantity: number; rejected_quantity: number; rejection_reason: string | null; required_work: string | null }[], notes: string) => action(`/trace-boxes/${boxID}/qc`, { lines, notes: notes.trim() || null }),
  completeWork: (boxID: string, requirementID: string) => action(`/trace-boxes/${boxID}/work-requirements/${requirementID}/complete`, {}),
  sendHandover: (boxID: string, target: { employee_id: string | null; department_id: string | null }, notes: string) => action(`/trace-boxes/${boxID}/handovers`, { ...target, notes: notes.trim() || null }),
  receiveHandover: (boxID: string, handoverID: string) => action(`/trace-boxes/${boxID}/handovers/${handoverID}/receive`, {}),
  completePacking: (boxID: string, notes: string) => action(`/trace-boxes/${boxID}/packing/complete`, { notes: notes.trim() || null }),
  completeFinalCheck: (boxID: string, passed: boolean, notes: string) => action(`/trace-boxes/${boxID}/final-checks`, { passed, notes: notes.trim() || null }),
  markReady: (boxID: string, notes: string) => action(`/trace-boxes/${boxID}/shipment-readiness`, { notes: notes.trim() || null }),
};
