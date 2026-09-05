"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Cancellation, ReturnCase, ReturnDisposition, returnsAPI } from "@/api/returns";

type Queue = "expected" | "received" | "inspection" | "completed";

export function ReturnsWorkspace() {
  const [returns, setReturns] = useState<ReturnCase[]>([]);
  const [cancellations, setCancellations] = useState<Cancellation[]>([]);
  const [selectedReturn, setSelectedReturn] = useState<ReturnCase | null>(null);
  const [selectedCancellation, setSelectedCancellation] = useState<Cancellation | null>(null);
  const [marketplace, setMarketplace] = useState("");
  const [received, setReceived] = useState<Record<string, number>>({});
  const [dispositions, setDispositions] = useState<Record<string, Exclude<ReturnDisposition, "pending">>>({});
  const [corrections, setCorrections] = useState<Record<string, number>>({});
  const [correctionReason, setCorrectionReason] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setBusy(true); setError("");
    try {
      const [returnItems, cancellationItems] = await Promise.all([returnsAPI.listReturns(marketplace), returnsAPI.listCancellations(marketplace)]);
      setReturns(returnItems); setCancellations(cancellationItems);
    } catch (cause) { setError(message(cause)); }
    finally { setBusy(false); }
  }, [marketplace]);
  useEffect(() => { load(); }, [load]);

  async function openReturn(id: string) {
    setError("");
    try {
      const item = await returnsAPI.getReturn(id); setSelectedReturn(item); setSelectedCancellation(null);
      setReceived(Object.fromEntries(item.items.map((line) => [line.id, line.received_quantity ?? line.expected_quantity])));
      setDispositions(Object.fromEntries(item.items.map((line) => [line.id, line.received_quantity === 0 ? "missing" : line.disposition === "pending" ? "restockable" : line.disposition])) as Record<string, Exclude<ReturnDisposition, "pending">>);
      setCorrections(Object.fromEntries(item.items.map((line) => [line.id, 0]))); setCorrectionReason("");
    } catch (cause) { setError(message(cause)); }
  }
  async function openCancellation(id: string) {
    setError("");
    try { setSelectedCancellation(await returnsAPI.getCancellation(id)); setSelectedReturn(null); }
    catch (cause) { setError(message(cause)); }
  }
  async function act(action: () => Promise<ReturnCase>) {
    setBusy(true); setError("");
    try { const item = await action(); await load(); await openReturn(item.id); }
    catch (cause) { setError(message(cause)); }
    finally { setBusy(false); }
  }
  async function actCancellation(action: () => Promise<Cancellation>) {
    setBusy(true); setError("");
    try { const item = await action(); await load(); await openCancellation(item.id); }
    catch (cause) { setError(message(cause)); }
    finally { setBusy(false); }
  }
  function submitReceive(event: FormEvent) {
    event.preventDefault(); if (!selectedReturn) return;
    void act(() => returnsAPI.receive(selectedReturn.id, selectedReturn.items.map((line) => ({ return_item_id: line.id, received_quantity: received[line.id] ?? 0 }))));
  }
  function submitInspection(event: FormEvent) {
    event.preventDefault(); if (!selectedReturn) return;
    void act(() => returnsAPI.inspect(selectedReturn.id, selectedReturn.items.map((line) => ({ return_item_id: line.id, disposition: dispositions[line.id] }))));
  }
  function submitCorrection(event: FormEvent) {
    event.preventDefault(); if (!selectedReturn || !confirm("Record this compensating stock correction? Inventory history will remain immutable.")) return;
    const items = selectedReturn.items.flatMap((line) => (corrections[line.id] ?? 0) > 0 ? [{ return_item_id: line.id, quantity: corrections[line.id] }] : []);
    if (items.length === 0) { setError("Enter at least one correction quantity."); return; }
    void act(() => returnsAPI.correctRestock(selectedReturn.id, items, correctionReason));
  }

  const queues: [Queue, string][] = [["expected", "Expected"], ["received", "Received"], ["inspection", "Needs inspection"], ["completed", "Completed"]];
  return <section className="returns-workspace">
    <div className="product-heading"><div><p className="eyebrow">Phase 8</p><h2>Returns and cancellations</h2><p className="muted">Physical evidence, explicit disposition, and traceable inventory impact.</p></div><div className="workspace-controls"><select aria-label="Marketplace" value={marketplace} onChange={(event) => setMarketplace(event.target.value)}><option value="">All marketplaces</option><option value="flipkart">Flipkart</option><option value="amazon">Amazon</option><option value="meesho">Meesho</option><option value="myntra">Myntra</option><option value="snapdeal">Snapdeal</option></select><button className="secondary" disabled={busy} onClick={load}>{busy ? "Loading…" : "Refresh"}</button></div></div>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="return-queues">{queues.map(([queue, label]) => <section className="panel" key={queue}><h3>{label}</h3><ul className="selectable-list">{returns.filter((item) => queueFor(item) === queue).map((item) => <li key={item.id}><button className="record-button" onClick={() => openReturn(item.id)}><span>{item.external_order_id ?? item.marketplace_order_id}<small>{item.marketplace} · {item.reason}</small></span><span className={`status status-${item.status}`}>{item.status}</span></button></li>)}</ul>{returns.every((item) => queueFor(item) !== queue) && <p className="empty-state">No {label.toLowerCase()} returns.</p>}</section>)}</div>
    <div className="return-detail-layout">
      <section className="panel"><h3>Cancellation queue</h3><ul className="selectable-list">{cancellations.map((item) => <li key={item.id}><button className="record-button" onClick={() => openCancellation(item.id)}><span>{item.external_order_id ?? item.marketplace_order_id}<small>{item.marketplace} · {item.outbound_state.replaceAll("_", " ")}</small></span><span className={`status status-${item.status}`}>{item.status}</span></button></li>)}</ul>{cancellations.length === 0 && <p className="empty-state">No cancellations.</p>}</section>
      <section className="panel detail-panel">{selectedReturn ? <ReturnDetail item={selectedReturn} busy={busy} received={received} setReceived={setReceived} dispositions={dispositions} setDispositions={setDispositions} corrections={corrections} setCorrections={setCorrections} correctionReason={correctionReason} setCorrectionReason={setCorrectionReason} onReceive={submitReceive} onInspect={submitInspection} onCorrection={submitCorrection} onRestock={() => { if (confirm("Restock all inspected restockable quantities? This changes sellable inventory.")) void act(() => returnsAPI.restock(selectedReturn.id)); }} onClose={() => { if (confirm("Close this return? Closure does not change inventory.")) void act(() => returnsAPI.closeReturn(selectedReturn.id)); }} /> : selectedCancellation ? <CancellationDetail item={selectedCancellation} busy={busy} onClose={() => { if (confirm("Close this cancellation? Closure does not change inventory.")) void actCancellation(() => returnsAPI.closeCancellation(selectedCancellation.id)); }} /> : <p className="empty-state">Select a return or cancellation to inspect its history.</p>}</section>
    </div>
  </section>;
}

