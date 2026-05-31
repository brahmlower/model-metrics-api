// Package api provides REST and MCP HTTP handlers for model data.
package api

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/restrukt-ai/model-metrics-scraper/internal/db"
	"github.com/restrukt-ai/model-metrics-scraper/pkg/aa"
)

var (
	errModelNotFound   = errors.New("model not found")
	errNoScrapes       = errors.New("no scrapes found")
	errInvalidScrapeID = errors.New("invalid scrape_id")
	errInvalidAsOf     = errors.New("invalid as_of")
	errUnmarshalModel  = errors.New("unmarshal model")
)

type daemonScraper interface {
	ScrapeAndStore(ctx context.Context) (db.Scrape, error)
}

type modelsResponse struct {
	ScrapeID  int64      `json:"scrapeId"`
	ScrapedAt time.Time  `json:"scrapedAt"`
	Models    []aa.Model `json:"models"`
}

// NewRouter creates an [http.Handler] with REST routes mounted at /api/v1.
func NewRouter(queries *db.Queries, d daemonScraper) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/scrapes", handleListScrapes(queries))
		r.Get("/scrapes/latest", handleGetLatestScrape(queries))
		r.Post("/scrapes", handleTriggerScrape(d))
		r.Get("/models", handleListModels(queries))
		r.Get("/models/{slug}", handleGetModel(queries))
	})

	return r
}

func handleListScrapes(queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scrapes, err := queries.ListScrapes(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, scrapes)
	}
}

func handleGetLatestScrape(queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scrape, err := queries.GetLatestScrape(r.Context())
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errNoScrapes)

			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, scrape)
	}
}

func handleTriggerScrape(d daemonScraper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scrape, err := d.ScrapeAndStore(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, scrape)
	}
}

func handleListModels(queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scrape, err := resolveScrape(r, queries)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, modelsResponse{Models: make([]aa.Model, 0)})

			return
		}

		if err != nil {
			writeError(w, http.StatusBadRequest, err)

			return
		}

		snaps, err := queries.ListModelsByScrapeID(r.Context(), scrape.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		models, err := unmarshalSnapshots(snaps)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		models = applyFiltersAndSort(r, models)
		resp := modelsResponse{
			ScrapeID:  scrape.ID,
			ScrapedAt: scrape.ScrapedAt,
			Models:    models,
		}

		writeJSON(w, resp)
	}
}

func handleGetModel(queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")

		snap, err := resolveModelSnap(r, queries, slug)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errModelNotFound)

			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		var m aa.Model

		err = json.Unmarshal([]byte(snap.Data), &m)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, m)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, code int, err error) {
	http.Error(w, err.Error(), code)
}

func resolveScrape(r *http.Request, queries *db.Queries) (db.Scrape, error) {
	if sid := r.URL.Query().Get("scrape_id"); sid != "" {
		id, err := strconv.ParseInt(sid, 10, 64)
		if err != nil {
			return db.Scrape{}, errInvalidScrapeID
		}

		return getScrapeByID(r.Context(), queries, id)
	}

	if asOf := r.URL.Query().Get("as_of"); asOf != "" {
		t, err := time.Parse(time.RFC3339, asOf)
		if err != nil {
			return db.Scrape{}, errInvalidAsOf
		}

		return queries.GetScrapeClosestTo(r.Context(), t)
	}

	return queries.GetLatestScrape(r.Context())
}

func getScrapeByID(ctx context.Context, queries *db.Queries, id int64) (db.Scrape, error) {
	scrapes, err := queries.ListScrapes(ctx)
	if err != nil {
		return db.Scrape{}, err
	}

	for _, s := range scrapes {
		if s.ID == id {
			return s, nil
		}
	}

	return db.Scrape{}, sql.ErrNoRows
}

func resolveModelSnap(
	r *http.Request,
	queries *db.Queries,
	slug string,
) (db.ModelSnapshot, error) {
	if sid := r.URL.Query().Get("scrape_id"); sid != "" {
		id, err := strconv.ParseInt(sid, 10, 64)
		if err != nil {
			return db.ModelSnapshot{}, errInvalidScrapeID
		}

		return queries.GetModelBySlugAndScrapeID(r.Context(), db.GetModelBySlugAndScrapeIDParams{
			ScrapeID: id,
			Slug:     slug,
		})
	}

	return queries.GetModelBySlug(r.Context(), slug)
}

func applyFiltersAndSort(r *http.Request, models []aa.Model) []aa.Model {
	if creator := r.URL.Query().Get("creator"); creator != "" {
		models = filterByCreator(models, creator)
	}

	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "intelligence_index"
	}

	order := r.URL.Query().Get("order")
	if order == "" {
		order = "desc"
	}

	sortModels(models, sortBy, order)

	return models
}

func unmarshalSnapshots(snaps []db.ModelSnapshot) ([]aa.Model, error) {
	models := make([]aa.Model, 0, len(snaps))

	for _, s := range snaps {
		var m aa.Model

		err := json.Unmarshal([]byte(s.Data), &m)
		if err != nil {
			return nil, fmt.Errorf("%w %s: %w", errUnmarshalModel, s.Slug, err)
		}

		models = append(models, m)
	}

	return models, nil
}

func filterByCreator(models []aa.Model, creator string) []aa.Model {
	creator = strings.ToLower(creator)
	filtered := make([]aa.Model, 0)

	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ModelCreatorName), creator) {
			filtered = append(filtered, m)
		}
	}

	return filtered
}

func sortModels(models []aa.Model, by, order string) {
	dir := 1
	if order == "desc" {
		dir = -1
	}

	slices.SortFunc(models, func(a, b aa.Model) int {
		return dir * cmpModelField(a, b, by)
	})
}

func cmpModelField(a, b aa.Model, by string) int {
	switch by {
	case "coding_index":
		return cmpNullableFloat(a.CodingIndex, b.CodingIndex)
	case "price_input":
		return cmpNullableFloat(a.Price1MInputTokens, b.Price1MInputTokens)
	case "price_output":
		return cmpNullableFloat(a.Price1MOutputTokens, b.Price1MOutputTokens)
	case "name":
		return cmp.Compare(a.Name, b.Name)
	default:
		return cmpNullableFloat(a.IntelligenceIndex, b.IntelligenceIndex)
	}
}

func cmpNullableFloat(a, b *float64) int {
	av, bv := 0.0, 0.0

	if a != nil {
		av = *a
	}

	if b != nil {
		bv = *b
	}

	return cmp.Compare(av, bv)
}
