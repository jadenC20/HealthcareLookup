# AI provider

The default backend provider is OpenRouter, using exactly `google/gemma-4-31b-it:free` via `https://openrouter.ai/api/v1/responses`. Set `OPENROUTER_API_KEY` as a backend-only Render environment secret. Do not send it to Pages, browser JavaScript or Git. Optional configuration: `CLEARCARE_AI_PROVIDER=openrouter`, `OPENROUTER_MODEL=google/gemma-4-31b-it:free`.

Existing extraction, interpretation and explanation methods share this adapter. Strict output structures and downstream evidence checks remain in place. No live provider calls have been made without a key. Provider selection restricts inference pricing to zero and excludes endpoints permitting data collection. If this model has no privacy-compatible endpoint, requests fail rather than silently relaxing those restrictions or choosing a different model. This is not a guarantee of zero retention or suitability for regulated health data.

PDF inputs explicitly request OpenRouter's free `cloudflare-ai` file parser rather than its default paid OCR. Native image input uses the selected model. PDF parsing also sends document contents to the parser; review all provider terms before uploading real health records.

Live web search is disabled for this configuration because external search tools can have separate charges. SBC discovery can still use the configured local plan catalog, and users may upload SBCs directly. Missing research returns the existing caution/fallback instead of invented source links. Paid search or another research integration requires a separate decision.

Legacy OpenAI configuration is available only with explicit `CLEARCARE_AI_PROVIDER=openai` and its corresponding backend credentials.

Sources: [model endpoints](https://openrouter.ai/api/v1/models/google/gemma-4-31b-it:free/endpoints), [Responses API](https://openrouter.ai/docs/api_reference/responses/overview), [PDF processing](https://openrouter.ai/docs/guides/overview/multimodal/pdfs).
