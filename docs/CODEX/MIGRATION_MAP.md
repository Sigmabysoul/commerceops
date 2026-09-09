# Phase 15 migration map

Baseline: `d90e4f7c0ceab032bc33a0618e1ceabdc20906b5`. Module: `github.com/commerceops/commerceops/services/api`; go.mod declares Go 1.24.
No package is merged or renamed internally. Direct dependencies include test imports;
self-imports in external test packages are not production import cycles.
All rows have behavior/schema/API impact NONE. Domain purposes are in ../MODULES.md.

## Package inventory

| Current | Target | Direct internal dependencies | Direct importers | Test files |
|---|---|---|---|---|
| `cmd/printer-agent` | `cmd/printer-agent` | `internal/printeragent` | none | none |
| `cmd/server` | `cmd/server` | `internal/app`, `internal/config` | none | none |
| `internal/app` | `internal/app` | `internal/auth`, `internal/authorization`, `internal/automation`, `internal/batch`, `internal/config`, `internal/consignment`, `internal/core`, `internal/health`, `internal/inventory`, `internal/marketplace`, `internal/marketplace/amazon`, `internal/marketplace/snapdeal`, `internal/platform/database`, `internal/platform/httpserver`, `internal/platform/objectstorage`, `internal/platform/pdfextractor`, `internal/platform/pdfgenerator`, `internal/printing`, `internal/product`, `internal/reporting`, `internal/returns` | `cmd/server` | storage_test.go |
| `internal/audit` | `internal/platform/audit` | none | `internal/automation`, `internal/batch`, `internal/consignment`, `internal/core`, `internal/inventory`, `internal/marketplace`, `internal/printing`, `internal/product`, `internal/returns` | none |
| `internal/auth` | `internal/platform/auth` | `internal/platform/httpserver` | `internal/app`, `internal/authorization`, `internal/automation`, `internal/batch`, `internal/consignment`, `internal/core`, `internal/inventory`, `internal/marketplace`, `internal/printing`, `internal/product`, `internal/reporting`, `internal/returns` | password_test.go |
| `internal/authorization` | `internal/platform/authorization` | `internal/auth` | `internal/app`, `internal/automation`, `internal/batch`, `internal/consignment`, `internal/core`, `internal/inventory`, `internal/marketplace`, `internal/printing`, `internal/product`, `internal/reporting`, `internal/returns` | none |
| `internal/automation` | `internal/domain/automation` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/platform/domainevent`, `internal/platform/httpserver`, `internal/platform/objectstorage`, `internal/printing` | `internal/app` | http_test.go, integration_test.go, schedule_test.go |
| `internal/batch` | `internal/domain/batch` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/platform/domainevent`, `internal/platform/httpserver`, `internal/platform/objectstorage`, `internal/platform/pdfgenerator` | `internal/app`, `internal/returns` | integration_test.go |
| `internal/config` | `internal/platform/config` | none | `cmd/server`, `internal/app` | config_test.go |
| `internal/consignment` | `internal/domain/consignment` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/inventory`, `internal/platform/domainevent`, `internal/platform/httpserver`, `internal/reporting` | `internal/app` | integration_test.go |
| `internal/core` | `internal/domain/core` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/platform/httpserver` | `internal/app` | phase1_integration_test.go |
| `internal/health` | `internal/platform/health` | `internal/platform/httpserver` | `internal/app` | handler_test.go |
| `internal/inventory` | `internal/domain/inventory` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/platform/httpserver` | `internal/app`, `internal/consignment`, `internal/returns` | integration_test.go |
| `internal/marketplace` | `internal/domain/marketplace` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/marketplace/amazon`, `internal/marketplace/flipkart`, `internal/marketplace/meesho`, `internal/marketplace/myntra`, `internal/marketplace/snapdeal`, `internal/platform/httpserver`, `internal/platform/objectstorage`, `internal/platform/pdfextractor` | `internal/app` | amazon_integration_test.go, integration_test.go, json_test.go, lease_integration_test.go, meesho_integration_test.go, myntra_integration_test.go, snapdeal_integration_test.go |
| `internal/marketplace/amazon` | `internal/domain/marketplace/amazon` | `internal/platform/pdfextractor`, `internal/platform/pdfgenerator` | `internal/app`, `internal/marketplace` | fixture_test.go, parser_test.go, print_test.go |
| `internal/marketplace/flipkart` | `internal/domain/marketplace/flipkart` | `internal/platform/pdfextractor` | `internal/marketplace` | fixture_test.go, parser_test.go |
| `internal/marketplace/meesho` | `internal/domain/marketplace/meesho` | `internal/platform/pdfextractor` | `internal/marketplace` | parser_test.go, private_fixture_test.go |
| `internal/marketplace/myntra` | `internal/domain/marketplace/myntra` | none | `internal/marketplace` | csv_test.go |
| `internal/marketplace/snapdeal` | `internal/domain/marketplace/snapdeal` | `internal/platform/pdfextractor`, `internal/platform/pdfgenerator` | `internal/app`, `internal/marketplace` | parser_test.go, print_test.go |
| `internal/platform/database` | `internal/platform/database` | none | `internal/app` | core_integration_test.go |
| `internal/platform/domainevent` | `internal/platform/domainevent` | none | `internal/automation`, `internal/batch`, `internal/consignment` | none |
| `internal/platform/httpserver` | `internal/platform/httpserver` | none | `internal/app`, `internal/auth`, `internal/automation`, `internal/batch`, `internal/consignment`, `internal/core`, `internal/health`, `internal/inventory`, `internal/marketplace`, `internal/printing`, `internal/product`, `internal/reporting`, `internal/returns` | none |
| `internal/platform/objectstorage` | `internal/platform/objectstorage` | none | `internal/app`, `internal/automation`, `internal/batch`, `internal/marketplace`, `internal/printing` | local_test.go, s3_test.go |
| `internal/platform/pdfextractor` | `internal/platform/documents/pdf/extractor` | none | `internal/app`, `internal/marketplace/amazon`, `internal/marketplace`, `internal/marketplace/flipkart`, `internal/marketplace/meesho`, `internal/marketplace/snapdeal`, `internal/platform/pdfgenerator` | poppler_test.go |
| `internal/platform/pdfgenerator` | `internal/platform/documents/pdf/generator` | `internal/platform/pdfextractor`, `internal/platform/pdfgenerator` | `internal/app`, `internal/batch`, `internal/marketplace/amazon`, `internal/marketplace/snapdeal` | poppler_test.go, source_pages_test.go |
| `internal/printeragent` | `internal/platform/printeragent` | none | `cmd/printer-agent` | journal_test.go, runner_test.go |
| `internal/printing` | `internal/domain/printing` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/platform/httpserver`, `internal/platform/objectstorage` | `internal/app`, `internal/automation` | integration_test.go |
| `internal/product` | `internal/domain/product` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/platform/httpserver` | `internal/app` | integration_test.go |
| `internal/reporting` | `internal/domain/reporting` | `internal/auth`, `internal/authorization`, `internal/platform/httpserver` | `internal/app`, `internal/consignment`, `internal/returns` | integration_test.go |
| `internal/returns` | `internal/domain/returns` | `internal/audit`, `internal/auth`, `internal/authorization`, `internal/batch`, `internal/inventory`, `internal/platform/httpserver`, `internal/reporting` | `internal/app` | integration_test.go, lifecycle_integration_test.go, phase_completion_integration_test.go |

