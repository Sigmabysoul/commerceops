Consignment allocation uses an Inventory-owned transaction-scoped boundary to
reserve aggregated canonical Product Master requirements. It changes
`reserved` and Available but not On-hand. Pre-outbound cancellation releases
those reservations. Only confirmation of a fully packed consignment deducts
On-hand and Reserved together and creates immutable `consignment_out` entries;
workflow progress, UI refreshes, printing, and reprinting remain
inventory-neutral. Consignment and Inventory changes share one PostgreSQL
transaction, so neither can commit alone.
