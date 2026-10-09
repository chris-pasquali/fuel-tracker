# Fillwise

Greater Sydney petrol intelligence. **Go**, **Postgres**, **Redis**, **Docker**, and **React + Vite**.

The API pulls every prescribed fuel type from the NSW [FuelCheck](https://www.fuelcheck.nsw.gov.au/) API, stores current and historical prices, and answers:

- what day of the week is cheapest
- which suburbs around you are cheapest right now
- how the weekly cycle is moving

## Architecture

```
FuelCheck ──► Go collector ──► Postgres (stations, snapshots, daily city/suburb history)
                    │
                    ├── Redis (overview + around-you cache, collect lock)
                    │
                    └── Go HTTP API ──► React (Vite) dashboard
```

| Service | Role |
| --- | --- |
| `api` | Go HTTP server, collector, analytics, migrations |
| `postgres` | Current prices + daily station/suburb/city history |
| `redis` | 60–90s response cache and collection lock |
| `web` | React + Vite UI, nginx in Docker |

## Run with Docker

```bash
docker compose up --build
```

- API: http://localhost:8080/api/health
- UI: http://localhost:8088

## Run locally

```bash
cd api && go test ./...
DATABASE_URL=postgres://fillwise:fillwise@127.0.0.1:5432/fillwise?sslmode=disable \
REDIS_URL=redis://127.0.0.1:6379 \
go run ./cmd/server

cd web && npm install && npm run dev
```

Open http://127.0.0.1:5173 (Vite proxies `/api` to `:8080`).

Register your own key at [api.nsw](https://api.nsw.gov.au/Product/Index/22) and set `NSW_FUEL_API_KEY` / `NSW_FUEL_API_SECRET`.

## API

| Method | Path |
| --- | --- |
| GET | `/api/overview?fuel=E10` |
| GET | `/api/stations?fuel=E10&q=bondi` |
| GET | `/api/trends?fuel=E10&days=90` |
| GET | `/api/around?q=Parramatta&radius_km=10&fuel=E10` |
| GET | `/api/places` |
| POST | `/api/collect` |

History starts from the first collection. Until 21 live days exist, weekday/trend panels use a Sydney weekly-cycle model calibrated to today’s live snapshot, and the UI labels that.
