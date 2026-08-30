const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type InventoryBalance = { product_id: string; internal_code: string; product_name: string; on_hand: number; reserved: number; available: number; updated_at: string };
export type InventoryTransaction = { id: string; product_id: string; transaction_type: "stock_in" | "manual_adjustment" | "correction" | "ecommerce_out"; quantity_delta: number; previous_balance: number; resulting_balance: number; reason: string; reference_type: string | null; reference_id: string | null; actor_user_id: string; created_at: string };
export type Reservation = { id: string; product_id: string; quantity: number; status: "active" | "released"; reason: string; source_type: string; source_id: string; release_reason: string | null; created_at: string; released_at: string | null };
type Command = { product_id: string; quantity: number; reason: string; idempotency_key: string };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? `Request failed (${response.status})`); }
  return await response.json() as T;
}

export const inventoryAPI = {
  balances: () => request<{ inventory: InventoryBalance[] }>("/inventory"),
  transactions: (productID = "", type = "") => request<{ transactions: InventoryTransaction[] }>(`/inventory/transactions?product_id=${encodeURIComponent(productID)}&type=${encodeURIComponent(type)}`),
  stockIn: (input: Command) => request<{ transaction: InventoryTransaction; idempotent_replay: boolean }>("/inventory/stock-in", { method: "POST", body: JSON.stringify(input) }),
  adjust: (input: Command) => request<{ transaction: InventoryTransaction; idempotent_replay: boolean }>("/inventory/adjustments", { method: "POST", body: JSON.stringify(input) }),
  correct: (input: Command) => request<{ transaction: InventoryTransaction; idempotent_replay: boolean }>("/inventory/corrections", { method: "POST", body: JSON.stringify(input) }),
  confirmOutbound: (batchID: string, idempotencyKey: string) => request<{ transactions: InventoryTransaction[]; idempotent_replay: boolean }>(`/inventory/batches/${batchID}/confirm-outbound`, { method: "POST", body: JSON.stringify({ idempotency_key: idempotencyKey }) }),
  reservations: () => request<{ reservations: Reservation[] }>("/inventory/reservations"),
  reserve: (input: { product_id: string; quantity: number; reason: string; source_type: string; source_id: string; idempotency_key: string }) => request<{ reservation: Reservation; idempotent_replay: boolean }>("/inventory/reservations", { method: "POST", body: JSON.stringify(input) }),
  release: (id: string, reason: string, idempotencyKey: string) => request<{ reservation: Reservation; idempotent_replay: boolean }>(`/inventory/reservations/${id}/release`, { method: "POST", body: JSON.stringify({ reason, idempotency_key: idempotencyKey }) }),
};
