# Cursus

Real-time backend for Rome metro lines (MA, MB, MB1, MC) powered by Roma Mobilità GTFS-RT feeds.

## Quickstart

```bash
cp .env.example .env
docker compose up
```

The API will be available at `http://localhost:8085` when started with Docker Compose.
For local development with `make run`, the default port is `8080`.

## API

| Endpoint | Description |
|---|---|
| `GET /health` | Service health and feed age |
| `GET /api/v1/lines` | All lines with headway and active vehicle count |
| `GET /api/v1/lines/{line_id}` | Line detail (`MA`, `MB`, `MB1`, `MC`) |
| `GET /api/v1/lines/{line_id}/vehicles` | Live vehicle positions for a line |
| `GET /api/v1/stations/{stop_id}` | Upcoming arrivals at a stop |
| `GET /api/v1/stations/{stop_id}/next-arrival` | Next single arrival at a stop |
| `GET /api/v1/metrics` | Global snapshot: vehicles, headways, feed age |

## Architecture

```
HTTP API (api/)
    │
Domain (metrics/, store/)
    │
Anti-Corruption Layer (acl/)
    │
Roma Mobilità GTFS-RT (external)
```

The ACL (`internal/acl/`) is the only package that knows about protobuf or HTTP. No external type crosses into the domain.

## Development

```bash
make build          # compile binary
make run            # run locally (requires .env)
make test           # run tests with race detector
make test-coverage  # 100% coverage gate
make coverage-html  # open coverage report in browser
make lint           # go vet
```

## Configuration

All parameters are set via environment variables. See `.env.example` for defaults.

| Variable | Default | Description |
|---|---|---|
| `CURSUS_PORT` | `8080` | HTTP listen port |
| `CURSUS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `CURSUS_POLL_INTERVAL` | `30s` | How often to fetch feeds |
| `CURSUS_GTFS_TRIP_UPDATES_URL` | Roma Mobilità `_feed.pb` URL | GTFS-RT TripUpdates feed |
| `CURSUS_GTFS_VEHICLE_POSITIONS_URL` | Roma Mobilità `_feed.pb` URL | GTFS-RT VehiclePositions feed |
| `CURSUS_GTFS_FETCH_TIMEOUT` | `10s` | Per-request HTTP timeout |
| `CURSUS_GTFS_MAX_RETRIES` | `3` | Max retry attempts with exponential backoff |
| `CURSUS_METRO_ROUTE_IDS_MA` | `MEA` | Comma-separated route_ids for line MA |
| `CURSUS_METRO_ROUTE_IDS_MB` | `MEB` | Comma-separated route_ids for line MB |
| `CURSUS_METRO_ROUTE_IDS_MB1` | `MEB1` | Comma-separated route_ids for line MB1 |
| `CURSUS_METRO_ROUTE_IDS_MC` | `MEC` | Comma-separated route_ids for line MC |

### MB1 branch detection

Roma Mobilità may expose MB1 as a distinct `route_id` (`MB1`) or as a branch of MB with a different headsign. Set `CURSUS_METRO_ROUTE_IDS_MB1` to a comma-separated list of all `route_id` values that should map to MB1. Unknown metro `route_id` values are logged at WARN level to aid discovery.
