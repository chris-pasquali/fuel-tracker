package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	addressRE    = regexp.MustCompile(`(?i),\s*([A-Za-z0-9 '#./-]+?)\s+NSW\s+(\d{4})\s*$`)
	postcodeTail = regexp.MustCompile(`(?i)NSW\s+(\d{4})\s*$`)
	roadTokens   = map[string]struct{}{"rd": {}, "road": {}, "hwy": {}, "highway": {}, "st": {}, "street": {}, "ave": {}, "av": {}, "parade": {}, "pde": {}, "bvd": {}, "blvd": {}, "dr": {}, "drive": {}, "ter": {}, "terrace": {}, "ct": {}, "close": {}, "pl": {}, "place": {}, "cres": {}, "crescent": {}, "ln": {}, "lane": {}, "way": {}, "&": {}, "and": {}, "cnr": {}, "corner": {}}
	sydneyRanges = [][2]int{{2000, 2249}, {2555, 2574}, {2745, 2777}}
)

type Station struct {
	Code      string   `json:"code"`
	StationID string   `json:"station_id,omitempty"`
	Brand     string   `json:"brand"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	Suburb    string   `json:"suburb"`
	Postcode  string   `json:"postcode"`
	Region    string   `json:"region"`
	Lat       *float64 `json:"lat"`
	Lon       *float64 `json:"lon"`
	IsAdBlue  bool     `json:"is_adblue"`
}

type Observation struct {
	StationCode string  `json:"station_code"`
	FuelType    string  `json:"fuel_type"`
	PriceCPL    float64 `json:"price_cpl"`
	LastUpdated string  `json:"last_updated,omitempty"`
}

func ParseAddress(address string) (suburb, postcode string) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", ""
	}
	if m := addressRE.FindStringSubmatch(address); len(m) == 3 {
		return titleCase(collapseSpace(m[1])), m[2]
	}
	loc := postcodeTail.FindStringSubmatchIndex(address)
	if loc == nil {
		return "", ""
	}
	pc := address[loc[2]:loc[3]]
	head := strings.TrimRight(address[:loc[0]], " ,")
	parts := strings.FieldsFunc(head, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
	var suburbParts []string
	for i := len(parts) - 1; i >= 0; i-- {
		tok := parts[i]
		cleaned := strings.TrimSuffix(strings.ToLower(tok), ".")
		if _, road := roadTokens[cleaned]; road || isMostlyNumeric(tok) {
			break
		}
		suburbParts = append([]string{tok}, suburbParts...)
		if len(suburbParts) >= 3 {
			break
		}
	}
	if len(suburbParts) == 0 {
		return "", pc
	}
	return titleCase(strings.Join(suburbParts, " ")), pc
}

func IsSydneyPostcode(pc string) bool {
	n, err := strconv.Atoi(pc)
	if err != nil {
		return false
	}
	for _, r := range sydneyRanges {
		if n >= r[0] && n <= r[1] {
			return true
		}
	}
	return false
}

func InSydneyBBox(lat, lon float64) bool {
	return lat >= -34.15 && lat <= -33.58 && lon >= 150.60 && lon <= 151.34
}

func IsGreaterSydney(address string, lat, lon *float64) bool {
	_, pc := ParseAddress(address)
	if IsSydneyPostcode(pc) {
		return true
	}
	if pc != "" {
		return false
	}
	if lat == nil || lon == nil {
		return false
	}
	return InSydneyBBox(*lat, *lon)
}

func RegionFor(postcode string) string {
	n, err := strconv.Atoi(postcode)
	if err != nil {
		return "Greater Sydney"
	}
	switch {
	case n == 2000 || n == 2001 || (n >= 2006 && n <= 2011):
		return "Sydney CBD"
	case n >= 2015 && n <= 2036:
		return "Eastern Suburbs"
	case (n >= 2037 && n <= 2050) || (n >= 2130 && n <= 2138):
		return "Inner West"
	case n >= 2060 && n <= 2076:
		return "Lower North Shore"
	case n >= 2077 && n <= 2087:
		return "Upper North Shore"
	case n >= 2088 && n <= 2108:
		return "Northern Beaches"
	case n >= 2109 && n <= 2126:
		return "Ryde / Hunter's Hill"
	case n >= 2127 && n <= 2159:
		return "Hills / Parramatta"
	case n >= 2160 && n <= 2180:
		return "Fairfield / Liverpool"
	case n >= 2190 && n <= 2200:
		return "Canterbury-Bankstown"
	case n >= 2203 && n <= 2214:
		return "Inner South / St George"
	case n >= 2216 && n <= 2234:
		return "Sutherland Shire"
	case n >= 2555 && n <= 2574:
		return "Macarthur"
	case n >= 2745 && n <= 2761:
		return "Penrith / Nepean"
	case n >= 2762 && n <= 2770:
		return "Blacktown / North West"
	case n >= 2773 && n <= 2777:
		return "Blue Mountains fringe"
	default:
		return "Greater Sydney"
	}
}

func HaversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	dlat := (lat2 - lat1) * math.Pi / 180
	dlon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

func titleCase(s string) string {
	return strings.Title(strings.ToLower(strings.TrimSpace(s)))
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func isMostlyNumeric(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
