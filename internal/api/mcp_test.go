package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/restrukt-ai/model-metrics-api/internal/db"
	"github.com/restrukt-ai/model-metrics-api/pkg/aa"
	_ "modernc.org/sqlite"
)

func callTool(t *testing.T, queries *db.Queries, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()

	handler := listModelsHandler(queries)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = args

	return handler(context.Background(), req)
}

func decodeModels(t *testing.T, result *mcp.CallToolResult) []aa.Model {
	t.Helper()

	tc, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("expected TextContent in tool result")
	}

	var models []aa.Model

	if err := json.Unmarshal([]byte(tc.Text), &models); err != nil {
		t.Fatal(err)
	}

	return models
}

func TestListModelsHandlerBenchMin(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "high", "High", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "low", "Low", "Org", 0.30)

	result, err := callTool(t, queries, map[string]any{"bench": "gpqa", "min": 0.5})
	if err != nil {
		t.Fatal(err)
	}

	models := decodeModels(t, result)

	if len(models) != 1 || models[0].Slug != "high" {
		t.Fatalf("expected only high, got %v", models)
	}
}

func TestListModelsHandlerBenchMax(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "high", "High", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "low", "Low", "Org", 0.30)

	result, err := callTool(t, queries, map[string]any{"bench": "gpqa", "max": 0.5})
	if err != nil {
		t.Fatal(err)
	}

	models := decodeModels(t, result)

	if len(models) != 1 || models[0].Slug != "low" {
		t.Fatalf("expected only low, got %v", models)
	}
}

func TestListModelsHandlerBenchRange(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "top", "Top", "Org", 0.95)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "mid", "Mid", "Org", 0.60)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "bot", "Bot", "Org", 0.20)

	result, err := callTool(t, queries, map[string]any{"bench": "gpqa", "min": 0.5, "max": 0.8})
	if err != nil {
		t.Fatal(err)
	}

	models := decodeModels(t, result)

	if len(models) != 1 || models[0].Slug != "mid" {
		t.Fatalf("expected only mid, got %v", models)
	}
}

func TestListModelsHandlerBenchNoBoundsExcludesNil(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "scored-a", "A", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "scored-b", "B", "Org", 0.50)
	insertTestModel(ctx, t, queries, scrape.ID, "no-score", "No Score", "Org")

	result, err := callTool(t, queries, map[string]any{"bench": "gpqa"})
	if err != nil {
		t.Fatal(err)
	}

	models := decodeModels(t, result)

	if len(models) != 2 {
		t.Fatalf("expected 2 models (nil score excluded), got %d", len(models))
	}
}

func TestListModelsHandlerUnknownBench(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = callTool(t, queries, map[string]any{"bench": "notABench"})
	if err == nil {
		t.Fatal("expected error for unknown bench")
	}
}

func TestGetModelHandler(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "claude-test", "Claude Test", "Anthropic")

	handler := getModelHandler(queries)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"slug": "claude-test"}

	result, err := handler(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	tc, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("expected TextContent")
	}

	var m aa.Model

	err = json.Unmarshal([]byte(tc.Text), &m)
	if err != nil {
		t.Fatal(err)
	}

	if m.Slug != "claude-test" {
		t.Errorf("slug = %s, want claude-test", m.Slug)
	}
}

func TestGetModelHandlerNotFound(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 0})
	if err != nil {
		t.Fatal(err)
	}

	handler := getModelHandler(queries)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"slug": "does-not-exist"}

	result, err := handler(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	tc, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("expected TextContent")
	}

	if tc.Text != "model not found" {
		t.Errorf("text = %q, want 'model not found'", tc.Text)
	}
}

func TestSearchModelsHandler(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "gpt-5", "GPT-5", "OpenAI")
	insertTestModel(ctx, t, queries, scrape.ID, "claude-5", "Claude 5", "Anthropic")

	handler := searchModelsHandler(queries)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": "claude"}

	result, err := handler(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	tc, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("expected TextContent")
	}

	var models []aa.Model

	err = json.Unmarshal([]byte(tc.Text), &models)
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 1 || models[0].Slug != "claude-5" {
		t.Fatalf("expected only claude-5, got %v", models)
	}
}

func TestAllModelsResource(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "model-a", "Model A", "Org")
	insertTestModel(ctx, t, queries, scrape.ID, "model-b", "Model B", "Org")

	handler := allModelsResource(queries)

	contents, err := handler(ctx, mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}

	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}

	if tc.URI != "aa://models" {
		t.Errorf("URI = %s, want aa://models", tc.URI)
	}

	var models []aa.Model

	err = json.Unmarshal([]byte(tc.Text), &models)
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
}

func TestModelBySlugResource(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 1})
	if err != nil {
		t.Fatal(err)
	}

	insertTestModel(ctx, t, queries, scrape.ID, "my-model", "My Model", "Org")

	handler := modelBySlugResource(queries)

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "aa://models/my-model"

	contents, err := handler(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}

	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}

	var m aa.Model

	err = json.Unmarshal([]byte(tc.Text), &m)
	if err != nil {
		t.Fatal(err)
	}

	if m.Slug != "my-model" {
		t.Errorf("slug = %s, want my-model", m.Slug)
	}
}

func TestModelBySlugResourceNotFound(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	_, err := queries.InsertScrape(ctx, db.InsertScrapeParams{ScrapedAt: time.Now(), ModelCount: 0})
	if err != nil {
		t.Fatal(err)
	}

	handler := modelBySlugResource(queries)

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "aa://models/does-not-exist"

	_, err = handler(ctx, req)
	if !errors.Is(err, errModelNotFound) {
		t.Errorf("expected errModelNotFound, got %v", err)
	}
}

func TestListModelsHandlerBenchWithLimit(t *testing.T) {
	t.Parallel()

	_, queries := newTestDB(t)
	ctx := context.Background()

	scrape, err := queries.InsertScrape(ctx, db.InsertScrapeParams{
		ScrapedAt:  time.Now(),
		ModelCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	insertModelWithGpqa(ctx, t, queries, scrape.ID, "a", "A", "Org", 0.90)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "b", "B", "Org", 0.80)
	insertModelWithGpqa(ctx, t, queries, scrape.ID, "c", "C", "Org", 0.70)

	result, err := callTool(t, queries, map[string]any{"bench": "gpqa", "min": 0.5, "limit": 2})
	if err != nil {
		t.Fatal(err)
	}

	models := decodeModels(t, result)

	if len(models) != 2 {
		t.Fatalf("expected 2 models (limit applied), got %d", len(models))
	}
}
