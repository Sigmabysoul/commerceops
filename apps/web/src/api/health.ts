import type { HealthStatus } from "@/types/health";

const apiBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export async function getHealth(signal?: AbortSignal): Promise<HealthStatus> {
  const response = await fetch(`${apiBaseURL}/api/v1/health`, { signal, cache: "no-store" });
  const body = (await response.json()) as HealthStatus;
  if (!response.ok) {
    return { status: "unavailable", database: body.database ?? "unavailable" };
  }
  return body;
}
