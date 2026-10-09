import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  Around,
  Fuel,
  Overview,
  Station,
  Trends,
  WeekdayReport,
  api,
} from "./api";

function cpl(n: number | null | undefined) {
  if (n == null || Number.isNaN(n)) return "—";
  return `${n.toFixed(1)} ¢/L`;
}

function when(iso?: string) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-AU", { timeZone: "Australia/Sydney" });
}

function BarChart({
  values,
  labels,
  highlight,
}: {
  values: Array<number | null>;
  labels: string[];
  highlight?: string;
}) {
  const nums = values.filter((v): v is number => v != null);
  const min = Math.min(...nums);
  const max = Math.max(...nums);
  const span = Math.max(1, max - min);
  return (
    <div className="bars">
      {values.map((v, i) => {
        const h = v == null ? 8 : 18 + ((v - min) / span) * 82;
        const on = labels[i] === highlight;
        return (
          <div key={labels[i]} className={`bar ${on ? "on" : ""}`} title={`${labels[i]} ${cpl(v)}`}>
            <div className="col" style={{ height: `${h}%` }} />
            <span>{labels[i].slice(0, 3)}</span>
            <em>{v == null ? "—" : v.toFixed(1)}</em>
          </div>
        );
      })}
    </div>
  );
}

function Spark({ series }: { series: Array<{ date: string; median_cpl: number }> }) {
  if (series.length < 2) return <p className="muted">Not enough history yet.</p>;
  const w = 640;
  const h = 140;
  const pad = 8;
  const ys = series.map((s) => s.median_cpl);
  const min = Math.min(...ys);
  const max = Math.max(...ys);
  const span = Math.max(0.5, max - min);
  const pts = series
    .map((s, i) => {
      const x = pad + (i / (series.length - 1)) * (w - pad * 2);
      const y = h - pad - ((s.median_cpl - min) / span) * (h - pad * 2);
      return `${x},${y}`;
    })
    .join(" ");
  return (
    <svg className="spark" viewBox={`0 0 ${w} ${h}`} role="img" aria-label="90-day median trend">
      <polyline fill="none" stroke="currentColor" strokeWidth="2.4" points={pts} />
    </svg>
  );
}

function StationRow({ s }: { s: Station }) {
  return (
    <li>
      <strong>{s.price_cpl.toFixed(1)}</strong>
      <div>
        <b>{s.name}</b>
        <span>
          {s.brand} · {s.suburb} {s.postcode}
          {s.distance_km ? ` · ${s.distance_km.toFixed(1)} km` : ""}
        </span>
      </div>
    </li>
  );
}

