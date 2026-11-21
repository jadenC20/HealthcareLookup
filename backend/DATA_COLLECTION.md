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

Restart the backend after changing files. Inspect `GET /data/status` for loaded counts and issues. `POST /data/reload` intentionally returns 403 and instructs the operator to restart. The legacy Python loader is not the active Go validation workflow.

## Future bill references

Medicare PFS/OPPS/lab schedules, NCCI edits, hospital price-transparency files, insurer Transparency in Coverage files, and optional patient EOBs are intended evidence inputs. They are **not** currently ingested by the bill-review engine. Current manual checks must not be described as an automated fair-price determination. No anonymous facility-charge observation table is implemented yet.
