package caresearch

import (
	"clearcare/evidence"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// SearchDirectory returns public registry identities, not verified coverage.
func SearchDirectory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, city, state, kind := strings.TrimSpace(q.Get("q")), strings.TrimSpace(q.Get("city")), strings.ToUpper(strings.TrimSpace(q.Get("state"))), q.Get("type")
	if query == "" || city == "" || len(query) > 150 || len(city) > 100 || len(state) != 2 || (kind != "all" && kind != "facilities" && kind != "professionals") {
		evidence.JSON(w, 422, map[string]string{"detail": "Enter a specialty, city, two-letter state, and provider type."})
		return
	}
	params := url.Values{"version": {"2.1"}, "taxonomy_description": {query}, "city": {city}, "state": {state}, "limit": {"100"}}
	if kind == "facilities" {
		params.Set("enumeration_type", "NPI-2")
	}
	if kind == "professionals" {
		params.Set("enumeration_type", "NPI-1")
	}
	var registry struct {
		Errors  []any `json:"Errors"`
		Results []struct {
			Number json.Number `json:"number"`
			Type   string      `json:"enumeration_type"`
			Basic  struct {
				First        string `json:"first_name"`
				Last         string `json:"last_name"`
				Organization string `json:"organization_name"`
				Credential   string `json:"credential"`
				Status       string `json:"status"`
			} `json:"basic"`
			Addresses []struct {
				Purpose string `json:"address_purpose"`
				Address string `json:"address_1"`
				City    string `json:"city"`
				State   string `json:"state"`
				Phone   string `json:"telephone_number"`
			} `json:"addresses"`
			Taxonomies []struct {
				Description string `json:"desc"`
				Primary     bool   `json:"primary"`
			} `json:"taxonomies"`
		} `json:"results"`
	}
	if err := evidence.PublicJSON(r.Context(), "https://npiregistry.cms.hhs.gov/api/?"+params.Encode(), &registry); err != nil || len(registry.Errors) > 0 {
		evidence.JSON(w, 502, map[string]string{"detail": "Provider search is temporarily unavailable. Please try again."})
		return
	}
	results := []map[string]any{}
	for _, provider := range registry.Results {
		if provider.Basic.Status != "A" {
			continue
		}
		name := strings.TrimSpace(provider.Basic.First + " " + provider.Basic.Last)
		providerType := "professionals"
		if provider.Type == "NPI-2" {
			name = provider.Basic.Organization
			providerType = "facilities"
		}
		specialty := ""
		for _, taxonomy := range provider.Taxonomies {
			if specialty == "" || taxonomy.Primary {
				specialty = taxonomy.Description
			}
			if taxonomy.Primary {
				break
			}
		}
		for _, address := range provider.Addresses {
			if address.Purpose != "LOCATION" {
				continue
			}
			results = append(results, map[string]any{"npi": provider.Number.String(), "name": name, "credential": provider.Basic.Credential, "provider_type": providerType, "specialty": specialty, "address": address.Address, "city": address.City, "state": address.State, "phone": address.Phone, "source_url": "https://npiregistry.cms.hhs.gov/provider-view/" + provider.Number.String()})
			break
		}
	}
	evidence.JSON(w, 200, map[string]any{"results": results, "note": "Public registry listings. Confirm insurance network participation and appointment availability directly.", "limit": 100})
}
