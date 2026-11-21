package billreview

import "clearcare/evidence"

func reviewGrid(result ReviewResult) evidence.Grid {
	items := []evidence.Item{}
	for _, c := range result.Comparisons {
		data := map[string]any{"line_id": c.LineID, "code": c.Code, "charge": c.Charge, "benchmark_total": c.BenchmarkTotal, "over_under_benchmark": c.Difference, "reference": c.Reference}
		cautions := []any{}
		if c.Reference.Origin == "public_search" {
			cautions = append(cautions, map[string]any{"code": "public_reference_fallback", "message": "This public rate was extracted by AI and requires source verification."})
		}
		items = append(items, evidence.NewItem("bill_comparison", c.LineID, "Procedure "+c.Code, "Line "+c.LineID, []evidence.Field{evidence.Available("charge", "Charge", c.Charge, "usd"), evidence.Available("benchmark_total", "Benchmark", c.BenchmarkTotal, "usd"), evidence.Available("over_under_benchmark", "Difference", c.Difference, "usd")}, evidence.URLSource(c.Reference.Source), cautions, data))
	}
	if findings, ok := result.Analysis["findings"].([]Finding); ok {
		for _, f := range findings {
			items = append(items, evidence.NewItem("bill_finding", f.Rule+":"+firstID(f.IDs), f.Title, f.Message, []evidence.Field{evidence.Available("line_ids", "Bill lines", f.IDs, "list"), evidence.Optional("question", "Question to ask", f.Question, "text")}, nil, nil, map[string]any{"rule": f.Rule, "line_ids": f.IDs, "evidence": f.Evidence, "question": f.Question}))
		}
	}
	return evidence.NewGrid(items)
}
func firstID(ids []string) string {
	if len(ids) == 0 {
		return "general"
	}
	return ids[0]
}

func extractionGrid(documents []ExtractedDocument) evidence.Grid {
	items := []evidence.Item{}
	for _, document := range documents {
		for _, line := range document.Lines {
			items = append(items, evidence.NewItem(document.Kind+"_line", document.Kind+":"+line.ID, line.Description, document.Provider, []evidence.Field{
				evidence.Optional("service_date", "Service date", nilIfBlank(line.Date), "date"),
				evidence.Optional("code", "Code", nilIfBlank(line.Code), "text"),
				evidence.Optional("quantity", "Quantity", line.Quantity, "number"),
				evidence.Optional("charge", "Charge", line.Charge, "usd"),
				evidence.Optional("billed", "Billed", line.Billed, "usd"),
				evidence.Optional("plan_paid", "Plan paid", line.Paid, "usd"),
				evidence.Optional("adjustment", "Adjustment", line.Adjustment, "usd"),
				evidence.Optional("patient_responsibility", "Patient responsibility", line.Patient, "usd"),
				evidence.Available("page", "Page", line.Page, "number"),
			}, nil, []any{map[string]any{"code": "review_required", "message": "Confirm this AI-extracted line against the uploaded document."}}, map[string]any{"document_kind": document.Kind, "provider_name": document.Provider, "claim_number": document.Claim, "line": line}))
		}
	}
	return evidence.NewGrid(items)
}
func nilIfBlank(value string) any {
	if value == "" {
		return nil
	}
	return value
}
