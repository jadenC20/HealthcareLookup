package caresearch

import "clearcare/evidence"

func recommendationGrid(results []Recommendation) evidence.Grid {
	items := []evidence.Item{}
	for _, c := range results {
		id := c.NPI
		if id == "" {
			id = c.Source + "|" + c.Name + "|" + c.Address
		}
		cautions := []any{}
		if c.IdentityStatus != "registry_verified" {
			cautions = append(cautions, map[string]any{"code": "identity_requires_confirmation", "message": "Confirm the provider identity and current practice details."})
		}
		items = append(items, evidence.NewItem("provider", id, c.Name, c.Specialty, []evidence.Field{evidence.Optional("npi", "NPI", nilIfEmpty(c.NPI), "text"), evidence.Available("address", "Address", c.Address, "address"), evidence.Available("match_score", "Evidence score", c.Score, "score"), evidence.Available("reasons", "Match reasons", c.Reasons, "list"), evidence.Available("identity_status", "Identity status", c.IdentityStatus, "status")}, evidence.URLSource(c.Source), cautions, map[string]any{"npi": c.NPI, "name": c.Name, "specialty": c.Specialty, "address": c.Address, "city": c.City, "state": c.State, "match_score": c.Score, "reasons": c.Reasons, "review_evidence": c.ReviewEvidence, "preference_evidence": c.PreferenceEvidence, "origin": c.Origin, "identity_status": c.IdentityStatus}))
	}
	return evidence.NewGrid(items)
}
func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
