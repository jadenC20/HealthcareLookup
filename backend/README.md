# ClearCare backend

The active backend is the [Go application](goapp/README.md), with entry point [goapp/main.go](goapp/main.go). Older Python files remain as historical source; the development launcher runs Go.

The three product services are Bill Review, Care Search and Prescription Comparison. Accounts, private results, patient preferences and insurance import are shared capabilities. Implemented API behavior and data limitations are documented in the [product specification](../docs/PRODUCT.md).

- [Architecture and graphs](../ARCHITECTURE.md)
- [API contract](../docs/API.md)
- [Operations](../docs/OPERATIONS.md)
- [Reference imports and public sources](DATA_COLLECTION.md)

Run from `backend/goapp` with `go run .`, or use `npm run backend` from the repository root. The connected frontend uses Supabase authentication while Go keeps healthcare records in SQLite and verifies bearer identities against the configured Supabase project.
