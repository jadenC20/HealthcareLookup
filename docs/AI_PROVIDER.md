# AI provider

The default backend provider is Cloudflare Workers AI, using `@cf/meta/llama-4-scout-17b-16e-instruct`. Render sends REST requests to the private `https://clearcare-ai.agrigine-mcp.workers.dev/internal/ai` Worker. Configure `CLEARCARE_AI_PROVIDER=cloudflare`, `CLOUDFLARE_AI_MODEL`, and `CLOUDFLARE_AI_URL` on Render. The Worker uses an `AI` binding and the same backend-only `CLEARCARE_PROXY_SECRET` as Render. No AI API key is sent to browser code.

Extraction, interpretation and explanation share this REST adapter. JSON output is checked against the backend schema, and downstream evidence checks remain in place. Cloudflare's free Workers AI allocation is limited; usage beyond account allowances can fail. The selected model and account configuration determine availability and billing.

PDF inputs use Cloudflare's Markdown conversion; image inputs use the model's vision support. Benefits and bill lines require supporting evidence. When PDF page locations are unavailable, the bridge must not invent page citations.

Public research uses bounded search excerpts and only actual retrieved source URLs. SBC discovery also uses the configured local catalog, and users may upload SBCs directly. Missing research returns explicit limitations instead of invented source links.

OpenRouter and OpenAI remain optional explicit compatibility providers, using their respective backend-only credentials.

Sources: [Cloudflare model](https://developers.cloudflare.com/workers-ai/models/llama-4-scout-17b-16e-instruct/), [Workers AI pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/), [Markdown conversion](https://developers.cloudflare.com/workers-ai/features/markdown-conversion/).
