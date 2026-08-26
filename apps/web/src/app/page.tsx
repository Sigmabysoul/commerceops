import { HealthStatus } from "@/components/health-status";

export default function Home() {
  return (
    <main>
      <section>
        <p className="eyebrow">CommerceOps</p>
        <h1>Foundation health</h1>
        <p>This development page verifies connectivity to the Go API and its required PostgreSQL dependency.</p>
        <HealthStatus />
      </section>
    </main>
  );
}
