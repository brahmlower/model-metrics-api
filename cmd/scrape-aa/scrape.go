package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/brahmlower/model-metrics-api/pkg/aa"
	"github.com/spf13/cobra"
)

const top5Count = 5

func newScrapeCmd() *cobra.Command {
	var out string

	cmd := &cobra.Command{
		Use:   "scrape",
		Short: "Fetch models and write to a JSON file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScrape(cmd, out)
		},
	}
	cmd.Flags().StringVar(&out, "out", "models.json", "Output file path")

	return cmd
}

func runScrape(cmd *cobra.Command, flagOut string) error {
	ctx := cmd.Context()

	fmt.Fprint(os.Stderr, "Fetching https://artificialanalysis.ai/leaderboards/models ...\n")

	models, err := aa.NewScraper().Scrape(ctx)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Clean(flagOut))
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	encErr := enc.Encode(models)
	closeErr := f.Close()

	if encErr != nil {
		return encErr
	}

	if closeErr != nil {
		return closeErr
	}

	fmt.Fprintf(os.Stderr, "Saved %d models to %s\n", len(models), flagOut)
	printTop5(models)

	return nil
}

func printTop5(models []aa.Model) {
	type entry struct {
		name  string
		score float64
	}

	ranked := make([]entry, 0, len(models))

	for _, m := range models {
		if m.IntelligenceIndex == nil {
			continue
		}

		ranked = append(ranked, entry{m.Name, *m.IntelligenceIndex})
	}

	slices.SortFunc(ranked, func(a, b entry) int { return cmp.Compare(b.score, a.score) })

	fmt.Println("\nTop 5 by Intelligence Index:")

	for _, e := range ranked[:min(top5Count, len(ranked))] {
		fmt.Printf("  %-40s %.1f\n", e.name, e.score)
	}
}