## Approved batches

1. audit, auth, authorization, config, health, printeragent → platform.
2. core, product, inventory, reporting → domain.
3. batch, printing, returns, consignment, automation → domain.
4. marketplace and all existing adapter/testdata subtrees → domain/marketplace.
5. platform/pdfextractor and pdfgenerator → platform/documents/pdf/extractor and generator.

For each batch update every importer listed above, including tests and cmd/app wiring.
Run backend-format, backend-vet, backend-test with TEST_DATABASE_URL, and backend-build.
These cover all direct importers as well as affected packages. Review the diff and commit;
the prior batch commit is the rollback point. Final gate is make verify-full.

Automation migration paths gain one parent level. Printing/Automation cross-package
fixtures must track the intermediate location until Marketplace moves. Extractor's
Flipkart fixture path changes when Marketplace moves and again when Extractor moves.
Generator's own testdata stays colocated. Do not centralize fixtures or test helpers.

## Discovered relative-path / initialization references

No go:embed or package init functions were found in baseline Go sources. Review each
reference below during its owning batch (some are intentional invalid-input test values).

### internal/automation

```text
services/api/internal/automation/integration_test.go:75: migrations, err := filepath.Glob("../../migrations/*.up.sql")
services/api/internal/automation/integration_test.go:107: f.pdf, err = os.ReadFile("../marketplace/amazon/testdata/sanitized_label_invoice.pdf")
services/api/internal/automation/integration_test.go:483: down, err := os.ReadFile("../../migrations/000022_printing_automation.down.sql")
services/api/internal/automation/integration_test.go:485: up, err := os.ReadFile("../../migrations/000022_printing_automation.up.sql")
```

### internal/batch

```text
services/api/internal/batch/integration_test.go:564: root := filepath.Join("..", "..", "migrations")
```

### internal/consignment

