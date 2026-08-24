package main

import (
	"log/slog"
	"os"
)

// setupLogging configures slog with a GCP Cloud Logging compatible JSON handler.
// Maps "msg" -> "message", "level" -> "severity" with GCP severity values,
// and "time" -> "timestamp".
func setupLogging() {
	handler := newGCPHandler(slog.LevelDebug)
	slog.SetDefault(slog.New(handler))
}

// newGCPHandler creates an slog.Handler configured for GCP Cloud Logging stdout ingestion.
// Matches the pattern used in the onetrick-service API.
func newGCPHandler(level slog.Level) slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Only transform top-level keys
			if len(groups) > 0 {
				return a
			}

			switch a.Key {
			case slog.TimeKey:
				a.Key = "timestamp"
			case slog.MessageKey:
				a.Key = "message"
			case slog.LevelKey:
				a.Key = "severity"
				if l, ok := a.Value.Any().(slog.Level); ok {
					switch l {
					case slog.LevelDebug:
						a.Value = slog.StringValue("DEBUG")
					case slog.LevelInfo:
						a.Value = slog.StringValue("INFO")
					case slog.LevelWarn:
						a.Value = slog.StringValue("WARNING")
					case slog.LevelError:
						a.Value = slog.StringValue("ERROR")
					default:
						a.Value = slog.StringValue(l.String())
					}
				}
			}

			return a
		},
	}).WithAttrs([]slog.Attr{
		slog.Any("logging.googleapis.com/labels", map[string]string{
			"logger": "onetrick-server-tick",
		}),
	})
}