function ReturnDetail({ item, busy, received, setReceived, dispositions, setDispositions, corrections, setCorrections, correctionReason, setCorrectionReason, onReceive, onInspect, onCorrection, onRestock, onClose }: {
  item: ReturnCase; busy: boolean; received: Record<string, number>; setReceived: (value: Record<string, number>) => void;
  dispositions: Record<string, Exclude<ReturnDisposition, "pending">>; setDispositions: (value: Record<string, Exclude<ReturnDisposition, "pending">>) => void;
  corrections: Record<string, number>; setCorrections: (value: Record<string, number>) => void; correctionReason: string; setCorrectionReason: (value: string) => void;
  onReceive: (event: FormEvent) => void; onInspect: (event: FormEvent) => void; onCorrection: (event: FormEvent) => void; onRestock: () => void; onClose: () => void;
}) {
  return <><div className="status-line"><div><h3>Return detail</h3><p className="muted">{item.marketplace} · {item.external_order_id ?? item.marketplace_order_id}</p></div><span className={`status status-${item.status}`}>{item.status}</span></div>
    <p>{item.reason}</p><div className="table-scroll"><table><thead><tr><th>Product</th><th>Expected</th><th>Received</th><th>Disposition</th><th>Restocked</th><th>Corrected</th></tr></thead><tbody>{item.items.map((line) => <tr key={line.id}><td>{line.product_name}<small>{line.internal_code}</small></td><td>{line.expected_quantity}</td><td>{line.received_quantity ?? "—"}</td><td>{line.disposition}</td><td>{line.restocked_quantity}</td><td>{line.corrected_quantity}</td></tr>)}</tbody></table></div>
    {item.status === "expected" && <form className="action-panel" onSubmit={onReceive}><h4>Mark received</h4>{item.items.map((line) => <label key={line.id}>{line.internal_code}<input type="number" min="0" max={line.expected_quantity} required value={received[line.id] ?? 0} onChange={(event) => setReceived({ ...received, [line.id]: Number(event.target.value) })} /></label>)}<button disabled={busy}>Mark Received</button></form>}
    {item.status === "received" && <form className="action-panel" onSubmit={onInspect}><h4>Inspect every line</h4>{item.items.map((line) => <label key={line.id}>{line.internal_code}<select value={dispositions[line.id]} onChange={(event) => setDispositions({ ...dispositions, [line.id]: event.target.value as Exclude<ReturnDisposition, "pending"> })}><option value="restockable">Restockable</option><option value="damaged">Damaged</option><option value="wrong_product">Wrong product</option><option value="missing">Missing</option><option value="rejected">Rejected</option></select></label>)}<button disabled={busy}>Save inspection</button></form>}
    {item.status === "inspected" && <div className="action-panel"><h4>Inventory action</h4><p className="warning-text">Only restockable quantities will enter sellable inventory.</p><button disabled={busy} onClick={onRestock}>Restock</button></div>}
    {(item.status === "restocked" || item.status === "restock_corrected") && <form className="action-panel" onSubmit={onCorrection}><h4>Compensating correction</h4>{item.items.filter((line) => line.restocked_quantity > line.corrected_quantity).map((line) => <label key={line.id}>{line.internal_code}<input type="number" min="0" max={line.restocked_quantity - line.corrected_quantity} value={corrections[line.id] ?? 0} onChange={(event) => setCorrections({ ...corrections, [line.id]: Number(event.target.value) })} /></label>)}<label>Reason<input required maxLength={500} value={correctionReason} onChange={(event) => setCorrectionReason(event.target.value)} /></label><button className="secondary" disabled={busy}>Record correction</button></form>}
    {(["restocked", "restock_corrected", "damaged", "rejected"] as string[]).includes(item.status) && <div className="panel-actions"><span className="muted">Closure is inventory-neutral.</span><button disabled={busy} onClick={onClose}>Close return</button></div>}
    <History title="Status history" events={item.events} />
    <section className="history-block"><h4>Inventory impact</h4>{item.inventory_impact.length ? <ul>{item.inventory_impact.map((impact) => <li key={impact.transaction_id}><span>{impact.transaction_type}<small>{new Date(impact.created_at).toLocaleString()}</small></span><strong className={impact.quantity_delta > 0 ? "positive" : "negative"}>{impact.quantity_delta > 0 ? "+" : ""}{impact.quantity_delta}</strong></li>)}</ul> : <p className="empty-state">No inventory movement.</p>}</section>
  </>;
}

