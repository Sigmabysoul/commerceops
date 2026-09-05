// Phase 13 registers a minimal service worker so the mobile UI is installable.
// Business data and authenticated API responses are deliberately never cached.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
