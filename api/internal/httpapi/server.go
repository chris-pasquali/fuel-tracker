package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fillwise/internal/cache"
	"fillwise/internal/collector"
	"fillwise/internal/config"
	"fillwise/internal/domain"
	"fillwise/internal/store"
)

type Server struct {
	cfg       config.Config
	store     *store.Store
	cache     *cache.Cache
	collector *collector.Collector
}

func New(cfg config.Config, st *store.Store, c *cache.Cache, col *collector.Collector) *Server {
	return &Server{cfg: cfg, store: st, cache: c, collector: col}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/fuels", s.fuels)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/stations", s.stations)
	mux.HandleFunc("GET /api/trends", s.trends)
	mux.HandleFunc("GET /api/brands", s.brands)
	mux.HandleFunc("GET /api/regions", s.regions)
	mux.HandleFunc("GET /api/places", s.places)
	mux.HandleFunc("GET /api/around", s.around)
	mux.HandleFunc("POST /api/collect", s.collect)
	return withCORS(s.cfg.CORSOrigin, mux)
}

func withCORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	stations, obs, stationDays, cityDays, areaDays, last, err := s.store.Counts(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"stations": stations, "observations": obs, "station_daily_rows": stationDays,
		"city_history_rows": cityDays, "area_history_rows": areaDays, "last_snapshot": last,
		"redis": s.cache.Ping(r.Context()) == nil, "default_fuel": "E10",
		"server_time": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) fuels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, domain.Catalogue())
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	fuel := domain.OrDefault(r.URL.Query().Get("fuel"))
	key := "overview:" + fuel.Code
	var cached any
	if s.cache.GetJSON(r.Context(), key, &cached) {
		writeJSON(w, 200, cached)
		return
	}
	observed, latest, err := s.store.LatestBatch(r.Context(), fuel.Code)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	series, err := s.store.CitySeries(r.Context(), fuel.Code, 90)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	sum := domain.Summarise(prices(latest))
	weekday := domain.WeekdayAnalysis(series)
	cycle := domain.DetectCycle(series)
	liveDays, _ := s.store.LiveHistoryDays(r.Context(), fuel.Code)
	stations, _, _, _, _, _, _ := s.store.Counts(r.Context())
	var rec any
	if weekday.Best != nil && weekday.Worst != nil && weekday.SaveCPL != nil {
		conf := "model"
		if liveDays >= 21 {
			conf = "high"
		}
		rec = map[string]any{
			"best_weekday": weekday.Best.Weekday, "save_cpl": *weekday.SaveCPL, "confidence": conf,
			"rationale": fmt.Sprintf("%s has been the cheapest day on average (%.1f ¢/L city median). Filling then instead of %s has saved about %.1f ¢/L.",
				weekday.Best.Weekday, *weekday.Best.AvgMedianCPL, weekday.Worst.Weekday, *weekday.SaveCPL),
		}
	}
	hist := "seeded_cycle"
	if liveDays >= 21 {
		hist = "live"
	}
	body := map[string]any{
		"fuel": map[string]string{"code": fuel.Code, "name": fuel.Name, "unit": fuel.Unit},
		"observed_at": observed, "station_count": stations, "sample_count": len(latest),
		"summary": sum, "cheapest": take(latest, 8), "dearest": lastN(latest, 5),
		"recommendation": rec, "cycle": cycle, "weekday": weekday,
		"history_source": hist, "live_history_days": liveDays,
	}
	s.cache.SetJSON(r.Context(), key, body, 90*time.Second)
	writeJSON(w, 200, body)
}

