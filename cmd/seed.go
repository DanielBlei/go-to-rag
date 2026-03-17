package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/seed"
)

const defaultSeedDir = "./seeds"

var manifestPath string

func init() {
	seedCmd.Flags().StringVar(&manifestPath, "manifest", "", "path to a custom YAML manifest of documents to download")
	rootCmd.AddCommand(seedCmd)
}

var seedCmd = &cobra.Command{
	Use:   "seed [directory]",
	Short: "Download sample documents for testing the RAG pipeline",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSeed,
}

func runSeed(cmd *cobra.Command, args []string) error {
	dir := defaultSeedDir
	if len(args) > 0 {
		dir = args[0]
	}

	var (
		manifest *seed.Manifest
		err      error
	)
	if manifestPath != "" {
		manifest, err = seed.LoadManifest(manifestPath)
	} else {
		manifest, err = seed.DefaultManifest()
	}
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	log.Info().Str("dir", dir).Int("docs", len(manifest.Docs)).Msg("downloading sample documents")

	n, err := seed.Run(cmd.Context(), manifest, dir)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	if n == 0 {
		log.Info().Str("dir", dir).Msg("all files already exist, nothing downloaded")
	} else {
		log.Info().Int("files", n).Str("dir", dir).Msg("seed complete")
	}

	return nil
}
