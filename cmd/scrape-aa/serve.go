package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/restrukt-ai/model-metrics-scraper/internal/api"
	"github.com/restrukt-ai/model-metrics-scraper/internal/daemon"
	dbpkg "github.com/restrukt-ai/model-metrics-scraper/internal/db"
	"github.com/restrukt-ai/model-metrics-scraper/pkg/aa"

	_ "modernc.org/sqlite"
)

const (
	shutdownTimeout   = 10 * time.Second
	headerReadTimeout = 30 * time.Second
)

func newServeCmd() *cobra.Command {
	var addr, dbPath, interval string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run periodic scraping with REST and MCP APIs",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe(addr, dbPath, interval)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	cmd.Flags().StringVar(&dbPath, "db", "./data.db", "SQLite database path")
	cmd.Flags().StringVar(&interval, "interval", "1h", "Scrape interval")

	return cmd
}

func runServe(flagAddr, flagDB, flagInterval string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sqlDB, err := sql.Open("sqlite", flagDB)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	_, err = sqlDB.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON")
	if err != nil {
		return err
	}

	err = dbpkg.Migrate(sqlDB)
	if err != nil {
		return err
	}

	queries := dbpkg.New(sqlDB)

	dur, err := time.ParseDuration(flagInterval)
	if err != nil {
		return err
	}

	scraper := aa.NewScraper()
	d := daemon.New(scraper, queries, sqlDB, dur)

	r := buildRouter(flagAddr, queries, d)
	httpSrv := &http.Server{
		Addr:              flagAddr,
		Handler:           r,
		ReadHeaderTimeout: headerReadTimeout,
	}

	go func() {
		fmt.Fprint(os.Stderr, "starting initial scrape...\n")

		runErr := d.Run(ctx)
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			fmt.Fprintf(os.Stderr, "daemon run error: %v\n", runErr)
		}
	}()

	go func() {
		<-ctx.Done()

		shutCtx, c := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer c()

		shutErr := httpSrv.Shutdown(shutCtx)
		if shutErr != nil {
			fmt.Fprintf(os.Stderr, "shutdown error: %v\n", shutErr)
		}
	}()

	fmt.Fprintf(os.Stderr, "listening on %s\n", flagAddr)

	err = httpSrv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func buildRouter(addr string, queries *dbpkg.Queries, d *daemon.Daemon) http.Handler {
	r := chi.NewRouter()
	r.Mount("/", api.NewRouter(queries, d))

	mcpSrv := api.NewMCPServer(queries)
	sseSrv := mcpserver.NewSSEServer(mcpSrv,
		mcpserver.WithBaseURL("http://"+addr),
		mcpserver.WithStaticBasePath("/mcp"),
	)

	r.Handle("/mcp/sse", sseSrv)
	r.Handle("/mcp/message", sseSrv)

	return r
}
