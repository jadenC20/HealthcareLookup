package insurance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCMSCatalogPreservesVariantsAndYear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plans.csv")
	os.WriteFile(path, []byte("BusinessYear,StateCode,IssuerMarketplaceMarketingName,PlanMarketingName,PlanId,URLForSummaryofBenefitsCoverage\n2026,PA,Example,Gold,12345PA1234567-01,https://insurer.example/one.pdf\n2026,PA,Example,Gold,12345PA1234567-02,https://insurer.example/two.pdf\n2025,PA,Example,Gold,12345PA1234567-01,https://insurer.example/old.pdf\n"), 0600)
	c := LoadCatalog(path)
	rows, err := c.Find(context.Background(), testDetails())
	if err != nil || len(rows) != 2 {
		t.Fatalf("%+v %v", rows, err)
	}
	matches := usable(rows, testDetails())
	n := 0
	for _, m := range matches {
		if m.Match == "exact_variant" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("exact variants %d", n)
	}
}
