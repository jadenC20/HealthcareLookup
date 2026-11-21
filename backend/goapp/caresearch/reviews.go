package caresearch

import (
	"clearcare/insurance"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func loadReviews(dir string) []ReviewSummary {
	out := []ReviewSummary{}
	data, err := os.ReadFile(filepath.Join(dir, "patient_review_cohorts.json"))
	if err != nil || len(data) > 4<<20 {
		return out
	}
	var rows []ReviewSummary
	if json.Unmarshal(data, &rows) != nil {
		return out
	}
	for _, v := range rows {
		date, err := time.Parse("2006-01-02", v.Date)
		if err != nil || date.After(time.Now()) || time.Since(date) > 365*24*time.Hour || !v.SelfReported || v.Count < 10 || v.Rating < 1 || v.Rating > 5 || len(v.NPI) != 10 || insurance.SafeURL(v.Source) != nil {
			continue
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out
}
