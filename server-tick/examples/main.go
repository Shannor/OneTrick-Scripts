package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	clog "github.com/Shannor/canonical-logger"
)

type sampleSession struct {
	ID          string
	UserID      string
	CharacterID string
	Activities  []string
	ShouldFail  bool
}

func main() {
	ctx := context.Background()

	treeHandler := clog.NewTreeHandler(os.Stdout, &clog.ConsoleHandlerOptions{
		Level:   clog.LevelDebug,
		NoColor: false,
	})
	prettyJSONHandler := clog.NewPrettyJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       clog.LevelDebug,
		ReplaceAttr: gcpReplaceAttr,
	})
	gcpJSONHandler := clog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       clog.LevelDebug,
		ReplaceAttr: gcpReplaceAttr,
	})

	fmt.Println("=== 1. Server-Tick Success + LoopRollup (Colorized Tree View: clog.NewTreeHandler) ===")
	slog.SetDefault(slog.New(treeHandler))
	runSimulatedServerTick(ctx, false, false)

	fmt.Println("\n=== 2. Server-Tick Partial Session Failures + Manifest Warning (Colorized Tree View: clog.NewTreeHandler) ===")
	slog.SetDefault(slog.New(treeHandler))
	runSimulatedServerTick(ctx, true, true)

	fmt.Println("\n=== 3. Server-Tick Partial Session Failures + Manifest Warning (Pretty JSON: clog.NewPrettyJSONHandler) ===")
	slog.SetDefault(slog.New(prettyJSONHandler))
	runSimulatedServerTick(ctx, true, true)

	fmt.Println("\n=== 4. Server-Tick Fatal Boundary Failure (Compact GCP JSON: clog.NewJSONHandler) ===")
	slog.SetDefault(slog.New(gcpJSONHandler))
	_ = clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "request", clog.Fields{
			"taskIndex":             int64(0),
			"taskAttempt":           "1",
			"dryRun":                true,
			"projectId":             "gruntt-destiny",
			"firestoreEmulatorHost": "127.0.0.1:8081",
		})
		clog.WithGroupFields(ctx, "db.getSessions", clog.Fields{
			"collection": "sessions",
			"durationMs": int64(3001),
		})
		clog.WithFields(ctx, clog.Fields{
			"errorsTotal":    1,
			"tickDurationMs": int64(3002),
		})
		return fmt.Errorf("failed to get sessions: %w", errors.New("firestore emulator at 127.0.0.1:8081 timed out after 3s"))
	})
}

func runSimulatedServerTick(ctx context.Context, includeSessionFailure bool, includeManifestWarning bool) {
	sessions := []sampleSession{
		{ID: "sess_101", UserID: "usr_alpha", CharacterID: "char_hunter", Activities: []string{"act_9001", "act_9002"}},
		{ID: "sess_102", UserID: "usr_bravo", CharacterID: "char_titan", Activities: []string{"act_9003"}, ShouldFail: includeSessionFailure},
		{ID: "sess_103", UserID: "usr_charlie", CharacterID: "char_warlock", Activities: []string{"act_9004"}},
	}

	_ = clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "request", clog.Fields{
			"taskIndex":   int64(0),
			"taskAttempt": "1",
			"dryRun":      false,
			"skipSave":    false,
			"projectId":   "gruntt-destiny",
		})

		clog.WithGroupFields(ctx, "db.getSessions", clog.Fields{
			"collection":   "sessions",
			"rowsReturned": len(sessions),
			"durationMs":   int64(14),
		})

		if includeManifestWarning {
			clog.WithGroupFields(ctx, "db.batchedFetch.d2StatDefinitions", clog.Fields{
				"collection":     "d2StatDefinitions",
				"requestedCount": 6,
				"rowsReturned":   5,
				"missingCount":   1,
				"durationMs":     int64(8),
				"warning": clog.Fields{
					"type":       "missing_manifest_definition",
					"collection": "d2StatDefinitions",
					"docId":      "2996146975",
				},
			})
		}

		var (
			sessionsProcessed    int
			sessionsErrored      int
			loadoutsSaved        int
			activitiesFoundTotal int
			aggregatesAddedTotal int
			errorsTotal          int
		)

		_ = clog.LoopRollup(ctx, "sessionService.processSession", sessions, func(iterCtx context.Context, _ int, s sampleSession) error {
			clog.WithGroupFields(iterCtx, "session", clog.Fields{
				"sessionId":   s.ID,
				"userId":      s.UserID,
				"characterId": s.CharacterID,
			})

			clog.WithGroupFields(iterCtx, "db.getUser", clog.Fields{
				"collection": "users",
				"userId":     s.UserID,
				"durationMs": int64(5),
			})

			clog.WithGroupFields(iterCtx, "snapshotService.save", clog.Fields{
				"snapshotId":  "snap_" + s.ID,
				"characterId": s.CharacterID,
				"durationMs":  int64(110),
			})
			loadoutsSaved++

			if s.ShouldFail {
				sessionsErrored++
				errorsTotal++
				return fmt.Errorf("failed to get activities for session %s: %w", s.ID, errors.New("bungie API 503 Service Unavailable"))
			}

			clog.WithGroupFields(iterCtx, "activityService.getAllPvP", clog.Fields{
				"characterId":           s.CharacterID,
				"rawActivitiesReturned": len(s.Activities),
				"activitiesReturned":    len(s.Activities),
				"durationMs":            int64(95),
			})
			activitiesFoundTotal += len(s.Activities)

			_ = clog.LoopRollup(iterCtx, "aggregateService.processActivity", s.Activities, func(actCtx context.Context, _ int, actID string) error {
				clog.WithGroupField(actCtx, "activity", "activityId", actID)
				clog.WithGroupFields(actCtx, "aggregateService.setAggregate", clog.Fields{
					"aggregateId": "agg_" + actID,
					"activityId":  actID,
					"sessionId":   s.ID,
					"durationMs":  int64(42),
				})
				aggregatesAddedTotal++
				return nil
			})

			sessionsProcessed++
			return nil
		})

		clog.WithGroupFields(ctx, "sessionService.cleanupEmpty", clog.Fields{
			"collection":   "sessions",
			"rowsChecked":  4,
			"deletedCount": 1,
			"dryRun":       false,
			"durationMs":   int64(19),
		})

		clog.WithFields(ctx, clog.Fields{
			"durationMs":           int64(340),
			"tickDurationMs":       int64(340),
			"tickDurationSec":      0.34,
			"totalSessions":        len(sessions),
			"sessionsProcessed":    sessionsProcessed,
			"sessionsSkipped":      0,
			"sessionsErrored":      sessionsErrored,
			"loadoutsSaved":        loadoutsSaved,
			"loadoutsFailed":       0,
			"activitiesFoundTotal": activitiesFoundTotal,
			"aggregatesAddedTotal": aggregatesAddedTotal,
			"errorsTotal":          errorsTotal,
			"emptySessionsCleaned": 1,
		})
		return nil
	})
}

func gcpReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		a.Key = "timestamp"
		return a
	}
	return clog.GCPReplaceAttr(groups, a)
}
