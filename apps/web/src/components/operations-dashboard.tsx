"use client";

import { useCallback, useEffect, useState } from "react";
import { DashboardReport, reportingAPI } from "@/api/reporting";

type Preset = "today" | "yesterday" | "custom";

export function OperationsDashboard() {
  const [preset, setPreset] = useState<Preset>("today");
  const [marketplace, setMarketplace] = useState("");
  const [fromDate, setFromDate] = useState(localDate(new Date()));
  const [toDate, setToDate] = useState(localDate(addDays(new Date(), 1)));
  const [report, setReport] = useState<DashboardReport | null>(null);
  const [error, setError] = useState(""); const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setBusy(true); setError("");
    try { const [from, to] = range(preset, fromDate, toDate); setReport(await reportingAPI.dashboard(from, to, marketplace)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to load reporting"); }
    finally { setBusy(false); }
  }, [preset, fromDate, toDate, marketplace]);
  useEffect(() => { load(); }, [load]);

  const metrics = report ? [
    ["Orders processed", report.summary.orders_processed], ["Labels generated", report.summary.labels_generated],
    ["Print runs completed", report.summary.print_runs_completed], ["Batches", report.summary.batches],
    ...(report.inventory_access ? [["Outbound orders", report.summary.outbound_confirmed_orders ?? 0] as const] : []), ["Needs review", report.summary.unresolved_records],
    ...(report.returns_access&&report.returns ? [["Cancellations", report.returns.cancellations] as const, ["Returns received", report.returns.returns_received] as const] : []),
    ...(report.consignment_access&&report.consignment ? [["Pending consignments", report.consignment.pending] as const, ["Completed consignments", report.consignment.completed] as const] : []),
    ["Duplicates", report.summary.duplicate_records], ["Failed jobs", report.summary.failed_processing_jobs],
  ] as const : [];

  return <section className="reporting">
    <div className="product-heading"><div><p className="eyebrow">Phase 6</p><h2>Operations dashboard</h2><p className="muted">Authoritative marketplace, batch, print, and inventory activity.</p></div><button className="secondary" disabled={busy} onClick={load}>{busy ? "Loading…" : "Refresh"}</button></div>
    <div className="report-filters panel"><label>Range<select value={preset} onChange={(e)=>setPreset(e.target.value as Preset)}><option value="today">Today</option><option value="yesterday">Yesterday</option><option value="custom">Custom</option></select></label>{preset==="custom"&&<><label>From<input type="date" value={fromDate} onChange={(e)=>setFromDate(e.target.value)} /></label><label>To (exclusive)<input type="date" value={toDate} onChange={(e)=>setToDate(e.target.value)} /></label></>}<label>Marketplace<select value={marketplace} onChange={(e)=>setMarketplace(e.target.value)}><option value="">All</option><option value="flipkart">Flipkart</option><option value="amazon">Amazon</option></select></label></div>
    {error&&<p className="error" role="alert">{error}</p>}
    {report&&<>
      <div className="metric-grid">{metrics.map(([label,value])=><article key={label}><span>{label}</span><strong>{value.toLocaleString()}</strong></article>)}</div>
      {report.inventory_access&&report.inventory?<div className="panel report-block"><h3>Inventory snapshot and movement</h3><div className="summary-metrics"><Metric label="On hand" value={report.inventory.current_on_hand}/><Metric label="Reserved" value={report.inventory.current_reserved}/><Metric label="Available" value={report.inventory.current_available}/><Metric label="Stock in" value={report.inventory.stock_in}/><Metric label="Ecommerce out" value={report.inventory.stock_out}/><Metric label="Consignment out" value={report.inventory.consignment_out}/><Metric label="Return restock" value={report.inventory.return_restock}/><Metric label="Net movement" value={report.inventory.net_movement}/></div></div>:<p className="notice">Inventory metrics are hidden because this account does not have inventory access.</p>}
      {report.returns_access&&report.returns?<div className="panel report-block"><h3>Returns and cancellations</h3><div className="summary-metrics"><Metric label="Cancellations" value={report.returns.cancellations}/><Metric label="Returns received" value={report.returns.returns_received}/><Metric label="Received quantity" value={report.returns.received_quantity}/><Metric label="Restocked quantity" value={report.returns.restocked_quantity}/><Metric label="Damaged quantity" value={report.returns.damaged_quantity}/><MetricText label="Cohort return rate" value={`${report.returns.cohort_return_rate_percent.toLocaleString(undefined,{maximumFractionDigits:2})}%`}/></div><p className="muted">Cohort rate: resolved source orders created in this range that have a physically received return.</p></div>:<p className="notice">Return metrics are hidden because this account does not have returns access.</p>}
      {report.consignment_access&&report.consignment?<><div className="panel report-block"><h3>Consignment operations</h3><div className="summary-metrics"><Metric label="Pending" value={report.consignment.pending}/><Metric label="Completed in range" value={report.consignment.completed}/><MetricText label="Average completion" value={`${report.consignment.average_completion_hours.toLocaleString(undefined,{maximumFractionDigits:1})} hours`}/><Metric label="Inventory out" value={report.consignment.inventory_out}/></div></div><div className="report-columns"><Table title="Consignment product requirements" headers={["Product","Required"]} rows={report.consignment.products.map(x=>[`${x.product_name} · ${x.internal_code}`,x.required_quantity])}/><Table title="Department workload" headers={["Department","Pending","Outstanding units"]} rows={report.consignment.departments.map(x=>[x.department_name,x.pending_consignments,x.outstanding_quantity])}/></div></>:<p className="notice">Consignment metrics are hidden because this account does not have consignment access.</p>}
      <div className="report-columns"><Table title="Marketplace breakdown" headers={["Marketplace","Orders","Resolved","Review","Duplicate"]} rows={report.marketplaces.map(x=>[x.marketplace,x.orders,x.resolved,x.needs_review,x.duplicates])}/><Table title="Review and failure queue" headers={["Queue","Count"]} rows={[["Needs review",report.summary.unresolved_records],["Duplicates",report.summary.duplicate_records],["Failed jobs",report.summary.failed_processing_jobs]]}/></div>
      <Table title="Quantity by product" headers={["Product","Quantity"]} rows={report.product_quantities.map(x=>[`${x.product_name} · ${x.internal_code}`,x.quantity])}/>
      {report.inventory_access&&<Table title={`Product movement (${report.product_movement_total})`} headers={["Product","Orders","Stock in","Ecommerce out","Consignment out","Return restock","Adjustments","Net"]} rows={report.product_movements.map(x=>[`${x.product_name} · ${x.internal_code}`,x.order_quantity,x.stock_in,x.stock_out,x.consignment_out,x.return_restock,x.adjustments,x.net_movement])}/>}
    </>}
  </section>;
}

