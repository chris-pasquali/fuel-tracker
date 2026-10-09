package domain

import (
	"fmt"
	"sort"
	"time"
)

var Weekdays = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

type WeekdayRow struct {
	Weekday       string   `json:"weekday"`
	WeekdayIndex  int      `json:"weekday_index"`
	Samples       int      `json:"samples"`
	AvgMedianCPL  *float64 `json:"avg_median_cpl"`
	MedianCPL     *float64 `json:"median_cpl"`
	WeeksCheapest int      `json:"weeks_cheapest"`
	CheapestShare float64  `json:"cheapest_share"`
}

type WeekdayReport struct {
	Days          []WeekdayRow `json:"days"`
	Best          *WeekdayRow  `json:"best"`
	Worst         *WeekdayRow  `json:"worst"`
	SaveCPL       *float64     `json:"save_cpl"`
	WeeksObserved int          `json:"weeks_observed"`
}

type CycleReport struct {
	Phase               string  `json:"phase"`
	Label               string  `json:"label"`
	TypicalLengthDays   int     `json:"typical_length_days"`
	DaysSinceTrough     *int    `json:"days_since_trough"`
	TroughCount         int     `json:"trough_count"`
	ExpectedCheapWindow *string `json:"expected_cheap_window"`
	LatestMedianCPL     float64 `json:"latest_median_cpl"`
	Change4dCPL         float64 `json:"change_4d_cpl"`
}

type PricedStation struct {
	Station
	PriceCPL    float64 `json:"price_cpl"`
	LastUpdated string  `json:"last_updated,omitempty"`
	DistanceKM  float64 `json:"distance_km,omitempty"`
}

func WeekdayAnalysis(series []DailyRow) WeekdayReport {
	buckets := make([][]float64, 7)
	byWeek := map[string][][2]float64{}
	for _, row := range series {
		day, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			continue
		}
		wd := int(day.Weekday()+6) % 7
		buckets[wd] = append(buckets[wd], row.MedianCPL)
		y, w := day.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", y, w)
		byWeek[key] = append(byWeek[key], [2]float64{float64(wd), row.MedianCPL})
	}
	wins := make([]int, 7)
	weeks := 0
	for _, pts := range byWeek {
		if len(pts) < 5 {
			continue
		}
		best := pts[0]
		for _, p := range pts[1:] {
			if p[1] < best[1] {
				best = p
			}
		}
		wins[int(best[0])]++
		weeks++
	}
	if weeks == 0 {
		weeks = 1
	}
	days := make([]WeekdayRow, 7)
	var best, worst *WeekdayRow
	for i, name := range Weekdays {
		row := WeekdayRow{Weekday: name, WeekdayIndex: i, WeeksCheapest: wins[i], CheapestShare: float64(wins[i]) / float64(weeks)}
		if s := Summarise(buckets[i]); s != nil {
			row.Samples = s.N
			m, avg := s.Median, s.Mean
			row.MedianCPL = &m
			row.AvgMedianCPL = &avg
			copyRow := row
			if best == nil || avg < *best.AvgMedianCPL {
				best = &copyRow
			}
			if worst == nil || avg > *worst.AvgMedianCPL {
				worst = &copyRow
			}
		}
		days[i] = row
	}
	rep := WeekdayReport{Days: days, Best: best, Worst: worst, WeeksObserved: weeks}
	if best != nil && worst != nil {
		save := round1(*worst.AvgMedianCPL - *best.AvgMedianCPL)
		rep.SaveCPL = &save
	}
	return rep
}

func DetectCycle(series []DailyRow) CycleReport {
	if len(series) < 10 {
		return CycleReport{Phase: "unknown", Label: "Not enough history yet", TypicalLengthDays: 7}
	}
	medians := make([]float64, len(series))
	dates := make([]time.Time, len(series))
	for i, row := range series {
		medians[i] = row.MedianCPL
		dates[i], _ = time.Parse("2006-01-02", row.Date)
	}
	var troughs []int
	for i := 1; i < len(medians)-1; i++ {
		if medians[i] <= medians[i-1] && medians[i] <= medians[i+1] {
			troughs = append(troughs, i)
		}
	}
	typical := 7
	if len(troughs) > 1 {
		sum := 0
		for i := 1; i < len(troughs); i++ {
			sum += troughs[i] - troughs[i-1]
		}
		typical = int(float64(sum)/float64(len(troughs)-1) + 0.5)
		if typical < 5 {
			typical = 5
		}
		if typical > 14 {
			typical = 14
		}
	}
	window := medians[len(medians)-4:]
	delta := window[len(window)-1] - window[0]
	step := window[len(window)-1] - window[len(window)-2]
	phase, label := "plateau", "The cycle is flat — shop around by suburb, not by day"
	if delta >= 8 || step >= 6 {
		phase, label = "restoration", "Prices are jumping — the weekly restoration is on"
	} else if delta <= -3 || step <= -1.2 {
		phase, label = "discounting", "Prices are easing — wait if you can, or fill at an independent"
	}
	rep := CycleReport{Phase: phase, Label: label, TypicalLengthDays: typical, TroughCount: len(troughs), LatestMedianCPL: medians[len(medians)-1], Change4dCPL: round1(delta)}
	if len(troughs) > 0 {
		since := len(medians) - 1 - troughs[len(troughs)-1]
		rep.DaysSinceTrough = &since
		counts := map[int]int{}
		bestDay, bestN := 0, 0
		for _, idx := range troughs {
			wd := int(dates[idx].Weekday()+6) % 7
			counts[wd]++
			if counts[wd] > bestN {
				bestDay, bestN = wd, counts[wd]
			}
		}
		name := Weekdays[bestDay]
		rep.ExpectedCheapWindow = &name
	}
	return rep
}

func SortByPrice(rows []PricedStation) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PriceCPL == rows[j].PriceCPL {
			return rows[i].DistanceKM < rows[j].DistanceKM
		}
		return rows[i].PriceCPL < rows[j].PriceCPL
	})
}
