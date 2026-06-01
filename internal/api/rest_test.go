package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/restrukt-ai/model-metrics-api/internal/db"
	"github.com/restrukt-ai/model-metrics-api/pkg/aa"
	_ "modernc.org/sqlite"
)

var testDBCounter atomic.Int64

func newTestDB(t *testing.T) (*sql.DB, *db.Queries) {
	t.Helper()

	n := testDBCounter.Add(1)
	dsn := fmt.Sprintf("file:testdb%d?mode=memory&cache=shared", n)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { sqlDB.Close() })

	schema := `
CREATE TABLE IF NOT EXISTS scrapes (
    id          INTEGER  PRIMARY KEY AUTOINCREMENT,
    scraped_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    model_count INTEGER  NOT NULL
);
CREATE TABLE IF NOT EXISTS model_snapshots (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    scrape_id          INTEGER NOT NULL REFERENCES scrapes(id) ON DELETE CASCADE,
    model_id           TEXT    NOT NULL,
    slug               TEXT    NOT NULL,
    name               TEXT    NOT NULL,
    creator_name       TEXT    NOT NULL,
    intelligence_index REAL,
    coding_index       REAL,
    price_input        REAL,
    price_output       REAL,
    data               TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_scrape ON model_snapshots(scrape_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_slug   ON model_snapshots(slug);
`

	_, err = sqlDB.ExecContext(context.Background(), schema)
	if err != nil {
		t.Fatal(err)
	}

	return sqlDB, db.New(sqlDB)
}

type mockDaemon struct {
	sqlDB   *sql.DB
	queries *db.Queries
}

func (m *mockDaemon) ScrapeAndStore(ctx context.Context) (db.Scrape, error) {
	return m.queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 0,
	})
}

func newTestRouter(t *testing.T, sqlDB *sql.DB, queries *db.Queries) http.Handler {
	t.Helper()

	r := chi.NewRouter()
	r.Mount("/", NewRouter(queries, &mockDaemon{sqlDB: sqlDB, queries: queries}))

	return r
}

func TestListScrapes(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	router := newTestRouter(t, sqlDB, queries)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/scrapes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var scrapes []db.Scrape

	err = json.NewDecoder(rec.Body).Decode(&scrapes)
	if err != nil {
		t.Fatal(err)
	}

	if len(scrapes) != 1 {
		t.Fatalf("expected 1 scrape, got %d", len(scrapes))
	}
}

func TestListModels(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(
		ctx,
		db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2},
	)
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "model-a", "Model A", "Acme")
	insertTestModel(ctx, t, queries, scrape.ID, "model-b", "Model B", "Beta")

	router := newTestRouter(t, sqlDB, queries)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Models))
	}
}

func TestGetModelBySlug(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(
		ctx,
		db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 1},
	)
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "test-model", "Test Model", "Acme")

	router := newTestRouter(t, sqlDB, queries)

	req := httptest.NewRequestWithContext(
		ctx, http.MethodGet, "/api/v1/models/test-model", nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var m aa.Model

	err = json.NewDecoder(rec.Body).Decode(&m)
	if err != nil {
		t.Fatal(err)
	}

	if m.Slug != "test-model" {
		t.Fatalf("expected slug test-model, got %s", m.Slug)
	}
}

