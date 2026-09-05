"use client";

import { snapdealAPI } from "@/api/snapdeal";
import { MarketplaceProcessing } from "@/components/marketplace-processing";

export function SnapdealProcessing() {
  return <MarketplaceProcessing api={snapdealAPI} marketplace="snapdeal" phase="Phase 12" />;
}
