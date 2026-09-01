const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type ReturnStatus = "expected" | "received" | "inspected" | "inspection_pending" | "restocked" | "restock_corrected" | "damaged" | "rejected" | "closed";
export type ReturnDisposition = "pending" | "restockable" | "damaged" | "wrong_product" | "missing" | "rejected";

export type ReturnItem = {
  id: string; marketplace_order_item_id: string; product_id: string; internal_code: string; product_name: string;
  expected_quantity: number; received_quantity: number | null; disposition: ReturnDisposition; restocked_quantity: number; corrected_quantity: number;
};
export type LifecycleEvent = { id: string; event_type: string; actor_user_id: string; notes: string | null; created_at: string };
export type InventoryImpact = { transaction_id: string; product_id: string; transaction_type: string; quantity_delta: number; reference_type: string; reference_id: string; created_at: string };
export type ReturnCase = {
  id: string; marketplace_order_id: string; marketplace: string; external_order_id: string | null; status: ReturnStatus; reason: string; notes: string | null;
  created_by: string; received_by: string | null; received_at: string | null; closed_by: string | null; closed_at: string | null; created_at: string; updated_at: string;
  items: ReturnItem[]; events: LifecycleEvent[]; inventory_impact: InventoryImpact[];
};
export type Cancellation = {
  id: string; marketplace_order_id: string; marketplace: string; external_order_id: string | null; status: "recorded" | "closed"; outbound_state: "not_outbound" | "outbound_confirmed";
  reason: string; cancelled_at: string; recorded_by: string; closed_by: string | null; closed_at: string | null; created_at: string; updated_at: string; events: LifecycleEvent[];
};

const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(`${API_BASE_URL}${path}`, { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
    throw new Error(body.error?.message ?? `Request failed (${response.status})`);
  }
  return await response.json() as T;
};

const post = <T>(path: string, body: unknown) => request<T>(path, { method: "POST", body: JSON.stringify(body) });
const key = () => crypto.randomUUID();

export const returnsAPI = {
  listReturns: async (marketplace = "") => {
    const query = marketplace ? `?marketplace=${encodeURIComponent(marketplace)}` : "";
    return (await request<{ returns: ReturnCase[] }>(`/api/v1/returns${query}`)).returns;
  },
  getReturn: async (id: string) => (await request<{ return: ReturnCase }>(`/api/v1/returns/${id}`)).return,
  receive: async (id: string, items: { return_item_id: string; received_quantity: number }[], notes?: string) =>
    (await post<{ return: ReturnCase }>(`/api/v1/returns/${id}/receive`, { items, notes: notes || null, idempotency_key: key() })).return,
  inspect: async (id: string, items: { return_item_id: string; disposition: Exclude<ReturnDisposition, "pending"> }[], notes?: string) =>
    (await post<{ return: ReturnCase }>(`/api/v1/returns/${id}/inspect`, { items, notes: notes || null, idempotency_key: key() })).return,
  restock: async (id: string, notes?: string) =>
    (await post<{ return: ReturnCase }>(`/api/v1/returns/${id}/restock`, { notes: notes || null, idempotency_key: key() })).return,
  correctRestock: async (id: string, items: { return_item_id: string; quantity: number }[], reason: string) =>
    (await post<{ return: ReturnCase }>(`/api/v1/returns/${id}/restock-corrections`, { items, reason, idempotency_key: key() })).return,
  closeReturn: async (id: string, notes?: string) =>
    (await post<{ return: ReturnCase }>(`/api/v1/returns/${id}/close`, { notes: notes || null, idempotency_key: key() })).return,
  listCancellations: async (marketplace = "") => {
    const query = marketplace ? `?marketplace=${encodeURIComponent(marketplace)}` : "";
    return (await request<{ cancellations: Cancellation[] }>(`/api/v1/cancellations${query}`)).cancellations;
  },
  getCancellation: async (id: string) => (await request<{ cancellation: Cancellation }>(`/api/v1/cancellations/${id}`)).cancellation,
  closeCancellation: async (id: string, notes?: string) =>
    (await post<{ cancellation: Cancellation }>(`/api/v1/cancellations/${id}/close`, { notes: notes || null, idempotency_key: key() })).cancellation,
};
