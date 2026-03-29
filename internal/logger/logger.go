package logger

import (
	"log/slog"
	"os"
)

// Init initializes the global slog logger.
// If verbose is true, the level is set to Debug; otherwise Info.
func Init(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
