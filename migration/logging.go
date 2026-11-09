package main

import (
	"io"
	"log/slog"
	"os"

	clog "github.com/Shannor/canonical-logger"
)

func setupLogging() {
	var handler slog.Handler
	if _, isCloudRun := os.LookupEnv("CLOUD_RUN_JOB"); isCloudRun {
		handler = newGCPHandler(slog.LevelDebug)
	} else {
		handler = newPrettyHandler(os.Stdout, slog.LevelDebug)
	}
	slog.SetDefault(slog.New(handler))
}

func newGCPHandler(level slog.Level) slog.Handler {
	return newGCPHandlerWithWriter(os.Stdout, level)
}

func newGCPHandlerWithWriter(w io.Writer, level slog.Level) slog.Handler {
	return clog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: gcpReplaceAttr,
	}).WithAttrs(loggingLabels())
}

func newPrettyHandler(w io.Writer, level slog.Level) slog.Handler {
	return clog.NewPrettyJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: gcpReplaceAttr,
	}).WithAttrs(loggingLabels())
}

func gcpReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		a.Key = "timestamp"
		return a
	}
	return clog.GCPReplaceAttr(groups, a)
}

func loggingLabels() []slog.Attr {
	return []slog.Attr{
		slog.Any("logging.googleapis.com/labels", map[string]string{
			"logger": "onetrick-migration",
		}),
	}
}
