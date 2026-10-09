export type Fuel = { code: string; name: string; family: string; unit: string };

export type Summary = {
  n: number;
  min: number;
  p25: number;
  median: number;
  mean: number;
  p75: number;
  max: number;
};

export type Station = {
  code: string;
  brand: string;
  name: string;
  address: string;
  suburb: string;
  postcode: string;
  region: string;
  lat?: number | null;
  lon?: number | null;
  price_cpl: number;
  last_updated?: string;
  distance_km?: number;
};

export type WeekdayRow = {
  weekday: string;
  weekday_index: number;
  samples: number;
  avg_median_cpl: number | null;
  median_cpl: number | null;
  weeks_cheapest: number;
  cheapest_share: number;
};

export type WeekdayReport = {
  days: WeekdayRow[];
  best: WeekdayRow | null;
  worst: WeekdayRow | null;
  save_cpl: number | null;
  weeks_observed: number;
};

export type CycleReport = {
  phase: string;
  label: string;
  typical_length_days: number;
  days_since_trough: number | null;
  trough_count: number;
  expected_cheap_window: string | null;
  latest_median_cpl: number;
  change_4d_cpl: number;
};

export type DailyRow = {
  date: string;
  fuel_type: string;
  n: number;
  min_cpl: number;
  p25_cpl: number;
  median_cpl: number;
  mean_cpl: number;
  p75_cpl: number;
  max_cpl: number;
  source: string;
};

export type Overview = {
  fuel: Fuel;
  observed_at: string;
  station_count: number;
  sample_count: number;
  summary: Summary | null;
  cheapest: Station[];
  dearest: Station[];
  recommendation: {
    best_weekday: string;
    save_cpl: number;
    confidence: string;
    rationale: string;
  } | null;
  cycle: CycleReport;
  weekday: WeekdayReport;
  history_source: string;
  live_history_days: number;
};

export type Trends = {
  fuel: string;
  days: number;
  week_delta_cpl: number | null;
  series: DailyRow[];
};

export type Place = {
  suburb: string;
  postcode: string;
  region: string;
  n?: number;
};

export type Around = {
  fuel: string;
  origin: {
    label: string;
    kind: string;
    suburb: string;
    postcode: string;
    region: string;
    lat: number;
    lon: number;
  } | null;
  radius_km: number;
  message?: string;
  summary?: Summary | null;
  city_summary?: Summary | null;
  vs_city_cpl?: number | null;
  nearby_suburbs?: Array<{
    suburb: string;
    region: string;
    postcode: string;
    distance_km: number;
    cheapest_station: string;
    n: number;
    min: number;
    median: number;
    mean: number;
    max: number;
  }>;
  stations?: Station[];
  station_count?: number;
  weekday?: WeekdayReport;
  city_weekday?: WeekdayReport;
  trend?: DailyRow[];
  series_scope?: string;
  insight?: string;
};

export type Health = {
  stations: number;
  observations: number;
  last_snapshot: string;
  redis: boolean;
  server_time: string;
};

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${body || res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  fuels: () => getJSON<Fuel[]>("/api/fuels"),
  health: () => getJSON<Health>("/api/health"),
  overview: (fuel: string) => getJSON<Overview>(`/api/overview?fuel=${encodeURIComponent(fuel)}`),
  trends: (fuel: string, days = 90) =>
    getJSON<Trends>(`/api/trends?fuel=${encodeURIComponent(fuel)}&days=${days}`),
  stations: (fuel: string, q: string) =>
    getJSON<{ stations: Station[]; count: number; observed_at: string }>(
      `/api/stations?fuel=${encodeURIComponent(fuel)}&q=${encodeURIComponent(q)}&limit=60`,
    ),
  places: (q: string) => getJSON<Place[]>(`/api/places?q=${encodeURIComponent(q)}&limit=12`),
  around: (fuel: string, q: string, radiusKm: number, lat?: number, lon?: number) => {
    const params = new URLSearchParams({ fuel, radius_km: String(radiusKm) });
    if (q) params.set("q", q);
    if (lat != null && lon != null) {
      params.set("lat", String(lat));
      params.set("lon", String(lon));
    }
    return getJSON<Around>(`/api/around?${params.toString()}`);
  },
  collect: async () => {
    const res = await fetch("/api/collect", { method: "POST" });
    if (!res.ok) throw new Error(await res.text());
    return res.json() as Promise<{ station_count: number; price_count: number; source: string }>;
  },
};
