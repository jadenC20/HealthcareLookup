package caresearch

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type row map[string]any
type Service struct {
	providers, prices []row
	errors            []string
}

func read(path string) ([]row, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	records, e := csv.NewReader(f).ReadAll()
	if e != nil {
		return nil, e
	}
	if len(records) == 0 {
		return []row{}, nil
	}
	out := []row{}
	for _, values := range records[1:] {
		r := row{}
		for i, k := range records[0] {
			if i < len(values) {
				r[k] = strings.TrimSpace(values[i])
			}
		}
		r["is_demo"] = strings.HasPrefix(strings.ToUpper(str(r["source_url"])), "DEMO")
		out = append(out, r)
	}
	return out, nil
}
func str(v any) string { s, _ := v.(string); return s }
func Load(dir string) *Service {
	s := &Service{providers: []row{}, prices: []row{}, errors: []string{}}
	var err error
	s.providers, err = read(filepath.Join(dir, "providers.csv"))
	if err != nil {
		s.providers = []row{}
		s.errors = append(s.errors, err.Error())
	}
	s.prices, err = read(filepath.Join(dir, "service_prices.csv"))
	if err != nil {
		s.prices = []row{}
		s.errors = append(s.errors, err.Error())
	}
	return s
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func (s *Service) Status(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, row{"providers_loaded": len(s.providers), "prices_loaded": len(s.prices), "errors": s.errors})
}
func (s *Service) Providers(w http.ResponseWriter, r *http.Request) {
	out := []row{}
	for _, p := range s.providers {
		q := r.URL.Query()
		if q.Get("city") != "" && !strings.EqualFold(str(p["city"]), q.Get("city")) {
			continue
		}
		if !strings.Contains(strings.ToLower(str(p["specialty"])), strings.ToLower(q.Get("specialty"))) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return str(out[i]["name"]) < str(out[j]["name"]) })
	jsonOut(w, row{"results": out})
}
func (s *Service) Prices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("procedure_code") == "" {
		http.Error(w, `{"detail":"procedure_code is required"}`, 422)
		return
	}
	out := []row{}
	types := map[string]bool{}
	includes := map[string]bool{}
	missing, demo := false, false
	for _, price := range s.prices {
		if str(price["procedure_code"]) != q.Get("procedure_code") || q.Get("price_type") != "" && str(price["price_type"]) != q.Get("price_type") {
			continue
		}
		for _, p := range s.providers {
			if str(p["provider_id"]) != str(price["provider_id"]) || q.Get("city") != "" && !strings.EqualFold(str(p["city"]), q.Get("city")) {
				continue
			}
			n, e := strconv.ParseFloat(strings.ReplaceAll(strings.TrimPrefix(str(price["price"]), "$"), ",", ""), 64)
			if e != nil {
				continue
			}
			v := row{}
			for k, x := range p {
				v[k] = x
			}
			for k, x := range price {
				v[k] = x
			}
			v["price"] = n
			out = append(out, v)
			types[str(price["price_type"])] = true
			inc := str(price["includes"])
			if inc == "" {
				missing = true
			} else {
				includes[inc] = true
			}
			if price["is_demo"] == true {
				demo = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := str(out[i]["price_type"]), str(out[j]["price_type"])
		if a != b {
			return a < b
		}
		return out[i]["price"].(float64) < out[j]["price"].(float64)
	})
	warnings := []string{}
	if len(types) > 1 {
		warnings = append(warnings, "Results mix different price types. Only compare prices of the same type.")
	}
	if missing {
		warnings = append(warnings, "Some results do not say what the price includes.")
	}
	if len(includes) > 1 {
		warnings = append(warnings, "Prices include different things. Check before comparing.")
	}
	if demo {
		warnings = append(warnings, "Some results are demonstration data, not real prices.")
	}
	jsonOut(w, row{"results": out, "warnings": warnings})
}
