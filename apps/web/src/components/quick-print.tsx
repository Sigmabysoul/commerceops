"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { PrintAsset, Printer, PrinterJob, printingAPI } from "@/api/printing";

// QuickPrint only talks to CommerceOps. Keeping hardware access out of the
// browser makes the same authorized workflow safe on phones and desktops.
export function QuickPrint() {
  const [assets, setAssets] = useState<PrintAsset[]>([]);
  const [printers, setPrinters] = useState<Printer[]>([]);
  const [jobs, setJobs] = useState<PrinterJob[]>([]);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("");
  const [selected, setSelected] = useState<PrintAsset | null>(null);
  const [error, setError] = useState("");
  const [credential, setCredential] = useState("");

  const load = useCallback(async () => {
    const [assetResult, printerResult, jobResult] = await Promise.all([
      printingAPI.assets(), printingAPI.printers(), printingAPI.jobs(),
    ]);
    setAssets(assetResult.assets);
    setPrinters(printerResult.printers);
    setJobs(jobResult.printer_jobs);
  }, []);

  useEffect(() => { load().catch(() => undefined); }, [load]);
  const categories = useMemo(() => Array.from(new Set(assets.map((asset) => asset.category))).sort(), [assets]);
  const visible = assets.filter((asset) => (!category || asset.category === category)
    && (!search || `${asset.name} ${asset.description ?? ""}`.toLowerCase().includes(search.toLowerCase())));

  async function print(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    const data = new FormData(event.currentTarget);
    const copies = Number(data.get("copies"));
    // The server repeats this guardrail; this prompt is usability, not trust.
    if (copies > 20 && !window.confirm(`Print ${copies} copies?`)) return;
    try {
      await printingAPI.quickPrint(selected.id, String(data.get("printer")), copies, copies > 20, crypto.randomUUID());
      setSelected(null);
      await load();
    } catch (cause) { setError(message(cause)); }
  }

  async function createAgent(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    try {
      const result = await printingAPI.createAgent(String(new FormData(form).get("name")));
      // Kept only in memory because the server never returns this secret again.
      setCredential(result.credential);
      form.reset();
    } catch (cause) { setError(message(cause)); }
  }

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    try { await printingAPI.uploadAsset(new FormData(form)); form.reset(); await load(); }
    catch (cause) { setError(message(cause)); }
  }

  return <section className="panel quick-print">
    <p className="eyebrow">Printing platform</p><h2>Mobile quick print</h2>
    <p className="muted">Your device queues an authorized server job. It never connects directly to a printer.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="inline">
      <input aria-label="Search print library" placeholder="Search labels" value={search} onChange={(event) => setSearch(event.target.value)} />
      <select aria-label="Category" value={category} onChange={(event) => setCategory(event.target.value)}><option value="">All categories</option>{categories.map((item) => <option key={item}>{item}</option>)}</select>
    </div>
    <div className="grid">{visible.map((asset) => <button key={asset.id} className="panel" onClick={() => setSelected(asset)}><strong>{asset.favorite ? "★ " : ""}{asset.name}</strong><small>{asset.category} · default {asset.default_copies}</small></button>)}</div>
    {selected && <form onSubmit={print}><h3>Print {selected.name}</h3>
      <label>Copies<input name="copies" type="number" min="1" max="100" defaultValue={selected.default_copies} required /></label>
      <label>Printer<select name="printer" defaultValue={selected.default_printer_id ?? ""} required><option value="" disabled>Select printer</option>{printers.map((printer) => <option key={printer.id} value={printer.id} disabled={!printer.enabled || printer.status !== "online"}>{printer.friendly_name} · {printer.status}</option>)}</select></label>
      <div className="inline"><button type="submit">Confirm print</button><button type="button" onClick={() => setSelected(null)}>Cancel</button></div>
    </form>}
    <h3>Recently printed</h3><ul>{jobs.slice(0, 8).map((job) => <li key={job.id}><span>{job.origin_type} · {job.copies} copies</span><small>{job.status}{job.failure_message ? ` · ${job.failure_message}` : ""}</small></li>)}</ul>
    <details><summary>Printing administration</summary>
      <form className="inline" onSubmit={createAgent}><input name="name" placeholder="Agent friendly name" required /><button>Create Linux agent</button></form>
      {credential && <p role="status">Copy this credential now; it will not be shown again: <code>{credential}</code></p>}
      <form onSubmit={upload}><label>Name<input name="name" required /></label><label>Category<input name="category" required /></label><label>Description<input name="description" /></label><label>PDF<input name="file" type="file" accept="application/pdf" required /></label><label>Default copies<input name="default_copies" type="number" min="1" max="100" defaultValue="1" required /></label><label>Default printer<select name="default_printer_id"><option value="">None</option>{printers.map((printer) => <option key={printer.id} value={printer.id}>{printer.friendly_name}</option>)}</select></label><label><input name="favorite" type="checkbox" value="true" /> Favorite</label><button>Upload reusable PDF</button></form>
    </details>
  </section>;
}

function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
