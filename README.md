# ClearCare

Local, dependency-free frontend prototype for HealthcareLookup.

## Run

Requires Node.js 20 or later. Run `npm run dev`, then open http://127.0.0.1:4173. Run `npm run check` for JavaScript syntax checks.

## Implemented

- Responsive overview and navigation.
- Local file-selection UI with file-extension and size validation. Files are not read, transmitted, or persisted.
- Explicitly labeled sample bill review and downloadable sample inquiry.
- Procedure and prescription comparisons with working filters, empty states, and details dialogs.
- Keyboard focus indicators and native dialog behavior.

## Data and integration boundaries

All patients, providers, pharmacies, and prices are fictional. Bill findings are prewritten demonstration results. There is no AI analysis, real pricing feed, insurance verification, authentication, or database yet. This is a frontend only; selecting a document does not analyze it. Do not commit real patient documents or credentials.

Next integration points: local document extraction and validation, source-linked findings, verified provider directory and price records, and exact-plan network evidence. Keep bill arithmetic in deterministic backend rules and maintain source references with each finding.

## Project layout

- `index.html`: application shell and navigation
- `styles.css`: responsive design system
- `app.js`: views, fictional datasets, and browser interactions
- `server.mjs`: loopback-only static preview server with an explicit asset allowlist

No external fonts, scripts, analytics, or cloud services are required.
