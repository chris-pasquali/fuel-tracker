package store

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"fillwise/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func (s *Store) Migrate(ctx context.Context, files fs.FS) error {
	if _, err := s.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		if _, err := s.Pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
		if _, err := s.Pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertStations(ctx context.Context, stations []domain.Station) error {
	batch := &pgx.Batch{}
	for _, st := range stations {
		batch.Queue(`
			INSERT INTO stations (code, station_id, brand, name, address, suburb, postcode, region, lat, lon, is_adblue, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
			ON CONFLICT (code) DO UPDATE SET
				station_id=EXCLUDED.station_id, brand=EXCLUDED.brand, name=EXCLUDED.name,
				address=EXCLUDED.address, suburb=EXCLUDED.suburb, postcode=EXCLUDED.postcode,
				region=EXCLUDED.region, lat=EXCLUDED.lat, lon=EXCLUDED.lon, is_adblue=EXCLUDED.is_adblue,
				updated_at=NOW()`,
			st.Code, st.StationID, st.Brand, st.Name, st.Address, st.Suburb, st.Postcode, st.Region, st.Lat, st.Lon, st.IsAdBlue)
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range stations {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertObservations(ctx context.Context, obs []domain.Observation, observedAt time.Time, source string) error {
	batch := &pgx.Batch{}
	n := 0
	for _, o := range obs {
		if o.PriceCPL <= 0 {
			continue
		}
		n++
		batch.Queue(`INSERT INTO observations (station_code, fuel_type, price_cpl, last_updated, observed_at, source)
			VALUES ($1,$2,$3,$4,$5,$6)`, o.StationCode, o.FuelType, o.PriceCPL, o.LastUpdated, observedAt, source)
	}
	if n == 0 {
		return nil
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < n; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertStationDaily(ctx context.Context, day time.Time, obs []domain.Observation, source string) error {
	batch := &pgx.Batch{}
	n := 0
	for _, o := range obs {
		if o.PriceCPL <= 0 {
			continue
		}
		n++
		batch.Queue(`
			INSERT INTO station_daily (date, station_code, fuel_type, price_cpl, source)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (date, station_code, fuel_type) DO UPDATE SET price_cpl=EXCLUDED.price_cpl, source=EXCLUDED.source`,
			day, o.StationCode, o.FuelType, o.PriceCPL, source)
	}
	if n == 0 {
		return nil
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < n; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertDailyCity(ctx context.Context, rows []domain.DailyRow) error {
	for _, r := range rows {
		if _, err := s.Pool.Exec(ctx, `
			INSERT INTO daily_city (date, fuel_type, n, min_cpl, p25_cpl, median_cpl, mean_cpl, p75_cpl, max_cpl, source)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (date, fuel_type) DO UPDATE SET
				n=EXCLUDED.n, min_cpl=EXCLUDED.min_cpl, p25_cpl=EXCLUDED.p25_cpl,
				median_cpl=EXCLUDED.median_cpl, mean_cpl=EXCLUDED.mean_cpl, p75_cpl=EXCLUDED.p75_cpl,
				max_cpl=EXCLUDED.max_cpl,
				source=CASE WHEN daily_city.source='fuelcheck' THEN daily_city.source ELSE EXCLUDED.source END`,
			r.Date, r.FuelType, r.N, r.MinCPL, r.P25CPL, r.MedianCPL, r.MeanCPL, r.P75CPL, r.MaxCPL, r.Source); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertDailyArea(ctx context.Context, rows []domain.DailyRow) error {
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO daily_area (date, fuel_type, area_type, area_name, n, min_cpl, p25_cpl, median_cpl, mean_cpl, p75_cpl, max_cpl, source)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (date, fuel_type, area_type, area_name) DO UPDATE SET
				n=EXCLUDED.n, min_cpl=EXCLUDED.min_cpl, p25_cpl=EXCLUDED.p25_cpl,
				median_cpl=EXCLUDED.median_cpl, mean_cpl=EXCLUDED.mean_cpl, p75_cpl=EXCLUDED.p75_cpl,
				max_cpl=EXCLUDED.max_cpl,
				source=CASE WHEN daily_area.source='fuelcheck' THEN daily_area.source ELSE EXCLUDED.source END`,
			r.Date, r.FuelType, r.AreaType, r.AreaName, r.N, r.MinCPL, r.P25CPL, r.MedianCPL, r.MeanCPL, r.P75CPL, r.MaxCPL, r.Source)
	}
	if len(rows) == 0 {
		return nil
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecordRun(ctx context.Context, started time.Time, status string, stations, prices int, errMsg string) error {
	var errVal any
	if errMsg != "" {
		errVal = errMsg
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO collection_runs (started_at, finished_at, status, station_count, price_count, error)
		VALUES ($1,NOW(),$2,$3,$4,$5)`, started, status, stations, prices, errVal)
	return err
}

func (s *Store) LatestBatch(ctx context.Context, fuel string) (time.Time, []domain.PricedStation, error) {
	var observed time.Time
	err := s.Pool.QueryRow(ctx, `
		SELECT MAX(observed_at) FROM observations
		WHERE fuel_type=$1 AND source IN ('fuelcheck','snapshot')`, fuel).Scan(&observed)
	if err != nil || observed.IsZero() {
		return time.Time{}, nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT o.station_code, o.price_cpl, o.last_updated, s.brand, s.name, s.address, s.suburb, s.postcode, s.region, s.lat, s.lon
		FROM observations o JOIN stations s ON s.code=o.station_code
		WHERE o.fuel_type=$1 AND o.observed_at=$2
		ORDER BY o.price_cpl ASC`, fuel, observed)
	if err != nil {
		return observed, nil, err
	}
	defer rows.Close()
	var out []domain.PricedStation
	for rows.Next() {
		var p domain.PricedStation
		if err := rows.Scan(&p.Code, &p.PriceCPL, &p.LastUpdated, &p.Brand, &p.Name, &p.Address, &p.Suburb, &p.Postcode, &p.Region, &p.Lat, &p.Lon); err != nil {
			return observed, nil, err
		}
		out = append(out, p)
	}
	return observed, out, rows.Err()
}

func (s *Store) CitySeries(ctx context.Context, fuel string, days int) ([]domain.DailyRow, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.Pool.Query(ctx, `
		SELECT date::text, fuel_type, n, min_cpl, p25_cpl, median_cpl, mean_cpl, p75_cpl, max_cpl, source
		FROM daily_city WHERE fuel_type=$1 AND date>=$2 ORDER BY date`, fuel, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDaily(rows)
}

func (s *Store) AreaSeries(ctx context.Context, fuel, areaType, areaName string, days int) ([]domain.DailyRow, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.Pool.Query(ctx, `
		SELECT date::text, fuel_type, n, min_cpl, p25_cpl, median_cpl, mean_cpl, p75_cpl, max_cpl, source
		FROM daily_area WHERE fuel_type=$1 AND area_type=$2 AND area_name=$3 AND date>=$4 ORDER BY date`,
		fuel, areaType, areaName, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDaily(rows)
}

func scanDaily(rows pgx.Rows) ([]domain.DailyRow, error) {
	var out []domain.DailyRow
	for rows.Next() {
		var r domain.DailyRow
		if err := rows.Scan(&r.Date, &r.FuelType, &r.N, &r.MinCPL, &r.P25CPL, &r.MedianCPL, &r.MeanCPL, &r.P75CPL, &r.MaxCPL, &r.Source); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Counts(ctx context.Context) (stations, observations, stationDays, cityDays, areaDays int, last *time.Time, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM stations),
		(SELECT COUNT(*) FROM observations),
		(SELECT COUNT(*) FROM station_daily),
		(SELECT COUNT(*) FROM daily_city),
		(SELECT COUNT(*) FROM daily_area),
		(SELECT MAX(observed_at) FROM observations)`).Scan(&stations, &observations, &stationDays, &cityDays, &areaDays, &last)
	return
}

func (s *Store) LiveHistoryDays(ctx context.Context, fuel string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM daily_city WHERE fuel_type=$1 AND source='fuelcheck'`, fuel).Scan(&n)
	return n, err
}

func (s *Store) PriorCityHistory(ctx context.Context, before time.Time) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM daily_city WHERE date < $1`, before).Scan(&n)
	return n, err
}

func (s *Store) PriorAreaHistory(ctx context.Context, before time.Time) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM daily_area WHERE date < $1`, before).Scan(&n)
	return n, err
}

func (s *Store) Places(ctx context.Context, q string, limit int) ([]map[string]any, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT suburb, postcode, region, COUNT(*) AS n, AVG(lat) AS lat, AVG(lon) AS lon
		FROM stations
		WHERE suburb IS NOT NULL AND suburb <> '' AND suburb <> 'Unknown'
		GROUP BY suburb, postcode, region
		ORDER BY n DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	needle := strings.ToLower(strings.TrimSpace(q))
	for rows.Next() {
		var suburb, pc, region string
		var n int
		var lat, lon *float64
		if err := rows.Scan(&suburb, &pc, &region, &n, &lat, &lon); err != nil {
			return nil, err
		}
		if needle != "" && !strings.Contains(strings.ToLower(suburb), needle) && !strings.Contains(pc, needle) && !strings.Contains(strings.ToLower(region), needle) {
			continue
		}
		out = append(out, map[string]any{"suburb": suburb, "postcode": pc, "region": region, "n": n, "lat": lat, "lon": lon})
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}
