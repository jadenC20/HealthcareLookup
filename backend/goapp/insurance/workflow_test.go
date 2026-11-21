package insurance

import (
	"context"
	"errors"
	"testing"
)

type fakeAI struct {
	extraction        Extraction
	extractionErr     error
	candidates        []Candidate
	discoveryErr      error
	interpretation    Interpretation
	interpretationErr error
	searchCalls       int
}

func (a *fakeAI) Extract(context.Context, Document) (Extraction, error) {
	return a.extraction, a.extractionErr
}
func (a *fakeAI) Discover(context.Context, Details) ([]Candidate, error) {
	a.searchCalls++
	return a.candidates, a.discoveryErr
}
func (a *fakeAI) Interpret(context.Context, Document) (Interpretation, error) {
	return a.interpretation, a.interpretationErr
}

type fakeCatalog struct {
	candidates []Candidate
	err        error
}

func (c fakeCatalog) Find(context.Context, Details) ([]Candidate, error) { return c.candidates, c.err }

type fakeFetcher struct {
	calls int
	err   error
}

func (f *fakeFetcher) Fetch(context.Context, string) (Document, error) {
	f.calls++
	return NewDocument([]byte("%PDF-1.4\nfixture"), "application/pdf", "https://insurer.example/sbc.pdf"), f.err
}
func testDetails() Details {
	return Details{Insurer: "Example Insurance", PlanName: "Example Gold", PlanID: "12345PA1234567-01", State: "PA", Year: 2026}
}
func testCandidate() Candidate {
	return Candidate{Details: testDetails(), URL: "https://insurer.example/sbc.pdf", SourceURL: "https://insurer.example/plans", Source: "cms_catalog"}
}
func allBenefits() []Benefit {
	b := []Benefit{}
	for _, name := range RequiredBenefits {
		b = append(b, Benefit{Name: name, Value: "See the plan terms", Quote: "Synthetic supporting text", Page: 1})
	}
	return b
}
func hasCaution(r Result, code string) bool {
	for _, c := range r.Cautions {
		if c.Code == code {
			return true
		}
	}
	return false
}
func TestExactConfirmedVariantDownloadsAndInterprets(t *testing.T) {
	ai := &fakeAI{interpretation: Interpretation{IsSBC: true, Details: testDetails(), Benefits: allBenefits()}}
	fetch := &fakeFetcher{}
	engine := Engine{AI: ai, Catalog: fakeCatalog{candidates: []Candidate{testCandidate()}}, Fetcher: fetch}
	d := testDetails()
	r := engine.Run(context.Background(), Document{}, &d)
	if r.Status != "ready_with_caution" || len(r.Benefits) != len(RequiredBenefits) || fetch.calls != 1 || ai.searchCalls != 0 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if !hasCaution(r, "not_verified") || !r.MayBeInaccurate || r.Caution == "" || r.Document.SHA256 == "" {
		t.Fatal("missing verification caution or provenance")
	}
}
func TestUnconfirmedCardDoesNotSilentlySelect(t *testing.T) {
	ai := &fakeAI{extraction: Extraction{Details: testDetails()}}
	fetch := &fakeFetcher{}
	engine := Engine{AI: ai, Catalog: fakeCatalog{candidates: []Candidate{testCandidate()}}, Fetcher: fetch}
	r := engine.Run(context.Background(), Document{}, nil)
	if r.Status != "needs_confirmation" || fetch.calls != 0 || !hasCaution(r, "unconfirmed_details") {
		t.Fatalf("unexpected: %+v", r)
	}
}
func TestMissingAIKeepsActionableCaution(t *testing.T) {
	engine := Engine{AI: &fakeAI{extractionErr: stageError("openai_not_configured", "API key missing")}}
	r := engine.Run(context.Background(), Document{}, nil)
	if !r.MayBeInaccurate || !hasCaution(r, "openai_not_configured") || len(r.Cautions[0].Missing) == 0 {
		t.Fatalf("unexpected: %+v", r)
	}
}
func TestFallbackAndSourceFailureRemainVisible(t *testing.T) {
	c := testCandidate()
	c.Source = "web_search"
	ai := &fakeAI{candidates: []Candidate{c}}
	engine := Engine{AI: ai, Catalog: fakeCatalog{err: stageError("catalog_not_configured", "missing catalog")}, Fetcher: &fakeFetcher{}}
	d := testDetails()
	r := engine.Run(context.Background(), Document{}, &d)
	if !hasCaution(r, "catalog_not_configured") || r.Status != "needs_confirmation" || len(r.Candidates) != 1 {
		t.Fatalf("unexpected: %+v", r)
	}
}
func TestMismatchedYearWithholdsBenefits(t *testing.T) {
	d := testDetails()
	wrong := d
	wrong.Year = 2025
	engine := Engine{AI: &fakeAI{interpretation: Interpretation{IsSBC: true, Details: wrong, Benefits: allBenefits()}}}
	r := engine.Upload(context.Background(), NewResult(d, true), NewDocument([]byte("%PDF-fixture"), "application/pdf", "user_upload"))
	if len(r.Benefits) != 0 || !hasCaution(r, "document_mismatch") {
		t.Fatalf("unexpected: %+v", r)
	}
}
func TestMissingEvidenceDoesNotBecomeKnownBenefit(t *testing.T) {
	b := allBenefits()
	b[0].Quote = ""
	engine := Engine{AI: &fakeAI{interpretation: Interpretation{IsSBC: true, Details: testDetails(), Benefits: b}}}
	r := engine.Upload(context.Background(), NewResult(testDetails(), true), Document{})
	if len(r.Benefits) != len(RequiredBenefits)-1 || !hasCaution(r, "missing_benefits") {
		t.Fatalf("unexpected: %+v", r)
	}
}
func TestFailureAfterSelectionIsPartial(t *testing.T) {
	c := testCandidate()
	c.Identify()
	prev := NewResult(testDetails(), true)
	prev.Candidates = []Candidate{c}
	engine := Engine{Fetcher: &fakeFetcher{err: errors.New("timeout")}}
	r, err := engine.Select(context.Background(), prev, c.ID)
	if err != nil || !r.MayBeInaccurate || r.Status != "partial" {
		t.Fatalf("unexpected: %+v %v", r, err)
	}
	if _, err = engine.Select(context.Background(), prev, "invented"); err == nil {
		t.Fatal("arbitrary selection accepted")
	}
}
func TestWrongVariantNotAutoSelected(t *testing.T) {
	d := testDetails()
	c := testCandidate()
	c.Details.PlanID = "12345PA1234567-02"
	ai := &fakeAI{}
	engine := Engine{AI: ai, Catalog: fakeCatalog{candidates: []Candidate{c}}, Fetcher: &fakeFetcher{}}
	r := engine.Run(context.Background(), Document{}, &d)
	if r.Status == "ready_with_caution" {
		t.Fatal("wrong variant accepted")
	}
}
