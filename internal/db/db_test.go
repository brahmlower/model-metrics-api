package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/restrukt-ai/model-metrics-scraper/internal/db"
)

func openMemDB(t *testing.T) *db.Queries {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })

	err = db.Migrate(sqlDB)
	if err != nil {
		t.Fatal(err)
	}

	return db.New(sqlDB)
}

func TestInsertAndQueryScrape(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	inserted, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  now,
		ModelCount: 42,
	})
	if err != nil {
		t.Fatal(err)
	}

	if inserted.ModelCount != 42 {
		t.Errorf("model_count = %d, want 42", inserted.ModelCount)
	}

	latest, err := queries.GetLatestScrape(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if latest.ID != inserted.ID {
		t.Errorf("ID mismatch: got %d, want %d", latest.ID, inserted.ID)
	}
}

func TestInsertAndListModelSnapshots(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now().UTC(),
		ModelCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	snapshots := []db.InsertModelSnapshotParams{
		{
			ScrapeID:          scrape.ID,
			ModelID:           "id-1",
			Slug:              "model-a",
			Name:              "Model A",
			CreatorName:       "Acme",
			IntelligenceIndex: sql.NullFloat64{Float64: 90.0, Valid: true},
			Data:              `{}`,
		},
		{
			ScrapeID:          scrape.ID,
			ModelID:           "id-2",
			Slug:              "model-b",
			Name:              "Model B",
			CreatorName:       "Beta",
			IntelligenceIndex: sql.NullFloat64{Float64: 80.0, Valid: true},
			Data:              `{}`,
		},
		{
			ScrapeID:          scrape.ID,
			ModelID:           "id-3",
			Slug:              "model-c",
			Name:              "Model C",
			CreatorName:       "Corp",
			IntelligenceIndex: sql.NullFloat64{Float64: 70.0, Valid: true},
			Data:              `{}`,
		},
	}

	for _, p := range snapshots {
		err := queries.InsertModelSnapshot(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
	}

	results, err := queries.ListModelsByLatestScrape(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d snapshots, want 3", len(results))
	}

	// Should be ordered by intelligence_index DESC
	if results[0].Slug != "model-a" {
		t.Errorf("first model = %s, want model-a", results[0].Slug)
	}
}

func TestGetModelBySlug(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now().UTC(),
		ModelCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []db.InsertModelSnapshotParams{
		{ScrapeID: scrape.ID, ModelID: "id-1", Slug: "alpha", Name: "Alpha", CreatorName: "A", Data: `{"id":"id-1"}`},
		{ScrapeID: scrape.ID, ModelID: "id-2", Slug: "beta", Name: "Beta", CreatorName: "B", Data: `{"id":"id-2"}`},
	} {
		err := queries.InsertModelSnapshot(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
	}

	snap, err := queries.GetModelBySlug(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}

	if snap.Slug != "alpha" {
		t.Errorf("slug = %s, want alpha", snap.Slug)
	}
}

func TestListScrapes(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	times := []time.Time{
		time.Now().UTC().Add(-2 * time.Hour),
		time.Now().UTC().Add(-1 * time.Hour),
		time.Now().UTC(),
	}

	for _, at := range times {
		_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: at, ModelCount: 1})
		if err != nil {
			t.Fatal(err)
		}
	}

	scrapes, err := queries.ListScrapes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(scrapes) != 3 {
		t.Fatalf("got %d scrapes, want 3", len(scrapes))
	}

	// Should be ordered DESC by scraped_at
	if !scrapes[0].ScrapedAt.After(scrapes[1].ScrapedAt) {
		t.Errorf("expected descending order: scrapes[0]=%v scrapes[1]=%v", scrapes[0].ScrapedAt, scrapes[1].ScrapedAt)
	}
}

func TestListModelsByScrapeID(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	s1, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now().UTC().Add(-time.Hour), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	s2, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now().UTC(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []db.InsertModelSnapshotParams{
		{ScrapeID: s1.ID, ModelID: "old-1", Slug: "old-model", Name: "Old", CreatorName: "A", Data: `{}`},
		{ScrapeID: s2.ID, ModelID: "new-1", Slug: "new-alpha", Name: "New Alpha", CreatorName: "B", Data: `{}`},
		{ScrapeID: s2.ID, ModelID: "new-2", Slug: "new-beta", Name: "New Beta", CreatorName: "C", Data: `{}`},
	} {
		if err := queries.InsertModelSnapshot(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	snaps, err := queries.ListModelsByScrapeID(ctx, s2.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(snaps) != 2 {
		t.Fatalf("got %d snapshots for s2, want 2", len(snaps))
	}

	for _, s := range snaps {
		if s.ScrapeID != s2.ID {
			t.Errorf("snapshot scrape_id = %d, want %d", s.ScrapeID, s2.ID)
		}
	}
}

func TestGetModelBySlugAndScrapeID(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	s1, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now().UTC().Add(-time.Hour), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	s2, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now().UTC(), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Same slug in both scrapes, different data
	for _, p := range []db.InsertModelSnapshotParams{
		{ScrapeID: s1.ID, ModelID: "id-v1", Slug: "my-model", Name: "My Model v1", CreatorName: "A", Data: `{"id":"id-v1"}`},
		{ScrapeID: s2.ID, ModelID: "id-v2", Slug: "my-model", Name: "My Model v2", CreatorName: "A", Data: `{"id":"id-v2"}`},
	} {
		if err := queries.InsertModelSnapshot(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	snap, err := queries.GetModelBySlugAndScrapeID(ctx, db.GetModelBySlugAndScrapeIDParams{
		ScrapeID: s1.ID,
		Slug:     "my-model",
	})
	if err != nil {
		t.Fatal(err)
	}

	if snap.ModelID != "id-v1" {
		t.Errorf("got model_id %s, want id-v1", snap.ModelID)
	}
}

func TestGetScrapeClosestTo(t *testing.T) {
	t.Parallel()

	queries := openMemDB(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	twoHoursAgo := base.Add(-2 * time.Hour)
	oneHourAgo := base.Add(-1 * time.Hour)

	for _, at := range []time.Time{twoHoursAgo, oneHourAgo} {
		_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
			ScrapedAt:  at,
			ModelCount: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	ninetyMinsAgo := base.Add(-90 * time.Minute)

	scrape, err := queries.GetScrapeClosestTo(ctx, ninetyMinsAgo)
	if err != nil {
		t.Fatal(err)
	}

	// Closest scrape at or before T-90m is T-2h
	if !scrape.ScrapedAt.Equal(twoHoursAgo) {
		t.Errorf("scraped_at = %v, want %v", scrape.ScrapedAt, twoHoursAgo)
	}
}
