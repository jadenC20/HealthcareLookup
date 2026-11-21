package prescription

import (
	"clearcare/insurance"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
)

type medicineSearchFixture struct {
	research        insurance.SearchEvidence
	offers          []MedicineSearchOffer
	searchErr       error
	structuredErr   error
	searchCalls     int
	structuredCalls int
	query           string
	input           map[string]any
}

func (f *medicineSearchFixture) Search(_ context.Context, query string) (insurance.SearchEvidence, error) {
	f.searchCalls++
	f.query = query
	return f.research, f.searchErr
}

func (f *medicineSearchFixture) Structured(_ context.Context, _ string, input, _ any, dest any) error {
	f.structuredCalls++
	f.input = input.(map[string]any)
	if f.structuredErr != nil {
		return f.structuredErr
	}
	raw, err := json.Marshal(map[string]any{"offers": f.offers})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

func medicineTestPrice(value float64) *float64 { return &value }

func TestMedicineNameOnlySearchGroundsAndSortsRetrievedPrices(t *testing.T) {
	const firstURL, secondURL = "https://pharmacy.example/metformin", "https://other.example/metformin"
	firstQuote := "Example Pharmacy: Metformin 500 mg tablets, 30 tablets, price $12.00 with coupon."
	secondQuote := "Other Pharmacy: Metformin 500mg tablets, 30 tablets, price USD 8.50. Members only."
	ai := &medicineSearchFixture{
		research: insurance.SearchEvidence{Text: "Research summary", Sources: []string{firstURL, secondURL}, Excerpts: map[string]string{firstURL: firstQuote, secondURL: secondQuote}},
		offers: []MedicineSearchOffer{
			{Medicine: "Metformin 500 mg tablets", Pharmacy: "Example Pharmacy", Price: medicineTestPrice(12), Currency: "USD", Quantity: "30 tablets", SourceURL: firstURL, Quote: firstQuote, Conditions: "with coupon"},
			{Medicine: "Metformin 500mg tablets", Pharmacy: "Other Pharmacy", Price: medicineTestPrice(8.5), Currency: "USD", Quantity: "30 tablets", SourceURL: secondURL, Quote: secondQuote, Conditions: "Members only"},
		},
	}
	w := httptest.NewRecorder()
	searchWithMedicineAI(w, httptest.NewRequest("POST", "/prescription-comparison/search", strings.NewReader(`{"medicine":"metformin 500mg tablets"}`)), ai)
	var result MedicineSearchResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Status != "prices_found" || len(result.Offers) != 2 || *result.Offers[0].Price != 8.5 {
		t.Fatalf("medicine-only search failed: %d %s %v", w.Code, w.Body.String(), err)
	}
	if ai.input["search_text"] != "Research summary" || len(ai.input["source_excerpts"].(map[string]string)) != 2 || len(ai.input["source_urls"].([]string)) != 2 {
		t.Fatal("raw search evidence was not explicitly included in structured extraction")
	}
	if !strings.Contains(ai.query, "metformin 500mg tablets") || len(ai.query) >= 260 || ai.searchCalls != 1 || ai.structuredCalls != 1 {
		t.Fatal("simple search did not use a concise medicine query or introduced extra product-selection steps")
	}
}

func TestMedicineSearchWithoutRawExcerptsReturnsLinksWithoutModelPrices(t *testing.T) {
	ai := &medicineSearchFixture{research: insurance.SearchEvidence{Text: "Metformin price $1.00", Sources: []string{"https://pharmacy.example/metformin"}}}
	result := searchMedicines(context.Background(), "metformin", ai)
	if result.Status != "links_only" || len(result.Sources) != 1 || len(result.Offers) != 0 || ai.structuredCalls != 0 {
		t.Fatalf("research summary produced unverified prices: %+v", result)
	}
}

func TestMedicineSearchRejectsUnprovenAmountsAndDifferentProducts(t *testing.T) {
	const source = "https://pharmacy.example/metformin"
	for _, test := range []struct {
		name, product, quote, excerpt, currency, url string
		price                                        float64
		wantOffer                                    bool
	}{
		{"summary-only-quote", "Metformin 500mg tablets", "Metformin 500mg tablets price $1.00", "Metformin 500mg tablets price $12.00", "USD", source, 1, false},
		{"wrong-strength", "Metformin 1000mg tablets", "Metformin 1000mg tablets price $12.00", "Metformin 1000mg tablets price $12.00", "USD", source, 12, false},
		{"different-medicine", "Glipizide 500mg tablets", "Glipizide 500mg tablets price $12.00", "Glipizide 500mg tablets price $12.00", "USD", source, 12, false},
		{"combination-product", "Metformin Glipizide 500mg tablets", "Metformin Glipizide 500mg tablets price $12.00", "Metformin Glipizide 500mg tablets price $12.00", "USD", source, 12, false},
		{"uncited-url", "Metformin 500mg tablets", "Metformin 500mg tablets price $12.00", "Metformin 500mg tablets price $12.00", "USD", "https://invented.example/price", 12, false},
		{"different-price", "Metformin 500mg tablets", "Metformin 500mg tablets price $10.00", "Metformin 500mg tablets price $10.00", "USD", source, 12, true},
		{"missing-currency", "Metformin 500mg tablets", "Metformin 500mg tablets price $12.00", "Metformin 500mg tablets price $12.00", "", source, 12, true},
		{"savings-not-price", "Metformin 500mg tablets", "Metformin 500mg tablets save $20.00", "Metformin 500mg tablets save $20.00", "USD", source, 20, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ai := &medicineSearchFixture{
				research: insurance.SearchEvidence{Text: test.quote, Sources: []string{source}, Excerpts: map[string]string{source: test.excerpt}},
				offers:   []MedicineSearchOffer{{Medicine: test.product, Price: medicineTestPrice(test.price), Currency: test.currency, SourceURL: test.url, Quote: test.quote}},
			}
			result := searchMedicines(context.Background(), "metformin 500mg tablets", ai)
			if result.Status != "links_only" || (len(result.Offers) == 1) != test.wantOffer {
				t.Fatalf("unsupported offer handling failed: %+v", result)
			}
			for _, offer := range result.Offers {
				if offer.Price != nil {
					t.Fatal("unverified medicine price survived validation")
				}
			}
		})
	}
}

