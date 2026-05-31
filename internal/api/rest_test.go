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
	"github.com/restrukt-ai/model-metrics-scraper/internal/db"
	"github.com/restrukt-ai/model-metrics-scraper/pkg/aa"
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
