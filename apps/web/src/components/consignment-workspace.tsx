"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Employee } from "@/api/core";
import { Consignment, ConsignmentDepartment, ConsignmentLine, consignmentAPI } from "@/api/consignments";
import { Product, productAPI } from "@/api/products";

export function ConsignmentWorkspace({ employees }: { employees: Employee[] }) {
  const [items, setItems] = useState<Consignment[]>([]);
  const [departments, setDepartments] = useState<ConsignmentDepartment[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [selected, setSelected] = useState<Consignment | null>(null);
  const [status, setStatus] = useState("");
  const [department, setDepartment] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [lineCount, setLineCount] = useState(1);

  const load = useCallback(async () => {
    const [consignments, departmentList, productList] = await Promise.all([
      consignmentAPI.list(status, department, query),
      consignmentAPI.departments(),
      productAPI.products(),
    ]);

    setItems(consignments.consignments);
    setDepartments(departmentList.departments);
    setProducts(productList.products.filter((product) => product.status === "active"));
    setSelected((current) =>
      current
        ? consignments.consignments.find((consignment) => consignment.id === current.id) ?? null
        : null,
    );
  }, [status, department, query]);
  useEffect(() => { load().catch((cause) => setError(message(cause))); }, [load]);
  async function perform(call: () => Promise<{ consignment: Consignment }>) { setError(""); try { const result = await call(); setSelected(result.consignment); await load(); } catch (cause) { setError(message(cause)); } }
  async function create(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); const lines = Array.from({length: lineCount}, (_, index) => ({ product_id: String(data.get(`product_id_${index}`)), department_id: String(data.get(`department_id_${index}`)), required_quantity: Number(data.get(`quantity_${index}`)) })); try { const result = await consignmentAPI.create({ order_reference: String(data.get("order_reference")), dealer_reference: optional(data,"dealer_reference"), pouch_reference: optional(data,"pouch_reference"), source_type: String(data.get("source_type")) as "manual"|"import", source_reference: optional(data,"source_reference"), notes: optional(data,"notes"), lines }); form.reset(); setLineCount(1); setSelected(result.consignment); await load(); } catch (cause) { setError(message(cause)); } }
  return <section className="panel"><p className="eyebrow">Phase 9</p><h2>Consignment operations</h2><p className="muted">Department-scoped picking, packing, reservation and traceable outbound confirmation. Pouch references are searchable but are not assumed globally unique.</p>{error && <p className="error" role="alert">{error}</p>}
    <details><summary>Create consignment</summary><form className="grid" onSubmit={create}><label>Order / SO reference<input name="order_reference" required /></label><label>Dealer/customer reference<input name="dealer_reference" /></label><label>Pouch/file reference<input name="pouch_reference" /></label><label>Source<select name="source_type"><option value="manual">Manual</option><option value="import">Imported</option></select></label><label>Source trace<input name="source_reference" placeholder="Required for imported records" /></label><label>Notes<input name="notes" /></label>{Array.from({length: lineCount},(_,index)=><fieldset key={index}><legend>Product requirement {index+1}</legend><label>Product<select name={`product_id_${index}`} required><option value="">Select product</option>{products.map((p) => <option key={p.id} value={p.id}>{p.internal_code} · {p.name}</option>)}</select></label><label>Department<select name={`department_id_${index}`} required><option value="">Select department</option>{departments.filter((d) => d.status === "active").map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}</select></label><label>Required quantity<input name={`quantity_${index}`} type="number" min="1" required /></label></fieldset>)}<div className="inline"><button type="button" onClick={()=>setLineCount((count)=>count+1)}>Add product line</button>{lineCount>1&&<button type="button" onClick={()=>setLineCount((count)=>count-1)}>Remove last line</button>}<button>Create</button></div></form></details>
    <details><summary>Configure departments</summary><DepartmentAdmin departments={departments} employees={employees} onSaved={load} onError={setError} /></details>
    <div className="inline"><input aria-label="Search consignments" placeholder="Order, dealer or pouch" value={query} onChange={(e) => setQuery(e.target.value)} /><select aria-label="Status" value={status} onChange={(e) => setStatus(e.target.value)}><option value="">All states</option>{["created","allocated","picking","ready","packing","packed","outbound","completed","cancelled"].map((x) => <option key={x}>{x}</option>)}</select><select aria-label="Department" value={department} onChange={(e) => setDepartment(e.target.value)}><option value="">All visible departments</option>{departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}</select></div>
    <div className="grid"><div><h3>Board</h3><ul>{items.map((item) => <li key={item.id}><button onClick={() => setSelected(item)}>{item.order_reference} · {item.status}</button><small>{item.pouch_reference ?? "No pouch"} · {item.lines.length} visible line(s)</small></li>)}</ul></div>{selected && <ConsignmentDetail item={selected} perform={perform} />}</div>
  </section>;
}

