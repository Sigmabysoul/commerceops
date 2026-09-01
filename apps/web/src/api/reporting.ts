const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type DashboardReport = {
  from: string; to: string; marketplace?: string;
  summary: { orders_processed: number; labels_generated: number; print_runs_completed: number; batches: number; outbound_confirmed_orders?: number; unresolved_records: number; duplicate_records: number; failed_processing_jobs: number };
  marketplaces: { marketplace: string; orders: number; resolved: number; needs_review: number; duplicates: number }[];
  inventory_access: boolean;
  inventory?: { current_on_hand: number; current_reserved: number; current_available: number; stock_in: number; stock_out: number; return_restock: number; adjustments: number; net_movement: number };
  returns_access: boolean;
  returns?: { cancellations: number; returns_received: number; received_quantity: number; restocked_quantity: number; damaged_quantity: number; closed_returns: number; closed_cancellations: number; cohort_returned_orders: number; cohort_resolved_orders: number; cohort_return_rate_percent: number };
  product_movements: { product_id: string; internal_code: string; product_name: string; order_quantity: number; stock_in: number; stock_out: number; return_restock: number; adjustments: number; net_movement: number }[];
  product_movement_total: number;
  product_quantities: { product_id: string; internal_code: string; product_name: string; quantity: number }[];
};

export const reportingAPI = {
  dashboard: async (from: Date, to: Date, marketplace = "") => {
    const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), limit: "100" });
    if (marketplace) query.set("marketplace", marketplace);
    const response = await fetch(`${API_BASE_URL}/api/v1/reports/dashboard?${query}`, { credentials: "include" });
    if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? `Request failed (${response.status})`); }
    return await response.json() as DashboardReport;
  },
};
