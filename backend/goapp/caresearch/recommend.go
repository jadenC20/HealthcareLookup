package caresearch

import (
	"clearcare/evidence"
	"clearcare/insurance"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Demographics describe the patient. They never filter a physician's identity.
type Demographics struct {
	AgeGroup      string `json:"age_group"`
	Gender        string `json:"gender"`
	RaceEthnicity string `json:"race_ethnicity"`
}
type Preferences struct {
	Specialty       string       `json:"specialty"`
	City            string       `json:"city"`
	State           string       `json:"state"`
	Language        string       `json:"language"`
	InsurancePlan   string       `json:"insurance_plan"`
	Telehealth      bool         `json:"telehealth"`
	Accessibility   []string     `json:"accessibility"`
	Demographics    Demographics `json:"demographics"`
	UseDemographics bool         `json:"use_demographics"`
}

func (p Preferences) Validate() bool {
	for _, a := range p.Accessibility {
		if len(a) > 100 {
			return false
		}
	}
	if len(p.State) == 2 && (p.State[0] < 'A' || p.State[0] > 'Z' || p.State[1] < 'A' || p.State[1] > 'Z') {
		return false
	}
	return len(p.Specialty) <= 150 && len(p.City) <= 100 && len(p.State) == 2 && strings.ToUpper(p.State) == p.State && len(p.Language) <= 80 && len(p.InsurancePlan) <= 200 && len(p.Accessibility) <= 10 && len(p.Demographics.AgeGroup) <= 50 && len(p.Demographics.Gender) <= 80 && len(p.Demographics.RaceEthnicity) <= 100 && p.Specialty != "" && p.City != ""
}

type Recommendation struct {
	NPI                string               `json:"npi"`
	Name               string               `json:"name"`
	Specialty          string               `json:"specialty"`
	Address            string               `json:"address"`
	City               string               `json:"city"`
	State              string               `json:"state"`
	Source             string               `json:"source_url"`
	Score              int                  `json:"match_score"`
	Reasons            []string             `json:"reasons"`
	ReviewEvidence     []ReviewSummary      `json:"review_evidence"`
	PreferenceEvidence []PreferenceEvidence `json:"preference_evidence"`
	Origin             string               `json:"origin"`
	IdentityStatus     string               `json:"identity_status"`
}
type ReviewSummary struct {
	NPI           string  `json:"npi"`
	AgeGroup      string  `json:"patient_age_group"`
	Gender        string  `json:"patient_gender"`
	RaceEthnicity string  `json:"patient_race_ethnicity"`
	Count         int     `json:"sample_size"`
	Rating        float64 `json:"mean_rating"`
	Source        string  `json:"source_url"`
	Date          string  `json:"retrieved_date"`
	SelfReported  bool    `json:"demographics_self_reported"`
}
type CareResult struct {
	MatchingBasis []string                 `json:"matching_basis"`
	Results       []Recommendation         `json:"results"`
	Research      insurance.SearchEvidence `json:"research"`
	Explanation   string                   `json:"explanation"`
	Uncertainty   evidence.Report          `json:"uncertainty"`
	Grid          evidence.Grid            `json:"grid"`
	ResearchGrid  evidence.Grid            `json:"research_grid"`
}

func (s *Service) Recommend(ctx context.Context, p Preferences) CareResult {
	result := CareResult{Results: []Recommendation{}, Uncertainty: evidence.New()}
	result.MatchingBasis = []string{"specialty_and_location", "available_service_preferences"}
	query := url.Values{"version": {"2.1"}, "enumeration_type": {"NPI-1"}, "taxonomy_description": {p.Specialty}, "city": {p.City}, "state": {p.State}, "limit": {"50"}}
	var registry struct {
		Errors  []any `json:"Errors"`
		Results []struct {
			Number json.Number `json:"number"`
			Basic  struct {
				First  string `json:"first_name"`
				Last   string `json:"last_name"`
				Status string `json:"status"`
			} `json:"basic"`
			Addresses []struct {
				Purpose string `json:"address_purpose"`
				Address string `json:"address_1"`
				City    string `json:"city"`
				State   string `json:"state"`
			} `json:"addresses"`
			Taxonomies []struct {
				Description string `json:"desc"`
				Primary     bool   `json:"primary"`
			} `json:"taxonomies"`
		} `json:"results"`
	}
	err := evidence.PublicJSON(ctx, "https://npiregistry.cms.hhs.gov/api/?"+query.Encode(), &registry)
	if err != nil || len(registry.Errors) > 0 {
		result.Uncertainty.Warn("provider_registry", "unavailable", "The public NPI registry could not complete the lookup.", "Retry the search.", "registry_results")
	} else {
		for _, v := range registry.Results {
			if v.Basic.Status != "A" {
				continue
			}
			for _, a := range v.Addresses {
				if a.Purpose != "LOCATION" || !strings.EqualFold(a.City, p.City) || a.State != p.State {
					continue
				}
				specialties := []string{}
				for _, t := range v.Taxonomies {
					specialties = append(specialties, t.Description)
				}
				result.Results = append(result.Results, Recommendation{NPI: string(v.Number), Name: strings.TrimSpace(v.Basic.First + " " + v.Basic.Last), Specialty: strings.Join(specialties, ", "), Address: a.Address, City: a.City, State: a.State, Source: "https://npiregistry.cms.hhs.gov/provider-view/" + string(v.Number), Score: 60, Reasons: []string{"NPI registry result for requested specialty and city"}, ReviewEvidence: []ReviewSummary{}, Origin: "npi_registry", IdentityStatus: "registry_verified"})
				break
			}
		}
	}
	if len(result.Results) == 0 {
		for _, v := range s.providers {
			if strings.EqualFold(str(v["city"]), p.City) && strings.EqualFold(str(v["state"]), p.State) && strings.Contains(evidence.Clean(str(v["specialty"])), evidence.Clean(p.Specialty)) && v["is_demo"] != true {
				result.Results = append(result.Results, Recommendation{NPI: str(v["npi"]), Name: str(v["name"]), Specialty: str(v["specialty"]), Address: str(v["address"]), City: p.City, State: p.State, Source: str(v["source_url"]), Score: 40, Reasons: []string{"Matches locally sourced specialty and city"}, ReviewEvidence: []ReviewSummary{}, Origin: "local_source", IdentityStatus: "source_listed"})
			}
		}
	}
	// Demographic matching happens locally against explicitly self-reported aggregate reviews.
	if p.UseDemographics {
		matched := false
		for i := range result.Results {
			for _, v := range s.reviews {
				if v.NPI != result.Results[i].NPI || v.NPI == "" || !v.SelfReported || v.Count < 10 {
					continue
				}
				d := p.Demographics
				if d.AgeGroup == "" && d.Gender == "" && d.RaceEthnicity == "" {
					continue
				}
				if d.AgeGroup != "" && !strings.EqualFold(d.AgeGroup, v.AgeGroup) || d.Gender != "" && !strings.EqualFold(d.Gender, v.Gender) || d.RaceEthnicity != "" && !strings.EqualFold(d.RaceEthnicity, v.RaceEthnicity) {
					continue
				}
				result.Results[i].ReviewEvidence = append(result.Results[i].ReviewEvidence, v)
				matched = true
			}
			if len(result.Results[i].ReviewEvidence) > 0 {
				v := result.Results[i].ReviewEvidence[0]
				result.Results[i].Score += int((v.Rating - 1) * 5)
				result.Results[i].Reasons = append(result.Results[i].Reasons, fmt.Sprintf("Self-reported patient cohort review average %.1f/5, n=%d; observational evidence", v.Rating, v.Count))
			}
		}
		if !matched {
			result.Uncertainty.Warn("patient_reviews", "demographic_evidence_missing", "No qualifying self-reported patient review cohort matched these demographics.", "Use the available specialty/location match; add verified aggregate review data when available.", "patient_demographic_review_data")
		}
	}
	sort.SliceStable(result.Results, func(i, j int) bool {
		if result.Results[i].Score == result.Results[j].Score {
			return result.Results[i].NPI < result.Results[j].NPI
		}
		return result.Results[i].Score > result.Results[j].Score
	})
	if len(result.Results) > 20 {
		result.Results = result.Results[:20]
	}
	// Deliberately omit patient demographics and identities from public web queries.
	publicQuery := fmt.Sprintf("Find official provider directory and patient experience review sources for %s doctors in %s, %s. Preferred language: %s; telehealth requested: %t; plan: %s. Do not claim network participation, availability or patient demographic review statistics without explicit evidence.", p.Specialty, p.City, p.State, p.Language, p.Telehealth, p.InsurancePlan)
	publicQuery += " Requested accessibility: " + strings.Join(p.Accessibility, ", ") + ". Prioritize these registry candidates: "
	for i, c := range result.Results {
		if i >= 10 {
			break
		}
		publicQuery += fmt.Sprintf("%s, NPI %s, %s; ", c.Name, c.NPI, c.Address)
	}
	result.Research, err = insurance.NewOpenAI().Search(ctx, publicQuery)
	if err != nil {
		result.Uncertainty.Failure("web_search", err)
	}
	if len(result.Results) == 0 {
		publicFallback(ctx, p, &result)
	}
	enrich(ctx, p, &result)
	result.Uncertainty.Warn("provider_verification", "unverified_details", "Registry listings and public reviews do not verify current appointments, network participation, accessibility, language services, or clinical quality.", "Confirm these details with the practice and insurer.", "appointments", "insurance_network", "accessibility", "language", "telehealth", "clinical_quality")
	if len(result.Results) == 0 {
		result.Uncertainty.Warn("provider_match", "no_results", "No verified directory candidates were found.", "Broaden city or specialty and inspect the public search sources.", "provider_candidates")
	}
	result.Explanation = "Ranked by specialty/location evidence, optional self-reported patient cohort reviews, and source-backed language, telehealth and accessibility matches. Match scores describe available evidence, not clinical quality. Web-extracted attributes require confirmation; insurance network status does not affect scores without verification."
	cohortUsed := false
	for _, c := range result.Results {
		if len(c.ReviewEvidence) > 0 {
			cohortUsed = true
			break
		}
	}
	if cohortUsed {
		result.MatchingBasis = append(result.MatchingBasis, "patient_review_cohorts")
	} else if p.UseDemographics {
		result.MatchingBasis = append(result.MatchingBasis, "general_matching_fallback")
	}
	result.Grid = recommendationGrid(result.Results)
	result.ResearchGrid = evidence.SourceGrid(result.Research.Sources)
	return result
}
func (s *Service) HandleRecommend(w http.ResponseWriter, r *http.Request) {
	var p Preferences
	if evidence.Decode(r, &p) != nil || !p.Validate() {
		evidence.JSON(w, 422, map[string]string{"detail": "Provide specialty, city, uppercase state and valid preferences."})
		return
	}
	evidence.JSON(w, 200, s.Recommend(r.Context(), p))
}
