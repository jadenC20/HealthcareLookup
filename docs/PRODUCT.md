# Product specification

This document records the user's product vision. It is not a claim that every feature is implemented. The implementation matrix in the [root README](../README.md) is authoritative for current functionality.

## Three product features

### 1. Bill Review

The user uploads a PDF, photograph, or screenshot of a medical bill, invoice, or receipt. An EOB (Explanation of Benefits) is an optional upload beside the bill, associated with that review rather than a permanent profile document. AI should read messy documents, explain charges, and help the user identify questionable billing or pricing.

Any estimate must name its reference and comparison basis. Intended reference sources are Medicare Physician Fee Schedule, OPPS and clinical laboratory fee schedules; CMS NCCI coding edits; hospital price-transparency and insurer Transparency in Coverage files; and the patient's own EOB. These sources answer different questions. A benchmark difference is not automatically an amount owed back, and an SBC is not a price list or proof of claim eligibility. NCCI checks require the applicable context and exceptions; they are not an unconditional code-pair blacklist.

The intended charge dataset records the charging location, observed charge, and an estimated over/under-benchmark amount without embedding PII in that charge row. Separately owned account/review data can provide the user's access to their review. The schema, aggregation/consent policy, and estimation rules for that dataset have not yet been implemented or finalized. The current review JSON is user-linked and must not be described as de-identified.

Open questions: which reference takes precedence for each charge type; how to present uncertain savings; whether to generate dispute/negotiation letters automatically; whether contributions form a shared dataset. Existing deterministic inquiry drafts are not the final product specification.

### 2. Care Search

Find the doctor who best fits the user, using profile context and an explanation of why each doctor fits. The current directory/price filters are a starting implementation, not the full intended product.

Candidate profile inputs to discuss: reason for visit or specialty, location/travel radius, insurance plan, budget, appointment timing, language, accessibility needs, telehealth preference, and optional provider preferences. Which fields are required, ranking weights, data providers, and network/availability verification remain unresolved. Insurance import can supply plan context but does not independently verify a doctor's network status.

### 3. Prescription Comparison

Find less expensive prescribed medicine. Product decisions still needed: whether to compare the exact drug across pharmacies, generic equivalents, or alternative treatments requiring prescriber review; supported geographic scope; pricing feeds; insurance versus cash pricing; and discount eligibility. No medication comparison backend has been implemented.

## Shared insurance profile

Confirmed workflow:

- Insurance-card upload is the primary optional profile entry point.
- OpenAI reads available public plan attributes.
- Automatically search for the applicable SBC using those attributes and authoritative plan sources.
- Ask for missing information or candidate confirmation when the exact plan/year/variant is ambiguous.
- Retrieve and interpret the actual SBC; cite its source and supporting page/quote for each returned benefit.
- Offer manual SBC upload when automatic discovery/retrieval cannot finish, including employer plans with documents behind a login.
- Do not stop all progress when one stage fails. Preserve useful results and flag what is missing with a caution that the result may be inaccurate.

This backend implements that insurance workflow. CMS catalogue matching is available when an operator supplies the CSV; hosted OpenAI web search is the automatic fallback, including public employer/insurer sources. Neither search nor card possession guarantees access to employer-only documents. There is no insurer login or eligibility integration in this version.

## AI and uncertainty contract

OpenAI is the selected provider. It may extract, locate, organize, and explain information. It must not invent prices, coverage terms, source URLs, or missing amounts. Unknowns remain unknown. Results carry `steps`, `cautions`, `missing`, and `action` fields so the caller can explain gaps rather than silently treating partial data as complete.

A caution is not permission to present an arbitrary plan as correct. Plan-ID/year/state or insurer conflicts with the retrieved SBC withhold benefits. Candidate-only matches require selection. Missing benefit evidence removes that benefit from the interpreted list. Readable portions remain available when safe to separate.

Compliance assessment is scheduled for a later phase. The prototype and its documentation make no HIPAA-compliance claim.
