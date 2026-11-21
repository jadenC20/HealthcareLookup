// Every ClearCare backend capability, described once as an MCP tool.
// Each entry maps to exactly one fixed backend route; callers can never
// choose an arbitrary upstream path, host or user ID.

const str = (description, extra = {}) => ({ type: 'string', description, ...extra });
const num = (description, extra = {}) => ({ type: 'number', description, ...extra });
const bool = (description) => ({ type: 'boolean', description });
const obj = (properties, required = []) => ({ type: 'object', properties, required, additionalProperties: false });
const id = str('Identifier returned by a previous tool call.', { pattern: '^[A-Za-z0-9_-]{1,128}$' });
const page = { limit: num('Page size, 1-100.', { minimum: 1, maximum: 100 }), cursor: str('Cursor from the previous page.') };
const wait = { wait: bool('Wait (up to ~50s) for the background job to finish and return its result. Defaults to true.') };
const file = (what) => ({
  file_base64: str(`${what} file contents, base64 encoded (no data: prefix).`),
  filename: str('Original filename, e.g. card.png.'),
  content_type: str('MIME type.', { enum: ['application/pdf', 'image/png', 'image/jpeg'] }),
});
const billLine = obj({
  line_id: str('Line identifier.'), service_date: str('YYYY-MM-DD.'), description: str('Line description.'),
  code: str('CPT/HCPCS code.'), quantity: num('Units.'), charge: num('Billed amount in USD.'),
  modifiers: { type: 'array', items: { type: 'string' } },
}, ['description', 'charge']);
const bill = obj({ provider_name: str('Billing provider.'), lines: { type: 'array', items: billLine, minItems: 1 } }, ['lines']);
const billRequest = { type: 'object', description: 'Bill review request: { bill, eob? } as documented in docs/API.md.', properties: { bill, eob: { type: ['object', 'null'] } }, required: ['bill'] };
const preferences = {
  specialty: str('Specialty, e.g. Family Medicine.'), city: str('City.'), state: str('Two-letter state.', { pattern: '^[A-Z]{2}$' }),
  language: str('Preferred language.'), insurance_plan: str('Plan name.'), telehealth: bool('Wants telehealth.'),
  accessibility: { type: 'array', items: { type: 'string' } },
  demographics: obj({ age_group: str(''), gender: str(''), race_ethnicity: str('') }), use_demographics: bool('Use self-reported demographics.'),
};
const location = {
  zip: str('5-digit US ZIP code.', { pattern: '^[0-9]{5}$' }), city: str('City.'), state: str('Two-letter state.', { pattern: '^[A-Z]{2}$' }),
  latitude: num('Latitude.', { minimum: -90, maximum: 90 }), longitude: num('Longitude.', { minimum: -180, maximum: 180 }),
  radius_miles: num('Pharmacy search radius in miles (1-25, default 5).', { minimum: 1, maximum: 25 }),
  delivery: bool('Include mail-order/delivery options (default true).'),
};

