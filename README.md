# ClearCare — Terra frontend

The `codex/clearcare-frontend` branch contains the four-screen frontend rebuilt from `FrontendWeb.zip`: Overview, Understand My Bill, Find Affordable Care, and Prescription Savings.

## Preview

Requires Node.js 20 or newer. The generated CSS, fonts, icons, and page templates are committed, so a preview needs no dependency installation:

```powershell
npm run dev
```

Open http://127.0.0.1:4180. Set `PORT` to use another local port. The server listens only on loopback and serves an explicit asset allowlist. No backend process is started, and no `/api` requests are made.

## Edit and build

```powershell
npm ci
npm run build
npm run check
```

The design uses the supplied Terra palette, Nunito Sans body text, Literata overview headlines, and locally bundled Material Symbols. There is no runtime Tailwind CDN or remote font dependency.

## Frontend interactions

- Hash navigation across all four screens, with active states and browser back/forward support.
- Procedure/provider search, sample ZIP filtering, category empty states, and cash/plan price toggles.
- Provider detail dialogs, an illustrative local map, benefits, search, and notification dialogs.
- Prescription filtering, dosage selection, a non-redeemable coupon preview, and eligibility information.
- Local PDF/PNG/JPEG file selection and drop handling, including size/type validation. Files are not read, uploaded, or analyzed.
- Editable sample billing inquiry, text download, and browser Print / Save PDF.
- Demo profile display-name editing for the current browser session only.

All figures, listings, plan information, reviews, and savings are illustrative. A sample review is never generated from a selected file. Booking and advocacy actions do not send requests. Compliance and live insurer-connection claims from the reference have been replaced with explicit preview labels.

## Files

- `index.html`: shared application shell and accessible dialogs
- `pages/`: four HTML screen templates adapted from the supplied reference
- `app.js`: frontend-only navigation and interactions
- `styles/input.css`, `tailwind.config.cjs`: design tokens, responsive styles, and Tailwind source
- `styles.css`: generated CSS for the dependency-free preview
- `assets/`: local fonts, icons, licenses, and illustrative map
- `server.mjs`: frontend static server

The existing backend source, `dev.mjs`, and historical `integration.js` are retained for future integration but are not loaded by this frontend. See `backend/README.md` for the existing backend documentation. This change intentionally does not connect or modify backend behavior.
