"use client";

import { FormEvent, useEffect, useState } from "react";
import { JobDetails, MarketplaceProcessingAPI } from "@/api/marketplace-processing";

type Props = {
  api: MarketplaceProcessingAPI;
  marketplace: string;
  phase: string;
  sourceType?: "PDF" | "CSV";
  requireIdempotency?: boolean;
};

export function MarketplaceProcessing({ api, marketplace, phase, sourceType = "PDF", requireIdempotency = false }: Props) {
  const [details, setDetails] = useState<JobDetails | null>(null);
  const [jobID, setJobID] = useState("");
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);
  const [duplicate, setDuplicate] = useState(false);
  const displayName = marketplace.charAt(0).toUpperCase() + marketplace.slice(1);

  useEffect(() => {
    if (!jobID || details?.job.status === "processed" || details?.job.status === "needs_review" || details?.job.status === "failed") return;
    const timer = setInterval(() => api.job(jobID).then(setDetails).catch((cause) => setError(message(cause))), 1200);
    return () => clearInterval(timer);
  }, [api, jobID, details?.job.status]);

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const file = data.get("file");
    const idempotencyKey = String(data.get("idempotency_key") ?? "").trim();
    if (!(file instanceof File) || !file.size) return;
    setUploading(true);
    setError("");
    setDetails(null);
    try {
      const result = await api.upload(file, idempotencyKey || undefined);
      setJobID(result.job.id);
      setDuplicate(result.duplicate_source);
      setDetails(await api.job(result.job.id));
      form.reset();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setUploading(false);
    }
  }

  async function retry() {
    if (!details) return;
    try {
      const result = await api.retry(details.job.id);
      setDetails({ ...details, job: result.job, orders: [], errors: [] });
      setJobID(result.job.id);
    } catch (cause) {
      setError(message(cause));
    }
  }

  const unit = sourceType === "CSV" ? "rows" : "pages";
  return <section className="flipkart"><div className="product-heading"><div><p className="eyebrow">{phase}</p><h2>{displayName} processing</h2><p className="muted">Upload a {displayName} {sourceType}. Processing continues in the shared bounded background queue.</p></div></div>
    <section className="panel"><form className="upload-row" onSubmit={upload}><label>{displayName} {sourceType}<input name="file" type="file" accept={sourceType === "CSV" ? "text/csv,.csv" : "application/pdf,.pdf"} required /></label>{requireIdempotency && <label>Import key<input name="idempotency_key" maxLength={128} required /></label>}<button disabled={uploading}>{uploading ? "Uploading…" : "Upload and process"}</button></form>{error && <p className="error" role="alert">{error}</p>}{duplicate && <p className="notice">This exact source file was already uploaded. Showing its existing job.</p>}</section>
    {details && <section className="panel results"><div className="status-line"><h2>Processing results</h2><span className={`status status-${details.job.status}`}>{details.job.status.replace("_", " ")}</span></div><p className="muted">{details.job.processed_pages}/{details.job.total_pages} {unit} · parser {details.job.parser_version}</p>{["needs_review", "failed", "processed"].includes(details.job.status) && <button onClick={retry}>Reprocess with current SKU training</button>}
      {marketplace === "myntra" && <p className="notice">Myntra CSV does not provide authoritative quantity evidence. Imported rows remain blocked from print-ready, outbound, and quantity-dependent return flows.</p>}
      {details.orders.map((order) => <article key={order.id}><div><strong>{sourceType === "CSV" ? "Row" : "Page"} {order.source_page}</strong> · {order.awb ?? "AWB missing"}<small>{order.marketplace_order_id ?? "Order ID missing"} · {reviewLabel(order)}</small></div>{order.items.map((item, index) => <div key={index}><span>{item.raw_sku ?? "SKU missing"} → {item.product_id ?? "Product training required"}</span><small>Quantity: {item.quantity ?? "Needs Quantity Evidence"} ({item.quantity_source}) · {item.resolution_status}</small></div>)}</article>)}
      {details.errors.length > 0 && <div className="review"><h3>Warnings and errors</h3><ul>{details.errors.map((item, index) => <li key={`${item.code}-${index}`}><span>{item.code}<small>{item.source_page ? `Page ${item.source_page} · ` : ""}{item.message}</small></span></li>)}</ul></div>}
    </section>}
  </section>;
}

function message(cause: unknown) {
  return cause instanceof Error ? cause.message : "Something went wrong";
}

function reviewLabel(order: JobDetails["orders"][number]) {
  if (order.status === "duplicate") return "Duplicate";
  if (order.items.some((item) => !item.product_id)) return "Needs Product Mapping";
  if (order.items.some((item) => item.quantity === null)) return "Needs Quantity Evidence";
  return order.status === "resolved" ? "Resolved" : "Failed";
}
