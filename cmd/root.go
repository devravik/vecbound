package cmd

import (
	"fmt"
	"os"

	"github.com/devravik/vecbound/internal/config"
	"github.com/devravik/vecbound/internal/logger"
	"github.com/spf13/cobra"
)

var (
	cfgFile  string
	verbose  bool
	cpuLimit int
	memLimit int
	Cfg      *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "vecbound",
	Short: "High-speed local text vectorization CLI",
	Long: `VecBound crawls directories, chunks content, and generates a SQLite
Vector DB using local ONNX embeddings. No Python. No Docker. No API keys.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logger.Init(verbose)

		path := cfgFile
		if path == "" {
			path = config.DefaultConfigPath()
		}

		cfg, err := config.LoadFromFile(path)
		if err != nil && cfgFile != "" {
			// Only error out if the user explicitly passed a config file
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Apply global resource limits from flags if provided
		if cpuLimit > 0 {
			cfg.MaxCPU = cpuLimit
			cfg.Workers = cpuLimit // Workers should follow MaxCPU
		}
		if memLimit > 0 {
			cfg.MaxMem = memLimit
		}

		Cfg = cfg
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.vecbound/config.json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	// Resource Governor Flags
	rootCmd.PersistentFlags().IntVar(&cpuLimit, "max-cpu", 0, "Limit the number of concurrent CPU workers (defaults to half of CPU cores)")
	rootCmd.PersistentFlags().IntVar(&memLimit, "max-mem", 0, "Soft memory limit in MB for batch processing (default 512MB)")
}
