package prescription

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Location says where the patient wants to pick up or receive a medicine.
// Request values win; otherwise the account layer fills it from the profile.
type Location struct {
	ZIP         string   `json:"zip"`
	City        string   `json:"city"`
	State       string   `json:"state"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	RadiusMiles float64  `json:"radius_miles"`
	Delivery    *bool    `json:"delivery"`
	Source      string   `json:"source,omitempty"` // request | profile | preferences
}

func (l Location) Empty() bool {
	return l.ZIP == "" && strings.TrimSpace(l.City) == "" && (l.Latitude == nil || l.Longitude == nil)
}

type NearbyPharmacy struct {
	Name          string  `json:"name"`
	Brand         string  `json:"brand"`
	Address       string  `json:"address"`
	Phone         string  `json:"phone"`
	OpeningHours  string  `json:"opening_hours"`
	DistanceMiles float64 `json:"distance_miles"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	MapURL        string  `json:"map_url"`
	SourceURL     string  `json:"source_url"`
}

type Suggestion struct {
	Kind         string               `json:"kind"` // pickup | delivery
	Title        string               `json:"title"`
	Detail       string               `json:"detail"`
	Offer        *MedicineSearchOffer `json:"offer,omitempty"`
	Pharmacy     *NearbyPharmacy      `json:"pharmacy,omitempty"`
	URL          string               `json:"url"`
	PriceKnown   bool                 `json:"price_known"`
	Confirmation string               `json:"confirmation"`
}

type Suggestions struct {
	Location         *ResolvedLocation `json:"location"`
	NearbyPharmacies []NearbyPharmacy  `json:"nearby_pharmacies"`
	Pickup           []Suggestion      `json:"pickup"`
	Delivery         []Suggestion      `json:"delivery"`
	Best             *Suggestion       `json:"best"`
	Notes            []string          `json:"notes"`
}

