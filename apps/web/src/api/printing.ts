const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type Printer = { id: string; agent_id: string; friendly_name: string; status: "online" | "offline"; enabled: boolean; location: string | null; last_seen_at: string | null };
export type PrintAsset = { id: string; name: string; category: string; description: string | null; default_printer_id: string | null; default_copies: number; favorite: boolean; active: boolean; page_count: number };
export type PrinterJob = { id: string; printer_id: string; print_library_asset_id: string | null; copies: number; origin_type: string; status: "queued" | "claimed" | "printing" | "completed" | "failed" | "cancelled"; failure_message: string | null; created_at: string };
type APIError = { error?: { message?: string } };

// Browser calls use only the session cookie. Agent credentials deliberately do
// not exist in this client or frontend persistence.
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as APIError;
    throw new Error(body.error?.message ?? `Request failed (${response.status})`);
  }
  return await response.json() as T;
}

export const printingAPI = {
  printers: () => request<{ printers: Printer[] }>("/printers"),
  assets: (search = "", category = "") => request<{ assets: PrintAsset[] }>(`/print-library-assets?search=${encodeURIComponent(search)}&category=${encodeURIComponent(category)}`),
  jobs: () => request<{ printer_jobs: PrinterJob[] }>("/printer-jobs"),
  quickPrint: (asset_id: string, printer_id: string, copies: number, large_quantity_confirmed: boolean, idempotency_key: string) => request<{ printer_job: PrinterJob; idempotent_replay: boolean }>("/printer-jobs", { method: "POST", body: JSON.stringify({ asset_id, printer_id, copies, large_quantity_confirmed, idempotency_key }) }),
  queueArtifact: (artifact_id: string, printer_id: string, copies: number, idempotency_key: string) => request<{ printer_job: PrinterJob; idempotent_replay: boolean }>(`/print-artifacts/${artifact_id}/printer-jobs`, { method: "POST", body: JSON.stringify({ printer_id, copies, idempotency_key }) }),
  createAgent: (friendly_name: string) => request<{ agent: { id: string; friendly_name: string }; credential: string }>("/printer-agents", { method: "POST", body: JSON.stringify({ friendly_name, platform: "linux_cups" }) }),
  uploadAsset: async (form: FormData) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/print-library-assets`, { method: "POST", credentials: "include", body: form });
    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as APIError;
      throw new Error(body.error?.message ?? `Upload failed (${response.status})`);
    }
    return await response.json() as { asset: PrintAsset };
  },
};