func TestMedicineSearchHasUsefulEmptyAndUnavailableStates(t *testing.T) {
	for _, test := range []struct {
		ai   medicineSearchFixture
		want string
	}{
		{medicineSearchFixture{searchErr: errors.New("private upstream detail")}, "unavailable"},
		{medicineSearchFixture{research: insurance.SearchEvidence{Sources: []string{"http://localhost/private"}}}, "no_results"},
		{medicineSearchFixture{research: insurance.SearchEvidence{Sources: []string{"https://pharmacy.example/price"}, Excerpts: map[string]string{"https://pharmacy.example/price": "metformin"}}, structuredErr: errors.New("private extraction detail")}, "links_only"},
	} {
		result := searchMedicines(context.Background(), "metformin", &test.ai)
		raw, _ := json.Marshal(result)
		if result.Status != test.want || result.Offers == nil || result.Sources == nil || strings.Contains(string(raw), "private") {
			t.Fatalf("unhelpful or unsafe search failure: %s", raw)
		}
	}
}

func TestMedicineSearchInputAndAmountValidation(t *testing.T) {
	for _, body := range []string{`{}`, `{"medicine":"x"}`, `{"medicine":"` + strings.Repeat("x", 201) + `"}`, `{"medicine":"metformin","zip":"10001"}`, `{"medicine":true}`} {
		ai := &medicineSearchFixture{}
		w := httptest.NewRecorder()
		searchWithMedicineAI(w, httptest.NewRequest("POST", "/prescription-comparison/search", strings.NewReader(body)), ai)
		if w.Code != 422 || ai.searchCalls != 0 {
			t.Fatalf("invalid simple-search request reached AI: %q", body)
		}
	}
	if medicinePriceInQuote("metformin $9.999", 9.99) || medicinePriceInQuote("metformin 500mg", 500) || medicinePriceInQuote("metformin save $20", 20) || medicinePriceInQuote("metformin price $12, save $20", 20) || medicinePriceInQuote("metformin $20 shipping", 20) || medicinePriceInQuote("metformin $12", math.NaN()) {
		t.Fatal("price parsing accepted an unsupported amount")
	}
}

func TestMedicineSearchSeparatesRequestedPackCountFromProductStrength(t *testing.T) {
	const source = "https://pharmacy.example/metformin"
	for _, test := range []struct {
		quantity, quote string
		wantPrice       bool
	}{
		{"30 tablets", "Metformin 500 mg tablets, 30 tablets, price $8.00.", true},
		{"90 tablets", "Metformin 500 mg tablets, 90 tablets, price $8.00.", false},
		{"", "Metformin 500 mg tablets price $8.00.", false},
	} {
		ai := &medicineSearchFixture{
			research: insurance.SearchEvidence{Text: test.quote, Sources: []string{source}, Excerpts: map[string]string{source: test.quote}},
			offers:   []MedicineSearchOffer{{Medicine: "Metformin 500 mg tablets", Price: medicineTestPrice(8), Currency: "USD", Quantity: test.quantity, SourceURL: source, Quote: test.quote}},
		}
		const requested = "metformin500 mg,30tablets"
		result := searchMedicines(context.Background(), requested, ai)
		if result.Medicine != requested || !strings.Contains(ai.query, requested) || len(result.Offers) != 1 || (result.Offers[0].Price != nil) != test.wantPrice {
			t.Fatalf("package count confused product identity or price: quantity=%q result=%+v", test.quantity, result)
		}
		if ai.input["requested_product"] != "metformin500 mg,tablets" || ai.input["requested_quantity"] != "30 tablet" {
			t.Fatalf("product/quantity extraction was not explicit: %#v", ai.input)
		}
	}
	product, pack, ambiguous := medicineProductAndPack("metformin 500 mg tablets")
	if product != "metformin 500 mg tablets" || pack != nil || ambiguous {
		t.Fatal("dose strength was interpreted as a package count")
	}
}
