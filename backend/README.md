# ClearCare backend

The active server is the [Go application](goapp/README.md). The older Python packages and entry point remain as historical source; the development launcher does not run them.

ClearCare has three product features: Bill Review, Care Search, and Prescription Comparison. Account/profile and insurance import are shared infrastructure, not a fourth product. See the [product specification](../docs/PRODUCT.md) for the distinction between implemented behavior and planned work.

Implemented backend capabilities include accounts, sessions, saved reviews, manual bill/EOB checks, PDF reports, CSV provider/price lookup, and OpenAI-backed insurance card extraction, SBC discovery, download, and interpretation. Insurance failures produce saved partial results with explicit cautions and missing fields. Bill document extraction and prescription pricing remain unimplemented.

- [Architecture and diagrams](../ARCHITECTURE.md)
- [API contract](../docs/API.md)
- [Configuration, operation, and troubleshooting](../docs/OPERATIONS.md)
- [Data sources and CSV preparation](DATA_COLLECTION.md)

Start the Go API from `backend/goapp` with `go run .`. It listens on `127.0.0.1:8000`. Export configuration first; the server does not automatically read `.env` files.
