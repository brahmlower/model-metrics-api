# model-metrics-scraper

Scrapes model metrics from [artificialanalysis.ai](https://artificialanalysis.ai/leaderboards/models)
and serves them via a REST API and MCP server.

## Quick start

```bash
task build
bin/scrape-aa serve
```

```bash
# All models, sorted by Intelligence Index (default)
curl http://localhost:8080/api/v1/models | jq '.models[0:5] | .[].name'

# Anthropic models sorted by coding score
curl "http://localhost:8080/api/v1/models?creator=anthropic&sort_by=coding_index" | jq '.models[].name'

# Models scoring above 0.5 on GPQA, cheapest first
curl "http://localhost:8080/api/v1/models?bench=gpqa&min=0.5&sort_by=price_input&order=asc" \
  | jq '.models[] | {name, gpqa, price1mInputTokens}'

# Single model
curl http://localhost:8080/api/v1/models/claude-sonnet-4-5 \
  | jq '{name, intelligenceIndex, price1mInputTokens}'
```

## Concepts

**Scrape** — one fetch from artificialanalysis.ai, timestamped and stored in SQLite. The server
scrapes on startup and then on each `--interval` tick. Every scrape is kept, so you can query
historical data.

**Slug** — a stable URL-safe identifier for each model (e.g. `claude-sonnet-4-5`). Slugs are
consistent across scrapes, so you can track a model over time.

## Commands

### `scrape` — one-off scrape to JSON

```bash
bin/scrape-aa scrape [--out models.json]
```

Fetches all models, writes a JSON file, prints the top 5 by Intelligence Index.

### `serve` — daemon with REST + MCP APIs

```bash
bin/scrape-aa serve [--addr :8080] [--db ./data.db] [--interval 1h]
```

Scrapes immediately on start, then on each interval. Stores history in SQLite.

## REST API

Base: `http://localhost:8080/api/v1`

### Endpoints

| Method | Path | Description |
| ------ | ---- | ----------- |
| `GET` | `/scrapes` | List all scrapes (newest first) |
| `GET` | `/scrapes/latest` | Latest scrape metadata |
| `POST` | `/scrapes` | Trigger an immediate re-scrape |
| `GET` | `/models` | Models from latest scrape |
| `GET` | `/models/{slug}` | Single model by slug (latest scrape) |

### `/models` query parameters

All parameters are optional and compose freely.

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `scrape_id` | integer | — | Return models from this specific scrape |
| `as_of` | RFC3339 string | — | Return models from the closest scrape at or before this time |
| `creator` | string | — | Filter by creator name (case-insensitive substring) |
| `bench` | string | — | Filter by benchmark score (see [Benchmark fields](#benchmark-fields)) |
| `min` | float | — | Minimum benchmark score, inclusive (requires `bench`) |
| `max` | float | — | Maximum benchmark score, inclusive (requires `bench`) |
| `sort_by` | string | `intelligence_index` | Sort field: `intelligence_index`, `coding_index`, `price_input`, `price_output`, `name` |
| `order` | `asc` \| `desc` | `desc` | Sort direction |

**Examples:**

```bash
# Models from a specific historical scrape
curl "http://localhost:8080/api/v1/models?scrape_id=3"

# Models as of a specific point in time
curl "http://localhost:8080/api/v1/models?as_of=2025-01-01T00:00:00Z"

# Combine filters and sort
curl "http://localhost:8080/api/v1/models?creator=openai&bench=hle&min=0.2&sort_by=price_input&order=asc"

# Historical model snapshot
curl "http://localhost:8080/api/v1/models/gpt-4o?scrape_id=1"
```

### Benchmark fields

Pass one of these field names as the `bench` query parameter.

| Field | Benchmark |
| ----- | --------- |
| `intelligenceIndex` | AA Intelligence Index (composite) |
| `codingIndex` | AA Coding Index (composite) |
| `agenticIndex` | AA Agentic Index (composite) |
| `gpqa` | GPQA Diamond — graduate-level science questions |
| `hle` | Humanity's Last Exam |
| `mmmuPro` | MMMU-Pro — multimodal understanding |
| `omniscience` | Omniscience — factual knowledge & non-hallucination |
| `scicode` | SciCode — scientific coding tasks |
| `critpt` | CritPT — critical-point reasoning |
| `gdpvalNormalized` | GDPVal — long-context dialogue |
| `ifbench` | IFBench — instruction following |
| `lcr` | LCR — long-context retrieval |
| `tau2` | TAU-bench v2 — tool use and agentic tasks |
| `apexAgents` | APEX Agents |
| `itbenchSre` | ITBench SRE — IT/site-reliability operations |
| `terminalbenchHard` | TerminalBench Hard — CLI tasks |

### Response shapes

**Scrape object** (returned by `/scrapes` and `POST /scrapes`):

```json
{
  "id": 4,
  "scrapedAt": "2025-05-31T12:00:00Z",
  "modelCount": 183
}
```

**Models response** (returned by `GET /models`):

```json
{
  "scrapeId": 4,
  "scrapedAt": "2025-05-31T12:00:00Z",
  "models": [ ...model objects... ]
}
```

**Model object** — key fields:

```json
{
  "id": "claude-sonnet-4-5",
  "slug": "claude-sonnet-4-5",
  "name": "Claude Sonnet 4.5",
  "shortName": "Sonnet 4.5",
  "modelCreatorName": "Anthropic",
  "modelCreatorSlug": "anthropic",

  "intelligenceIndex": 76.4,
  "codingIndex": 71.2,
  "agenticIndex": 68.0,
  "gpqa": 0.718,
  "hle": 0.241,

  "price1mInputTokens": 3.0,
  "price1mOutputTokens": 15.0,
  "cacheHitPrice": 0.3,

  "contextWindowTokens": 200000,
  "totalParameters": null,
  "reasoningModel": false,
  "isOpenWeights": false,
  "deprecated": false,

  "medianOutputTokensPerSecond": 98.4,
  "medianTimeToFirstTokenSeconds": 0.61,

  "inputModalityText": true,
  "inputModalityImage": true,
  "inputModalitySpeech": false,
  "outputModalityText": true
}
```

Nullable fields are `null` when the data isn't available for that model. The full model object
contains ~60 additional fields for pricing blends, token-count breakdowns, openness metadata,
and per-domain omniscience scores.

## MCP Server

SSE endpoint: `http://localhost:8080/mcp/sse`

Connect with any MCP-compatible client (Claude Desktop, Continue, etc.) using the SSE transport.

### Tools

#### `list_models`

Returns models from the latest scrape as a JSON array.

| Parameter | Type | Required | Default | Description |
| --------- | ---- | -------- | ------- | ----------- |
| `sort_by` | string | no | `intelligence_index` | `intelligence_index`, `coding_index`, `price_input`, `price_output`, `name` |
| `creator` | string | no | — | Filter by creator name (case-insensitive substring) |
| `bench` | string | no | — | Filter by benchmark field name (see table above) |
| `min` | number | no | — | Minimum bench score, inclusive |
| `max` | number | no | — | Maximum bench score, inclusive |
| `limit` | integer | no | `50` | Maximum number of results |

#### `get_model`

Returns a single model by slug.

| Parameter | Type | Required | Description |
| --------- | ---- | -------- | ----------- |
| `slug` | string | **yes** | Model slug (e.g. `claude-sonnet-4-5`) |

Returns `"model not found"` if the slug doesn't exist.

#### `search_models`

Case-insensitive substring search across model name and creator name.

| Parameter | Type | Required | Description |
| --------- | ---- | -------- | ----------- |
| `query` | string | **yes** | Search string |

### Resources

| URI | Description |
| --- | ----------- |
| `aa://models` | Full model list from latest scrape as JSON array |
| `aa://models/{slug}` | Single model as JSON (e.g. `aa://models/claude-sonnet-4-5`) |

## Development

```bash
task build     # compile to bin/scrape-aa
task test      # go test -race ./...
task lint      # golangci-lint + govulncheck + nilaway + go-arch-lint
task generate  # sqlc generate (after schema/query changes)
```

Requires
- [Task](https://taskfile.dev)
- [golangci-lint](https://golangci-lint.run)
- [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
- [nilaway](https://github.com/uber-go/nilaway)
- [sqlc](https://sqlc.dev)
- [go-arch-lint](https://github.com/fe3dback/go-arch-lint)