func (s *Server) stations(w http.ResponseWriter, r *http.Request) {
	fuel := domain.OrDefault(r.URL.Query().Get("fuel"))
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	region := r.URL.Query().Get("region")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 400 {
		limit = 80
	}
	observed, latest, err := s.store.LatestBatch(r.Context(), fuel.Code)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	filtered := filterStations(latest, q, region)
	count := len(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	writeJSON(w, 200, map[string]any{"fuel": fuel.Code, "observed_at": observed, "count": count, "stations": filtered})
}

func (s *Server) trends(w http.ResponseWriter, r *http.Request) {
	fuel := domain.OrDefault(r.URL.Query().Get("fuel"))
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 14 || days > 180 {
		days = 90
	}
	series, err := s.store.CitySeries(r.Context(), fuel.Code, days)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var weekDelta any
	if len(series) >= 14 {
		d := avgMedian(series[len(series)-7:]) - avgMedian(series[len(series)-14:len(series)-7])
		weekDelta = float64(int(d*10+0.5)) / 10
	}
	writeJSON(w, 200, map[string]any{"fuel": fuel.Code, "days": days, "week_delta_cpl": weekDelta, "series": series})
}

func (s *Server) brands(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, groupLatest(r.Context(), s, r.URL.Query().Get("fuel"), "brand", func(p domain.PricedStation) string { return p.Brand }))
}

func (s *Server) regions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, groupLatest(r.Context(), s, r.URL.Query().Get("fuel"), "region", func(p domain.PricedStation) string { return p.Region }))
}

func (s *Server) places(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 40
	}
	items, err := s.store.Places(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) around(w http.ResponseWriter, r *http.Request) {
	fuel := domain.OrDefault(r.URL.Query().Get("fuel"))
	q := r.URL.Query().Get("q")
	radius, _ := strconv.ParseFloat(r.URL.Query().Get("radius_km"), 64)
	if radius < 2 || radius > 30 {
		radius = 10
	}
	var lat, lon *float64
	if v := r.URL.Query().Get("lat"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			lat = &n
		}
	}
	if v := r.URL.Query().Get("lon"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			lon = &n
		}
	}
	_, latest, err := s.store.LatestBatch(r.Context(), fuel.Code)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	origin := resolveOrigin(latest, q, lat, lon)
	if origin == nil {
		writeJSON(w, 200, map[string]any{"fuel": fuel.Code, "origin": nil, "radius_km": radius, "message": "Choose a suburb, postcode, or your location to rank nearby prices."})
		return
	}
	var nearby []domain.PricedStation
	for _, st := range latest {
		if st.Lat == nil || st.Lon == nil {
			continue
		}
		km := domain.HaversineKM(origin.Lat, origin.Lon, *st.Lat, *st.Lon)
		if km <= radius {
			st.DistanceKM = float64(int(km*100+0.5)) / 100
			nearby = append(nearby, st)
		}
	}
	domain.SortByPrice(nearby)
	suburbs := rankSuburbs(nearby)
	city := domain.Summarise(prices(latest))
	local := domain.Summarise(prices(nearby))
	var vsCity any
	if city != nil && local != nil {
		vsCity = float64(int((local.Median-city.Median)*10+0.5)) / 10
	}
	citySeries, _ := s.store.CitySeries(r.Context(), fuel.Code, 90)
	suburbSeries, _ := s.store.AreaSeries(r.Context(), fuel.Code, "suburb", origin.Suburb, 90)
	regionSeries, _ := s.store.AreaSeries(r.Context(), fuel.Code, "region", origin.Region, 90)
	localSeries, scope := citySeries, "city"
	if len(suburbSeries) >= 21 {
		localSeries, scope = suburbSeries, "suburb"
	} else if len(regionSeries) >= 21 {
		localSeries, scope = regionSeries, "region"
	}
	localWeek := domain.WeekdayAnalysis(localSeries)
	cityWeek := domain.WeekdayAnalysis(citySeries)
	if len(nearby) > 20 {
		nearby = nearby[:20]
	}
	if len(suburbs) > 12 {
		suburbs = suburbs[:12]
	}
	writeJSON(w, 200, map[string]any{
		"fuel": fuel.Code, "origin": origin, "radius_km": radius, "summary": local, "city_summary": city,
		"vs_city_cpl": vsCity, "nearby_suburbs": suburbs, "stations": nearby, "station_count": len(nearby),
		"weekday": localWeek, "city_weekday": cityWeek, "trend": localSeries, "series_scope": scope,
		"insight": aroundInsight(origin, radius, suburbs, vsCity, localWeek, cityWeek, fuel.Name),
	})
}

