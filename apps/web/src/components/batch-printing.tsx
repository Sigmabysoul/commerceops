"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AssignmentRule, Batch, EligibleOrder, MarketplaceKey, PrintArtifact, PrintJob, batchAPI } from "@/api/batches";
import { Employee, coreAPI } from "@/api/core";
import { Product, productAPI } from "@/api/products";

type Operation = "creating" | "readying" | "generating" | "downloading" | "assigning" | "reprinting" | null;

export function BatchPrinting() {
  const [marketplace, setMarketplace] = useState<MarketplaceKey>("flipkart");
  const [orders, setOrders] = useState<EligibleOrder[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [batch, setBatch] = useState<Batch | null>(null);
  const [printJob, setPrintJob] = useState<PrintJob | null>(null);
  const [sortLabels, setSortLabels] = useState(false);
  const [exportInvoices, setExportInvoices] = useState(false);
  const [operation, setOperation] = useState<Operation>(null);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState("");
  const [printJobs, setPrintJobs] = useState<PrintJob[]>([]);
  const [reprintReason, setReprintReason] = useState("");
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [assignmentRules, setAssignmentRules] = useState<AssignmentRule[]>([]);
  const [defaultWorker, setDefaultWorker] = useState("");
  const [productWorkers, setProductWorkers] = useState<Record<string, string>>({});
  const createRequest = useRef<IdempotentRequest | null>(null);
  const printRequest = useRef<IdempotentRequest | null>(null);
  const reprintRequest = useRef<IdempotentRequest | null>(null);

  const loadEligible = useCallback(async () => {
    const result = await batchAPI.eligibleOrders(marketplace);
    setOrders(result.orders);
    setSelected((current) => current.filter((id) => result.orders.some((order) => order.order_id === id)));
  }, [marketplace]);

  useEffect(() => {
    loadEligible().catch(() => setAvailable(false));
  }, [loadEligible]);

  const loadAssignments = useCallback(async () => {
    const [ruleResult, employeeResult, productResult] = await Promise.allSettled([batchAPI.assignmentRules(marketplace), coreAPI.employees(), productAPI.products()]);
    if (ruleResult.status === "fulfilled") {
      const rules = ruleResult.value.worker_assignment_rules;
      setAssignmentRules(rules);
      setDefaultWorker(rules.find((rule) => rule.product_id === null)?.employee_id ?? "");
      setProductWorkers(Object.fromEntries(rules.filter((rule) => rule.product_id).map((rule) => [rule.product_id as string, rule.employee_id])));
    }
    if (employeeResult.status === "fulfilled") setEmployees(employeeResult.value.employees.filter((employee) => employee.status === "active"));
    if (productResult.status === "fulfilled") setProducts(productResult.value.products.filter((product) => product.status === "active"));
  }, [marketplace]);

  useEffect(() => { loadAssignments().catch((cause) => setError(message(cause))); }, [loadAssignments]);

  const loadPrintJobs = useCallback(async (batchID: string) => {
    setPrintJobs((await batchAPI.printJobs(batchID)).print_jobs);
  }, []);

  useEffect(() => {
    if (batch?.status === "ready") loadPrintJobs(batch.id).catch((cause) => setError(message(cause)));
  }, [batch?.id, batch?.status, loadPrintJobs]);

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
      const signature = `${marketplace}|${selected.join("|")}`;
      const result = await batchAPI.create(marketplace, selected, idempotencyKey(createRequest, signature));
      createRequest.current = null;
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
    try {
      const signature = `${batch.id}|${sortLabels}|${exportInvoices}`;
      const result = await batchAPI.generate(batch.id, sortLabels, exportInvoices, idempotencyKey(printRequest, signature));
      printRequest.current = null;
      setPrintJob(result.print_job);
      await loadPrintJobs(batch.id);
    }
    catch (cause) { setError(message(cause)); }
    finally { setOperation(null); }
  }

  async function saveAssignments() {
    if (!defaultWorker) return;
    setOperation("assigning"); setError("");
    const rules = [{ product_id: null, employee_id: defaultWorker, priority: 100 }, ...Object.entries(productWorkers).filter(([, employeeID]) => employeeID).map(([productID, employeeID]) => ({ product_id: productID, employee_id: employeeID, priority: 10 }))];
    try { setAssignmentRules((await batchAPI.replaceAssignmentRules(marketplace, rules)).worker_assignment_rules); }
    catch (cause) { setError(message(cause)); }
    finally { setOperation(null); }
  }

  async function reprint(source: PrintJob) {
    const reason = reprintReason.trim(); if (!reason) return;
    setOperation("reprinting"); setError("");
    try {
      const signature = `${source.id}|${reason}`;
      const result = await batchAPI.reprint(source.id, reason, idempotencyKey(reprintRequest, signature));
      reprintRequest.current = null; setPrintJob(result.print_job); setReprintReason("");
      await loadPrintJobs(source.batch_id);
    } catch (cause) { setError(message(cause)); }
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

  function selectMarketplace(value: MarketplaceKey) {
    setMarketplace(value);
    setOrders([]); setSelected([]); setBatch(null); setPrintJob(null); setPrintJobs([]);
    setAssignmentRules([]); setDefaultWorker(""); setProductWorkers({}); setExportInvoices(false); setAvailable(true); setError("");
    createRequest.current = null; printRequest.current = null; reprintRequest.current = null;
  }

  if (!available) return <section className="panel batch-printing"><h2>Batch printing</h2><p className="muted">Batch printing is not available for this account.</p></section>;

  return <section className="batch-printing">
    <div className="product-heading"><div><p className="eyebrow">Shared marketplace workflow</p><h2>Batch printing</h2><p className="muted">Select processed marketplace orders, review Product Master totals, and generate traceable output.</p></div><div className="workspace-controls"><select aria-label="Batch marketplace" value={marketplace} onChange={(event) => selectMarketplace(event.target.value as MarketplaceKey)}><option value="flipkart">Flipkart</option><option value="amazon">Amazon</option><option value="meesho">Meesho</option><option value="myntra">Myntra</option></select><button className="secondary" disabled={operation !== null} onClick={() => loadEligible().catch((cause) => setError(message(cause)))}>Refresh orders</button></div></div>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="batch-layout">
      <section className="panel batch-orders"><div className="status-line"><div><h2>Eligible orders</h2><p className="muted">{orders.length} available · {selected.length} selected</p></div>{orders.length > 0 && <button className="secondary" onClick={() => setSelected(selected.length === orders.length ? [] : orders.map((order) => order.order_id))}>{selected.length === orders.length ? "Clear all" : "Select all"}</button>}</div>
        {orders.length === 0 ? <p className="empty-state">No processed orders are currently eligible for a batch.</p> : <ul className="selectable-list">{orders.map((order) => <li key={order.order_id}><label><input type="checkbox" checked={selected.includes(order.order_id)} onChange={() => toggle(order.order_id)} /><span><strong>{order.marketplace_order_id ?? "Order ID unavailable"}</strong><small>{order.awb ?? "AWB unavailable"} · source page {order.source_page}</small></span></label><span className={`status ${order.unresolved_count ? "status-needs_review" : "status-processed"}`}>{order.unresolved_count ? `${order.unresolved_count} unresolved` : "resolved"}</span></li>)}</ul>}
        <div className="panel-actions"><span className={selectedUnresolved ? "warning-text" : "muted"}>{selectedUnresolved ? `${selectedUnresolved} unresolved items will block readiness.` : "Only the selected order IDs are submitted."}</span><button disabled={!selected.length || operation !== null} onClick={createBatch}>{operation === "creating" ? "Creating…" : "Create batch"}</button></div>
      </section>

      <section className="panel batch-summary"><div className="status-line"><h2>Batch summary</h2>{batch && <span className={`status status-${batch.status}`}>{batch.status}</span>}</div>
        {!batch ? <p className="empty-state">Create a batch to preview its server-derived totals and printing options.</p> : <><dl className="summary-metrics"><div><dt>Marketplace</dt><dd>{batch.marketplace_key}</dd></div><div><dt>Orders</dt><dd>{batch.order_count}</dd></div><div><dt>Products</dt><dd>{batch.product_totals?.length ?? 0}</dd></div><div><dt>Unresolved</dt><dd>{batch.unresolved_count}</dd></div></dl>
          {(batch.product_totals?.length ?? 0) > 0 && <div className="product-totals"><h3>Product totals</h3><ul>{batch.product_totals?.map((product) => <li key={product.product_id}><span><strong>{product.internal_code}</strong> · {product.product_name}<small>{product.order_line_count} order lines</small></span><strong>{product.total_quantity}</strong></li>)}</ul></div>}
          {(batch.worker_totals?.length ?? 0) > 0 && <div className="product-totals"><h3>Worker totals</h3><ul>{batch.worker_totals?.map((worker) => <li key={worker.employee_id}><span><strong>{worker.employee_name}</strong><small>{worker.product_count} products · {worker.order_line_count} order lines</small></span><strong>{worker.total_quantity}</strong></li>)}</ul></div>}
          {batch.status === "draft" && <div className="panel-actions"><span className="muted">Ready batches can generate printable output.</span><button disabled={batch.unresolved_count > 0 || operation !== null} onClick={readyBatch}>{operation === "readying" ? "Marking ready…" : "Mark ready"}</button></div>}
          {batch.status === "ready" && batch.marketplace_key !== "myntra" && <div className="print-options"><h3>Printable output</h3><label><input type="checkbox" checked={sortLabels} onChange={(event) => setSortLabels(event.target.checked)} /><span><strong>Sort Labels</strong><small>Use the server-configured Product Master ordering.</small></span></label>{batch.marketplace_key !== "meesho" && <label><input type="checkbox" checked={exportInvoices} onChange={(event) => setExportInvoices(event.target.checked)} /><span><strong>Export Invoices</strong><small>Create a separate invoice PDF in corresponding order.</small></span></label>}{batch.marketplace_key === "meesho" && <p className="muted">Meesho output preserves each complete source label page. Invoice export is unavailable because no deterministic invoice association is established.</p>}<button disabled={operation !== null} onClick={generate}>{operation === "generating" ? "Generating…" : "Generate PDFs"}</button></div>}
          {batch.status === "ready" && batch.marketplace_key === "myntra" && <p className="notice">Myntra PDF output is unavailable until a representative label establishes a safe enrichment contract.</p>}
        </>}
      </section>
    </div>
    {employees.length > 0 && products.length > 0 && <section className="panel assignment-config"><div className="status-line"><div><h2>Worker assignments</h2><p className="muted">Exact Product Master rules override the required {marketplace} fallback worker.</p></div><span className="status">{assignmentRules.length} rules</span></div><div className="assignment-grid"><label>Fallback worker<select value={defaultWorker} onChange={(event) => setDefaultWorker(event.target.value)}><option value="">Select worker</option>{employees.map((employee) => <option key={employee.id} value={employee.id}>{employee.display_name}</option>)}</select></label>{products.map((product) => <label key={product.id}>{product.internal_code} · {product.name}<select value={productWorkers[product.id] ?? ""} onChange={(event) => setProductWorkers((current) => ({ ...current, [product.id]: event.target.value }))}><option value="">Use fallback</option>{employees.map((employee) => <option key={employee.id} value={employee.id}>{employee.display_name}</option>)}</select></label>)}</div><div className="panel-actions"><span className="muted">Assignments are snapshotted when a batch becomes ready.</span><button disabled={!defaultWorker || operation !== null} onClick={saveAssignments}>{operation === "assigning" ? "Saving…" : "Save assignments"}</button></div></section>}
    {printJob && <section className="panel print-result"><div className="status-line"><div><h2>Print output</h2><p className="muted">Generation {printJob.generation_version}</p></div><span className={`status status-${printJob.status}`}>{printJob.status}</span></div>
      {printJob.status === "generating" && <div className="progress" role="progressbar" aria-label="Generating printable PDFs"><span /></div>}
      {printJob.status === "failed" && <p className="error" role="alert">{printJob.error_message ?? "PDF generation failed."}</p>}
      {printJob.status === "ready" && <><dl className="summary-metrics"><div><dt>Pages</dt><dd>{printJob.artifacts.find((artifact) => artifact.kind === "labels")?.page_count ?? 0}</dd></div><div><dt>Label order</dt><dd>{printJob.sort_labels ? "Sorted" : "Original"}</dd></div><div><dt>Files</dt><dd>{printJob.artifacts.length}</dd></div></dl><div className="artifact-list">{printJob.artifacts.map((artifact) => <article key={artifact.id}><div><strong>{artifact.kind === "labels" ? "Shipping labels PDF" : "Invoices PDF"}</strong><small>{artifact.page_count} pages · {formatBytes(artifact.size_bytes)}</small></div><button disabled={operation !== null} onClick={() => download(artifact)}>Download</button></article>)}</div></>}
    </section>}
    {batch?.status === "ready" && printJobs.length > 0 && <section className="panel print-history"><div className="status-line"><div><h2>Print history</h2><p className="muted">Every regeneration remains linked to its source print job.</p></div><span className="status">{printJobs.length} jobs</span></div><label className="reprint-reason">Reprint reason<input value={reprintReason} maxLength={500} placeholder="For example: damaged paper" onChange={(event) => setReprintReason(event.target.value)} /></label><div className="artifact-list">{printJobs.map((job) => <article key={job.id}><div><strong>{job.source_print_job_id ? "Reprint" : "Original print"}</strong><small>{new Date(job.created_at).toLocaleString()} · {job.status}{job.reprint_reason ? ` · ${job.reprint_reason}` : ""}</small></div><button disabled={job.status !== "ready" || !reprintReason.trim() || operation !== null} onClick={() => reprint(job)}>{operation === "reprinting" ? "Reprinting…" : "Reprint"}</button></article>)}</div></section>}
  </section>;
}

function formatBytes(bytes: number) { return bytes < 1024 * 1024 ? `${Math.ceil(bytes / 1024)} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB`; }
type IdempotentRequest = { signature: string; key: string };
function idempotencyKey(reference: React.MutableRefObject<IdempotentRequest | null>, signature: string) {
  if (reference.current?.signature !== signature) reference.current = { signature, key: crypto.randomUUID() };
  return reference.current.key;
}
function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
