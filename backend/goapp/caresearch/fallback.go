package caresearch

import (
	"clearcare/evidence"
	"clearcare/insurance"
	"context"
	"strings"
)

func publicFallback(ctx context.Context, p Preferences, result *CareResult) {
	if len(result.Research.Sources) == 0 {
		return
	}
	var parsed struct {
		Candidates []struct {
			Name      string `json:"name"`
			Specialty string `json:"specialty"`
			Address   string `json:"address"`
			City      string `json:"city"`
			State     string `json:"state"`
			NPI       string `json:"npi"`
			Source    string `json:"source_url"`
			Quote     string `json:"quote"`
		} `json:"candidates"`
	}
	s := evidence.String
	schema := evidence.Object(map[string]any{"candidates": evidence.Array(evidence.Object(map[string]any{"name": s(), "specialty": s(), "address": s(), "city": s(), "state": s(), "npi": s(), "source_url": s(), "quote": s()}))})
	err := insurance.NewOpenAI().Structured(ctx, "Extract at most 20 provider candidates explicitly listed in the supplied search evidence. Require exact name, specialty, practice address, requested city/state, an exact supporting quote, and a verbatim source URL from evidence.sources. NPI may be empty and must never be guessed. Do not infer insurance participation, availability, quality, language, accessibility, telehealth, or patient demographics.", map[string]any{"requested_specialty": p.Specialty, "requested_city": p.City, "requested_state": p.State, "evidence": result.Research}, schema, &parsed)
	if err != nil {
		result.Uncertainty.Failure("provider_fallback_extraction", err)
		return
	}
	seen := map[string]bool{}
	for _, c := range parsed.Candidates {
		key := evidence.Clean(c.Name + "|" + c.Address)
		if key == "" || seen[key] || c.Name == "" || c.Address == "" || c.Quote == "" || !strings.EqualFold(c.City, p.City) || c.State != p.State || !strings.Contains(evidence.Clean(c.Specialty), evidence.Clean(p.Specialty)) || !evidence.SourceAllowed(c.Source, result.Research.Sources) {
			continue
		}
		seen[key] = true
		result.Results = append(result.Results, Recommendation{NPI: c.NPI, Name: c.Name, Specialty: c.Specialty, Address: c.Address, City: c.City, State: c.State, Source: c.Source, Score: 20, Reasons: []string{"Public directory fallback; confirm provider identity and current practice details"}, ReviewEvidence: []ReviewSummary{}, PreferenceEvidence: []PreferenceEvidence{}, Origin: "public_search", IdentityStatus: "requires_confirmation"})
		if len(result.Results) >= 20 {
			break
		}
	}
	if len(result.Results) > 0 {
		result.Uncertainty.Warn("provider_match", "public_directory_fallback", "Registry and local data returned no candidates, so the grid includes AI-extracted public directory listings.", "Confirm every listing with the practice and insurer.", "registry_verified_identity")
	}
}
