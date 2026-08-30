"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Batch, EligibleOrder, PrintArtifact, PrintJob, batchAPI } from "@/api/batches";

type Operation = "creating" | "readying" | "generating" | "downloading" | null;

export function BatchPrinting() {
  const [orders, setOrders] = useState<EligibleOrder[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [batch, setBatch] = useState<Batch | null>(null);
  const [printJob, setPrintJob] = useState<PrintJob | null>(null);
  const [sortLabels, setSortLabels] = useState(false);
  const [exportInvoices, setExportInvoices] = useState(false);
  const [operation, setOperation] = useState<Operation>(null);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState("");

  const loadEligible = useCallback(async () => {
    const result = await batchAPI.eligibleOrders();
    setOrders(result.orders);
    setSelected((current) => current.filter((id) => result.orders.some((order) => order.order_id === id)));
  }, []);

  useEffect(() => {
    loadEligible().catch(() => setAvailable(false));
  }, [loadEligible]);

  useEffect(() => {
    if (printJob?.status !== "generating") return;
    const timer = window.setInterval(() => {
      batchAPI.printJob(printJob.id).then((result) => setPrintJob(result.print_job)).catch((cause) => setError(message(cause)));
    }, 1200);
    return () => window.clearInterval(timer);
  }, [printJob?.id, printJob?.status]);

  const selectedOrders = useMemo(() => orders.filter((order) => selected.includes(order.order_id)), [orders, selected]);
  const selectedUnresolved = selectedOrders.reduce((total, order) => total + order.unresolved_count, 0);

  async function createBatch() {
    setOperation("creating"); setError(""); setPrintJob(null);
    try {
      const result = await batchAPI.create(selected, crypto.randomUUID());
      setBatch(result.batch); setSelected([]); await loadEligible();
    } catch (cause) { setError(message(cause)); }
    finally { setOperation(null); }
  }

  async function readyBatch() {
    if (!batch) return;
    setOperation("readying"); setError("");
    try { setBatch((await batchAPI.ready(batch.id)).batch); }
    catch (cause) { setError(message(cause)); }
    finally { setOperation(null); }
  }

  async function generate() {
    if (!batch) return;
    setOperation("generating"); setError(""); setPrintJob(null);
    try { setPrintJob((await batchAPI.generate(batch.id, sortLabels, exportInvoices, crypto.randomUUID())).print_job); }
    catch (cause) { setError(message(cause)); }
    finally { setOperation(null); }
  }

  async function download(artifact: PrintArtifact) {
    setOperation("downloading"); setError("");
    try { await batchAPI.downloadArtifact(artifact); }
    catch (cause) { setError(message(cause)); }
    finally { setOperation(null); }
  }

  function toggle(id: string) {
    setSelected((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id]);
  }

  if (!available) return <section className="panel batch-printing"><h2>Batch printing</h2><p className="muted">Batch printing is not available for this account.</p></section>;

  return <section className="batch-printing">
    <div className="product-heading"><div><p className="eyebrow">Phase 4</p><h2>Batch printing</h2><p className="muted">Select processed Flipkart orders, review product totals, and generate print-ready PDFs.</p></div><button className="secondary" disabled={operation !== null} onClick={() => loadEligible().catch((cause) => setError(message(cause)))}>Refresh orders</button></div>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="batch-layout">
      <section className="panel batch-orders"><div className="status-line"><div><h2>Eligible orders</h2><p className="muted">{orders.length} available · {selected.length} selected</p></div>{orders.length > 0 && <button className="secondary" onClick={() => setSelected(selected.length === orders.length ? [] : orders.map((order) => order.order_id))}>{selected.length === orders.length ? "Clear all" : "Select all"}</button>}</div>
        {orders.length === 0 ? <p className="empty-state">No processed orders are currently eligible for a batch.</p> : <ul className="selectable-list">{orders.map((order) => <li key={order.order_id}><label><input type="checkbox" checked={selected.includes(order.order_id)} onChange={() => toggle(order.order_id)} /><span><strong>{order.marketplace_order_id ?? "Order ID unavailable"}</strong><small>{order.awb ?? "AWB unavailable"} · source page {order.source_page}</small></span></label><span className={`status ${order.unresolved_count ? "status-needs_review" : "status-processed"}`}>{order.unresolved_count ? `${order.unresolved_count} unresolved` : "resolved"}</span></li>)}</ul>}
        <div className="panel-actions"><span className={selectedUnresolved ? "warning-text" : "muted"}>{selectedUnresolved ? `${selectedUnresolved} unresolved items will block readiness.` : "Only the selected order IDs are submitted."}</span><button disabled={!selected.length || operation !== null} onClick={createBatch}>{operation === "creating" ? "Creating…" : "Create batch"}</button></div>
      </section>

      <section className="panel batch-summary"><div className="status-line"><h2>Batch summary</h2>{batch && <span className={`status status-${batch.status}`}>{batch.status}</span>}</div>
        {!batch ? <p className="empty-state">Create a batch to preview its server-derived totals and printing options.</p> : <><dl className="summary-metrics"><div><dt>Orders</dt><dd>{batch.order_count}</dd></div><div><dt>Products</dt><dd>{batch.product_totals?.length ?? 0}</dd></div><div><dt>Unresolved</dt><dd>{batch.unresolved_count}</dd></div></dl>
          {(batch.product_totals?.length ?? 0) > 0 && <div className="product-totals"><h3>Product totals</h3><ul>{batch.product_totals?.map((product) => <li key={product.product_id}><span><strong>{product.internal_code}</strong> · {product.product_name}<small>{product.order_line_count} order lines</small></span><strong>{product.total_quantity}</strong></li>)}</ul></div>}
          {batch.status === "draft" && <div className="panel-actions"><span className="muted">Ready batches can generate printable output.</span><button disabled={batch.unresolved_count > 0 || operation !== null} onClick={readyBatch}>{operation === "readying" ? "Marking ready…" : "Mark ready"}</button></div>}
          {batch.status === "ready" && <div className="print-options"><h3>Printable output</h3><label><input type="checkbox" checked={sortLabels} onChange={(event) => setSortLabels(event.target.checked)} /><span><strong>Sort Labels</strong><small>Use the server-configured Product Master ordering.</small></span></label><label><input type="checkbox" checked={exportInvoices} onChange={(event) => setExportInvoices(event.target.checked)} /><span><strong>Export Invoices</strong><small>Create a separate invoice PDF in corresponding order.</small></span></label><button disabled={operation !== null} onClick={generate}>{operation === "generating" ? "Generating…" : "Generate PDFs"}</button></div>}
        </>}
      </section>
    </div>
    {printJob && <section className="panel print-result"><div className="status-line"><div><h2>Print output</h2><p className="muted">Generation {printJob.generation_version}</p></div><span className={`status status-${printJob.status}`}>{printJob.status}</span></div>
      {printJob.status === "generating" && <div className="progress" role="progressbar" aria-label="Generating printable PDFs"><span /></div>}
      {printJob.status === "failed" && <p className="error" role="alert">{printJob.error_message ?? "PDF generation failed."}</p>}
      {printJob.status === "ready" && <><dl className="summary-metrics"><div><dt>Pages</dt><dd>{printJob.artifacts.find((artifact) => artifact.kind === "labels")?.page_count ?? 0}</dd></div><div><dt>Label order</dt><dd>{printJob.sort_labels ? "Sorted" : "Original"}</dd></div><div><dt>Files</dt><dd>{printJob.artifacts.length}</dd></div></dl><div className="artifact-list">{printJob.artifacts.map((artifact) => <article key={artifact.id}><div><strong>{artifact.kind === "labels" ? "Shipping labels PDF" : "Invoices PDF"}</strong><small>{artifact.page_count} pages · {formatBytes(artifact.size_bytes)}</small></div><button disabled={operation !== null} onClick={() => download(artifact)}>Download</button></article>)}</div></>}
    </section>}
  </section>;
}

function formatBytes(bytes: number) { return bytes < 1024 * 1024 ? `${Math.ceil(bytes / 1024)} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB`; }
function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
