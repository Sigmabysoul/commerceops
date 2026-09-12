"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { TraceBox, TraceOptions, traceabilityAPI } from "@/api/traceability";

export function TraceabilityWorkspace() {
  const [items, setItems] = useState<TraceBox[]>([]);
  const [selected, setSelected] = useState<TraceBox | null>(null);
  const [options, setOptions] = useState<TraceOptions>({ products: [], employees: [], departments: [] });
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    const [boxes, optionResult] = await Promise.all([traceabilityAPI.list(), traceabilityAPI.options()]);
    setItems(boxes.trace_boxes);
    setOptions(optionResult.options);
  }, []);
  useEffect(() => { load().catch((cause) => setError(message(cause))); }, [load]);

  async function show(id: string) {
    setError("");
    try { setSelected((await traceabilityAPI.get(id)).trace_box); }
    catch (cause) { setError(message(cause)); }
  }
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(""); const form = event.currentTarget;
    try { const result = await traceabilityAPI.create(String(new FormData(form).get("label") ?? "")); form.reset(); setSelected(result.trace_box); await load(); }
    catch (cause) { setError(message(cause)); }
  }
  async function resolve(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    try { setSelected((await traceabilityAPI.resolve(String(new FormData(event.currentTarget).get("identifier") ?? ""))).trace_box); }
    catch (cause) { setError(message(cause)); }
  }
  async function perform(call: () => Promise<{ trace_box: TraceBox }>) {
    setError("");
    try { const result = await call(); setSelected(result.trace_box); await load(); }
    catch (cause) { setError(message(cause)); }
  }

  return <section className="panel traceability-workspace"><p className="eyebrow">Phase 21</p><h2>Trace Boxes and worker flow</h2><p className="muted">Scan a box, complete full-quantity QC, resolve rework, hand it over, and record packing gates. These actions never change Inventory.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="trace-controls"><form className="inline" onSubmit={resolve}><label>Scan or enter Trace Box ID<input name="identifier" placeholder="TBX_…" autoComplete="off" required /></label><button>Resolve</button></form><form className="inline" onSubmit={create}><label>New box label<input name="label" placeholder="Optional operator label" maxLength={200} /></label><button>Create Trace Box</button></form></div>
    <div className="trace-layout"><div><h3>Recent boxes</h3>{items.length === 0 ? <p className="empty-state">No Trace Boxes yet.</p> : <ul>{items.map((item) => <li key={item.id}><button className="record-button" onClick={() => void show(item.id)}><span><strong>{item.label ?? "Unlabelled Trace Box"}</strong><small>{item.opaque_identifier}</small></span><time>{new Date(item.created_at).toLocaleDateString()}</time></button></li>)}</ul>}</div>
      {selected ? <TraceBoxDetail item={selected} options={options} perform={perform} /> : <div className="empty-state">Select or scan a Trace Box to see its contents and history.</div>}
    </div>
  </section>;
}

