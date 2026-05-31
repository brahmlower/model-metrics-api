-- name: InsertScrape :one
INSERT INTO scrapes (scraped_at, model_count) VALUES (?, ?) RETURNING *;

-- name: ListScrapes :many
SELECT * FROM scrapes ORDER BY scraped_at DESC;

-- name: GetLatestScrape :one
SELECT * FROM scrapes ORDER BY scraped_at DESC LIMIT 1;

-- name: InsertModelSnapshot :exec
INSERT INTO model_snapshots
    (scrape_id, model_id, slug, name, creator_name,
     intelligence_index, coding_index, price_input, price_output, data)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListModelsByLatestScrape :many
SELECT ms.* FROM model_snapshots ms
WHERE ms.scrape_id = (SELECT id FROM scrapes ORDER BY scraped_at DESC LIMIT 1)
ORDER BY ms.intelligence_index DESC;

-- name: ListModelsByScrapeID :many
SELECT * FROM model_snapshots WHERE scrape_id = ? ORDER BY intelligence_index DESC;

-- name: GetModelBySlug :one
SELECT ms.* FROM model_snapshots ms
WHERE ms.slug = ?
  AND ms.scrape_id = (SELECT id FROM scrapes ORDER BY scraped_at DESC LIMIT 1);

-- name: GetModelBySlugAndScrapeID :one
SELECT * FROM model_snapshots WHERE scrape_id = ? AND slug = ?;

-- name: GetScrapeClosestTo :one
SELECT * FROM scrapes WHERE scraped_at <= ? ORDER BY scraped_at DESC LIMIT 1;
