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
CREATE INDEX IF NOT EXISTS idx_snapshots_intel  ON model_snapshots(intelligence_index DESC);
