package collector

import (
	"context"
	"log"
	"time"

	"fillwise/internal/config"
	"fillwise/internal/domain"
	"fillwise/internal/fuelcheck"
	"fillwise/internal/store"
)

type Collector struct {
	cfg    config.Config
	store  *store.Store
	client *fuelcheck.Client
}

func New(cfg config.Config, st *store.Store) *Collector {
	return &Collector{cfg: cfg, store: st, client: fuelcheck.New(cfg)}
}

func (c *Collector) CollectLive(ctx context.Context) (int, int, error) {
	started := time.Now().UTC()
	payload, err := c.client.FetchAll()
	if err != nil {
		_ = c.store.RecordRun(ctx, started, "error", 0, 0, err.Error())
		return 0, 0, err
	}
	_ = fuelcheck.WriteSnapshot(c.cfg.SnapshotPath, fuelcheck.SydneyOnly(payload))
	stations, obs := fuelcheck.FilterSydney(payload)
	if err := c.persist(ctx, stations, obs, started, "fuelcheck"); err != nil {
		_ = c.store.RecordRun(ctx, started, "error", 0, 0, err.Error())
		return 0, 0, err
	}
	_ = c.store.RecordRun(ctx, started, "ok", len(stations), len(obs), "")
	return len(stations), len(obs), nil
}

func (c *Collector) LoadSnapshot(ctx context.Context) (int, int, error) {
	payload, err := fuelcheck.LoadSnapshot(c.cfg.SnapshotPath)
	if err != nil {
		return 0, 0, err
	}
	stations, obs := fuelcheck.FilterSydney(payload)
	started := time.Now().UTC()
	if err := c.persist(ctx, stations, obs, started, "snapshot"); err != nil {
		return 0, 0, err
	}
	_ = c.store.RecordRun(ctx, started, "ok", len(stations), len(obs), "")
	return len(stations), len(obs), nil
}

func (c *Collector) persist(ctx context.Context, stations []domain.Station, obs []domain.Observation, observedAt time.Time, source string) error {
	if err := c.store.UpsertStations(ctx, stations); err != nil {
		return err
	}
	if err := c.store.InsertObservations(ctx, obs, observedAt, source); err != nil {
		return err
	}
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		loc = time.FixedZone("AEST", 10*3600)
	}
	day := observedAt.In(loc)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	if err := c.store.UpsertStationDaily(ctx, day, obs, source); err != nil {
		return err
	}
	byFuel := map[string][]float64{}
	bySuburb := map[[2]string][]float64{}
	byRegion := map[[2]string][]float64{}
	index := map[string]domain.Station{}
	for _, st := range stations {
		index[st.Code] = st
	}
	for _, o := range obs {
		byFuel[o.FuelType] = append(byFuel[o.FuelType], o.PriceCPL)
		st := index[o.StationCode]
		if st.Suburb != "" {
			bySuburb[[2]string{o.FuelType, st.Suburb}] = append(bySuburb[[2]string{o.FuelType, st.Suburb}], o.PriceCPL)
		}
		if st.Region != "" {
			byRegion[[2]string{o.FuelType, st.Region}] = append(byRegion[[2]string{o.FuelType, st.Region}], o.PriceCPL)
		}
	}
	date := day.Format("2006-01-02")
	var city []domain.DailyRow
	for fuel, values := range byFuel {
		if row := domain.DailyFrom(date, fuel, source, values); row != nil {
			city = append(city, *row)
		}
	}
	if err := c.store.UpsertDailyCity(ctx, city); err != nil {
		return err
	}
	var areas []domain.DailyRow
	for key, values := range bySuburb {
		if row := domain.DailyFrom(date, key[0], source, values); row != nil {
			row.AreaType, row.AreaName = "suburb", key[1]
			areas = append(areas, *row)
		}
	}
	for key, values := range byRegion {
		if row := domain.DailyFrom(date, key[0], source, values); row != nil {
			row.AreaType, row.AreaName = "region", key[1]
			areas = append(areas, *row)
		}
	}
	return c.store.UpsertDailyArea(ctx, areas)
}

func (c *Collector) SeedHistory(ctx context.Context) error {
	loc, _ := time.LoadLocation("Australia/Sydney")
	end := time.Now().In(loc)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	n, err := c.store.PriorCityHistory(ctx, end)
	if err != nil {
		return err
	}
	if n == 0 {
		for _, fuel := range domain.Fuels {
			_, latest, err := c.store.LatestBatch(ctx, fuel.Code)
			if err != nil || len(latest) == 0 {
				continue
			}
			values := make([]float64, 0, len(latest))
			for _, p := range latest {
				values = append(values, p.PriceCPL)
			}
			sum := domain.Summarise(values)
			if sum == nil {
				continue
			}
			if err := c.store.UpsertDailyCity(ctx, domain.SynthesiseSeries(fuel.Code, end, c.cfg.HistoryDays, *sum)); err != nil {
				return err
			}
		}
	}
	an, err := c.store.PriorAreaHistory(ctx, end)
	if err != nil || an > 0 {
		return err
	}
	for _, fuel := range domain.Fuels {
		_, latest, err := c.store.LatestBatch(ctx, fuel.Code)
		if err != nil || len(latest) == 0 {
			continue
		}
		cityVals := make([]float64, 0, len(latest))
		suburb := map[string][]float64{}
		region := map[string][]float64{}
		for _, p := range latest {
			cityVals = append(cityVals, p.PriceCPL)
			suburb[p.Suburb] = append(suburb[p.Suburb], p.PriceCPL)
			region[p.Region] = append(region[p.Region], p.PriceCPL)
		}
		city := domain.Summarise(cityVals)
		if city == nil {
			continue
		}
		base := domain.SynthesiseSeries(fuel.Code, end, c.cfg.HistoryDays, *city)
		var rows []domain.DailyRow
		for name, vals := range suburb {
			if len(vals) < 2 {
				continue
			}
			if s := domain.Summarise(vals); s != nil {
				offset := s.Median - city.Median
				for _, row := range base {
					rows = append(rows, domain.ShiftRow(row, offset, "suburb", name))
				}
			}
		}
		for name, vals := range region {
			if s := domain.Summarise(vals); s != nil {
				offset := s.Median - city.Median
				for _, row := range base {
					rows = append(rows, domain.ShiftRow(row, offset, "region", name))
				}
			}
		}
		if err := c.store.UpsertDailyArea(ctx, rows); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) Bootstrap(ctx context.Context) error {
	stations, prices, err := c.CollectLive(ctx)
	if err != nil {
		log.Printf("live collect failed, using snapshot: %v", err)
		stations, prices, err = c.LoadSnapshot(ctx)
		if err != nil {
			return err
		}
	}
	log.Printf("bootstrap snapshot: %d stations, %d prices", stations, prices)
	return c.SeedHistory(ctx)
}

func (c *Collector) StartLoop(ctx context.Context) {
	t := time.NewTicker(c.cfg.CollectInterval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				if _, _, err := c.CollectLive(ctx); err != nil {
					log.Printf("scheduled collect failed: %v", err)
				}
			}
		}
	}()
}
