# model-metrics-scraper

Scrapes model metrics from [artificialanalysis.ai](https://artificialanalysis.ai/leaderboards/models) and serves them via a REST API and MCP server.

## Commands

### `scrape` — one-off scrape to JSON

```bash
bin/scrape-aa scrape [--out models.json]
```

Fetches all models and writes a JSON file. Prints the top 5 by Intelligence Index.

### `serve` — daemon with REST + MCP APIs

```bash
bin/scrape-aa serve [--addr :8080] [--db ./data.db] [--interval 1h]
```

Scrapes immediately on start, then on each interval. Stores history in SQLite.

## REST API

Base: `http://localhost:8080/api/v1`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/scrapes` | List all scrapes |
| `GET` | `/scrapes/latest` | Latest scrape metadata |
| `POST` | `/scrapes` | Trigger an immediate re-scrape |
| `GET` | `/models` | Models from latest scrape |
| `GET` | `/models?scrape_id=N` | Models from a specific scrape |
| `GET` | `/models?as_of=RFC3339` | Models from the closest historical scrape |
| `GET` | `/models?creator=anthropic` | Filter by creator (case-insensitive) |
| `GET` | `/models?sort_by=price_input&order=asc` | Sort (`intelligence_index`, `coding_index`, `price_input`, `price_output`, `name`) |
| `GET` | `/models/{slug}` | Single model by slug |

## MCP Server

SSE endpoint: `http://localhost:8080/mcp/sse`

**Tools:**
- `list_models` — list models with optional `sort_by`, `creator`, `limit`
- `get_model` — get a model by `slug`
- `search_models` — search by `query` (name or creator substring)

**Resources:**
- `aa://models` — full model list as JSON
- `aa://models/{slug}` — single model as JSON

## Development

```bash
task build     # compile to bin/scrape-aa
task test      # go test -race ./...
task lint      # golangci-lint + go-arch-lint
task generate  # sqlc generate (after schema/query changes)
```

Requires [Task](https://taskfile.dev), [golangci-lint](https://golangci-lint.run), [sqlc](https://sqlc.dev), and [go-arch-lint](https://github.com/fe3dback/go-arch-lint).