func TestGetModelBySlugNotFound(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	router := newTestRouter(t, sqlDB, queries)

	req := httptest.NewRequestWithContext(
		ctx, http.MethodGet, "/api/v1/models/nonexistent", nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTriggerScrape(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/scrapes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var scrape db.Scrape

	err := json.NewDecoder(rec.Body).Decode(&scrape)
	if err != nil {
		t.Fatal(err)
	}

	if scrape.ID == 0 {
		t.Fatal("expected non-zero scrape ID")
	}
}

func insertTestModel(
	ctx context.Context,
	t *testing.T,
	queries *db.Queries,
	scrapeID int64,
	slug, name, creator string,
) {
	t.Helper()

	m := aa.Model{
		ID:               "id-" + slug,
		Slug:             slug,
		Name:             name,
		ModelCreatorName: creator,
	}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	err = queries.InsertModelSnapshot(ctx, db.InsertModelSnapshotParams{
		ScrapeID:          scrapeID,
		ModelID:           m.ID,
		Slug:              m.Slug,
		Name:              m.Name,
		CreatorName:       m.ModelCreatorName,
		IntelligenceIndex: sql.NullFloat64{},
		CodingIndex:       sql.NullFloat64{},
		PriceInput:        sql.NullFloat64{},
		PriceOutput:       sql.NullFloat64{},
		Data:              string(b),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func insertFullModel(
	ctx context.Context,
	t *testing.T,
	queries *db.Queries,
	scrapeID int64,
	m aa.Model,
) {
	t.Helper()

	nullf := func(p *float64) sql.NullFloat64 {
		if p == nil {
			return sql.NullFloat64{}
		}

		return sql.NullFloat64{Float64: *p, Valid: true}
	}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	err = queries.InsertModelSnapshot(ctx, db.InsertModelSnapshotParams{
		ScrapeID:          scrapeID,
		ModelID:           m.ID,
		Slug:              m.Slug,
		Name:              m.Name,
		CreatorName:       m.ModelCreatorName,
		IntelligenceIndex: nullf(m.IntelligenceIndex),
		CodingIndex:       nullf(m.CodingIndex),
		PriceInput:        nullf(m.Price1MInputTokens),
		PriceOutput:       nullf(m.Price1MOutputTokens),
		Data:              string(b),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetLatestScrape(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 3})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/scrapes/latest", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var scrape db.Scrape

	err = json.NewDecoder(rec.Body).Decode(&scrape)
	if err != nil {
		t.Fatal(err)
	}

	if scrape.ModelCount != 3 {
		t.Errorf("model_count = %d, want 3", scrape.ModelCount)
	}
}

func TestGetLatestScrapeEmpty(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/scrapes/latest", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListModelsByScrapeID(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	s1, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now().Add(-time.Hour), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	s2, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, s1.ID, "old-model", "Old Model", "Org")
	insertTestModel(ctx, t, queries, s2.ID, "new-model", "New Model", "Org")

	url := fmt.Sprintf("/api/v1/models?scrape_id=%d", s1.ID)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Models) != 1 || resp.Models[0].Slug != "old-model" {
		t.Fatalf("expected only old-model, got %v", resp.Models)
	}

	if resp.ScrapeID != s1.ID {
		t.Errorf("scrape_id = %d, want %d", resp.ScrapeID, s1.ID)
	}
}

func TestListModelsAsOf(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	// s1 = T-2h (older), s2 = T-1h (newer).
	// Store and query in UTC so SQLite string comparison works correctly.
	now := time.Now().UTC().Truncate(time.Second)

	s1, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: now.Add(-2 * time.Hour), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	_, err = queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: now.Add(-time.Hour), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, s1.ID, "old-model", "Old Model", "Org")

	asOf := now.Add(-90 * time.Minute).Format(time.RFC3339)
	url := fmt.Sprintf("/api/v1/models?as_of=%s", asOf)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if resp.ScrapeID != s1.ID {
		t.Errorf("scrape_id = %d, want %d (s1)", resp.ScrapeID, s1.ID)
	}
}

func TestListModelsCreatorFilter(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "claude-4", "Claude 4", "Anthropic")
	insertTestModel(ctx, t, queries, scrape.ID, "gemini-2", "Gemini 2", "Google")

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?creator=anthropic", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Models) != 1 || resp.Models[0].Slug != "claude-4" {
		t.Fatalf("expected only claude-4, got %v", resp.Models)
	}
}

func TestListModelsSortByName(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "zephyr", "Zephyr", "Org")
	insertTestModel(ctx, t, queries, scrape.ID, "alpha", "Alpha", "Org")

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?sort_by=name&order=asc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Models))
	}

	if resp.Models[0].Name != "Alpha" {
		t.Errorf("first model = %s, want Alpha", resp.Models[0].Name)
	}
}