function ConsignmentDetail({ item, perform }: { item: Consignment; perform: (call: () => Promise<{ consignment: Consignment }>) => Promise<void> }) {
  const next: Record<string,string> = { allocated:"picking", picking:"ready", ready:"packing", packing:"packed", outbound:"completed" };
  return <div><h3>{item.order_reference}</h3><p><strong>{item.status}</strong> · version {item.version}</p><p>{item.dealer_reference ?? "No dealer reference"} · pouch {item.pouch_reference ?? "—"}</p>{item.lines.map((line) => <LineEditor key={line.id} item={item} line={line} perform={perform} />)}<div className="inline">{item.status === "created" && <button onClick={() => perform(() => consignmentAPI.allocate(item))}>Allocate and reserve</button>}{next[item.status] && <button onClick={() => perform(() => consignmentAPI.transition(item,next[item.status]))}>Move to {next[item.status]}</button>}{item.status === "packed" && <button onClick={() => confirm("Confirm physical outbound and deduct inventory?") && perform(() => consignmentAPI.outbound(item))}>Confirm outbound</button>}{!["outbound","completed","cancelled"].includes(item.status) && <button onClick={() => { const reason = prompt("Cancellation reason"); if (reason) void perform(() => consignmentAPI.cancel(item,reason)); }}>Cancel</button>}</div><details><summary>Audit trail ({item.events.length})</summary><ol>{item.events.map((event) => <li key={event.id}>{event.event_type} · {new Date(event.created_at).toLocaleString()} {event.notes ?? ""}</li>)}</ol></details></div>;
}
function LineEditor({ item, line, perform }: { item: Consignment; line: ConsignmentLine; perform: (call: () => Promise<{ consignment: Consignment }>) => Promise<void> }) {
  const [ready,setReady]=useState(line.ready_quantity);const [packed,setPacked]=useState(line.packed_quantity);useEffect(()=>{setReady(line.ready_quantity);setPacked(line.packed_quantity)},[line.ready_quantity,line.packed_quantity]);
  return <div><p><strong>{line.internal_code}</strong> · {line.product_name}<small>{line.department_name} · required {line.required_quantity} · {line.progress}</small></p><div className="inline"><label>Ready<input type="number" min="0" max={line.required_quantity} value={ready} onChange={(e)=>setReady(Number(e.target.value))} /></label><label>Packed<input type="number" min="0" max={ready} value={packed} onChange={(e)=>setPacked(Number(e.target.value))} /></label><button disabled={!["allocated","picking","ready","packing"].includes(item.status)} onClick={() => perform(() => consignmentAPI.progress(item,line,ready,packed))}>Save progress</button></div></div>;
}
function DepartmentAdmin({ departments, employees, onSaved, onError }: { departments: ConsignmentDepartment[]; employees: Employee[]; onSaved: () => Promise<void>; onError: (value: string) => void }) {
  async function create(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const form=event.currentTarget; try{await consignmentAPI.createDepartment(String(new FormData(form).get("name")));form.reset();await onSaved()}catch(cause){onError(message(cause))} }
  return <><form className="inline" onSubmit={create}><input name="name" placeholder="Department name" required /><button>Add department</button></form>{departments.map((d) => <details key={d.id}><summary>{d.name} · {d.status} · {d.members.length} member(s)</summary><button onClick={async()=>{try{await consignmentAPI.updateDepartment(d,d.status==="active"?"inactive":"active");await onSaved()}catch(cause){onError(message(cause))}}}>{d.status==="active"?"Deactivate":"Activate"}</button>{employees.filter((e)=>e.status==="active").map((e) => <label key={e.id}><input type="checkbox" checked={d.members.some((m) => m.employee_id === e.id)} onChange={async (event) => { const ids=d.members.map((m)=>m.employee_id);const next=event.target.checked?[...ids,e.id]:ids.filter((id)=>id!==e.id);try{await consignmentAPI.setMembers(d.id,next);await onSaved()}catch(cause){onError(message(cause))} }} />{e.display_name}</label>)}</details>)}</>;
}
function optional(data: FormData, key: string) { const value=String(data.get(key)??"").trim(); return value || null; }
function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