function Metric({label,value}:{label:string;value:number}){return <div><dt>{label}</dt><dd>{value.toLocaleString()}</dd></div>}
function MetricText({label,value}:{label:string;value:string}){return <div><dt>{label}</dt><dd>{value}</dd></div>}
function Table({title,headers,rows}:{title:string;headers:string[];rows:(string|number)[][]}){return <section className="panel report-block"><h3>{title}</h3><div className="table-scroll"><table><thead><tr>{headers.map(x=><th key={x}>{x}</th>)}</tr></thead><tbody>{rows.map((row,i)=><tr key={i}>{row.map((x,j)=><td key={j}>{typeof x==="number"?x.toLocaleString():x}</td>)}</tr>)}</tbody></table>{rows.length===0&&<p className="empty-state">No activity in this period.</p>}</div></section>}
function startOfDay(value:Date){return new Date(value.getFullYear(),value.getMonth(),value.getDate())}
function addDays(value:Date,days:number){const result=new Date(value);result.setDate(result.getDate()+days);return result}
function localDate(value:Date){const y=value.getFullYear(),m=String(value.getMonth()+1).padStart(2,"0"),d=String(value.getDate()).padStart(2,"0");return `${y}-${m}-${d}`}
function range(preset:Preset,from:string,to:string):[Date,Date]{const now=new Date();if(preset==="today"){const start=startOfDay(now);return[start,addDays(start,1)]}if(preset==="yesterday"){const end=startOfDay(now);return[addDays(end,-1),end]}return[new Date(`${from}T00:00:00`),new Date(`${to}T00:00:00`)]}