func TestListModelsSortByCodingIndex(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	p := func(v float64) *float64 { return &v }

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	insertFullModel(ctx, t, queries, scrape.ID, aa.Model{ID: "a", Slug: "low", Name: "Low Coder", ModelCreatorName: "Org", CodingIndex: p(20.0)})
	insertFullModel(ctx, t, queries, scrape.ID, aa.Model{ID: "b", Slug: "high", Name: "High Coder", ModelCreatorName: "Org", CodingIndex: p(90.0)})

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?sort_by=coding_index&order=desc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Models[0].Slug != "high" {
		t.Errorf("first model = %s, want high", resp.Models[0].Slug)
	}
}

func TestListModelsSortByPrice(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	p := func(v float64) *float64 { return &v }

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	insertFullModel(ctx, t, queries, scrape.ID, aa.Model{ID: "a", Slug: "cheap", Name: "Cheap", ModelCreatorName: "Org", Price1MInputTokens: p(0.50)})
	insertFullModel(ctx, t, queries, scrape.ID, aa.Model{ID: "b", Slug: "pricey", Name: "Pricey", ModelCreatorName: "Org", Price1MInputTokens: p(5.00)})

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?sort_by=price_input&order=asc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Models[0].Slug != "cheap" {
		t.Errorf("first model = %s, want cheap", resp.Models[0].Slug)
	}
}

func TestGetModelByScrapeID(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	s1, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now().Add(-time.Hour), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	s2, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, s1.ID, "my-model", "My Model v1", "Org")
	insertTestModel(ctx, t, queries, s2.ID, "my-model", "My Model v2", "Org")

	url := fmt.Sprintf("/api/v1/models/my-model?scrape_id=%d", s1.ID)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var m aa.Model

	err = json.NewDecoder(rec.Body).Decode(&m)
	if err != nil {
		t.Fatal(err)
	}

	if m.Name != "My Model v1" {
		t.Errorf("name = %s, want 'My Model v1'", m.Name)
	}
}

func TestBenchField(t *testing.T) {
	t.Parallel()

	v := 0.75
	p := &v

	cases := []struct {
		name  string
		model aa.Model
	}{
		{"agenticIndex", aa.Model{AgenticIndex: p}},
		{"apexAgents", aa.Model{ApexAgents: p}},
		{"codingIndex", aa.Model{CodingIndex: p}},
		{"critpt", aa.Model{Critpt: p}},
		{"gdpvalNormalized", aa.Model{GdpvalNormalized: p}},
		{"gpqa", aa.Model{Gpqa: p}},
		{"hle", aa.Model{Hle: p}},
		{"ifbench", aa.Model{Ifbench: p}},
		{"intelligenceIndex", aa.Model{IntelligenceIndex: p}},
		{"itbenchSre", aa.Model{ItbenchSre: p}},
		{"lcr", aa.Model{Lcr: p}},
		{"mmmuPro", aa.Model{MmmuPro: p}},
		{"omniscience", aa.Model{Omniscience: p}},
		{"scicode", aa.Model{Scicode: p}},
		{"tau2", aa.Model{Tau2: p}},
		{"terminalbenchHard", aa.Model{TerminalbenchHard: p}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := benchField(tc.model, tc.name)
			if !ok {
				t.Fatalf("expected ok=true for bench %q", tc.name)
			}

			if got == nil || *got != v {
				t.Fatalf("expected %v, got %v", v, got)
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()

		_, ok := benchField(aa.Model{}, "notABench")
		if ok {
			t.Fatal("expected ok=false for unknown bench")
		}
	})
}

func TestFilterByBench(t *testing.T) {
	t.Parallel()

	p := func(v float64) *float64 { return &v }

	models := []aa.Model{
		{Slug: "a", Gpqa: p(0.90)},
		{Slug: "b", Gpqa: p(0.50)},
		{Slug: "c", Gpqa: nil},
		{Slug: "d", Gpqa: p(0.30)},
	}

	t.Run("unknown bench", func(t *testing.T) {
		t.Parallel()

		_, ok := filterByBench(models, "notABench", nil, nil)
		if ok {
			t.Fatal("expected ok=false for unknown bench")
		}
	})

	t.Run("empty models unknown bench", func(t *testing.T) {
		t.Parallel()

		_, ok := filterByBench([]aa.Model{}, "notABench", nil, nil)
		if ok {
			t.Fatal("expected ok=false for unknown bench with empty slice")
		}
	})

	t.Run("no bounds excludes nil scores", func(t *testing.T) {
		t.Parallel()

		got, ok := filterByBench(models, "gpqa", nil, nil)
		if !ok {
			t.Fatal("expected ok=true")
		}

		if len(got) != 3 {
			t.Fatalf("expected 3 models (non-nil gpqa), got %d", len(got))
		}
	})

	t.Run("min filter", func(t *testing.T) {
		t.Parallel()

		got, ok := filterByBench(models, "gpqa", p(0.60), nil)
		if !ok {
			t.Fatal("expected ok=true")
		}

		if len(got) != 1 || got[0].Slug != "a" {
			t.Fatalf("expected only model-a, got %v", got)
		}
	})

	t.Run("max filter", func(t *testing.T) {
		t.Parallel()

		got, ok := filterByBench(models, "gpqa", nil, p(0.50))
		if !ok {
			t.Fatal("expected ok=true")
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 models, got %d", len(got))
		}
	})

	t.Run("min and max range", func(t *testing.T) {
		t.Parallel()

		got, ok := filterByBench(models, "gpqa", p(0.40), p(0.60))
		if !ok {
			t.Fatal("expected ok=true")
		}

		if len(got) != 1 || got[0].Slug != "b" {
			t.Fatalf("expected only model-b, got %v", got)
		}
	})

	t.Run("boundaries are inclusive", func(t *testing.T) {
		t.Parallel()

		// min=0.50, max=0.90: a(0.90) and b(0.50) included; d(0.30) excluded; c(nil) excluded
		got, ok := filterByBench(models, "gpqa", p(0.50), p(0.90))
		if !ok {
			t.Fatal("expected ok=true")
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 models (a and b), got %d", len(got))
		}
	})
}

func TestListModelsWithBench(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "model-a", "Model A", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "model-b", "Model B", "Org", 0.40)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?bench=gpqa&min=0.6", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Models) != 1 || resp.Models[0].Slug != "model-a" {
		t.Fatalf("expected only model-a, got %v", resp.Models)
	}
}