function TraceBoxDetail({ item, options, perform }: { item: TraceBox; options: TraceOptions; perform: (call: () => Promise<{ trace_box: TraceBox }>) => Promise<void> }) {
  async function content(event: FormEvent<HTMLFormElement>, operation: "add" | "remove") {
    event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); const productID = String(data.get("product_id")); const quantity = Number(data.get("quantity")); const notes = String(data.get("notes") ?? "");
    await perform(() => operation === "add" ? traceabilityAPI.addContent(item.id, productID, quantity, notes) : traceabilityAPI.removeContent(item.id, productID, quantity, notes)); form.reset();
  }
  async function custody(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); const [kind, id] = String(data.get("custodian")).split(":", 2); const target = { employee_id: kind === "employee" ? id : null, department_id: kind === "department" ? id : null };
    await perform(() => traceabilityAPI.transferCustody(item.id, target, String(data.get("notes") ?? ""))); form.reset();
  }
  async function qc(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = event.currentTarget; const data = new FormData(form);
    const lines = item.contents.map((content) => { const rejected = Number(data.get(`rejected-${content.product_id}`) ?? 0); return { product_id: content.product_id, checked_quantity: content.quantity, passed_quantity: content.quantity - rejected, rejected_quantity: rejected, rejection_reason: rejected > 0 ? String(data.get(`reason-${content.product_id}`)) : null, required_work: rejected > 0 ? String(data.get(`work-${content.product_id}`)) : null }; });
    await perform(() => traceabilityAPI.recordQC(item.id, lines, String(data.get("notes") ?? ""))); form.reset();
  }
  async function handover(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); const [kind, id] = String(data.get("target")).split(":", 2);
    await perform(() => traceabilityAPI.sendHandover(item.id, { employee_id: kind === "employee" ? id : null, department_id: kind === "department" ? id : null }, String(data.get("notes") ?? ""))); form.reset();
  }
  async function finalCheck(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); await perform(() => traceabilityAPI.completeFinalCheck(item.id, String(data.get("passed")) === "true", String(data.get("notes") ?? ""))); form.reset();
  }
  const status = item.workflow.status.replaceAll("_", " ");
  return <article className="detail-panel"><h3>{item.label ?? "Unlabelled Trace Box"}</h3><p className="trace-identifier">{item.opaque_identifier}</p><div className="trace-status"><span><strong>Workflow</strong><small>{status}</small></span><span><strong>Current custody</strong><small>{item.current_custody?.custodian_name ?? "Not assigned"}</small></span></div>
    <h4>Current contents</h4>{item.contents.length === 0 ? <p className="muted">The box has no recorded contents.</p> : <ul>{item.contents.map((content) => <li key={content.product_id}><span><strong>{content.internal_code}</strong><small>{content.product_name}</small></span><strong>{content.quantity}</strong></li>)}</ul>}
    <details><summary>Add or remove Product quantity</summary><form className="stack" onSubmit={(event) => void content(event, String(new FormData(event.currentTarget).get("operation")) as "add" | "remove")}><label>Operation<select name="operation"><option value="add">Add</option><option value="remove">Remove</option></select></label><label>Product<select name="product_id" required><option value="">Select product</option>{options.products.map((product) => <option key={product.id} value={product.id}>{product.internal_code} · {product.name}</option>)}</select></label><label>Quantity<input name="quantity" type="number" min="1" required /></label><label>Reason or note<input name="notes" maxLength={2000} /></label><button>Record content change</button></form></details>
    <details><summary>Transfer custody</summary><form className="stack" onSubmit={(event) => void custody(event)}><label>Employee or department<select name="custodian" required><option value="">Select custodian</option><optgroup label="Employees">{options.employees.map((employee) => <option key={employee.id} value={`employee:${employee.id}`}>{employee.name}</option>)}</optgroup><optgroup label="Departments">{options.departments.map((department) => <option key={department.id} value={`department:${department.id}`}>{department.name}</option>)}</optgroup></select></label><label>Note<input name="notes" maxLength={2000} /></label><button>Record custody transfer</button></form></details>
    <details open><summary>Quality check</summary>{item.contents.length === 0 ? <p className="muted">Add contents before QC.</p> : <form className="stack" onSubmit={(event) => void qc(event)}>{item.contents.map((content) => <fieldset className="trace-qc-line" key={content.product_id}><legend>{content.internal_code} · {content.product_name} ({content.quantity})</legend><label>Rejected quantity<input name={`rejected-${content.product_id}`} type="number" min="0" max={content.quantity} defaultValue="0" required /></label><label>Rejection reason<select name={`reason-${content.product_id}`} defaultValue="wrong_sticker"><option value="damaged">Damaged</option><option value="wrong_sticker">Wrong sticker</option><option value="dirty">Dirty</option><option value="missing_component">Missing component</option><option value="packaging_damaged">Packaging damaged</option><option value="wrong_product">Wrong product</option><option value="manufacturing_defect">Manufacturing defect</option><option value="other">Other</option></select></label><label>Required work<select name={`work-${content.product_id}`} defaultValue="sticker_replacement"><option value="sticker_replacement">Sticker replacement</option><option value="cleaning">Cleaning</option><option value="repacking">Repacking</option><option value="component_check">Component check</option><option value="repair">Repair</option><option value="other">Other</option></select></label></fieldset>)}<label>QC note<input name="notes" maxLength={2000} /></label><button>Complete full-box QC</button></form>}</details>
    {item.workflow.work_requirements.length > 0 && <section className="trace-work"><h4>Required work</h4><ul>{item.workflow.work_requirements.map((work) => <li key={work.id}><span><strong>{work.work_type.replaceAll("_", " ")} · {work.quantity}</strong><small>{work.internal_code} · {work.rejection_reason.replaceAll("_", " ")}</small></span>{work.completed_at ? <small>Completed</small> : <button onClick={() => void perform(() => traceabilityAPI.completeWork(item.id, work.id))}>Complete work</button>}</li>)}</ul></section>}
    <details><summary>Two-step handover</summary>{item.workflow.pending_handover ? <div className="stack"><p>In transit to <strong>{item.workflow.pending_handover.target_name}</strong>.</p><button onClick={() => void perform(() => traceabilityAPI.receiveHandover(item.id, item.workflow.pending_handover!.id))}>Receive handover</button></div> : <form className="stack" onSubmit={(event) => void handover(event)}><label>Target employee or department<select name="target" required><option value="">Select target</option><optgroup label="Employees">{options.employees.map((employee) => <option key={employee.id} value={`employee:${employee.id}`}>{employee.name}</option>)}</optgroup><optgroup label="Departments">{options.departments.map((department) => <option key={department.id} value={`department:${department.id}`}>{department.name}</option>)}</optgroup></select></label><label>Handover note<input name="notes" maxLength={2000} /></label><button>Send handover</button></form>}</details>
    <details><summary>Packing and final verification</summary><div className="stack"><button onClick={() => void perform(() => traceabilityAPI.completePacking(item.id, ""))}>Complete packing</button><form className="stack" onSubmit={(event) => void finalCheck(event)}><label>Final result<select name="passed"><option value="true">Pass</option><option value="false">Fail</option></select></label><label>Note (required on failure)<input name="notes" maxLength={2000} /></label><button>Record final check</button></form><button onClick={() => void perform(() => traceabilityAPI.markReady(item.id, ""))}>Mark ready for shipment</button></div></details>
    <details><summary>Immutable history ({item.events.length})</summary><ol>{item.events.map((event) => <li key={event.id}><span>{event.event_type.replaceAll("_", " ")}<small>{event.notes ?? "No note"}</small></span><time>{new Date(event.created_at).toLocaleString()}</time></li>)}</ol></details>
  </article>;
}

function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
