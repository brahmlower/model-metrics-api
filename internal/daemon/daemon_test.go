package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/restrukt-ai/model-metrics-api/internal/db"
	"github.com/restrukt-ai/model-metrics-api/pkg/aa"
)

var testDBCounter atomic.Int64

type mockScraper struct {
	models []aa.Model
}

func (m *mockScraper) Scrape(_ context.Context) ([]aa.Model, error) {
	return m.models, nil
}

type failingScraper struct{ err error }

func (f *failingScraper) Scrape(_ context.Context) ([]aa.Model, error) {
	return nil, f.err
}

func openTestDB(t *testing.T) (*sql.DB, *db.Queries) {
	t.Helper()

	n := testDBCounter.Add(1)
	dsn := fmt.Sprintf("file:daemontest%d?mode=memory&cache=shared", n)

	sqlDB, err := sql.Open("sqlite", dsn)
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

func TestNew(t *testing.T) {
	t.Parallel()

	sqlDB, queries := openTestDB(t)
	ms := &mockScraper{}

	d := New(ms, queries, sqlDB, 5*time.Minute)

	if d == nil {
		t.Fatal("expected non-nil Daemon")
	}

	if d.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", d.interval)
	}
}

func TestScrapeAndStore(t *testing.T) {
	t.Parallel()

	sqlDB, queries := openTestDB(t)

	f64 := func(v float64) *float64 { return &v }
	ms := &mockScraper{
		models: []aa.Model{
			{ID: "id-1", Slug: "model-1", Name: "Model 1", ModelCreatorName: "Acme", IntelligenceIndex: f64(42.0)},
			{ID: "id-2", Slug: "model-2", Name: "Model 2", ModelCreatorName: "Beta", IntelligenceIndex: f64(10.0)},
		},
	}

	d := New(ms, queries, sqlDB, time.Hour)

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

func TestRunCancels(t *testing.T) {
	t.Parallel()

	sqlDB, queries := openTestDB(t)
	d := New(&mockScraper{}, queries, sqlDB, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := d.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestRunInitialScrapeError(t *testing.T) {
	t.Parallel()

	sqlDB, queries := openTestDB(t)
	scrapeErr := errors.New("network down")
	d := New(&failingScraper{err: scrapeErr}, queries, sqlDB, time.Hour)

	err := d.Run(context.Background())
	if !errors.Is(err, scrapeErr) {
		t.Fatalf("expected scrapeErr, got %v", err)
	}
}
