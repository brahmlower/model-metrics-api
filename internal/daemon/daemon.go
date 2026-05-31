// Package daemon provides the periodic scrape-and-store loop.
package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/restrukt-ai/model-metrics-scraper/internal/db"
	"github.com/restrukt-ai/model-metrics-scraper/pkg/aa"
)

// scraper is the interface Daemon uses to fetch model data.
// Using an interface allows test code to inject a mock scraper.
type scraper interface {
	Scrape(ctx context.Context) ([]aa.Model, error)
}

// Daemon runs periodic model scraping and persists results to the database.
type Daemon struct {
	scraper  scraper
	queries  *db.Queries
	sqlDB    *sql.DB
	interval time.Duration
}

// New creates a Daemon that scrapes on the given interval.
func New(s *aa.Scraper, queries *db.Queries, sqlDB *sql.DB, interval time.Duration) *Daemon {
	return &Daemon{
		scraper:  s,
		queries:  queries,
		sqlDB:    sqlDB,
		interval: interval,
	}
}

// Run scrapes immediately on start, then on each interval tick until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	_, err := d.ScrapeAndStore(ctx)
	if err != nil {
		return fmt.Errorf("initial scrape: %w", err)
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, err := d.ScrapeAndStore(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "periodic scrape error: %v\n", err)
			}
		}
	}
}

// ScrapeAndStore performs one scrape cycle and persists all results to the database.
func (d *Daemon) ScrapeAndStore(ctx context.Context) (db.Scrape, error) {
	models, err := d.scraper.Scrape(ctx)
	if err != nil {
		return db.Scrape{}, fmt.Errorf("scrape: %w", err)
	}

	scrape, err := d.queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now().UTC(),
		ModelCount: int64(len(models)),
	})
	if err != nil {
		return db.Scrape{}, fmt.Errorf("insert scrape: %w", err)
	}

	for _, m := range models {
		err := d.insertSnapshot(ctx, scrape.ID, m)
		if err != nil {
			return db.Scrape{}, err
		}
	}

	return scrape, nil
}

func (d *Daemon) insertSnapshot(ctx context.Context, scrapeID int64, m aa.Model) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal model %s: %w", m.ID, err)
	}

	return d.queries.InsertModelSnapshot(ctx, db.InsertModelSnapshotParams{
		ScrapeID:          scrapeID,
		ModelID:           m.ID,
		Slug:              m.Slug,
		Name:              m.Name,
		CreatorName:       m.ModelCreatorName,
		IntelligenceIndex: nullFloat64(m.IntelligenceIndex),
		CodingIndex:       nullFloat64(m.CodingIndex),
		PriceInput:        nullFloat64(m.Price1MInputTokens),
		PriceOutput:       nullFloat64(m.Price1MOutputTokens),
		Data:              string(data),
	})
}

func nullFloat64(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}

	return sql.NullFloat64{Float64: *p, Valid: true}
}