type ResolvedLocation struct {
	Label     string  `json:"label"`
	State     string  `json:"state"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Source    string  `json:"source"`
}

type geoFetcher func(ctx context.Context, url string, dest any) error

// Mail-order pharmacies that sell to patients across the US. Delivery to a
// specific state is never assumed verified; the patient confirms at checkout.
var mailOrder = []struct{ key, name, url string }{
	{"costplusdrugs.com", "Mark Cuban Cost Plus Drugs", "https://costplusdrugs.com/medications/"},
	{"pharmacy.amazon.com", "Amazon Pharmacy", "https://pharmacy.amazon.com/"},
}

var chainKeys = []string{"cvs", "walgreens", "walmart", "rite aid", "kroger", "costco", "publix", "h-e-b", "heb", "safeway", "albertsons", "meijer", "target", "sam's club", "giant", "wegmans", "harris teeter", "hy-vee", "shoprite", "stop & shop", "fred meyer", "king soopers", "ralphs", "vons", "jewel-osco", "winn-dixie", "food lion", "duane reade"}

var zipPattern = regexp.MustCompile(`^[0-9]{5}$`)
var statePattern = regexp.MustCompile(`^[A-Z]{2}$`)

func resolveLocation(ctx context.Context, loc Location, fetch geoFetcher) (*ResolvedLocation, error) {
	source := loc.Source
	if source == "" {
		source = "request"
	}
	if loc.Latitude != nil && loc.Longitude != nil && math.Abs(*loc.Latitude) <= 90 && math.Abs(*loc.Longitude) <= 180 {
		label := strings.TrimSpace(strings.Trim(loc.City+", "+loc.State, ", "))
		if label == "" {
			label = fmt.Sprintf("%.3f, %.3f", *loc.Latitude, *loc.Longitude)
		}
		return &ResolvedLocation{Label: label, State: strings.ToUpper(loc.State), Latitude: *loc.Latitude, Longitude: *loc.Longitude, Source: source}, nil
	}
	var place struct {
		Places []struct {
			Name  string `json:"place name"`
			State string `json:"state abbreviation"`
			Lat   string `json:"latitude"`
			Lon   string `json:"longitude"`
		} `json:"places"`
		PostCode string `json:"post code"`
	}
	if zipPattern.MatchString(loc.ZIP) {
		if err := fetch(ctx, "https://api.zippopotam.us/us/"+loc.ZIP, &place); err != nil {
			return nil, err
		}
	} else {
		city, state := splitCityState(loc.City, loc.State)
		if city == "" || !statePattern.MatchString(state) {
			return nil, fmt.Errorf("location needs a ZIP code or a city with a two-letter state")
		}
		if err := fetch(ctx, "https://api.zippopotam.us/us/"+strings.ToLower(state)+"/"+url.PathEscape(strings.ToLower(city)), &place); err != nil {
			return nil, err
		}
		for i := range place.Places {
			place.Places[i].State = state
		}
	}
	if len(place.Places) == 0 {
		return nil, fmt.Errorf("location not found")
	}
	p := place.Places[0]
	if city, _ := splitCityState(loc.City, loc.State); loc.ZIP == "" {
		for _, candidate := range place.Places {
			if strings.EqualFold(candidate.Name, city) {
				p = candidate
				break
			}
		}
	}
	var lat, lon float64
	if _, err := fmt.Sscan(p.Lat, &lat); err != nil {
		return nil, err
	}
	if _, err := fmt.Sscan(p.Lon, &lon); err != nil {
		return nil, err
	}
	label := p.Name + ", " + p.State
	if loc.ZIP != "" {
		label += " " + loc.ZIP
	}
	return &ResolvedLocation{Label: label, State: p.State, Latitude: lat, Longitude: lon, Source: source}, nil
}

// splitCityState accepts "Philadelphia, PA" in the city field.
func splitCityState(city, state string) (string, string) {
	city, state = strings.TrimSpace(city), strings.ToUpper(strings.TrimSpace(state))
	if i := strings.LastIndex(city, ","); i > 0 {
		tail := strings.ToUpper(strings.TrimSpace(city[i+1:]))
		if statePattern.MatchString(tail) {
			if state == "" {
				state = tail
			}
			city = strings.TrimSpace(city[:i])
		}
	}
	return city, state
}

func nearbyPharmacies(ctx context.Context, at ResolvedLocation, radiusMiles float64, fetch geoFetcher) ([]NearbyPharmacy, error) {
	meters := int(radiusMiles * 1609.34)
	query := fmt.Sprintf(`[out:json][timeout:20];nwr["amenity"="pharmacy"](around:%d,%f,%f);out center 60;`, meters, at.Latitude, at.Longitude)
	var response struct {
		Elements []struct {
			Type   string                      `json:"type"`
			ID     int64                       `json:"id"`
			Lat    float64                     `json:"lat"`
			Lon    float64                     `json:"lon"`
			Center *struct{ Lat, Lon float64 } `json:"center"`
			Tags   map[string]string           `json:"tags"`
		} `json:"elements"`
	}
	if err := fetch(ctx, "https://overpass-api.de/api/interpreter?data="+url.QueryEscape(query), &response); err != nil {
		return nil, err
	}
	out := []NearbyPharmacy{}
	for _, e := range response.Elements {
		lat, lon := e.Lat, e.Lon
		if e.Center != nil {
			lat, lon = e.Center.Lat, e.Center.Lon
		}
		name := firstNonEmpty(e.Tags["name"], e.Tags["brand"])
		if name == "" || lat == 0 {
			continue
		}
		street := strings.TrimSpace(e.Tags["addr:housenumber"] + " " + e.Tags["addr:street"])
		address := strings.Trim(strings.Join(nonEmpty(street, e.Tags["addr:city"], strings.TrimSpace(e.Tags["addr:state"]+" "+e.Tags["addr:postcode"])), ", "), ", ")
		out = append(out, NearbyPharmacy{
			Name: name, Brand: e.Tags["brand"], Address: address, Phone: firstNonEmpty(e.Tags["phone"], e.Tags["contact:phone"]),
			OpeningHours: e.Tags["opening_hours"], DistanceMiles: math.Round(haversineMiles(at.Latitude, at.Longitude, lat, lon)*10) / 10,
			Latitude: lat, Longitude: lon,
			MapURL:    fmt.Sprintf("https://www.openstreetmap.org/?mlat=%f&mlon=%f#map=18/%f/%f", lat, lon, lat, lon),
			SourceURL: fmt.Sprintf("https://www.openstreetmap.org/%s/%d", e.Type, e.ID),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DistanceMiles < out[j].DistanceMiles })
	if len(out) > 12 {
		out = out[:12]
	}
	return out, nil
}

// chainOf returns the normalized chain key mentioned in a name or URL.
func chainOf(values ...string) string {
	text := strings.ToLower(strings.Join(values, " "))
	for _, key := range chainKeys {
		if strings.Contains(text, key) {
			if key == "heb" {
				return "h-e-b"
			}
			return key
		}
	}
	return ""
}

func offerHost(offer MedicineSearchOffer) string {
	u, err := url.Parse(offer.SourceURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// buildSuggestions pairs verified offers with real nearby stores of the same
// chain and with mail-order pharmacies. It never invents prices: a suggestion
// without a verified offer is marked price_known=false.
func buildSuggestions(result MedicineSearchResult, at *ResolvedLocation, stores []NearbyPharmacy, wantDelivery bool) Suggestions {
	s := Suggestions{Location: at, NearbyPharmacies: stores, Pickup: []Suggestion{}, Delivery: []Suggestion{}, Notes: []string{}}
	if s.NearbyPharmacies == nil {
		s.NearbyPharmacies = []NearbyPharmacy{}
	}
	usedStores := map[string]bool{}
	for i := range result.Offers {
		offer := result.Offers[i]
		chain := chainOf(offer.Pharmacy, offerHost(offer))
		if chain == "" {
			continue
		}
		for j := range stores {
			store := stores[j]
			if chainOf(store.Name, store.Brand) != chain || usedStores[chain] {
				continue
			}
			usedStores[chain] = true
			title := store.Name
			detail := fmt.Sprintf("%.1f mi away", store.DistanceMiles)
			if store.Address != "" {
				detail += " · " + store.Address
			}
			s.Pickup = append(s.Pickup, Suggestion{Kind: "pickup", Title: title, Detail: detail, Offer: &offer, Pharmacy: &store, URL: offer.SourceURL, PriceKnown: offer.Price != nil, Confirmation: "Published price for this chain; call the store to confirm stock and the price at this location."})
			break
		}
	}
	// Nearest stores without a matched price still help: the patient can ask
	// for a cash/coupon quote.
	for j := range stores {
		if len(s.Pickup) >= 5 {
			break
		}
		store := stores[j]
		chain := chainOf(store.Name, store.Brand)
		if chain != "" && usedStores[chain] || chain == "" && usedStores[store.Name] {
			continue
		}
		if chain != "" {
			usedStores[chain] = true
		} else {
			usedStores[store.Name] = true
		}
		detail := fmt.Sprintf("%.1f mi away", store.DistanceMiles)
		if store.Address != "" {
			detail += " · " + store.Address
		}
		s.Pickup = append(s.Pickup, Suggestion{Kind: "pickup", Title: store.Name, Detail: detail, Pharmacy: &store, URL: store.MapURL, Confirmation: "No published price was found for this pharmacy; call to ask for its cash or coupon price."})
	}
	if wantDelivery {
		seen := map[string]bool{}
		for i := range result.Offers {
			offer := result.Offers[i]
			host := offerHost(offer)
			for _, m := range mailOrder {
				if host == m.key && !seen[m.key] {
					seen[m.key] = true
					s.Delivery = append(s.Delivery, Suggestion{Kind: "delivery", Title: m.name, Detail: deliveryDetail(at), Offer: &offer, URL: offer.SourceURL, PriceKnown: offer.Price != nil, Confirmation: "Mail-order price; shipping fees, prescription transfer and delivery to your address are confirmed at checkout."})
				}
			}
		}
		for _, m := range mailOrder {
			if !seen[m.key] {
				s.Delivery = append(s.Delivery, Suggestion{Kind: "delivery", Title: m.name, Detail: deliveryDetail(at), URL: m.url, Confirmation: "No price for this medicine was verified here; check the pharmacy's site for your medicine and whether it ships to you."})
			}
		}
	}
	// Best = cheapest verified price among pickup and delivery suggestions.
	for _, group := range [][]Suggestion{s.Pickup, s.Delivery} {
		for i := range group {
			c := group[i]
			if c.Offer == nil || c.Offer.Price == nil {
				continue
			}
			if s.Best == nil || *c.Offer.Price < *s.Best.Offer.Price {
				copy := c
				s.Best = &copy
			}
		}
	}
	sort.SliceStable(s.Pickup, func(i, j int) bool {
		a, b := s.Pickup[i], s.Pickup[j]
		if a.PriceKnown != b.PriceKnown {
			return a.PriceKnown
		}
		if a.PriceKnown && *a.Offer.Price != *b.Offer.Price {
			return *a.Offer.Price < *b.Offer.Price
		}
		return a.Pharmacy.DistanceMiles < b.Pharmacy.DistanceMiles
	})
	if at == nil {
		s.Notes = append(s.Notes, "Add your ZIP code or city to your profile to see nearby pharmacies.")
	} else if len(stores) == 0 {
		s.Notes = append(s.Notes, "No pharmacies were found near "+at.Label+" in the map data; try a larger radius.")
	}
	s.Notes = append(s.Notes, "Pharmacy locations come from OpenStreetMap and may be incomplete or outdated. Prices are published estimates, not quotes.")
	return s
}

func deliveryDetail(at *ResolvedLocation) string {
	if at != nil && at.State != "" {
		return "Ships by mail; confirm delivery to " + at.State + " at checkout"
	}
	return "Ships by mail; confirm delivery to your address at checkout"
}

func haversineMiles(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 3958.8
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nonEmpty(values ...string) []string {
	out := []string{}
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

// suggest resolves the location, finds nearby stores and builds suggestions.
// Failures degrade to notes; the price result is always returned.
func suggest(ctx context.Context, result MedicineSearchResult, loc Location, fetch geoFetcher) Suggestions {
	want := loc.Delivery == nil || *loc.Delivery
	radius := loc.RadiusMiles
	if radius < 1 || radius > 25 {
		radius = 5
	}
	if loc.Empty() {
		return buildSuggestions(result, nil, nil, want)
	}
	at, err := resolveLocation(ctx, loc, fetch)
	if err != nil {
		s := buildSuggestions(result, nil, nil, want)
		s.Notes[0] = "Your location could not be found (" + err.Error() + "). Check the ZIP code or city and state."
		return s
	}
	stores, err := nearbyPharmacies(ctx, *at, radius, fetch)
	s := buildSuggestions(result, at, stores, want)
	if err != nil {
		s.Notes = append([]string{"Nearby pharmacy lookup is temporarily unavailable."}, s.Notes...)
	}
	return s
}
