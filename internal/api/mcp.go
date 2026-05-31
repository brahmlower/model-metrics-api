package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/restrukt-ai/model-metrics-scraper/internal/db"
	"github.com/restrukt-ai/model-metrics-scraper/pkg/aa"
)

const defaultModelLimit = 50

// NewMCPServer creates and configures an MCP server with model data tools and resources.
func NewMCPServer(queries *db.Queries) *server.MCPServer {
	s := server.NewMCPServer("model-metrics-scraper", "1.0.0")

	s.AddTool(
		mcp.NewTool(
			"list_models",
			mcp.WithString(
				"sort_by",
				mcp.Description(
					"Sort field: intelligence_index, coding_index, price_input, price_output, name",
				),
			),
			mcp.WithString(
				"creator",
				mcp.Description("Filter by creator name (case-insensitive substring)"),
			),
			mcp.WithNumber("limit", mcp.Description("Max results to return (default 50)")),
		),
		listModelsHandler(queries),
	)

	s.AddTool(
		mcp.NewTool("get_model",
			mcp.WithString("slug", mcp.Required(), mcp.Description("Model slug")),
		),
		getModelHandler(queries),
	)

	s.AddTool(
		mcp.NewTool(
			"search_models",
			mcp.WithString(
				"query",
				mcp.Required(),
				mcp.Description("Search query for model name or creator"),
			),
		),
		searchModelsHandler(queries),
	)

	s.AddResource(
		mcp.NewResource("aa://models", "All Models",
			mcp.WithMIMEType("application/json"),
			mcp.WithResourceDescription("All models from the latest scrape as JSON"),
		),
		allModelsResource(queries),
	)

	s.AddResourceTemplate(
		mcp.NewResourceTemplate("aa://models/{slug}", "Model by Slug",
			mcp.WithTemplateMIMEType("application/json"),
			mcp.WithTemplateDescription("A single model by slug"),
		),
		modelBySlugResource(queries),
	)

	return s
}

func listModelsHandler(queries *db.Queries) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		snaps, err := queries.ListModelsByLatestScrape(ctx)
		if err != nil {
			return nil, err
		}

		models, err := unmarshalSnapshots(snaps)
		if err != nil {
			return nil, err
		}

		creator := req.GetString("creator", "")
		if creator != "" {
			models = filterByCreator(models, creator)
		}

		sortBy := req.GetString("sort_by", "intelligence_index")
		sortModels(models, sortBy, "desc")

		limit := req.GetInt("limit", defaultModelLimit)
		if limit > 0 && limit < len(models) {
			models = models[:limit]
		}

		b, err := json.Marshal(models)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(b)), nil
	}
}

func getModelHandler(queries *db.Queries) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		slug, err := req.RequireString("slug")
		if err != nil {
			return nil, err
		}

		snap, err := queries.GetModelBySlug(ctx, slug)
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultText("model not found"), nil
		}

		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(snap.Data), nil
	}
}

func searchModelsHandler(queries *db.Queries) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return nil, err
		}

		snaps, err := queries.ListModelsByLatestScrape(ctx)
		if err != nil {
			return nil, err
		}

		models, err := unmarshalSnapshots(snaps)
		if err != nil {
			return nil, err
		}

		filtered := searchByNameOrCreator(models, query)

		b, err := json.Marshal(filtered)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(b)), nil
	}
}

func allModelsResource(queries *db.Queries) server.ResourceHandlerFunc {
	return func(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		snaps, err := queries.ListModelsByLatestScrape(ctx)
		if err != nil {
			return nil, err
		}

		models, err := unmarshalSnapshots(snaps)
		if err != nil {
			return nil, err
		}

		b, err := json.Marshal(models)
		if err != nil {
			return nil, err
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "aa://models",
				MIMEType: "application/json",
				Text:     string(b),
			},
		}, nil
	}
}

func modelBySlugResource(queries *db.Queries) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		slug := strings.TrimPrefix(req.Params.URI, "aa://models/")

		snap, err := queries.GetModelBySlug(ctx, slug)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errModelNotFound
		}

		if err != nil {
			return nil, err
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     snap.Data,
			},
		}, nil
	}
}

func searchByNameOrCreator(models []aa.Model, query string) []aa.Model {
	q := strings.ToLower(query)
	filtered := make([]aa.Model, 0)

	for _, m := range models {
		nameMatch := strings.Contains(strings.ToLower(m.Name), q)
		creatorMatch := strings.Contains(strings.ToLower(m.ModelCreatorName), q)

		if nameMatch || creatorMatch {
			filtered = append(filtered, m)
		}
	}

	return filtered
}
