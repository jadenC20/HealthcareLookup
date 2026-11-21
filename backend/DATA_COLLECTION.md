# Data sources and preparation

This guide covers the data actually consumed by the Go backend. The [product specification](../docs/PRODUCT.md) describes additional sources needed for future bill review and care ranking.

## Insurance SBC catalog

The insurance workflow searches a configured local catalog first, then uses OpenAI web search when an exact unique variant is unavailable. The catalog is optional; without it, results include a `catalog_not_configured` caution. No catalog is bundled or automatically downloaded.

Download and extract the relevant coverage year's Plan Attributes CSV from [CMS Marketplace public-use files](https://www.cms.gov/marketplace/resources/data/public-use-files). Set `CLEARCARE_PLAN_CATALOG` to its path and restart the backend. CMS's [2027 Plan Attributes dictionary](https://www.cms.gov/files/document/planattributes-datadictionary-py27.pdf) describes the SBC URL and plan fields. Marketplace data is not a complete directory of employer or other insurance plans.

The loader accepts these CMS headers, or a normalized CSV using [insurance_plans.csv](data/templates/insurance_plans.csv):

| Normalized column | CMS header | Meaning |
|---|---|---|
| insurer | IssuerMarketplaceMarketingName or IssuerName | Public issuer name |
| plan_name | PlanMarketingName | Public plan name |
| plan_id | PlanId or StandardComponentId | Full variant ID when supplied; otherwise base ID |
| state | StateCode | Uppercase state abbreviation |
| year | BusinessYear | Coverage year |
| network | NetworkId | Optional public network identifier |
| sbc_url | URLForSummaryofBenefitsCoverage | Public HTTPS PDF URL |
| source_url | Optional source_url | Provenance; defaults to CMS PUF landing page |

Normalized catalogs are trusted operator-supplied reference data and use the same `cms_catalog` source classification. Populate them only from checked CMS/issuer documents and record the actual source URL. Do not add member identifiers or infer a variant suffix from a base ID. Automatic selection requires a unique full variant ID, matching state/year, and user-confirmed plan details. A base ID alone does not satisfy that condition.

Malformed CSV makes the catalog unavailable. Rows lacking a usable public URL, state, or year are skipped. An empty usable catalog yields a caution. Restart after updates; the catalog is read once at startup. A source URL is provenance, not proof of enrollment. The retrieved PDF's identity is checked again before benefits are returned.

[HealthCare.gov describes the SBC](https://www.healthcare.gov/health-care-law-protections/summary-of-benefits-and-coverage/). It is a plan summary, not an EOB or a live member-benefits response. A card does not guarantee discovery of an exact public SBC. Private portal documents must be uploaded manually; the backend does not log into insurer portals.

## Care Search CSV files

Copy the headers from [providers.csv](data/templates/providers.csv) and [service_prices.csv](data/templates/service_prices.csv) into `backend/data/providers.csv` and `backend/data/service_prices.csv`, or use `CLEARCARE_DATA_DIR` for another directory.

Providers need a stable `provider_id`, identifying/location fields, source URL, and retrieval date. Price rows reference that same ID and include procedure code, description, price, price type, payer/plan where applicable, inclusions, source URL, and retrieval date. Preserve the supplied column names.

Use actual provider directories and published hospital price files. Keep gross charges, discounted cash prices, and payer-negotiated amounts distinct. Do not invent an amount or assume facility fees cover professional services. Do not mark network status confirmed without checking an appropriate insurer source. Demo rows are examples, not observed prices.

Restart the backend after changing files. Inspect `GET /data/status` for loaded counts and issues. `POST /data/reload` intentionally returns 403 and instructs the operator to restart. The Go loader validates these files at startup.

## Bill price references and NCCI imports

Prepare `bill_benchmarks.csv` using the [template](data/templates/bill_benchmarks.csv) in `CLEARCARE_DATA_DIR`. Normalize authoritative Medicare PFS/OPPS/lab, hospital price-transparency or insurer TiC reference rows into this contract. The backend does not automatically download or normalize arbitrary large source files. Do not mix technical/professional components or units when importing; `setting` must distinguish the applicable context.

Required columns: code, state, setting, facility_id, payer, plan, price_type, unit_price, effective_from, effective_to, source_url. Dates use YYYY-MM-DD and source URLs must be public HTTPS. Price types are `medicare`, `negotiated`, or `cash`. Prices must be positive finite numbers. Medicare references match code/state/setting/date; negotiated and cash rows additionally require an exact facility NPI, and negotiated rows require payer/plan. No match means no estimate. These differences are benchmarks, never automatic refunds.

Prepare `ncci_edits.json` using the [empty template](data/templates/ncci_edits.json). Each row has `code1`, `code2`, `setting`, `effective_from`, `effective_to`, `modifier_indicator` (0 or 1), and `source_url`. Import the applicable official CMS release; supply explicit end dates for the version you loaded. Same-date code pairs generate review flags. Modifier-dependent edits retain an exception caution; the app does not adjudicate claims. Reference files are read for each bill review, so changes apply to subsequent requests.

Private EOB uploads are optional claim-specific evidence, not public benchmark imports. Charge observations are stored separately from ownership and private review data. They contain no patient/account/claim fields, but the database includes a separate ownership mapping and must not be called anonymous.

## Patient review cohort data

Place `patient_review_cohorts.json` in the care data directory; [template](data/templates/patient_review_cohorts.json). Each row contains `npi`, `patient_age_group`, `patient_gender`, `patient_race_ethnicity`, `sample_size`, `mean_rating`, `source_url`, `retrieved_date`, and `demographics_self_reported`.

Use only legitimately obtained aggregate reviews with explicitly self-reported patient demographics. Never infer them from names/photos or substitute doctor demographics. Rows require at least ten reviews, a rating from 1 to 5, a public HTTPS source, and retrieval within one year. Empty selected demographic fields are ignored during matching; all specified fields must match. The newest matching row affects ranking. No qualifying data produces a missing-evidence caution. The ten-review floor is a prototype threshold, not a guarantee of statistical reliability. Restart to reload cohorts. No fabricated or bundled demographic reviews are provided.

## Public APIs and web search

- [CMS NPI Registry API](https://npiregistry.cms.hhs.gov/api-page): individual-provider identity/taxonomy/location candidates, version 2.1; not insurance or appointment verification.
- [NLM RxNorm getDrugs](https://lhncbc.nlm.nih.gov/RxNav/APIs/api-RxNorm.getDrugs.html): public drug-product lookup; selected products are resolved through the RxNorm properties endpoint. RxNorm supplies identity, not retail prices.
- [OpenAI web search](https://developers.openai.com/api/docs/guides/tools-web-search): cited public provider, review, price and billing-reference research. Structured candidates/offers retain exact URLs from tool source metadata and explicit uncertainty.

NLM attribution: This product uses publicly available data from the U.S. National Library of Medicine (NLM), National Institutes of Health, Department of Health and Human Services; NLM is not responsible for the product and does not endorse or recommend this or any other product.

There is no universal live pharmacy-price, insurance-network or demographic-review feed configured. Sources may be missing, blocked or conditional; those conditions remain visible in API cautions.

## Imports are optional

Without usable local price rows, Bill Review automatically attempts public-source rate extraction using returned web-search sources. An accepted fallback reference needs matching code, dates, setting, state and applicable facility/payer context, plus a quote. Its origin is `public_search`, it remains cautioned, and it is not saved as a charge observation. Without an applicable rate, bill/EOB checks and inquiry generation still complete. Missing NCCI data falls back to general checks and public coding research, without claiming verified code-pair violations. Missing patient-review cohorts falls back to general provider matching. Operators do not need to populate these files to use the workflows.
