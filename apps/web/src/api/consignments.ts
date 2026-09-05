const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type DepartmentMember = { employee_id: string; name: string; user_id: string | null };
export type ConsignmentDepartment = { id: string; name: string; status: "active" | "inactive"; members: DepartmentMember[] };
export type ConsignmentLine = { id: string; product_id: string; internal_code: string; product_name: string; department_id: string; department_name: string; required_quantity: number; ready_quantity: number; packed_quantity: number; progress: "pending" | "ready" | "packed"; version: number };
export type ConsignmentEvent = { id: string; event_type: string; actor_user_id: string; notes: string | null; metadata: Record<string, unknown>; created_at: string };
export type Consignment = { id: string; order_reference: string; dealer_reference: string | null; pouch_reference: string | null; source_type: "manual" | "import"; source_reference: string | null; status: "created" | "allocated" | "picking" | "ready" | "packing" | "packed" | "outbound" | "completed" | "cancelled"; notes: string | null; version: number; created_at: string; updated_at: string; lines: ConsignmentLine[]; events: ConsignmentEvent[] };
export type ConsignmentLineInput = { product_id: string; department_id: string; required_quantity: number };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? `Request failed (${response.status})`); }
  return await response.json() as T;
}
const key = () => crypto.randomUUID();
const action = (path: string, version: number, notes?: string) => request<{ consignment: Consignment; idempotent_replay: boolean }>(path, { method: "POST", body: JSON.stringify({ expected_version: version, notes: notes || null, idempotency_key: key() }) });

export const consignmentAPI = {
  departments: () => request<{ departments: ConsignmentDepartment[] }>("/consignment-departments"),
  createDepartment: (name: string) => request<{ department: ConsignmentDepartment }>("/consignment-departments", { method: "POST", body: JSON.stringify({ name, status: "active" }) }),
  updateDepartment: (item: ConsignmentDepartment, status: "active" | "inactive") => request<{ department: ConsignmentDepartment }>(`/consignment-departments/${item.id}`, { method: "PATCH", body: JSON.stringify({ name: item.name, status }) }),
  setMembers: (id: string, employeeIDs: string[]) => request<{ department: ConsignmentDepartment }>(`/consignment-departments/${id}/members`, { method: "PUT", body: JSON.stringify({ employee_ids: employeeIDs }) }),
  list: (status = "", departmentID = "", query = "") => request<{ consignments: Consignment[] }>(`/consignments?status=${encodeURIComponent(status)}&department_id=${encodeURIComponent(departmentID)}&q=${encodeURIComponent(query)}`),
  get: (id: string) => request<{ consignment: Consignment }>(`/consignments/${id}`),
  create: (input: { order_reference: string; dealer_reference: string | null; pouch_reference: string | null; source_type: "manual" | "import"; source_reference: string | null; notes: string | null; lines: ConsignmentLineInput[] }) => request<{ consignment: Consignment; idempotent_replay: boolean }>("/consignments", { method: "POST", body: JSON.stringify({ ...input, idempotency_key: key() }) }),
  allocate: (item: Consignment) => action(`/consignments/${item.id}/allocate`, item.version),
  transition: (item: Consignment, target_status: string) => request<{ consignment: Consignment; idempotent_replay: boolean }>(`/consignments/${item.id}/transition`, { method: "POST", body: JSON.stringify({ target_status, expected_version: item.version, idempotency_key: key() }) }),
  progress: (item: Consignment, line: ConsignmentLine, ready_quantity: number, packed_quantity: number) => request<{ consignment: Consignment; idempotent_replay: boolean }>(`/consignments/${item.id}/lines/${line.id}/progress`, { method: "POST", body: JSON.stringify({ ready_quantity, packed_quantity, expected_version: line.version, idempotency_key: key() }) }),
  outbound: (item: Consignment) => action(`/consignments/${item.id}/confirm-outbound`, item.version),
  cancel: (item: Consignment, notes: string) => action(`/consignments/${item.id}/cancel`, item.version, notes),
};
