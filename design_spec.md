# Garmin Training Volume Tracker — Design Spec

*2026-11-11*

## Overview

A personal web application that pulls activity data from Garmin Connect and displays weekly training volumes. The user can view total kilometers and total hours per week, filtered by sport type.

**Goals**
- Sync activities from Garmin Connect into a local SQLite database
- Display a weekly summary table with total distance (km) and total duration (hours)
- Toggle between: All activities, Running only, Biking only
- Entirely self-hosted; no external services beyond Garmin Connect

## Architecture

**Components**
- **Go net/http** — single binary, serves both the JSON API and the static HTML page; no framework needed
- **SQLite (mattn/go-sqlite3)** — stores synced activities locally; avoids repeated calls to Garmin
- **Garmin Connect Personal API** — official REST API, authenticated via OAuth 2.0 token
- **Vanilla JS** — no framework; button clicks swap a query param and re-fetch `/api/weeks`

**Request paths**

| Request | Path | Handler |
|---------|------|---------|
| Load UI | `GET /` | serve `static/index.html` |
| Get weekly data | `GET /api/weeks?sport=` | query SQLite, return JSON |
| Trigger sync | `GET /api/sync` | fetch Garmin, upsert DB |

**Package layout**
```
run_vol/
├── cmd/
│   └── main.go              — flag parsing, server boot, route registration
├── internal/
│   ├── garmin/
│   │   └── client.go        — Personal API OAuth + activity list fetcher
│   ├── store/
│   │   └── store.go         — SQLite open/migrate, Upsert, WeeklySummary
│   └── handlers/
│       ├── sync.go          — GET /api/sync
│       └── weeks.go         — GET /api/weeks?sport=all|running|cycling
└── static/
    └── index.html           — single-page UI
```

## Data Model (SQLite)

```sql
CREATE TABLE activities (
    id          TEXT PRIMARY KEY,  -- Garmin activity ID
    sport       TEXT NOT NULL,     -- 'running' | 'cycling' | 'other'
    started_at  TEXT NOT NULL,     -- ISO-8601 UTC timestamp
    week        TEXT NOT NULL,     -- ISO week label: '2026-W45'
    distance_km REAL NOT NULL,     -- metres converted to km at ingest
    duration_s  INTEGER NOT NULL   -- seconds
);

CREATE INDEX idx_week_sport ON activities(week, sport);
```

**Weekly aggregation query** (parameterised on sport):

```sql
-- all sports
SELECT week,
       ROUND(SUM(distance_km), 2)         AS km,
       ROUND(SUM(duration_s) / 3600.0, 2) AS hours
FROM   activities
GROUP  BY week
ORDER  BY week DESC;

-- single sport  (? = 'running' or 'cycling')
SELECT week,
       ROUND(SUM(distance_km), 2),
       ROUND(SUM(duration_s) / 3600.0, 2)
FROM   activities
WHERE  sport = ?
GROUP  BY week
ORDER  BY week DESC;
```

**Sport normalisation at ingest**

| Garmin activity type | Stored as |
|---------------------|-----------|
| running, trail_running | running |
| cycling, indoor_cycling, virtual_ride | cycling |
| anything else | other |

## Garmin Connect Integration

Garmin provides an official **Garmin Connect Personal API** for accessing your own activity data. There is no Go client library for it, so requests are made directly with `net/http`.

**Base URL**: `https://apis.garmin.com/wellness-api/rest`

**Authentication**: OAuth 2.0 with the Authorization Code flow.
1. Register the app in the Garmin Developer Portal to obtain a `client_id` and `client_secret`.
2. Redirect the user to the Garmin authorization URL to obtain an authorization code.
3. Exchange the code for an `access_token` and `refresh_token` via POST to the token endpoint.
4. All API requests include `Authorization: Bearer <access_token>`.
5. Refresh the access token automatically when it expires (tokens expire after 1 hour).

**Credentials**: `GARMIN_CLIENT_ID`, `GARMIN_CLIENT_SECRET`, and the token pair are read from / persisted to environment variables or a local token file; never stored in the activities database.

**Incremental sync strategy**
- Read `MAX(started_at)` from the local DB.
- Fetch only activities with `start_date > max_date` from Garmin.
- Each activity is upserted (`INSERT OR REPLACE`) so re-syncing is safe and idempotent.
- On first run (empty DB) a full historical fetch is performed, paginated in batches of 100.

**Rate limiting**: add a 500 ms delay between paginated requests to avoid being throttled.

## Backend: Go Server

**Routes**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve `static/index.html` |
| GET | `/api/weeks?sport=all\|running\|cycling` | Return JSON array of weekly summaries |
| GET | `/api/sync` | Trigger incremental Garmin sync, return `{synced: N}` |

**Weekly summary JSON shape**
```json
[
  { "week": "2026-W45", "km": 52.3, "hours": 4.75 },
  { "week": "2026-W44", "km": 38.1, "hours": 3.20 }
]
```

**Key implementation notes**
- `store.WeeklySummary(sport string)` returns `[]WeekRow` — the SQL differs only by the WHERE clause.
- `store.Upsert([]Activity)` runs in a single transaction for performance.
- Server reads `--addr` flag (default `:8080`) and `--db` flag (default `./garmin.db`).
- Garmin client is instantiated once at startup and stored in handler context; the OAuth access token is refreshed automatically when expired.

## Frontend: UI Layout

```
+--------------------------------------------------+
|  Training Volume Tracker            [Sync]        |
+--------------------------------------------------+
|  [ All ]  [ Running ]  [ Biking ]                |
+----------+--------------+------------------------+
|  Week    |  Distance    |  Duration              |
+----------+--------------+------------------------+
| 2026-W45 |   52.3 km    |  4 h 45 m              |
| 2026-W44 |   38.1 km    |  3 h 12 m              |
|  ...     |   ...        |  ...                   |
+----------+--------------+------------------------+
```

**Interaction details**
- Three filter buttons (All / Running / Biking): clicking one calls `fetchWeeks(sport)`, updates the table, and highlights the active button.
- Sync button calls `GET /api/sync` and re-fetches the table on completion.
- Duration displayed as `Xh Ym` (not raw decimal hours).
- Active filter button gets a distinct background colour; others are outlined.
- No page reload on filter change — pure JS `fetch()` + DOM update.

**Tech**: single `index.html` file; inline `<style>` and `<script>`; ~60 lines of JS; no build step, no bundler, no dependencies.

## Data Flow

**Sync path** (user clicks Sync)

1. Browser sends `GET /api/sync`
2. Handler reads `MAX(started_at)` from SQLite
3. Garmin client fetches activities newer than that date (paginated, 100 per page)
4. Each activity is normalised (sport type, ISO week, metres → km) and upserted into `activities`
5. Handler returns `{"synced": N}` to browser
6. Browser re-fetches `GET /api/weeks?sport=<current>` and re-renders the table

**Display path** (user clicks a filter button)

1. Browser sends `GET /api/weeks?sport=running` (or `cycling` or `all`)
2. Handler runs the appropriate SQLite GROUP BY query
3. Returns JSON array sorted by week descending
4. Browser clears the table body and inserts a row per week

**Configuration**

| Variable | Source | Default |
|----------|--------|---------|
| `GARMIN_EMAIL` | env var | (required) |
| `GARMIN_PASSWORD` | env var | (required) |
| `--addr` | CLI flag | `:8080` |
| `--db` | CLI flag | `./garmin.db` |