export default function App() {
  const [fuels, setFuels] = useState<Fuel[]>([]);
  const [fuel, setFuel] = useState("E10");
  const [overview, setOverview] = useState<Overview | null>(null);
  const [trends, setTrends] = useState<Trends | null>(null);
  const [stations, setStations] = useState<Station[]>([]);
  const [query, setQuery] = useState("");
  const [aroundQ, setAroundQ] = useState("Parramatta");
  const [around, setAround] = useState<Around | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.fuels().then(setFuels).catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    Promise.all([api.overview(fuel), api.trends(fuel)])
      .then(([o, t]) => {
        if (cancelled) return;
        setOverview(o);
        setTrends(t);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [fuel]);

  useEffect(() => {
    const t = setTimeout(() => {
      api
        .stations(fuel, query)
        .then((r) => setStations(r.stations))
        .catch((e) => setError(String(e)));
    }, 200);
    return () => clearTimeout(t);
  }, [fuel, query]);

  const weekday: WeekdayReport | undefined = overview?.weekday;

  const cheapDay = overview?.recommendation?.best_weekday;

  async function lookupAround(e?: FormEvent) {
    e?.preventDefault();
    setBusy(true);
    setError(null);
    try {
      setAround(await api.around(fuel, aroundQ, 10));
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  async function useMyLocation() {
    if (!navigator.geolocation) {
      setError("Geolocation is not available in this browser.");
      return;
    }
    setBusy(true);
    navigator.geolocation.getCurrentPosition(
      async (pos) => {
        try {
          setAround(await api.around(fuel, "", 10, pos.coords.latitude, pos.coords.longitude));
        } catch (err) {
          setError(String(err));
        } finally {
          setBusy(false);
        }
      },
      () => {
        setBusy(false);
        setError("Could not read your location.");
      },
    );
  }

  async function refresh() {
    setBusy(true);
    try {
      await api.collect();
      const [o, t] = await Promise.all([api.overview(fuel), api.trends(fuel)]);
      setOverview(o);
      setTrends(t);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  const historyNote = useMemo(() => {
    if (!overview) return "";
    if (overview.history_source === "live") {
      return `${overview.live_history_days} live days of Greater Sydney history.`;
    }
    return "Weekday and trend panels use a Sydney weekly-cycle model until 21 live days exist.";
  }, [overview]);

  return (
    <div className="page">
      <header>
        <div>
          <p className="eyebrow">Greater Sydney</p>
          <h1>Fillwise</h1>
          <p className="lede">Current petrol prices, the weekly cycle, and cheaper suburbs around you.</p>
        </div>
        <div className="tools">
          <label>
            Fuel
            <select value={fuel} onChange={(e) => setFuel(e.target.value)}>
              {fuels.map((f) => (
                <option key={f.code} value={f.code}>
                  {f.name}
                </option>
              ))}
            </select>
          </label>
          <button type="button" onClick={refresh} disabled={busy}>
            {busy ? "Working…" : "Refresh prices"}
          </button>
        </div>
      </header>

      {error && <p className="banner">{error}</p>}

      <section className="hero">
        <article>
          <p className="kicker">City median</p>
          <p className="stat">{cpl(overview?.summary?.median)}</p>
          <p className="muted">
            {overview?.sample_count ?? 0} stations · observed {when(overview?.observed_at)}
          </p>
        </article>
        <article>
          <p className="kicker">Cheapest right now</p>
          <p className="stat">{cpl(overview?.summary?.min)}</p>
          <p className="muted">{overview?.cheapest?.[0]?.name ?? "Waiting for snapshot"}</p>
        </article>
        <article>
          <p className="kicker">Best day to fill</p>
          <p className="stat">{cheapDay ?? "—"}</p>
          <p className="muted">
            {overview?.recommendation
              ? `Save about ${overview.recommendation.save_cpl.toFixed(1)} ¢/L vs the dearest day`
              : "Collecting history"}
          </p>
        </article>
        <article>
          <p className="kicker">Cycle</p>
          <p className="stat phase">{overview?.cycle?.phase ?? "—"}</p>
          <p className="muted">{overview?.cycle?.label}</p>
        </article>
      </section>

      <p className="note">{historyNote}</p>

      <section className="grid two">
        <article className="card">
          <h2>Weekday pattern</h2>
          {weekday && (
            <BarChart
              labels={weekday.days.map((d) => d.weekday)}
              values={weekday.days.map((d) => d.avg_median_cpl)}
              highlight={cheapDay}
            />
          )}
          {overview?.recommendation && <p>{overview.recommendation.rationale}</p>}
        </article>
        <article className="card">
          <h2>90-day city median</h2>
          <Spark series={trends?.series ?? []} />
          <p className="muted">
            Week change: {trends?.week_delta_cpl == null ? "—" : `${trends.week_delta_cpl.toFixed(1)} ¢/L`}
          </p>
        </article>
      </section>

      <section className="card">
        <h2>Around you</h2>
        <form className="row" onSubmit={lookupAround}>
          <input
            value={aroundQ}
            onChange={(e) => setAroundQ(e.target.value)}
            placeholder="Suburb or postcode"
            aria-label="Suburb or postcode"
          />
          <button type="submit" disabled={busy}>
            Rank nearby
          </button>
          <button type="button" onClick={useMyLocation} disabled={busy}>
            Use my location
          </button>
        </form>
        {around?.message && <p className="muted">{around.message}</p>}
        {around?.insight && <p>{around.insight}</p>}
        {around?.nearby_suburbs && around.nearby_suburbs.length > 0 && (
          <ol className="suburbs">
            {around.nearby_suburbs.slice(0, 8).map((s) => (
              <li key={s.suburb}>
                <strong>{s.min.toFixed(1)}</strong>
                <div>
                  <b>{s.suburb}</b>
                  <span>
                    {s.cheapest_station} · {s.distance_km.toFixed(1)} km
                  </span>
                </div>
              </li>
            ))}
          </ol>
        )}
      </section>

      <section className="card">
        <h2>Stations</h2>
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search suburb, brand, or postcode"
          aria-label="Search stations"
        />
        <ol className="stations">
          {stations.map((s) => (
            <StationRow key={`${s.code}-${s.price_cpl}`} s={s} />
          ))}
        </ol>
      </section>

      <footer>
        Prices from NSW FuelCheck. Greater Sydney postcodes only. Not financial advice.
      </footer>
    </div>
  );
}
