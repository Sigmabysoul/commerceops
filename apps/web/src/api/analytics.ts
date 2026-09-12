const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

// This contract presents server-derived metrics; the browser only formats their values.
export type QualityMetrics = {
  checks: number;
  checked_quantity: number;
  passed_quantity: number;
  rejected_quantity: number;
  rejection_rate_percent: number | null;
};

export type WorkforceQuality = QualityMetrics & {
  employee_id: string;
  employee_name: string;
  completed_work_items: number;
  completed_work_quantity: number;
  final_checks: number;
  failed_final_checks: number;
  final_failure_rate_percent: number | null;
};

export type AnalyticsReport = {
  from: string;
  to: string;
  timezone: string;
  generated_at: string;
  metric_version: string;
  quality: QualityMetrics;
  daily: (QualityMetrics & { date: string })[];
  defects: { reason: string; rejected_quantity: number; share_percent: number }[];
  cycles: {
    kind: string;
    samples: number;
    average_hours: number | null;
    median_hours: number | null;
    p95_hours: number | null;
  }[];
  workforce: WorkforceQuality[];
  workforce_total: number;
  limit: number;
  offset: number;
};

export const analyticsAPI = {
  report: async (from: Date, to: Date, timezone: string, offset: number, signal: AbortSignal): Promise<AnalyticsReport> => {
    const query = new URLSearchParams({
      from: from.toISOString(), to: to.toISOString(), timezone, limit: "20", offset: String(offset),
    });
    const response = await fetch(`${API_BASE_URL}/api/v1/reports/operations-analytics?${query}`, {
      credentials: "include", signal, cache: "no-store",
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
      throw new Error(body.error?.message ?? `Unable to load quality analytics (${response.status})`);
    }
    return await response.json() as AnalyticsReport;
  },
};
