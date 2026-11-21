# ClearCare backend

The backend is the [Go application](goapp/README.md), with entry point [goapp/main.go](goapp/main.go). There is no Python implementation or local OCR dependency.

The three product services are Bill Review, Care Search and Prescription Comparison. Accounts, private results, patient preferences and insurance import are shared capabilities. Implemented API behavior and data limitations are documented in the [product specification](../docs/PRODUCT.md).

- [Architecture and graphs](../ARCHITECTURE.md)
- [API contract](../docs/API.md)
- [Operations](../docs/OPERATIONS.md)
- [Reference imports and public sources](DATA_COLLECTION.md)

Run from `backend/goapp` with `go run .`. Export configuration first; `.env` files are not loaded automatically. The frontend is unchanged and does not yet expose all new API workflows.
