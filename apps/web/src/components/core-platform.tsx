"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { AuditEntry, Company, Employee, Entitlement, Permission, Principal, Role, coreAPI } from "@/api/core";
import { ProductMaster } from "@/components/product-master";
import { FlipkartProcessing } from "@/components/flipkart-processing";
import { BatchPrinting } from "@/components/batch-printing";
import { InventoryWorkspace } from "@/components/inventory-workspace";
import { OperationsDashboard } from "@/components/operations-dashboard";
import { ReturnsWorkspace } from "@/components/returns-workspace";
import { ConsignmentWorkspace } from "@/components/consignment-workspace";

export function CorePlatform() {
  const [user, setUser] = useState<Principal | null>(null);
  const [company, setCompany] = useState<Company | null>(null);
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [entitlements, setEntitlements] = useState<Entitlement[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    const session = await coreAPI.session();
    setUser(session.user);
    const companyResult = await coreAPI.company();
    setCompany(companyResult.company);
    const [employeeResult, roleResult, permissionResult, entitlementResult, auditResult] = await Promise.allSettled([
      coreAPI.employees(), coreAPI.roles(), coreAPI.permissions(), coreAPI.entitlements(), coreAPI.audit(),
    ]);
    if (employeeResult.status === "fulfilled") setEmployees(employeeResult.value.employees);
    if (roleResult.status === "fulfilled") setRoles(roleResult.value.roles);
    if (permissionResult.status === "fulfilled") setPermissions(permissionResult.value.permissions);
    if (entitlementResult.status === "fulfilled") setEntitlements(entitlementResult.value.module_entitlements);
    if (auditResult.status === "fulfilled") setAudit(auditResult.value.audit_logs);
  }, []);

  useEffect(() => { load().catch(() => setUser(null)); }, [load]);

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    const data = new FormData(event.currentTarget);
    try { await coreAPI.login(String(data.get("email")), String(data.get("password")), String(data.get("company"))); await load(); }
    catch (cause) { setError(message(cause)); }
  }

  async function create(event: FormEvent<HTMLFormElement>, kind: "employee" | "role") {
    event.preventDefault(); setError(""); const form = event.currentTarget; const value = String(new FormData(form).get("name"));
    try { if (kind === "employee") await coreAPI.createEmployee(value); else await coreAPI.createRole(value); form.reset(); await load(); }
    catch (cause) { setError(message(cause)); }
  }

  if (!user) return <Login onSubmit={login} error={error} />;
  return <main className="shell"><header><div><p className="eyebrow">CommerceOps · Core Platform</p><h1>{company?.name ?? "Company administration"}</h1><p>{user.email} · tenant {user.company_id}</p></div><button onClick={() => coreAPI.logout().then(() => setUser(null))}>Log out</button></header>
    {error && <p className="error" role="alert">{error}</p>}<div className="grid">
      <Panel title="Employees"><CreateForm label="Employee name" action="Add employee" onSubmit={(event) => create(event, "employee")} /><ul>{employees.map((item) => <li key={item.id}><span>{item.display_name}</span><small>{item.status}</small></li>)}</ul></Panel>
      <Panel title="Roles"><CreateForm label="Role name" action="Add role" onSubmit={(event) => create(event, "role")} />{roles.map((role) => <RoleEditor key={role.id} role={role} permissions={permissions} onSaved={load} onError={setError} />)}</Panel>
      <Panel title="Module access"><p className="muted">Entitlements control availability, independently of billing.</p><ul>{entitlements.map((item) => <li key={item.module_key}><span>{item.module_key}</span><button disabled={item.module_key === "core"} onClick={() => coreAPI.setEntitlement(item.module_key, !item.enabled).then(load).catch((cause) => setError(message(cause)))}>{item.enabled ? "Enabled" : "Disabled"}</button></li>)}</ul></Panel>
      <Panel title="Recent audit activity"><ul>{audit.map((item) => <li key={item.id}><span>{item.action}<small>{item.target_type} · {new Date(item.occurred_at).toLocaleString()}</small></span></li>)}</ul></Panel>
    </div><OperationsDashboard /><ConsignmentWorkspace employees={employees} /><ReturnsWorkspace /><ProductMaster /><FlipkartProcessing /><BatchPrinting /><InventoryWorkspace /></main>;
}

function Login({ onSubmit, error }: { onSubmit: (event: FormEvent<HTMLFormElement>) => void; error: string }) {
  return <main className="login"><section><p className="eyebrow">CommerceOps</p><h1>Sign in</h1><p>Use the company ID issued by your administrator.</p><form onSubmit={onSubmit}><label>Email<input name="email" type="email" autoComplete="username" required /></label><label>Password<input name="password" type="password" autoComplete="current-password" required /></label><label>Company ID<input name="company" required /></label><button type="submit">Sign in</button></form>{error && <p className="error" role="alert">{error}</p>}</section></main>;
}
function Panel({ title, children }: { title: string; children: React.ReactNode }) { return <section className="panel"><h2>{title}</h2>{children}</section>; }
function CreateForm({ label, action, onSubmit }: { label: string; action: string; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) { return <form className="inline" onSubmit={onSubmit}><label><span className="sr-only">{label}</span><input name="name" placeholder={label} required /></label><button>{action}</button></form>; }
function RoleEditor({ role, permissions, onSaved, onError }: { role: Role; permissions: Permission[]; onSaved: () => Promise<void>; onError: (value: string) => void }) {
  const [selected, setSelected] = useState(role.permissions); useEffect(() => setSelected(role.permissions), [role.permissions]);
  return <details><summary>{role.name} <small>{selected.length} permissions</small></summary><div className="checks">{permissions.map((permission) => <label key={permission.key}><input type="checkbox" checked={selected.includes(permission.key)} onChange={(event) => setSelected(event.target.checked ? [...selected, permission.key] : selected.filter((key) => key !== permission.key))} />{permission.key}</label>)}</div><button onClick={() => coreAPI.setRolePermissions(role.id, selected).then(onSaved).catch((cause) => onError(message(cause)))}>Save permissions</button></details>;
}
function message(cause: unknown) { return cause instanceof Error ? cause.message : "Something went wrong"; }
