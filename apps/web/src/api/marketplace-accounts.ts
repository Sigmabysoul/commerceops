const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type MarketplaceAccount = {
  id: string;
  marketplace_key: string;
  business_identity_id: string;
  internal_key: string;
  display_name: string;
  status: "active" | "dormant" | "inactive";
  identity_name: string;
};

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, { credentials: "include" });
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
    throw new Error(body.error?.message ?? `Request failed (${response.status})`);
  }
  return response.json() as Promise<T>;
}

export const marketplaceAccountAPI = {
  accounts: () => request<{ marketplace_accounts: MarketplaceAccount[] }>("/marketplace-accounts"),
};