export const tools = [
  // Connection and public data
  { name: 'clearcare_health', title: 'Service health', description: 'Backend status, storage mode, AI configuration and enabled features.', method: 'GET', path: '/health', auth: false, input: obj({}) },
  { name: 'clearcare_sample_bill', title: 'Sample bill', description: 'A sample bill with known problems, useful for trying bill review.', method: 'GET', path: '/sample', auth: false, input: obj({}) },
  { name: 'clearcare_data_status', title: 'Reference data status', description: 'Which provider/price/benchmark reference datasets are loaded.', method: 'GET', path: '/data/status', auth: false, input: obj({}) },
  { name: 'clearcare_list_providers', title: 'List providers', description: 'Providers in the loaded directory, filtered by city and specialty.', method: 'GET', path: '/providers', auth: false, query: ['city', 'specialty'], input: obj({ city: str('City.'), specialty: str('Specialty substring.') }) },
  { name: 'clearcare_compare_service_prices', title: 'Compare service prices', description: 'Published prices for a service or procedure code.', method: 'GET', path: '/prices/compare', auth: false, query: ['q', 'procedure_code', 'city', 'price_type'], input: obj({ q: str('Service, e.g. "MRI knee".'), procedure_code: str('CPT/HCPCS code.'), city: str('City.'), price_type: str('Price type.', { enum: ['discounted_cash', 'payer_negotiated', 'gross_charge'] }) }) },

  // Account
  { name: 'clearcare_me', title: 'Current user', description: 'The signed-in user (id, email, name, city, plan).', method: 'GET', path: '/auth/me', input: obj({}) },
  { name: 'clearcare_update_profile', title: 'Update profile', description: 'Replace name, city and plan on the profile. City is used for nearby pharmacy suggestions.', method: 'PATCH', path: '/profile', body: 'json', input: obj({ name: str('Display name.'), city: str('City, e.g. "Philadelphia, PA".'), plan: str('Insurance plan label.') }, ['name']) },
  { name: 'clearcare_get_preferences', title: 'Get preferences', description: 'Saved patient preferences (care search defaults, location).', method: 'GET', path: '/profile/preferences', input: obj({}) },
  { name: 'clearcare_set_preferences', title: 'Set preferences', description: 'Replace saved patient preferences.', method: 'PUT', path: '/profile/preferences', body: 'json', input: obj(preferences) },
  { name: 'clearcare_account_pdf', title: 'Account PDF', description: 'Download a PDF export of the account.', method: 'GET', path: '/account/pdf', binary: true, input: obj({}) },
  { name: 'clearcare_delete_account', title: 'Delete account', description: 'Permanently delete the account and all private data. Requires confirm="DELETE MY ACCOUNT".', method: 'DELETE', path: '/account', destructive: true, confirm: 'DELETE MY ACCOUNT', input: obj({ confirm: str('Type exactly: DELETE MY ACCOUNT') }, ['confirm']) },

  // Insurance
  { name: 'clearcare_insurance_card_status', title: 'Insurance card status', description: 'Whether an insurance card is saved and its lookup status.', method: 'GET', path: '/profile/insurance-card', input: obj({}) },
  { name: 'clearcare_insurance_card_file', title: 'Download insurance card', description: 'Download the saved insurance card file.', method: 'GET', path: '/profile/insurance-card/file', binary: true, input: obj({}) },
  { name: 'clearcare_upload_insurance_card', title: 'Upload insurance card', description: 'Save/replace the insurance card (PNG, JPEG or PDF, max 10 MB); starts plan extraction and SBC discovery.', method: 'POST', path: '/profile/insurance-card', body: 'multipart', files: { file: '' }, input: obj(file('Insurance card'), ['file_base64', 'content_type']) },
  { name: 'clearcare_delete_insurance_card', title: 'Delete insurance card', description: 'Remove the card and derived insurance data.', method: 'DELETE', path: '/profile/insurance-card', destructive: true, input: obj({}) },
  { name: 'clearcare_insurance_status', title: 'Insurance status', description: 'Current insurance workflow revision and result (plan details, benefits, cautions).', method: 'GET', path: '/profile/insurance', input: obj({}) },
  { name: 'clearcare_insurance_context', title: 'Insurance context', description: 'Summarized insurance context used by other features, with coverage cautions.', method: 'GET', path: '/profile/insurance/context', input: obj({}) },
  { name: 'clearcare_confirm_insurance_details', title: 'Confirm insurance details', description: 'Confirm/correct all plan fields (full replacement) and run SBC discovery. Never put member IDs in plan_id.', method: 'PUT', path: '/profile/insurance/details', body: 'json', input: obj({ revision: str('Latest revision from clearcare_insurance_status.'), details: obj({ insurer: str(''), plan_name: str(''), plan_id: str('Public plan/variant ID.'), network: str(''), state: str('Two-letter state.'), year: num('Coverage year, 0 if unknown.'), employer: str('') }) }, ['revision', 'details']) },
  { name: 'clearcare_select_insurance_candidate', title: 'Select SBC candidate', description: 'Download and interpret a candidate SBC returned in the insurance result.', method: 'POST', path: '/profile/insurance/select', body: 'json', input: obj({ revision: str('Latest revision.'), candidate_id: str('Candidate ID from the result.') }, ['revision', 'candidate_id']) },
  { name: 'clearcare_retry_insurance', title: 'Retry insurance', description: 'Reinterpret the saved SBC or rerun extraction/discovery.', method: 'POST', path: '/profile/insurance/retry', body: 'json', input: obj({ revision: str('Latest revision.') }, ['revision']) },
  { name: 'clearcare_upload_sbc', title: 'Upload SBC', description: 'Upload a Summary of Benefits and Coverage PDF and interpret it.', method: 'POST', path: '/profile/insurance/sbc', body: 'multipart', files: { file: '' }, fields: ['revision'], input: obj({ ...file('SBC PDF'), revision: str('Latest revision.') }, ['file_base64', 'revision']) },
  { name: 'clearcare_download_sbc', title: 'Download SBC', description: 'Download the saved SBC PDF.', method: 'GET', path: '/profile/insurance/sbc', binary: true, input: obj({}) },
  { name: 'clearcare_delete_insurance', title: 'Delete insurance result', description: 'Remove the derived insurance result and SBC (keeps the card).', method: 'DELETE', path: '/profile/insurance', destructive: true, input: obj({}) },

  // Bill review
  { name: 'clearcare_extract_bill', title: 'Extract bill', description: 'Extract line items from a bill (and optional EOB) document. Returns a job; results need confirmation before analysis.', method: 'POST', path: '/bill-review/extract', body: 'multipart', async: true, files: { bill: 'bill_', eob: 'eob_' }, input: obj({ bill_file_base64: str('Bill PDF/PNG/JPEG, base64.'), bill_filename: str(''), bill_content_type: str('', { enum: ['application/pdf', 'image/png', 'image/jpeg'] }), eob_file_base64: str('Optional EOB, base64.'), eob_filename: str(''), eob_content_type: str('', { enum: ['application/pdf', 'image/png', 'image/jpeg'] }), ...wait }, ['bill_file_base64', 'bill_content_type']) },
  { name: 'clearcare_analyze_bill', title: 'Analyze bill', description: 'Run deterministic checks, benchmarks, coding flags and AI explanation on a confirmed bill.', method: 'POST', path: '/bill-review/analyze', body: 'json', async: true, input: obj({ confirmed: { const: true, description: 'Must be true: the user confirmed the bill data.' }, facility_id: str('Facility NPI, or empty.'), state: str('Two-letter state.'), setting: str('e.g. office.'), payer: str(''), plan: str(''), request: billRequest, ...wait }, ['confirmed', 'request']) },
  { name: 'clearcare_quick_bill_check', title: 'Quick bill check', description: 'Instant deterministic bill analysis without saving (no AI).', method: 'POST', path: '/bills/analyze', body: 'json', auth: false, input: billRequest },
  { name: 'clearcare_ai_bill_review', title: 'AI bill review', description: 'AI-assisted review of a bill. Requires ai_consent=true.', method: 'POST', path: '/bills/ai-review', body: 'json', input: obj({ ai_consent: { const: true }, bill, eob: { type: ['object', 'null'] } }, ['ai_consent', 'bill']) },
  { name: 'clearcare_extract_document', title: 'Extract document', description: 'Extract text/fields from a medical document upload.', method: 'POST', path: '/documents/extract', body: 'multipart', files: { file: '' }, fields: ['consent'], input: obj({ ...file('Document'), consent: { const: 'true', description: 'Must be "true" to allow AI processing.' } }, ['file_base64', 'consent']) },
  { name: 'clearcare_bill_observations', title: 'Bill observations', description: 'Your saved charge observations, paginated.', method: 'GET', path: '/bill-review/observations', query: ['limit', 'cursor'], input: obj(page) },
  { name: 'clearcare_save_review', title: 'Save review', description: 'Save a bill review to your account.', method: 'POST', path: '/reviews', body: 'json', input: { ...billRequest, properties: { ...billRequest.properties, save_ai: bool('Attach the saved AI analysis.') } } },
  { name: 'clearcare_list_reviews', title: 'List reviews', description: 'Saved bill reviews.', method: 'GET', path: '/reviews', input: obj({}) },
  { name: 'clearcare_get_review', title: 'Get review', description: 'One saved bill review.', method: 'GET', path: '/reviews/{id}', input: obj({ id }, ['id']) },
  { name: 'clearcare_delete_review', title: 'Delete review', description: 'Delete a saved bill review.', method: 'DELETE', path: '/reviews/{id}', destructive: true, input: obj({ id }, ['id']) },
  { name: 'clearcare_review_pdf', title: 'Review PDF', description: 'Download a saved review as PDF.', method: 'GET', path: '/reviews/{id}/pdf', binary: true, input: obj({ id }, ['id']) },
  { name: 'clearcare_report_pdf', title: 'Report PDF', description: 'Generate a PDF report for an unsaved bill review.', method: 'POST', path: '/reports/pdf', body: 'json', binary: true, input: billRequest },

  // Care search
  { name: 'clearcare_recommend_care', title: 'Recommend care', description: 'Ranked provider recommendations with public research; empty input uses saved preferences.', method: 'POST', path: '/care-search/recommend', body: 'json', async: true, input: obj({ ...preferences, ...wait }) },
  { name: 'clearcare_search_care', title: 'Search care', description: 'Search providers/facilities/services by text and location, optionally AI-ranked against your plan.', method: 'GET', path: '/care/search', query: ['q', 'city', 'state', 'type', 'ai_consent'], input: obj({ q: str('What you need, e.g. "dermatologist".'), city: str(''), state: str(''), type: str('Result type filter.'), ai_consent: str('"true" to allow AI ranking.', { enum: ['true', 'false'] }) }, ['q']) },
  { name: 'clearcare_care_recommendations', title: 'Care recommendations', description: 'Recommendations in a city (requires ai_consent="true").', method: 'GET', path: '/care/recommendations', query: ['city', 'specialty', 'include_demo', 'ai_consent'], input: obj({ city: str(''), specialty: str(''), include_demo: str('', { enum: ['true', 'false'] }), ai_consent: { const: 'true' } }, ['ai_consent']) },

  // Prescriptions
  { name: 'clearcare_lookup_drug', title: 'Look up drug', description: 'RxNorm lookup; returns exact products (RxCUI) to choose from.', method: 'GET', path: '/prescription-comparison/drugs', query: ['name'], input: obj({ name: str('Drug name.') }, ['name']) },
  { name: 'clearcare_compare_prescription', title: 'Compare prescription', description: 'Source-backed offers for one exact RxCUI, quantity and ZIP.', method: 'POST', path: '/prescription-comparison/compare', body: 'json', async: true, input: obj({ rxcui: str('RxCUI from clearcare_lookup_drug.'), quantity: num('1-1000.', { minimum: 1, maximum: 1000 }), zip: str('ZIP.'), price_type: str('', { enum: ['cash', 'coupon'] }), current_price: num('What you pay now (optional).'), ...wait }, ['rxcui', 'quantity', 'zip', 'price_type']) },
  { name: 'clearcare_find_medicine_prices', title: 'Find medicine prices', description: 'Search published prices for a medicine name, with suggestions: nearby pharmacies (from the given location or your profile city/preferences) and mail-order options that can ship to you.', method: 'POST', path: '/prescription-comparison/search', body: 'json', async: true, input: obj({ medicine: str('Medicine name, optionally strength and count, e.g. "atorvastatin 20 mg 30 tablets".'), ...location, ...wait }, ['medicine']) },

  // Jobs and results
  { name: 'clearcare_list_jobs', title: 'List jobs', description: 'Your background jobs.', method: 'GET', path: '/jobs', query: ['limit', 'cursor'], input: obj(page) },
  { name: 'clearcare_get_job', title: 'Get job', description: 'Job progress and, when completed, its result.', method: 'GET', path: '/jobs/{id}', input: obj({ id, ...wait }, ['id']), pollable: true },
  { name: 'clearcare_cancel_job', title: 'Cancel job', description: 'Cancel a queued/running job.', method: 'DELETE', path: '/jobs/{id}', input: obj({ id }, ['id']) },
  { name: 'clearcare_list_results', title: 'List results', description: 'Saved feature results.', method: 'GET', path: '/results', query: ['limit', 'cursor', 'feature'], input: obj({ ...page, feature: str('Filter, e.g. prescription_search, care_search, bill_review.') }) },
  { name: 'clearcare_get_result', title: 'Get result', description: 'One saved result.', method: 'GET', path: '/results/{id}', input: obj({ id }, ['id']) },
  { name: 'clearcare_delete_result', title: 'Delete result', description: 'Delete a saved result and its linked observations.', method: 'DELETE', path: '/results/{id}', destructive: true, input: obj({ id }, ['id']) },
];

// Path allowlist derived from the table, mirrored by the Pages proxy.
export const toolByName = new Map(tools.map((tool) => [tool.name, tool]));
