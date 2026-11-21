package insurance

import (
	"bytes"
	"context"
	"github.com/jung-kurt/gofpdf"
	"os"
	"testing"
	"time"
)

// Explicit opt-in only: synthetic documents, public plan search, real API billing.
func TestLiveAIProvider(t *testing.T) {
	if os.Getenv("CLEARCARE_LIVE_TEST") != "1" {
		t.Skip("set CLEARCARE_LIVE_TEST=1 to use the configured AI provider with synthetic documents")
	}
	ai := NewOpenAI()
	if ai.Key == "" {
		t.Fatal("the selected AI provider's backend API key must be configured")
	}
	pdf := func(text string) Document {
		p := gofpdf.New("P", "mm", "A4", "")
		p.AddPage()
		p.SetFont("Arial", "", 12)
		p.MultiCell(0, 7, text, "", "", false)
		var b bytes.Buffer
		if err := p.Output(&b); err != nil {
			t.Fatal(err)
		}
		return NewDocument(b.Bytes(), "application/pdf", "synthetic_fixture")
	}
	t.Run("card", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		x, err := ai.Extract(ctx, pdf("SYNTHETIC TEST INSURANCE CARD\nInsurer: Example Insurance\nPlan: Example Gold\nHIOS Plan ID: 12345PA1234567-01\nCoverage year: 2026\nState: PA"))
		if err != nil {
			t.Fatal(err)
		}
		if x.Details.PlanID != "12345PA1234567-01" || x.Details.Year != 2026 {
			t.Fatalf("unexpected extracted identity: %+v", x)
		}
	})
	t.Run("sbc", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		x, err := ai.Interpret(ctx, pdf("SYNTHETIC TEST DOCUMENT\nSummary of Benefits and Coverage\nExample Insurance - Example Gold\nHIOS Plan ID: 12345PA1234567-01\nCoverage period: 01/01/2026 - 12/31/2026\nState: PA\nIndividual in-network deductible: $500.\nIndividual in-network out-of-pocket limit: $3,000.\nPrimary care: $20 copay, deductible does not apply.\nSpecialist: $40 copay, deductible does not apply.\nEmergency room: 20% coinsurance after deductible.\nHospital stay: 20% coinsurance after deductible.\nPrescription drugs: generic $10 copay, deductible does not apply.\nExcluded services: cosmetic surgery."))
		if err != nil {
			t.Fatal(err)
		}
		if !x.IsSBC || len(x.Benefits) == 0 {
			t.Fatal("SBC not interpreted")
		}
	})
	t.Run("public_search", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		candidates, err := ai.Discover(ctx, Details{Insurer: "Aetna CVS Health", PlanName: "Silver", State: "VA", Year: 2026})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Public search returned %d source-backed candidates; no member information was searched.", len(candidates))
	})
}