func (s *Server) collect(w http.ResponseWriter, r *http.Request) {
	if !s.cache.Lock(r.Context(), "collect:lock", 2*time.Minute) {
		writeJSON(w, 429, map[string]string{"error": "a collection is already running"})
		return
	}
	stations, prices, err := s.collector.CollectLive(r.Context())
	s.cache.DelPrefix(r.Context(), "overview:")
	s.cache.DelPrefix(r.Context(), "around:")
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"station_count": stations, "price_count": prices, "source": "fuelcheck"})
}

type origin struct {
	Label, Kind, Suburb, Postcode, Region string
	Lat, Lon                              float64
}

func resolveOrigin(stations []domain.PricedStation, q string, lat, lon *float64) *origin {
	if lat != nil && lon != nil {
		o := &origin{Label: "Your location", Kind: "coordinates", Lat: *lat, Lon: *lon}
		bestKM := 1e9
		for _, st := range stations {
			if st.Lat == nil || st.Lon == nil {
				continue
			}
			km := domain.HaversineKM(*lat, *lon, *st.Lat, *st.Lon)
			if km < bestKM {
				bestKM = km
				o.Suburb, o.Postcode, o.Region = st.Suburb, st.Postcode, st.Region
			}
		}
		return o
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var matches []domain.PricedStation
	kind := "suburb"
	if len(q) == 4 && isDigits(q) {
		kind = "postcode"
		for _, st := range stations {
			if st.Postcode == q && st.Lat != nil {
				matches = append(matches, st)
			}
		}
	} else {
		lower := strings.ToLower(q)
		var exact, prefix []domain.PricedStation
		for _, st := range stations {
			sub := strings.ToLower(st.Suburb)
			if st.Lat == nil {
				continue
			}
			if sub == lower {
				exact = append(exact, st)
			} else if strings.HasPrefix(sub, lower) {
				prefix = append(prefix, st)
			}
		}
		if len(exact) > 0 {
			matches = exact
		} else {
			matches = prefix
		}
	}
	if len(matches) == 0 {
		return nil
	}
	var slat, slon float64
	for _, st := range matches {
		slat += *st.Lat
		slon += *st.Lon
	}
	n := float64(len(matches))
	return &origin{Label: matches[0].Suburb, Kind: kind, Lat: slat / n, Lon: slon / n, Suburb: matches[0].Suburb, Postcode: matches[0].Postcode, Region: matches[0].Region}
}

func rankSuburbs(nearby []domain.PricedStation) []map[string]any {
	grouped := map[string][]domain.PricedStation{}
	for _, st := range nearby {
		grouped[st.Suburb] = append(grouped[st.Suburb], st)
	}
	var out []map[string]any
	for suburb, rows := range grouped {
		sum := domain.Summarise(prices(rows))
		if sum == nil {
			continue
		}
		nearest := rows[0]
		for _, r := range rows {
			if r.DistanceKM < nearest.DistanceKM {
				nearest = r
			}
		}
		out = append(out, map[string]any{
			"suburb": suburb, "region": rows[0].Region, "postcode": rows[0].Postcode,
			"distance_km": nearest.DistanceKM, "cheapest_station": nearest.Name,
			"n": sum.N, "min": sum.Min, "median": sum.Median, "mean": sum.Mean, "max": sum.Max,
		})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j]["min"].(float64) < out[i]["min"].(float64) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func aroundInsight(o *origin, radius float64, suburbs []map[string]any, vsCity any, local, city domain.WeekdayReport, fuel string) string {
	if len(suburbs) == 0 {
		return fmt.Sprintf("No %s prices found within %.0f km of %s.", fuel, radius, o.Label)
	}
	cheapest := suburbs[0]
	vs := ""
	if v, ok := vsCity.(float64); ok {
		if v < -0.4 {
			vs = fmt.Sprintf(" This pocket sits %.1f ¢/L under the Sydney median.", -v)
		} else if v > 0.4 {
			vs = fmt.Sprintf(" Prices here run %.1f ¢/L above the Sydney median — worth a short drive.", v)
		} else {
			vs = " Local medians are tracking the city."
		}
	}
	dayNote := ""
	if local.Best != nil && local.WeeksObserved >= 8 {
		if city.Best != nil && local.Best.Weekday != city.Best.Weekday {
			dayNote = fmt.Sprintf(" Around here the cheap day has been %s, not the city-wide %s.", local.Best.Weekday, city.Best.Weekday)
		} else if local.SaveCPL != nil {
			dayNote = fmt.Sprintf(" The cheap fill day is still %s, about %.1f ¢/L under the worst day.", local.Best.Weekday, *local.SaveCPL)
		}
	} else if city.Best != nil {
		dayNote = fmt.Sprintf(" City-wide, %s is still the day to fill up.", city.Best.Weekday)
	}
	return fmt.Sprintf("Within %.0f km of %s, %s is the cheapest suburb right now (%.1f ¢/L at %s, %.1f km away).%s%s",
		radius, o.Label, cheapest["suburb"], cheapest["min"], cheapest["cheapest_station"], cheapest["distance_km"], vs, dayNote)
}

func filterStations(latest []domain.PricedStation, q, region string) []domain.PricedStation {
	var out []domain.PricedStation
	exact := false
	if q != "" {
		for _, st := range latest {
			if strings.ToLower(st.Suburb) == q {
				exact = true
				break
			}
		}
	}
	for _, st := range latest {
		if region != "" && st.Region != region {
			continue
		}
		if q != "" {
			if exact {
				if strings.ToLower(st.Suburb) != q && !strings.Contains(strings.ToLower(st.Name), q) {
					continue
				}
			} else if !strings.Contains(strings.ToLower(st.Name), q) && !strings.Contains(strings.ToLower(st.Suburb), q) && !strings.Contains(strings.ToLower(st.Brand), q) && !strings.Contains(st.Postcode, q) && !strings.Contains(strings.ToLower(st.Region), q) {
				continue
			}
		}
		out = append(out, st)
	}
	return out
}

func groupLatest(ctx context.Context, s *Server, fuel, label string, keyFn func(domain.PricedStation) string) []map[string]any {
	_, latest, err := s.store.LatestBatch(ctx, domain.OrDefault(fuel).Code)
	if err != nil {
		return nil
	}
	grouped := map[string][]float64{}
	for _, p := range latest {
		grouped[keyFn(p)] = append(grouped[keyFn(p)], p.PriceCPL)
	}
	var out []map[string]any
	for k, vals := range grouped {
		if sum := domain.Summarise(vals); sum != nil {
			out = append(out, map[string]any{label: k, "n": sum.N, "min": sum.Min, "median": sum.Median, "mean": sum.Mean, "max": sum.Max})
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j]["median"].(float64) < out[i]["median"].(float64) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func prices(rows []domain.PricedStation) []float64 {
	out := make([]float64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.PriceCPL)
	}
	return out
}

func take(rows []domain.PricedStation, n int) []domain.PricedStation {
	if len(rows) < n {
		return rows
	}
	return rows[:n]
}

func lastN(rows []domain.PricedStation, n int) []domain.PricedStation {
	if len(rows) == 0 {
		return rows
	}
	if len(rows) < n {
		n = len(rows)
	}
	out := make([]domain.PricedStation, n)
	for i := 0; i < n; i++ {
		out[i] = rows[len(rows)-1-i]
	}
	return out
}

func avgMedian(rows []domain.DailyRow) float64 {
	if len(rows) == 0 {
		return 0
	}
	sum := 0.0
	for _, r := range rows {
		sum += r.MedianCPL
	}
	return sum / float64(len(rows))
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
