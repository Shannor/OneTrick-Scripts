package main

import (
	"io"
	"log/slog"
	"os"

	clog "github.com/Shannor/canonical-logger"
)

// setupLogging configures slog with a GCP Cloud Logging compatible JSON handler.
// Maps "msg" -> "message", "level" -> "severity" with GCP severity values,
// and "time" -> "timestamp".
func setupLogging() {
	var handler slog.Handler
	if _, isCloudRun := os.LookupEnv("CLOUD_RUN_JOB"); isCloudRun {
		handler = newGCPHandler(slog.LevelDebug)
	} else {
		handler = newPrettyHandler(os.Stdout, slog.LevelDebug)
	}
	slog.SetDefault(slog.New(handler))
}

// newGCPHandler creates an slog.Handler configured for GCP Cloud Logging stdout ingestion.
// Matches the pattern used in the onetrick-service API.
func newGCPHandler(level slog.Level) slog.Handler {
	return newGCPHandlerWithWriter(os.Stdout, level)
}

func newGCPHandlerWithWriter(w io.Writer, level slog.Level) slog.Handler {
	return clog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: gcpReplaceAttr,
	})
}

func newPrettyHandler(w io.Writer, level slog.Level) slog.Handler {
	return clog.NewPrettyJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: gcpReplaceAttr,
	}).WithNoColor(true)
}

func gcpReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		a.Key = "timestamp"
		return a
	}
	return clog.GCPReplaceAttr(groups, a)
}
