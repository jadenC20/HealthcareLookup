# Go backend rewrite (in progress)

This is an experimental, partial implementation. It is not a replacement for the existing backend and is not connected to the frontend launcher. It currently implements health, the sample, bill analysis, provider search, and price comparison. CSV validation and bill-analysis behavior do not yet match the existing backend, so results should not be relied on.

Accounts, persistent reviews, document extraction and OCR, PDF exports, request security, and migration of existing data remain to be implemented and verified. Prescription pricing remains unavailable. The current server must not be exposed publicly.

With Go 1.24 or newer installed, run `go test ./...` in this directory. `CLEARCARE_DEMO=1 go run .` starts the incomplete API on loopback port 8000; stop any existing backend first. Run from this directory so the default data and sample paths resolve.