function CancellationDetail({ item, busy, onClose }: { item: Cancellation; busy: boolean; onClose: () => void }) {
  return <><div className="status-line"><div><h3>Cancellation detail</h3><p className="muted">{item.marketplace} · {item.external_order_id ?? item.marketplace_order_id}</p></div><span className={`status status-${item.status}`}>{item.status}</span></div><p>{item.reason}</p><dl className="summary-metrics"><div><dt>Outbound state</dt><dd>{item.outbound_state.replaceAll("_", " ")}</dd></div><div><dt>Cancelled</dt><dd>{new Date(item.cancelled_at).toLocaleString()}</dd></div></dl>{item.status === "recorded" && <div className="panel-actions"><span className="muted">Cancellation closure never restores stock.</span><button disabled={busy} onClick={onClose}>Close cancellation</button></div>}<History title="Closure history" events={item.events} /></>;
}

function History({ title, events }: { title: string; events: { id: string; event_type: string; notes: string | null; created_at: string }[] }) { return <section className="history-block"><h4>{title}</h4><ul>{events.map((event) => <li key={event.id}><span>{event.event_type.replaceAll("_", " ")}<small>{event.notes ?? "No notes"}</small></span><time>{new Date(event.created_at).toLocaleString()}</time></li>)}</ul></section>; }
function queueFor(item: ReturnCase): Queue { if (item.status === "expected") return "expected"; if (item.status === "received") return "received"; if (item.status === "inspected" || item.status === "inspection_pending") return "inspection"; return "completed"; }
function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
