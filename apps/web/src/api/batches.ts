const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type MarketplaceKey = "flipkart" | "amazon" | "meesho" | "myntra" | "snapdeal";

export type EligibleOrder = {
  order_id: string;
  source_file_id: string;
  processing_job_id: string;
  source_page: number;
  marketplace_order_id: string | null;
  awb: string | null;
  status: string;
  unresolved_count: number;
};

export type BatchMember = {
  order_id: string;
  position: number;
  source_file_id: string;
  processing_job_id: string;
  source_page: number;
  marketplace_order_id: string | null;
  awb: string | null;
  status: string;
};

export type ProductTotal = {
  product_id: string;
  internal_code: string;
  product_name: string;
  total_quantity: number;
  order_line_count: number;
};

export type WorkerTotal = {
  employee_id: string;
  employee_name: string;
  total_quantity: number;
  order_line_count: number;
  product_count: number;
};

export type AssignmentRule = {
  id: string;
  marketplace_key: MarketplaceKey;
  product_id: string | null;
  product_code: string | null;
  product_name: string | null;
  employee_id: string;
  employee_name: string;
  priority: number;
};

export type Batch = {
  id: string;
  marketplace_key: MarketplaceKey;
  status: "draft" | "ready" | "cancelled";
  created_by: string;
  order_count: number;
  unresolved_count: number;
  ready_at: string | null;
  cancelled_at: string | null;
  created_at: string;
  updated_at: string;
  members?: BatchMember[];
  product_totals?: ProductTotal[];
  worker_totals?: WorkerTotal[];
};

export type PrintArtifact = {
  id: string;
  kind: "labels" | "invoices";
  size_bytes: number;
  sha256: string;
  page_count: number;
};

export type PrintJob = {
  id: string;
  batch_id: string;
  status: "generating" | "ready" | "failed";
  sort_labels: boolean;
  export_invoices: boolean;
  generation_version: string;
  source_print_job_id: string | null;
  reprint_reason: string | null;
  error_code: string | null;
  error_message: string | null;
  completed_at: string | null;
  created_at: string;
  artifacts: PrintArtifact[];
};

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
  return (await response.json()) as T;
}

export const batchAPI = {
  eligibleOrders: (marketplace: MarketplaceKey) => request<{ orders: EligibleOrder[] }>(`/batch-eligible-orders?marketplace=${marketplace}`),
  create: (marketplace: MarketplaceKey, orderIDs: string[], idempotencyKey: string) => request<{ batch: Batch; idempotent_replay: boolean }>("/batches", {
    method: "POST",
    body: JSON.stringify({ marketplace_key: marketplace, order_ids: orderIDs, idempotency_key: idempotencyKey }),
  }),
  get: (id: string) => request<{ batch: Batch }>(`/batches/${id}`),
  ready: (id: string) => request<{ batch: Batch }>(`/batches/${id}/ready`, { method: "POST" }),
  generate: (batchID: string, sortLabels: boolean, exportInvoices: boolean, idempotencyKey: string) => request<{ print_job: PrintJob; idempotent_replay: boolean }>(`/batches/${batchID}/print-jobs`, {
    method: "POST",
    body: JSON.stringify({ sort_labels: sortLabels, export_invoices: exportInvoices, idempotency_key: idempotencyKey }),
  }),
  printJob: (id: string) => request<{ print_job: PrintJob }>(`/print-jobs/${id}`),
  printJobs: (batchID: string) => request<{ print_jobs: PrintJob[] }>(`/batches/${batchID}/print-jobs`),
  reprint: (printJobID: string, reason: string, idempotencyKey: string) => request<{ print_job: PrintJob; idempotent_replay: boolean }>(`/print-jobs/${printJobID}/reprints`, {
    method: "POST",
    body: JSON.stringify({ reason, idempotency_key: idempotencyKey }),
  }),
  assignmentRules: (marketplace: MarketplaceKey) => request<{ worker_assignment_rules: AssignmentRule[] }>(`/worker-assignment-rules?marketplace=${marketplace}`),
  replaceAssignmentRules: (marketplace: MarketplaceKey, rules: Array<{ product_id: string | null; employee_id: string; priority: number }>) => request<{ worker_assignment_rules: AssignmentRule[] }>("/worker-assignment-rules", {
    method: "PUT",
    body: JSON.stringify({ marketplace_key: marketplace, rules }),
  }),
  downloadArtifact: async (artifact: PrintArtifact) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/print-artifacts/${artifact.id}`, { credentials: "include" });
    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as APIError;
      throw new Error(body.error?.message ?? `Download failed (${response.status})`);
    }
    const url = URL.createObjectURL(await response.blob());
    const link = document.createElement("a");
    link.href = url;
    link.download = `${artifact.kind}.pdf`;
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
  },
};
