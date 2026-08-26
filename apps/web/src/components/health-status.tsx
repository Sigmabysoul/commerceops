"use client";

import { useEffect, useState } from "react";

import { getHealth } from "@/api/health";
import type { HealthStatus as Health } from "@/types/health";

export function HealthStatus() {
  const [health, setHealth] = useState<Health | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    getHealth(controller.signal).then(setHealth).catch((requestError: unknown) => {
      if (requestError instanceof DOMException && requestError.name === "AbortError") return;
      setError(true);
    });
    return () => controller.abort();
  }, []);

  if (error) return <p className="status unavailable">API unavailable</p>;
  if (!health) return <p className="status">Checking API and PostgreSQL…</p>;
  return <p className={`status ${health.status}`}>API: {health.status} · PostgreSQL: {health.database}</p>;
}
