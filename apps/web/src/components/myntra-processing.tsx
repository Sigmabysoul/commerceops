"use client";

import { myntraAPI } from "@/api/myntra";
import { MarketplaceProcessing } from "@/components/marketplace-processing";

export function MyntraProcessing() {
  return <MarketplaceProcessing api={myntraAPI} marketplace="myntra" phase="Phase 11 · Batch A" sourceType="CSV" requireIdempotency />;
}
