package domain

import (
	"math"
	"time"
)

const (
	CycleLength        = 7
	RestorationWeekday = time.Friday
)

func CycleFactor(day time.Time) float64 {
	phase := (int(day.Weekday()) - int(RestorationWeekday) + 7) % CycleLength
	if phase == 0 {
		return 1
	}
	if phase == 1 {
		return 0.86
	}
	v := 1 - float64(phase)/float64(CycleLength-1)
	if v < 0 {
		return 0
	}
	return v
}

func wobble(day time.Time, fuel string) float64 {
	seed := day.YearDay()*17 + sumRunes(fuel)
	return math.Sin(float64(seed)/37)*0.08 + math.Cos(float64(seed)/19)*0.04
}

func sumRunes(s string) int {
	n := 0
	for _, r := range s {
		n += int(r)
	}
	return n
}

func SynthesiseSeries(fuel string, end time.Time, days int, today Summary) []DailyRow {
	info := OrDefault(fuel)
	amplitude := info.TypicalCycleCPL
	todayFactor := CycleFactor(end)
	trough := today.Median - amplitude*todayFactor
	spread := today.Max - today.Min
	if spread < 8 {
		spread = 8
	}
	minGap := today.Median - today.Min
	maxGap := today.Max - today.Median
	rows := make([]DailyRow, 0, days)
	for offset := days; offset >= 1; offset-- {
		day := end.AddDate(0, 0, -offset)
		factor := CycleFactor(day) + wobble(day, fuel)
		if factor < 0 {
			factor = 0
		}
		if factor > 1 {
			factor = 1
		}
		median := trough + amplitude*factor
		values := []float64{
			median - minGap, median - spread*0.18, median - spread*0.18,
			median, median, median, median + (spread * 0.02),
			median + spread*0.16, median + spread*0.16, median + maxGap,
		}
		if row := DailyFrom(day.Format("2006-01-02"), fuel, "seeded_cycle", values); row != nil {
			rows = append(rows, *row)
		}
	}
	return rows
}

func ShiftRow(row DailyRow, offset float64, areaType, areaName string) DailyRow {
	row.MinCPL = round1(row.MinCPL + offset)
	row.P25CPL = round1(row.P25CPL + offset)
	row.MedianCPL = round1(row.MedianCPL + offset)
	row.MeanCPL = round1(row.MeanCPL + offset)
	row.P75CPL = round1(row.P75CPL + offset)
	row.MaxCPL = round1(row.MaxCPL + offset)
	row.AreaType = areaType
	row.AreaName = areaName
	row.Source = "seeded_cycle"
	return row
}
