"use client";

import { flipkartAPI } from "@/api/flipkart";
import { MarketplaceProcessing } from "@/components/marketplace-processing";

export function FlipkartProcessing() {
  return <MarketplaceProcessing api={flipkartAPI} marketplace="flipkart" phase="Phase 3" />;
}
