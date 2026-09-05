"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { automationAPI, AssetOption, PrinterOption, Execution, History, Metric, Rule, RuleInput, Schedule, Trigger } from "@/api/automation";

const triggers: Record<Trigger, string> = { scheduled: "Scheduled print", ecommerce_batch_ready: "Ecommerce batch becomes ready", consignment_packing: "Consignment starts packing", consignment_packed: "Consignment becomes packed" };
const blank: RuleInput = { name: "", enabled: false, paused: false, trigger_type: "scheduled", schedule: { mode: "weekdays", times: ["09:00"] }, asset_id: "", printer_id: "", copies: 1, daily_limit: null, failure_threshold: 3, backoff_seconds: 60, version: 0 };
const tabs = ["Rules", "Scheduled Prints", "Upcoming", "Recent Runs", "Failures", "Reporting"] as const;
type Tab = typeof tabs[number];
const message = (e: unknown) => e instanceof Error ? e.message : "Request failed";
const localTime = (at: string, zone: string) => new Date(at).toLocaleString(undefined, { timeZone: zone });

export function AutomationWorkspace() {
 const [rules, setRules] = useState<Rule[]>([]);
 const [runs, setRuns] = useState<Execution[]>([]);
 const [metrics, setMetrics] = useState<Metric[]>([]);
 const [upcoming, setUpcoming] = useState<{ rule: Rule; at: string }[]>([]);
 const [assets, setAssets] = useState<AssetOption[]>([]);
 const [printers, setPrinters] = useState<PrinterOption[]>([]);
 const [zone, setZone] = useState("UTC");
 const [tab, setTab] = useState<Tab>("Rules");
 const [draft, setDraft] = useState<RuleInput | null>(null);
 const [editing, setEditing] = useState<string | null>(null);
 const [times, setTimes] = useState("09:00");
 const [preview, setPreview] = useState<string[]>([]);
 const [history, setHistory] = useState<History[] | null>(null);
 const [filter, setFilter] = useState("");
 const [error, setError] = useState("");
 const [notice, setNotice] = useState("");
 const [busy, setBusy] = useState(false);
 const [canManage, setCanManage] = useState(false);
 const [canView, setCanView] = useState(false);
 const [testKeys, setTestKeys] = useState<Record<string, string>>({});
 const load = useCallback(async () => {
  const [r, z, u, m, e] = await Promise.all([automationAPI.rules(), automationAPI.timezone(), automationAPI.upcoming(), automationAPI.report(), automationAPI.runs(filter, tab === "Failures")]);
  setRules(r.rules); setZone(z.timezone); setUpcoming(u.upcoming); setMetrics(m.metrics); setRuns(e.runs);
 }, [filter, tab]);
 useEffect(() => {
  let active = true;
  automationAPI.rules().then(() => { if (active) setCanView(true); }).catch(e => { if (active) setError(message(e)); });
  return () => { active = false; };
 }, []);
 useEffect(() => { if (!canView) return; load().catch(e => setError(message(e))); const timer = setInterval(() => load().catch(e => setError(message(e))), 15000); return () => clearInterval(timer); }, [canView, load]);
 // Management endpoints and their tenant-scoped selections enforce automations.manage.
 async function openEditor(rule?: Rule) {
  await action(async () => {
   const options = await automationAPI.options(); setAssets(options.assets); setPrinters(options.printers);
   setEditing(rule?.id ?? null); setDraft(rule ? inputOf(rule) : structuredClone(blank)); setTimes(rule?.schedule.times?.join(", ") ?? "09:00"); setPreview([]);
  });
 }
 async function action(fn: () => Promise<void>) { setBusy(true); setError(""); setNotice(""); try { await fn(); } catch (e) { setError(message(e)); } finally { setBusy(false); } }
 function schedule(): Schedule { return draft?.trigger_type === "scheduled" ? { ...draft.schedule, times: times.split(",").map(t => t.trim()) } : {}; }
 async function save(e: FormEvent) { e.preventDefault(); if (!draft) return; await action(async () => { await automationAPI.save(editing, { ...draft, schedule: schedule() }); setDraft(null); setNotice("Rule saved"); await load(); }); }
 async function test(rule: Rule) {
  if (!window.confirm(`Create a real print job for ${rule.copies} copies of ${rule.asset_name} on ${rule.printer_name}?`)) return;
  const key = testKeys[rule.id] ?? crypto.randomUUID(); setTestKeys(current => ({ ...current, [rule.id]: key }));
  await action(async () => { await automationAPI.test(rule.id, key); setTestKeys(current => { const next = { ...current }; delete next[rule.id]; return next; }); setNotice("Test run queued; see Recent Runs"); await load(); });
 }
 if (!canView) return error ? <section className="panel"><h2>Printing automation</h2><p>{error}</p></section> : null;
 return <section className="panel automation-workspace"><p className="eyebrow">Printing automation</p><h2>Automation workspace</h2>
  <p>Company timezone: <strong>{zone}</strong></p>
  {error && <p role="alert" className="error">{error}</p>}{notice && <p role="status">{notice}</p>}
  <div className="inline"><button disabled={busy} onClick={() => action(load)}>Refresh</button><button onClick={() => { if (canManage) setCanManage(false); else action(async () => { await automationAPI.options(); setCanManage(true); }); }}>{canManage ? "Hide management" : "Manage automations"}</button></div>
  <nav className="inline" aria-label="Automation views">{tabs.map(t => <button key={t} aria-pressed={tab === t} onClick={() => setTab(t)}>{t}</button>)}</nav>
  {canManage && <details><summary>Company timezone</summary><form key={zone} onSubmit={e => { e.preventDefault(); const timezone = String(new FormData(e.currentTarget).get("timezone")); action(async () => { await automationAPI.setTimezone(timezone); await load(); setNotice("Timezone saved. Edit a rule to adopt it."); }); }}><label>IANA timezone<input name="timezone" defaultValue={zone} placeholder="Asia/Kolkata" required /></label><p>Existing rules keep their timezone until edited.</p><button disabled={busy}>Save timezone</button></form></details>}
  {(tab === "Rules" || tab === "Scheduled Prints") && <>
   {canManage && <button disabled={busy} onClick={() => openEditor()}>New rule</button>}
   <div className="grid">{rules.filter(r => tab === "Rules" || r.trigger_type === "scheduled").map(r => <article className="panel" key={r.id}><h3>{r.name}</h3><p>{triggers[r.trigger_type]}{r.trigger_type === "scheduled" && ` · ${r.schedule.mode} ${r.schedule.times?.join(", ")} · ${r.timezone}`}</p><p>{r.copies} copies · {r.asset_name} · {r.printer_name}</p><p>{!r.enabled ? "OFF" : r.paused ? "PAUSED" : "ON"} · version {r.version}{r.next_run_at && ` · Next: ${localTime(r.next_run_at, r.timezone)}`}</p>{r.backoff_until && <p>Backoff until {localTime(r.backoff_until, r.timezone)} · {r.consecutive_failures} failures</p>}
    <div className="inline">{canManage && <><button disabled={busy} onClick={() => openEditor(r)}>Edit</button><button disabled={busy} onClick={() => action(async () => { await automationAPI.pause(r); await load(); })}>{r.paused ? "Resume" : "Pause"}</button><button disabled={busy} onClick={() => test(r)}>Test run</button></>}<button disabled={busy} onClick={() => action(async () => { setHistory((await automationAPI.history(r.id)).history); })}>History</button></div></article>)}</div>{rules.length === 0 && <p>No automation rules yet.</p>}
  </>}
  {draft && <form onSubmit={save} className="panel"><h3>{editing ? "Edit rule" : "Create rule"}</h3>
   <label>Name<input maxLength={120} required value={draft.name} onChange={e => setDraft({ ...draft, name: e.target.value })} /></label>
   <label>Trigger<select value={draft.trigger_type} onChange={e => { setDraft({ ...draft, trigger_type: e.target.value as Trigger, schedule: e.target.value === "scheduled" ? { mode: "weekdays", times: ["09:00"] } : {} }); setPreview([]); }}>{Object.entries(triggers).map(([key, label]) => <option value={key} key={key}>{label}</option>)}</select></label>
   {draft.trigger_type === "scheduled" && <fieldset><legend>Schedule in {zone}</legend><label>Days<select value={draft.schedule.mode} onChange={e => { setDraft({ ...draft, schedule: { ...draft.schedule, mode: e.target.value as Schedule["mode"], weekdays: [] } }); setPreview([]); }}><option value="daily">Daily</option><option value="weekdays">Weekdays</option><option value="selected_weekdays">Selected weekdays</option></select></label>
    {draft.schedule.mode === "selected_weekdays" && <div className="inline">{["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((day, i) => <label key={day}><input type="checkbox" checked={draft.schedule.weekdays?.includes(i + 1) ?? false} onChange={e => { const days = draft.schedule.weekdays ?? []; setDraft({ ...draft, schedule: { ...draft.schedule, weekdays: e.target.checked ? [...days, i + 1] : days.filter(d => d !== i + 1) } }); setPreview([]); }} />{day}</label>)}</div>}
    <label>Clock times (HH:MM, separated by commas)<input required value={times} onChange={e => { setTimes(e.target.value); setPreview([]); }} placeholder="09:00, 14:30" /></label>
    <label>Start date<input type="date" value={draft.schedule.start_date ?? ""} onChange={e => { setDraft({ ...draft, schedule: { ...draft.schedule, start_date: e.target.value } }); setPreview([]); }} /></label><label>End date<input type="date" value={draft.schedule.end_date ?? ""} onChange={e => { setDraft({ ...draft, schedule: { ...draft.schedule, end_date: e.target.value } }); setPreview([]); }} /></label>
    <button type="button" disabled={busy} onClick={() => action(async () => { setPreview((await automationAPI.preview(schedule())).occurrences); })}>Preview next runs</button><ul>{preview.map(at => <li key={at}>{localTime(at, zone)} · {zone}</li>)}</ul>
   </fieldset>}
   <label>Reusable PDF<select required value={draft.asset_id} onChange={e => setDraft({ ...draft, asset_id: e.target.value })}><option value="">Select asset</option>{assets.filter(a => a.active).map(a => <option key={a.id} value={a.id}>{a.name}</option>)}</select></label>
   <label>Printer<select required value={draft.printer_id} onChange={e => setDraft({ ...draft, printer_id: e.target.value })}><option value="">Select printer</option>{printers.map(p => <option key={p.id} value={p.id}>{p.friendly_name} · {p.status}{!p.enabled && " · disabled"}</option>)}</select></label>
   <label>Copies per execution<input type="number" min={1} max={100} required value={draft.copies} onChange={e => setDraft({ ...draft, copies: Number(e.target.value) })} /></label>
   <label>Daily copy limit (optional)<input type="number" min={draft.copies} max={10000} value={draft.daily_limit ?? ""} onChange={e => setDraft({ ...draft, daily_limit: e.target.value ? Number(e.target.value) : null })} /></label>
   <label>Pause after consecutive failures<input type="number" min={1} max={20} required value={draft.failure_threshold} onChange={e => setDraft({ ...draft, failure_threshold: Number(e.target.value) })} /></label>
   <label>Initial failure backoff (seconds)<input type="number" min={1} max={3600} required value={draft.backoff_seconds} onChange={e => setDraft({ ...draft, backoff_seconds: Number(e.target.value) })} /></label>
   <label><input type="checkbox" checked={draft.enabled} onChange={e => setDraft({ ...draft, enabled: e.target.checked })} />Enabled</label><label><input type="checkbox" checked={draft.paused} onChange={e => setDraft({ ...draft, paused: e.target.checked })} />Paused</label>
   <div className="inline"><button disabled={busy}>Save rule</button><button type="button" onClick={() => setDraft(null)}>Cancel</button></div>
  </form>}
  {tab === "Upcoming" && <ul>{upcoming.length === 0 && <li>No scheduled runs upcoming.</li>}{upcoming.map(({ rule, at }) => <li key={rule.id}>{rule.name} · {localTime(at, rule.timezone)} {rule.timezone} · {rule.copies} copies · {rule.printer_name}</li>)}</ul>}
  {(tab === "Recent Runs" || tab === "Failures") && <><label>Rule<select value={filter} onChange={e => setFilter(e.target.value)}><option value="">All rules</option>{rules.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}</select></label><p>Showing the latest 200 runs. A queued job has not yet completed physical printing.</p><ul>{runs.length === 0 && <li>No runs to show.</li>}{runs.map(run => <li key={run.id}><strong>{run.snapshot.name} · version {run.rule_version}{run.test_run && " · TEST"}</strong><p>{localTime(run.created_at, run.snapshot.timezone)} · {run.snapshot.copies} copies · {run.snapshot.printer_name}</p><p>Execution: {run.status} · Print job: {run.job_status ?? "none"} · Attempts: {run.attempt_count}</p>{run.error && <p className="error">{run.error}</p>}{run.status === "failed" && <p>Retry available: {localTime(run.available_at, run.snapshot.timezone)}. Resume a paused rule before retrying.</p>}{run.job_status === "failed" && <p>Inspect the printer, then use the Printing retry workflow.</p>}{canManage && run.status === "failed" && <button disabled={busy} onClick={() => action(async () => { await automationAPI.retry(run.id); await load(); })}>Retry execution</button>}</li>)}</ul></>}
  {tab === "Reporting" && <><p>All-time totals derived from physical print jobs and their events. Reliability uses completed and failed job outcomes.</p><div className="grid">{metrics.map(m => <article className="panel" key={`${m.printer_id}-${m.origin}`}><h3>{m.printer_name} · {m.origin}</h3><p>{m.jobs} jobs · {m.copies} copies</p><p>{m.completed} completed · {m.failed} failed · {m.pending} pending · {m.cancelled} cancelled</p><p>{m.failure_events} failure events · Success rate: {m.completed + m.failed ? `${Math.round(100 * m.completed / (m.completed + m.failed))}%` : "No finished jobs"}</p></article>)}</div></>}
  {history && <aside className="panel"><h3>Rule history</h3><button onClick={() => setHistory(null)}>Close history</button><ul>{history.map(h => <li key={h.id}>{h.action} · {localTime(h.occurred_at, zone)}<details><summary>Change details</summary><pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{JSON.stringify(h.metadata, null, 2)}</pre></details></li>)}</ul></aside>}
 </section>;
}
function inputOf(r: Rule): RuleInput { return { name: r.name, enabled: r.enabled, paused: r.paused, trigger_type: r.trigger_type, schedule: r.schedule, asset_id: r.asset_id, printer_id: r.printer_id, copies: r.copies, daily_limit: r.daily_limit, failure_threshold: r.failure_threshold, backoff_seconds: r.backoff_seconds, version: r.version }; }
