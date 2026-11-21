package caresearch

import (
	"clearcare/evidence"
	"clearcare/insurance"
	"context"
	"sort"
	"strings"
)

type PreferenceEvidence struct {
	NPI           string   `json:"npi"`
	Language      string   `json:"language"`
	Telehealth    *bool    `json:"telehealth"`
	Accessibility []string `json:"accessibility"`
	Source        string   `json:"source_url"`
	Quote         string   `json:"quote"`
}

func enrich(ctx context.Context, p Preferences, result *CareResult) {
	if len(result.Results) == 0 || len(result.Research.Sources) == 0 {
		return
	}
	var parsed struct {
		Facts []PreferenceEvidence `json:"facts"`
	}
	schema := evidence.Object(map[string]any{"facts": evidence.Array(evidence.Object(map[string]any{"npi": evidence.String(), "language": evidence.String(), "telehealth": map[string]any{"type": []string{"boolean", "null"}}, "accessibility": evidence.Array(evidence.String()), "source_url": evidence.String(), "quote": evidence.String()}))})
	// Send only public candidate identities and requested service attributes to AI.
	candidates := []map[string]string{}
	for _, c := range result.Results {
		candidates = append(candidates, map[string]string{"npi": c.NPI, "name": c.Name, "address": c.Address, "city": c.City, "state": c.State})
	}
	input := map[string]any{"candidates": candidates, "language": p.Language, "telehealth": p.Telehealth, "accessibility": p.Accessibility, "evidence": result.Research}
	err := insurance.NewOpenAI().Structured(ctx, "Extract explicitly supported provider service attributes from evidence. Match a supplied candidate only when NPI or exact name and practice address establish identity. Never infer language from names, telehealth from specialty, accessibility from photographs or patient demographics. Use null for unknown telehealth. Include a supporting exact quote and an exact URL from evidence.sources. No demographic or clinical quality inference. At most one fact per NPI.", input, schema, &parsed)
	if err != nil {
		result.Uncertainty.Failure("preference_evidence", err)
		return
	}
	seen := map[string]bool{}
	for _, f := range parsed.Facts {
		if f.NPI == "" || seen[f.NPI] || f.Quote == "" || !evidence.SourceAllowed(f.Source, result.Research.Sources) {
			continue
		}
		seen[f.NPI] = true
		for i := range result.Results {
			c := &result.Results[i]
			if c.NPI != f.NPI {
				continue
			}
			c.PreferenceEvidence = append(c.PreferenceEvidence, f)
			if p.Language != "" && strings.EqualFold(p.Language, f.Language) {
				c.Score += 10
				c.Reasons = append(c.Reasons, "Public source reports requested language; confirm with practice")
			}
			if p.Telehealth && f.Telehealth != nil && *f.Telehealth {
				c.Score += 5
				c.Reasons = append(c.Reasons, "Public source reports telehealth; confirm appointment type")
			}
			matched := false
			for _, need := range p.Accessibility {
				for _, have := range f.Accessibility {
					if strings.EqualFold(need, have) {
						matched = true
					}
				}
			}
			if matched {
				c.Score += 5
				c.Reasons = append(c.Reasons, "Public source reports a requested accessibility feature; confirm details")
			}
		}
	}
	sort.SliceStable(result.Results, func(i, j int) bool {
		if result.Results[i].Score == result.Results[j].Score {
			return result.Results[i].NPI < result.Results[j].NPI
		}
		return result.Results[i].Score > result.Results[j].Score
	})
}
