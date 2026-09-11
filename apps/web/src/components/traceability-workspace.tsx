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

  return <section className="panel traceability-workspace"><p className="eyebrow">Phase 20</p><h2>Trace Boxes</h2><p className="muted">Scan an opaque box identifier, record explicit Product quantities, and preserve custody history. These actions never change Inventory.</p>
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
  return <article className="detail-panel"><h3>{item.label ?? "Unlabelled Trace Box"}</h3><p className="trace-identifier">{item.opaque_identifier}</p><p><strong>Current custody:</strong> {item.current_custody?.custodian_name ?? "Not assigned"}</p>
    <h4>Current contents</h4>{item.contents.length === 0 ? <p className="muted">The box has no recorded contents.</p> : <ul>{item.contents.map((content) => <li key={content.product_id}><span><strong>{content.internal_code}</strong><small>{content.product_name}</small></span><strong>{content.quantity}</strong></li>)}</ul>}
    <details><summary>Add or remove Product quantity</summary><form className="stack" onSubmit={(event) => void content(event, String(new FormData(event.currentTarget).get("operation")) as "add" | "remove")}><label>Operation<select name="operation"><option value="add">Add</option><option value="remove">Remove</option></select></label><label>Product<select name="product_id" required><option value="">Select product</option>{options.products.map((product) => <option key={product.id} value={product.id}>{product.internal_code} · {product.name}</option>)}</select></label><label>Quantity<input name="quantity" type="number" min="1" required /></label><label>Reason or note<input name="notes" maxLength={2000} /></label><button>Record content change</button></form></details>
    <details><summary>Transfer custody</summary><form className="stack" onSubmit={(event) => void custody(event)}><label>Employee or department<select name="custodian" required><option value="">Select custodian</option><optgroup label="Employees">{options.employees.map((employee) => <option key={employee.id} value={`employee:${employee.id}`}>{employee.name}</option>)}</optgroup><optgroup label="Departments">{options.departments.map((department) => <option key={department.id} value={`department:${department.id}`}>{department.name}</option>)}</optgroup></select></label><label>Note<input name="notes" maxLength={2000} /></label><button>Record custody transfer</button></form></details>
    <details><summary>Immutable history ({item.events.length})</summary><ol>{item.events.map((event) => <li key={event.id}><span>{event.event_type.replaceAll("_", " ")}<small>{event.notes ?? "No note"}</small></span><time>{new Date(event.created_at).toLocaleString()}</time></li>)}</ol></details>
  </article>;
}

function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
