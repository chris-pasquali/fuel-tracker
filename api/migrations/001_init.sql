CREATE TABLE IF NOT EXISTS stations (
    code TEXT PRIMARY KEY,
    station_id TEXT,
    brand TEXT NOT NULL,
    name TEXT NOT NULL,
    address TEXT,
    suburb TEXT,
    postcode TEXT,
    region TEXT,
    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,
    is_adblue BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS observations (
    id BIGSERIAL PRIMARY KEY,
    station_code TEXT NOT NULL REFERENCES stations(code),
    fuel_type TEXT NOT NULL,
    price_cpl DOUBLE PRECISION NOT NULL,
    last_updated TEXT,
    observed_at TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS station_daily (
    date DATE NOT NULL,
    station_code TEXT NOT NULL REFERENCES stations(code),
    fuel_type TEXT NOT NULL,
    price_cpl DOUBLE PRECISION NOT NULL,
    source TEXT NOT NULL,
    PRIMARY KEY (date, station_code, fuel_type)
);

CREATE TABLE IF NOT EXISTS daily_city (
    date DATE NOT NULL,
    fuel_type TEXT NOT NULL,
    n INTEGER NOT NULL,
    min_cpl DOUBLE PRECISION NOT NULL,
    p25_cpl DOUBLE PRECISION NOT NULL,
    median_cpl DOUBLE PRECISION NOT NULL,
    mean_cpl DOUBLE PRECISION NOT NULL,
    p75_cpl DOUBLE PRECISION NOT NULL,
    max_cpl DOUBLE PRECISION NOT NULL,
    source TEXT NOT NULL,
    PRIMARY KEY (date, fuel_type)
);

CREATE TABLE IF NOT EXISTS daily_area (
    date DATE NOT NULL,
    fuel_type TEXT NOT NULL,
    area_type TEXT NOT NULL,
    area_name TEXT NOT NULL,
    n INTEGER NOT NULL,
    min_cpl DOUBLE PRECISION NOT NULL,
    p25_cpl DOUBLE PRECISION NOT NULL,
    median_cpl DOUBLE PRECISION NOT NULL,
    mean_cpl DOUBLE PRECISION NOT NULL,
    p75_cpl DOUBLE PRECISION NOT NULL,
    max_cpl DOUBLE PRECISION NOT NULL,
    source TEXT NOT NULL,
    PRIMARY KEY (date, fuel_type, area_type, area_name)
);

CREATE TABLE IF NOT EXISTS collection_runs (
    id BIGSERIAL PRIMARY KEY,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    station_count INTEGER,
    price_count INTEGER,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_obs_fuel_time ON observations (fuel_type, observed_at);
CREATE INDEX IF NOT EXISTS idx_obs_station_fuel ON observations (station_code, fuel_type, observed_at);
CREATE INDEX IF NOT EXISTS idx_stations_region ON stations (region, suburb, brand);
CREATE INDEX IF NOT EXISTS idx_station_daily_fuel ON station_daily (fuel_type, date);
CREATE INDEX IF NOT EXISTS idx_daily_area_lookup ON daily_area (fuel_type, area_type, area_name, date);
