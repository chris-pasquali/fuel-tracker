package domain

import (
	"math"
	"sort"
)

type Summary struct {
	N      int     `json:"n"`
	Min    float64 `json:"min"`
	P25    float64 `json:"p25"`
	Median float64 `json:"median"`
	Mean   float64 `json:"mean"`
	P75    float64 `json:"p75"`
	Max    float64 `json:"max"`
}

type DailyRow struct {
	Date      string  `json:"date"`
	FuelType  string  `json:"fuel_type"`
	N         int     `json:"n"`
	MinCPL    float64 `json:"min_cpl"`
	P25CPL    float64 `json:"p25_cpl"`
	MedianCPL float64 `json:"median_cpl"`
	MeanCPL   float64 `json:"mean_cpl"`
	P75CPL    float64 `json:"p75_cpl"`
	MaxCPL    float64 `json:"max_cpl"`
	Source    string  `json:"source"`
	AreaType  string  `json:"area_type,omitempty"`
	AreaName  string  `json:"area_name,omitempty"`
}

func Summarise(values []float64) *Summary {
	clean := make([]float64, 0, len(values))
	for _, v := range values {
		if v > 0 {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Float64s(clean)
	sum := 0.0
	for _, v := range clean {
		sum += v
	}
	return &Summary{
		N: len(clean), Min: round1(clean[0]), P25: round1(percentile(clean, 0.25)),
		Median: round1(percentile(clean, 0.50)), Mean: round1(sum / float64(len(clean))),
		P75: round1(percentile(clean, 0.75)), Max: round1(clean[len(clean)-1]),
	}
}

func DailyFrom(date, fuel, source string, values []float64) *DailyRow {
	s := Summarise(values)
	if s == nil {
		return nil
	}
	return &DailyRow{
		Date: date, FuelType: fuel, N: s.N, MinCPL: s.Min, P25CPL: s.P25,
		MedianCPL: s.Median, MeanCPL: s.Mean, P75CPL: s.P75, MaxCPL: s.Max, Source: source,
	}
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := float64(len(sorted)-1) * q
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
