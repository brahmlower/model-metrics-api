// Command scrape-aa fetches model metrics from artificialanalysis.ai.
package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "scrape-aa",
	Short: "Fetch model metrics from artificialanalysis.ai",
}

func main() {
	rootCmd.AddCommand(newScrapeCmd(), newServeCmd())

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
