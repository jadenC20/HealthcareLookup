# Product specification

## Bill Review

Upload a PDF, photograph or screenshot of a bill, invoice or receipt, with an optional EOB beside it. OpenAI extracts only supported fields and line evidence. Missing monetary values remain null. Users confirm fields before submitting monetary analysis. Bill totals and EOB reconciliation remain deterministic.

Price estimates compare confirmed charge totals against applicable imported references. Match procedure code, service date, state, setting, facility and payer/plan as required. Medicare benchmarks, negotiated rates and cash prices retain distinct labels. An over/under-benchmark difference is not a refund or determination of patient liability. NCCI matches flag code pairs for review, preserving modifier/context uncertainty. The AI explains existing evidence and searches public sources; it cannot supply invented benchmark amounts.

Saved results are private. Separate charge observations contain facility NPI, procedure code, charge, benchmark, difference, price type and source. They contain no patient/account/claim fields or service dates. A private mapping links observations to the owner and saved result. Deleting the result or account removes its linked observations. No public contribution/aggregation program is enabled.

## Care Search

Find suitable providers using specialty, city/state, language, insurance plan, telehealth and accessibility preferences. Optional demographics describe **the patient**, not the physician: age group, gender and race/ethnicity. Users explicitly opt in with `use_demographics`.

The backend obtains individual-provider candidates from the public CMS NPI Registry and falls back to sourced local records when needed. A bounded public AI web search supplies research and source-backed service attributes. Registry candidates start at 60 match points; local fallback candidates start at 40. Matching self-reported patient cohort reviews can add 0–20 points, language 10, telehealth 5, accessibility 5. These implementation defaults are evidence-match scores, not clinical quality ratings or proof that one doctor is best.

Patient cohort comparisons use supplied aggregate review data only: same selected patient demographic fields, self-reported demographics, at least ten reviews, 1–5 average rating, valid source, and a retrieval date no more than one year old. The newest matching row contributes to ranking. The threshold is a prototype rule, not a statistical guarantee. No demographic inference from names/photos, no doctor-demographic filtering, and no fabricated review subgroup statistics. If no matching cohort exists, return a caution and continue other matching.

Patient demographics remain local to the matching calculation and saved profile; they are not included in public search or AI enrichment inputs. Public search uses specialty/location and service preferences. Insurance network, appointment availability and clinical quality remain unverified and do not receive inferred score boosts. Search is limited to the first 50 registry candidates and returns up to 20 recommendations; it is not exhaustive.

## Prescription Comparison

The user selected exact-prescription comparison first. RxNorm lookup returns clinical/branded drug candidates; the user chooses the exact strength/form/brand product. The backend resolves its canonical identity and searches public sources for matching quantity, ZIP and cash/coupon price type.

Only source-backed, exact-product offers with quotes survive validation. Offers are sorted by price within the requested type, with optional difference from the user's current price. Unknown prices are not zero, package prices are not prorated, and insurance prices are not inferred. Generic or therapeutic substitution is not automatically recommended. Coupon eligibility, inventory and checkout price require pharmacist confirmation.

## Shared capabilities and uncertainty

OpenAI is the selected AI provider. Insurance-card import discovers and interprets an SBC with candidate confirmation and manual upload fallback. SBC terms are not live membership, remaining deductible or claim approval.

All new product results carry `uncertainty.may_be_inaccurate`, an overall caution and per-step cautions with missing information and suggested actions. Insurance retains its original top-level caution contract. Product POSTs run as cancellable background jobs with duplicate-active-request protection; partial successful results are saved for the user. Raw document uploads for bill extraction are processed in memory, not stored; extracted results may include claim references and remain private.

Compliance assessment remains a later phase. No HIPAA compliance claim is made. Live provider success and runtime tests for the new product services are deferred. Data dependencies are explicit in the [data guide](../backend/DATA_COLLECTION.md).

## Optional-data fallback policy

No imported dataset is required to use a feature. Bill pricing falls back from imported references to source-backed public research, then to a useful bill/EOB review without a monetary benchmark. Public fallback rates retain quotes and uncertainty and are excluded from observations. General care matching continues without patient cohort data. Prescription identity outages return public identification research rather than an invented match. Every response makes the evidence actually used and still missing visible.

## Results are browsable items

Search results are intended for a grid of providers, drug products, pharmacy offers, bill findings and price comparisons. The backend supplies structured items with display labels, numeric fields, sources and cautions, not a chatbot answer. Research links are separate, explicitly unverified source cards. Supporting prose is optional detail rather than the primary result. Frontend implementation remains deferred.
