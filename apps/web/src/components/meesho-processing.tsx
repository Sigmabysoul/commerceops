"use client";

import { meeshoAPI } from "@/api/meesho";
import { MarketplaceProcessing } from "@/components/marketplace-processing";

export function MeeshoProcessing() {
  return <MarketplaceProcessing api={meeshoAPI} marketplace="meesho" phase="Phase 10 · Batch A" />;
}
