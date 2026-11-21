package caresearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"
)

func stubDirectoryRegistry(t *testing.T, body any, inspect func(url.Values)) {
	t.Helper()
	previous := searchDirectoryPublicJSON
	t.Cleanup(func() { searchDirectoryPublicJSON = previous })
	searchDirectoryPublicJSON = func(_ context.Context, target string, result any) error {
		u, err := url.Parse(target)
		if err != nil || u.Scheme != "https" || u.Host != "npiregistry.cms.hhs.gov" {
			t.Fatal("unexpected registry destination")
		}
		if inspect != nil {
			inspect(u.Query())
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, result)
	}
}

func TestDirectoryTranslatesPlainServicesWithoutChangingProviderKind(t *testing.T) {
	for _, test := range []struct{ query, kind, taxonomy, enumeration string }{
		{"MRI knee", "all", "Radiology", ""},
		{"CT scan", "facilities", "Radiology", "NPI-2"},
		{"chest X-rays", "professionals", "Radiology", "NPI-1"},
		{"ultrasound", "all", "Radiology", ""},
		{"blood test", "all", "Clinical Medical Laboratory", ""},
		{"bloodtest", "facilities", "Clinical Medical Laboratory", "NPI-2"},
		{"lab", "professionals", "Clinical Medical Laboratory", "NPI-1"},
		{"office visit", "all", "Family Medicine", ""},
		{"primarycare", "professionals", "Family Medicine", "NPI-1"},
		{"Dermatology", "all", "Dermatology", ""},
		{"General Practice", "all", "General Practice", ""},
	} {
		t.Run(test.query+test.kind, func(t *testing.T) {
			stubDirectoryRegistry(t, map[string]any{"results": []any{}}, func(query url.Values) {
				if query.Get("taxonomy_description") != test.taxonomy || query.Get("enumeration_type") != test.enumeration || query.Get("city") != "Philadelphia" || query.Get("state") != "PA" {
					t.Fatalf("service/location/kind mapping changed: %v", query)
				}
			})
			query := url.Values{"q": {test.query}, "city": {"Philadelphia"}, "state": {"pa"}, "type": {test.kind}}
			w := httptest.NewRecorder()
			SearchDirectory(w, httptest.NewRequest("GET", "/care/search?"+query.Encode(), nil))
			if w.Code != 200 {
				t.Fatalf("plain-service search failed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func directoryFixture(number, kind, status, city, state, purpose string) map[string]any {
	return map[string]any{
		"number": json.Number(number), "enumeration_type": kind,
		"basic":      map[string]any{"first_name": "Test", "last_name": "Physician", "organization_name": "Test Imaging", "status": status},
		"taxonomies": []any{map[string]any{"desc": "Radiology", "primary": true}},
		"addresses":  []any{map[string]any{"address_purpose": purpose, "address_1": "1 Main St", "city": city, "state": state}},
	}
}

func TestDirectoryRequiresActiveMatchingRegistryLocationAndKind(t *testing.T) {
	for _, kind := range []string{"all", "facilities", "professionals"} {
		t.Run(kind, func(t *testing.T) {
			rows := []any{
				directoryFixture("1234567890", "NPI-1", "A", "Philadelphia", "PA", "LOCATION"),
				directoryFixture("1234567891", "NPI-2", "A", "philadelphia", "pa", "LOCATION"),
				directoryFixture("1234567892", "NPI-1", "D", "Philadelphia", "PA", "LOCATION"),
				directoryFixture("1234567893", "NPI-1", "A", "Boston", "PA", "LOCATION"),
				directoryFixture("1234567894", "NPI-2", "A", "Philadelphia", "NJ", "LOCATION"),
				directoryFixture("1234567895", "NPI-1", "A", "Philadelphia", "PA", "MAILING"),
				directoryFixture("1234567896", "unknown", "A", "Philadelphia", "PA", "LOCATION"),
				directoryFixture("123", "NPI-1", "A", "Philadelphia", "PA", "LOCATION"),
				directoryFixture("1234567890", "NPI-1", "A", "Philadelphia", "PA", "LOCATION"),
			}
			stubDirectoryRegistry(t, map[string]any{"results": rows}, nil)
			query := url.Values{"q": {"MRI"}, "city": {"Philadelphia"}, "state": {"PA"}, "type": {kind}}
			w := httptest.NewRecorder()
			SearchDirectory(w, httptest.NewRequest("GET", "/care/search?"+query.Encode(), nil))
			var response struct {
				Results []map[string]any `json:"results"`
			}
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			want := 1
			if kind == "all" {
				want = 2
			}
			if len(response.Results) != want {
				t.Fatalf("unmatched/inactive/duplicate providers returned: %#v", response.Results)
			}
			for _, result := range response.Results {
				if kind != "all" && result["provider_type"] != kind {
					t.Fatal("registry ignored requested provider kind")
				}
				if result["source_url"] != "https://npiregistry.cms.hhs.gov/provider-view/"+str(result["npi"]) {
					t.Fatal("registry provenance changed")
				}
			}
		})
	}
}

func TestDirectoryDefaultsToAllKindsAndHandlesRegistryFailure(t *testing.T) {
	stubDirectoryRegistry(t, map[string]any{"results": []any{}}, func(query url.Values) {
		if query.Get("enumeration_type") != "" {
			t.Fatal("default search excluded organizational providers")
		}
	})
	w := httptest.NewRecorder()
	SearchDirectory(w, httptest.NewRequest("GET", "/care/search?q=bloodtest&city=Philadelphia&state=PA", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	searchDirectoryPublicJSON = func(context.Context, string, any) error { return errors.New("private upstream detail") }
	w = httptest.NewRecorder()
	SearchDirectory(w, httptest.NewRequest("GET", "/care/search?q=bloodtest&city=Philadelphia&state=PA", nil))
	if w.Code != 502 {
		t.Fatalf("registry failure status: %d", w.Code)
	}
}
