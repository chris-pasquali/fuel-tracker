package domain

import (
	"testing"
	"time"
)

func TestParseAddress(t *testing.T) {
	sub, pc := ParseAddress("32 Princes Hwy, Arncliffe NSW 2205")
	if sub != "Arncliffe" || pc != "2205" {
		t.Fatalf("got %q %q", sub, pc)
	}
	sub, pc = ParseAddress("275 PACIFIC HWY Hornsby NSW 2077")
	if sub != "Hornsby" || pc != "2077" {
		t.Fatalf("no-comma got %q %q", sub, pc)
	}
}

func TestGreaterSydneyFilter(t *testing.T) {
	if !IsGreaterSydney("1 Church St, PARRAMATTA NSW 2150", nil, nil) {
		t.Fatal("parramatta should be in")
	}
	if IsGreaterSydney("307 Ocean Beach Road, UMINA BEACH NSW 2257", nil, nil) {
		t.Fatal("central coast should be out")
	}
}

func TestHaversineBondiCBD(t *testing.T) {
	km := HaversineKM(-33.8688, 151.2093, -33.8915, 151.2767)
	if km < 5.5 || km > 8.5 {
		t.Fatalf("unexpected distance %.2f", km)
	}
}

func TestWeekdaySeedCheapDay(t *testing.T) {
	end := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	series := SynthesiseSeries("E10", end, 90, Summary{Median: 235.9, Min: 218.9, Max: 256.9})
	rep := WeekdayAnalysis(series)
	if rep.Best == nil {
		t.Fatal("no best day")
	}
	switch rep.Best.Weekday {
	case "Tuesday", "Wednesday", "Thursday":
	default:
		t.Fatalf("unexpected cheap day %s", rep.Best.Weekday)
	}
	if rep.SaveCPL == nil || *rep.SaveCPL < 4 {
		t.Fatalf("save too small: %v", rep.SaveCPL)
	}
}