```text
services/api/internal/consignment/integration_test.go:321: root := filepath.Join("..", "..", "migrations")
```

### internal/inventory

```text
services/api/internal/inventory/integration_test.go:159: root := filepath.Join("..", "..", "migrations")
```

### internal/marketplace

```text
services/api/internal/marketplace/amazon_integration_test.go:182: root := filepath.Join("..", "..", "migrations")
services/api/internal/marketplace/integration_test.go:321: root := filepath.Join("..", "..", "migrations")
services/api/internal/marketplace/meesho_integration_test.go:160: root := filepath.Join("..", "..", "migrations")
services/api/internal/marketplace/myntra_integration_test.go:25: data, err := os.ReadFile(filepath.Join("myntra", "testdata", "sanitized_packed_orders.csv"))
services/api/internal/marketplace/myntra_integration_test.go:121: root := filepath.Join("..", "..", "migrations")
services/api/internal/marketplace/snapdeal_integration_test.go:94: root := filepath.Join("..", "..", "migrations")
```

### internal/marketplace/amazon

```text
services/api/internal/marketplace/amazon/fixture_test.go:13: pdf, err := os.ReadFile("testdata/sanitized_label_invoice.pdf")
services/api/internal/marketplace/amazon/print_test.go:18: pdf, err := os.ReadFile("testdata/sanitized_label_invoice.pdf")
services/api/internal/marketplace/amazon/print_test.go:42: pdf, err := os.ReadFile("testdata/sanitized_label_invoice.pdf")
```

### internal/marketplace/flipkart

```text
services/api/internal/marketplace/flipkart/fixture_test.go:16: pdf, err := os.ReadFile("testdata/multi_page.pdf")
services/api/internal/marketplace/flipkart/fixture_test.go:34: pdf, err := os.ReadFile("testdata/modern_label_invoice.pdf")
services/api/internal/marketplace/flipkart/fixture_test.go:58: pdf, err := os.ReadFile("testdata/cropbox_label_invoice.pdf")
```

### internal/marketplace/meesho

```text
services/api/internal/marketplace/meesho/parser_test.go:77: data, err := os.ReadFile(filepath.Join("testdata", "sanitized_label.txt"))
```

### internal/marketplace/myntra

```text
services/api/internal/marketplace/myntra/csv_test.go:13: data, err := os.ReadFile(filepath.Join("testdata", "sanitized_packed_orders.csv"))
services/api/internal/marketplace/myntra/csv_test.go:34: data, _ := os.ReadFile(filepath.Join("testdata", "sanitized_packed_orders.csv"))
services/api/internal/marketplace/myntra/csv_test.go:44: data, _ := os.ReadFile(filepath.Join("testdata", "sanitized_packed_orders.csv"))
```

### internal/marketplace/snapdeal

```text
services/api/internal/marketplace/snapdeal/parser_test.go:14: data, err := os.ReadFile(filepath.Join("testdata", "sanitized_pages.txt"))
```

### internal/platform/objectstorage

```text
services/api/internal/platform/objectstorage/local_test.go:29: for _, key := range []string{"../escape", "/absolute"} {
```

### internal/platform/pdfextractor

```text
services/api/internal/platform/pdfextractor/poppler_test.go:12: fixture := filepath.Join("..", "..", "marketplace", "flipkart", "testdata", "multi_page.pdf")
```

### internal/platform/pdfgenerator

```text
services/api/internal/platform/pdfgenerator/poppler_test.go:18: pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
services/api/internal/platform/pdfgenerator/poppler_test.go:45: pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
services/api/internal/platform/pdfgenerator/source_pages_test.go:15: pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
services/api/internal/platform/pdfgenerator/source_pages_test.go:36: pdf, err := os.ReadFile("testdata/flipkart_a4.pdf")
```

### internal/printeragent

```text
services/api/internal/printeragent/journal_test.go:37: for _, printer := range []string{"", "-evil", "../../printer", "printer name", "x;touch"} {
```

### internal/printing

```text
services/api/internal/printing/integration_test.go:79: f.pdf, err = os.ReadFile(filepath.Join("..", "marketplace", "amazon", "testdata", "sanitized_label_invoice.pdf"))
services/api/internal/printing/integration_test.go:304: root := filepath.Join("..", "..", "migrations")
```

### internal/reporting

```text
services/api/internal/reporting/integration_test.go:198: root := filepath.Join("..", "..", "migrations")
```

### internal/returns

```text
services/api/internal/returns/integration_test.go:331: root := filepath.Join("..", "..", "migrations")
services/api/internal/returns/integration_test.go:385: root := filepath.Join("..", "..", "migrations")
services/api/internal/returns/integration_test.go:443: root := filepath.Join("..", "..", "migrations")
```
