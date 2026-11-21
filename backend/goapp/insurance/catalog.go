package insurance

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

// CSVCatalog accepts a CMS Plan Attributes PUF (extracted CSV) or the normalized
// template documented in DATA_COLLECTION.md. It is loaded once at startup.
type CSVCatalog struct {
	Rows []Candidate
	Err  error
}

func LoadCatalog(path string) *CSVCatalog {
	c := &CSVCatalog{}
	if path == "" {
		c.Err = stageError("catalog_not_configured", "No local CMS plan catalogue is configured; web search will be used.")
		return c
	}
	file, err := os.Open(path)
	if err != nil {
		c.Err = stageError("catalog_unavailable", "The configured plan catalogue could not be opened.")
		return c
	}
	defer file.Close()
	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		c.Err = stageError("catalog_invalid", "The plan catalogue header could not be read.")
		return c
	}
	columns := map[string]int{}
	for i, v := range header {
		columns[strings.TrimPrefix(v, "\ufeff")] = i
	}
	get := func(row []string, keys ...string) string {
		for _, key := range keys {
			if i, ok := columns[key]; ok && i < len(row) && row[i] != "" {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	for {
		row, e := reader.Read()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			c.Rows = nil
			c.Err = stageError("catalog_invalid", "The plan catalogue contains malformed CSV rows.")
			return c
		}
		year, _ := strconv.Atoi(get(row, "BusinessYear", "year"))
		planID := get(row, "PlanId", "StandardComponentId", "plan_id")
		candidate := Candidate{Details: Details{Insurer: get(row, "IssuerMarketplaceMarketingName", "IssuerName", "insurer"), PlanName: get(row, "PlanMarketingName", "plan_name"), PlanID: planID, State: get(row, "StateCode", "state"), Year: year, Network: get(row, "NetworkId", "network")}, URL: get(row, "URLForSummaryofBenefitsCoverage", "URLForSummaryOfBenefitsCoverage", "sbc_url"), Source: "cms_catalog", SourceURL: get(row, "source_url")}
		if candidate.SourceURL == "" {
			candidate.SourceURL = "https://www.cms.gov/marketplace/resources/data/public-use-files"
		}
		if candidate.URL == "" {
			continue
		}
		if SafeURL(candidate.URL) != nil || year == 0 || candidate.Details.State == "" {
			continue
		}
		c.Rows = append(c.Rows, candidate)
	}
	if len(c.Rows) == 0 {
		c.Err = stageError("catalog_empty", "The catalogue contains no usable SBC URLs with state and year.")
	}
	return c
}
func (c *CSVCatalog) Find(ctx context.Context, d Details) ([]Candidate, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	out := []Candidate{}
	for _, row := range c.Rows {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		p := row.Details
		if d.Year != 0 && p.Year != d.Year {
			continue
		}
		if d.State != "" && !strings.EqualFold(p.State, d.State) {
			continue
		}
		if d.PlanID != "" {
			if baseID(p.PlanID) != baseID(d.PlanID) {
				continue
			}
		} else {
			if !similar(p.Insurer, d.Insurer) || !similar(p.PlanName, d.PlanName) {
				continue
			}
		}
		out = append(out, row)
	}
	return out, nil
}
func similar(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}
