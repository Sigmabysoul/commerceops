const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type Product = { id: string; internal_code: string; name: string; brand: string | null; variant: string | null; size: string | null; pack_type: string | null; unit_count: number | null; status: "active" | "inactive" };
export type Marketplace = { key: string; display_name: string };
export type SKUMapping = { id: string; marketplace_key: string; product_id: string; sku: string; quantity_multiplier: number; interpretation_metadata: Record<string, unknown>; status: "active" | "inactive" };
export type Resolution = { status: "resolved" | "unresolved"; product?: Product; mapping?: SKUMapping };
type ProductInput = Omit<Product, "id">;
type MappingInput = Omit<SKUMapping, "id">;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}/api/v1${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? `Request failed (${response.status})`); }
  return await response.json() as T;
}

export const productAPI = {
  products: (query = "") => request<{ products: Product[] }>(`/products?q=${encodeURIComponent(query)}`),
  createProduct: (input: ProductInput) => request<{ product: Product }>("/products", { method: "POST", body: JSON.stringify(input) }),
  updateProduct: (id: string, input: ProductInput) => request<{ product: Product }>(`/products/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  marketplaces: () => request<{ marketplaces: Marketplace[] }>("/marketplaces"),
  mappings: () => request<{ sku_mappings: SKUMapping[] }>("/sku-mappings"),
  createMapping: (input: MappingInput) => request<{ sku_mapping: SKUMapping }>("/sku-mappings", { method: "POST", body: JSON.stringify(input) }),
  updateMapping: (id: string, input: MappingInput) => request<{ sku_mapping: SKUMapping }>(`/sku-mappings/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  resolve: (marketplaceKey: string, sku: string) => request<Resolution>("/sku-mappings/resolve", { method: "POST", body: JSON.stringify({ marketplace_key: marketplaceKey, sku }) }),
};