func TestListModelsWithBenchMaxOnly(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "model-a", "Model A", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "model-b", "Model B", "Org", 0.40)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?bench=gpqa&max=0.5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Models) != 1 || resp.Models[0].Slug != "model-b" {
		t.Fatalf("expected only model-b, got %v", resp.Models)
	}
}

func TestListModelsWithBenchNoBounds(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "model-a", "Model A", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "model-b", "Model B", "Org", 0.40)
	// model-c has no gpqa score
	insertTestModel(ctx, t, queries, scrape.ID, "model-c", "Model C", "Org")

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?bench=gpqa", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp modelsResponse

	err = json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatal(err)
	}

	// model-c has nil gpqa so should be excluded
	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models (nil score excluded), got %d", len(resp.Models))
	}
}

func TestListModelsWithInvalidMin(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?bench=gpqa&min=notanumber", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListModelsWithInvalidMax(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?bench=gpqa&max=notanumber", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListModelsWithUnknownBench(t *testing.T) {
	t.Parallel()

	sqlDB, queries := newTestDB(t)
	ctx := context.Background()
	router := newTestRouter(t, sqlDB, queries)

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/models?bench=notABench", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func insertModelWithGpqa(
	ctx context.Context,
	t *testing.T,
	queries *db.Queries,
	scrapeID int64,
	slug, name, creator string,
	gpqa float64,
) {
	t.Helper()

	m := aa.Model{
		ID:               "id-" + slug,
		Slug:             slug,
		Name:             name,
		ModelCreatorName: creator,
		Gpqa:             &gpqa,
	}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	err = queries.InsertModelSnapshot(ctx, db.InsertModelSnapshotParams{
		ScrapeID:          scrapeID,
		ModelID:           m.ID,
		Slug:              m.Slug,
		Name:              m.Name,
		CreatorName:       m.ModelCreatorName,
		IntelligenceIndex: sql.NullFloat64{},
		CodingIndex:       sql.NullFloat64{},
		PriceInput:        sql.NullFloat64{},
		PriceOutput:       sql.NullFloat64{},
		Data:              string(b),
	})
	if err != nil {
		t.Fatal(err)
	}
}
