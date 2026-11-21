package prescription

import "clearcare/evidence"

func drugGrid(drugs []Drug, source string) evidence.Grid {
	items := []evidence.Item{}
	for _, d := range drugs {
		items = append(items, evidence.NewItem("drug", d.ID, d.Name, "Select the exact prescribed product", []evidence.Field{evidence.Available("rxcui", "RxCUI", d.ID, "text"), evidence.Available("tty", "RxNorm type", d.Type, "text")}, evidence.URLSource(source), nil, map[string]any{"rxcui": d.ID, "name": d.Name, "tty": d.Type}))
	}
	return evidence.NewGrid(items)
}
func offerGrid(offers []Offer) evidence.Grid {
	items := []evidence.Item{}
	for _, o := range offers {
		items = append(items, evidence.NewItem("prescription_offer", o.SourceURL+"|"+o.Pharmacy, o.Pharmacy, o.DrugName, []evidence.Field{evidence.Available("price", "Price", o.Price, "usd"), evidence.Available("quantity", "Quantity", o.Quantity, "number"), evidence.Available("price_type", "Price type", o.PriceType, "text"), evidence.Optional("potential_savings", "Potential savings", o.Savings, "usd"), evidence.Optional("conditions", "Conditions", o.Conditions, "text"), evidence.Available("retrieved_at", "Retrieved", o.Retrieved, "datetime")}, evidence.URLSource(o.SourceURL), []any{map[string]any{"code": "quote_requires_confirmation", "message": "Confirm the final price, inventory, and eligibility with the pharmacy."}}, map[string]any{"drug_name": o.DrugName, "quantity": o.Quantity, "pharmacy": o.Pharmacy, "zip": o.ZIP, "price": o.Price, "price_type": o.PriceType, "currency": o.Currency, "conditions": o.Conditions, "source_url": o.SourceURL, "quote": o.Quote, "retrieved_at": o.Retrieved, "potential_savings": o.Savings}))
	}
	return evidence.NewGrid(items)
}
