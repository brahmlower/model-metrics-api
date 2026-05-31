package daemon

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/restrukt-ai/model-metrics-scraper/internal/db"
	"github.com/restrukt-ai/model-metrics-scraper/pkg/aa"
)

type mockScraper struct {
	models []aa.Model
}

func (m *mockScraper) Scrape(_ context.Context) ([]aa.Model, error) {
	return m.models, nil
}

func openTestDB(t *testing.T) (*sql.DB, *db.Queries) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })

	err = db.Migrate(sqlDB)
	if err != nil {
		t.Fatal(err)
	}

	return sqlDB, db.New(sqlDB)
}

func TestScrapeAndStore(t *testing.T) {
	t.Parallel()

	sqlDB, queries := openTestDB(t)

	f64 := func(v float64) *float64 { return &v }
	ms := &mockScraper{
		models: []aa.Model{
			{
				ID:                "id-1",
				Slug:              "model-1",
				Name:              "Model 1",
				ModelCreatorName:  "Acme",
				IntelligenceIndex: f64(42.0),
			},
			{
				ID:                "id-2",
				Slug:              "model-2",
				Name:              "Model 2",
				ModelCreatorName:  "Beta",
				IntelligenceIndex: f64(10.0),
			},
		},
	}

	d := &Daemon{
		scraper:  ms,
		queries:  queries,
		sqlDB:    sqlDB,
		interval: time.Hour,
	}

	scrape, err := d.ScrapeAndStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if scrape.ModelCount != 2 {
		t.Errorf("model_count = %d, want 2", scrape.ModelCount)
	}

	snaps, err := queries.ListModelsByScrapeID(context.Background(), scrape.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(snaps) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(snaps))
	}
}
