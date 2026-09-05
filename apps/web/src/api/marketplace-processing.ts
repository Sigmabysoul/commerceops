const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type Job = {
  id: string;
  status: "queued" | "processing" | "needs_review" | "processed" | "failed";
  parser_version: string;
  total_pages: number;
  processed_pages: number;
  created_at: string;
  updated_at: string;
};

export type JobDetails = {
  job: Job;
  orders: Array<{
    id: string;
    source_page: number;
    marketplace_order_id: string | null;
    awb: string | null;
    status: string;
    metadata: Record<string, unknown>;
    documents: Array<{ source_page: number; role: string; extraction_method: string }>;
    items: Array<{
      raw_sku: string | null;
      product_id: string | null;
      quantity: number | null;
      quantity_source: string;
      resolution_status: string;
      warnings: string[];
    }>;
  }>;
  errors: Array<{ source_page: number | null; severity: string; code: string; message: string }>;
};

export type MarketplaceProcessingAPI = {
  upload: (file: File, idempotencyKey?: string) => Promise<{ job: Job; duplicate_source: boolean }>;
  job: (id: string) => Promise<JobDetails>;
  retry: (id: string) => Promise<{ job: Job }>;
};

async function parse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
    throw new Error(body.error?.message ?? `Request failed (${response.status})`);
  }
  return await response.json() as T;
}

export function marketplaceProcessingAPI(marketplace: string): MarketplaceProcessingAPI {
  const jobsURL = `${API_BASE_URL}/api/v1/${marketplace}/jobs`;
  return {
    upload: async (file: File, idempotencyKey?: string) => {
      const data = new FormData();
      data.append("file", file);
      if (idempotencyKey) data.append("idempotency_key", idempotencyKey);
      return parse(await fetch(jobsURL, { method: "POST", credentials: "include", body: data }));
    },
    job: async (id: string) => parse(await fetch(`${jobsURL}/${id}`, { credentials: "include" })),
    retry: async (id: string) => parse(await fetch(`${jobsURL}/${id}`, { method: "POST", credentials: "include" })),
  };
}
