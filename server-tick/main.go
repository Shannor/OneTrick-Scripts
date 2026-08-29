package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"serverTick/bungie"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
)

type Config struct {
	taskNum       int64
	attemptNum    string
	DestinyAPIKey string
	SkipSave      bool
	DryRun        bool
	ProjectID     string
	EmulatorHost  string
}

func configFromEnv() (Config, error) {
	taskNum, err := stringToInt(os.Getenv("CLOUD_RUN_TASK_INDEX"))
	if err != nil {
		return Config{}, err
	}

	attemptNum := os.Getenv("CLOUD_RUN_TASK_ATTEMPT")
	apiKey := os.Getenv("D2_API_KEY")
	skipSave, err := stringToInt(os.Getenv("SKIP_SAVE"))
	if err != nil {
		return Config{}, err
	}
	config := Config{
		taskNum:       taskNum,
		attemptNum:    attemptNum,
		DestinyAPIKey: apiKey,
	}
	if skipSave == 1 {
		config.SkipSave = true
	}
	dryRun, err := stringToInt(os.Getenv("DRY_RUN"))
	if err != nil {
		return Config{}, err
	}
	if dryRun == 1 {
		config.DryRun = true
	}

	fs := flag.NewFlagSet("server-tick", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		useEmulator  bool
		emulatorHost string
		projIDFlag   string
		dryRunFlag   bool
	)
	fs.BoolVar(&useEmulator, "use-emulator", false, "Use local Firestore emulator/docker DB")
	fs.StringVar(&emulatorHost, "emulator-host", "", "Firestore emulator host:port (e.g. 0.0.0.0:8081)")
	fs.StringVar(&projIDFlag, "project-id", "", "Google Cloud / Firestore project ID (defaults to gruntt-destiny)")
	fs.BoolVar(&dryRunFlag, "dry-run", false, "Run without performing any writes")

	_ = fs.Parse(os.Args[1:])

	if dryRunFlag {
		config.DryRun = true
	}

	config.ProjectID = defaultProjectID
	if projID := os.Getenv("FIRESTORE_PROJECT_ID"); projID != "" {
		config.ProjectID = projID
	} else if projID := os.Getenv("PROJECT_ID"); projID != "" {
		config.ProjectID = projID
	}
	if projIDFlag != "" {
		config.ProjectID = projIDFlag
	}

	envEmulatorHost := os.Getenv("FIRESTORE_EMULATOR_HOST")
	if emulatorHost != "" {
		config.EmulatorHost = emulatorHost
	} else if envEmulatorHost != "" {
		config.EmulatorHost = envEmulatorHost
	} else if useEmulator {
		config.EmulatorHost = "0.0.0.0:8081"
	}

	return config, nil
}

func stringToInt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

const (
	defaultProjectID = "gruntt-destiny"
)

func main() {
	setupLogging()
	tickStartTime := time.Now()
	config, err := configFromEnv()
	if err != nil {
		slog.Error("failed to get config", "error", err)
		os.Exit(1)
	}
	l := slog.With("taskNum", config.taskNum, "dryRun", config.DryRun)
	if config.DryRun {
		l.Info("running in dry-run mode, no writes will be performed")
	}
	ctx := context.Background()

	if config.EmulatorHost != "" {
		os.Setenv("FIRESTORE_EMULATOR_HOST", config.EmulatorHost)
		l.Info("using local Firestore DB / emulator", "host", config.EmulatorHost, "projectId", config.ProjectID)
	} else {
		l.Info("connecting to production Firestore DB", "projectId", config.ProjectID)
	}

	db, err := firestore.NewClient(ctx, config.ProjectID)
	if err != nil {
		l.Error("failed to create client", "error", err)
		os.Exit(1)
	}

	defer func(db *firestore.Client) {
		err := db.Close()
		if err != nil {
			l.Error("failed to close db", "error", err)
		}
	}(db)

	hc := http.Client{}
	cli, err := bungie.NewClientWithResponses(
		"https://www.bungie.net/Platform",
		bungie.WithHTTPClient(&hc),
		bungie.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Add("X-API-KEY", config.DestinyAPIKey)
			req.Header.Add("Accept", "application/json")
			req.Header.Add("Content-Type", "application/json")
			req.Header.Add("User-Agent", "oneTrick-backend")
			return nil
		}),
	)
	if err != nil {
		l.Error("failed to start destiny client", "error", err)
		os.Exit(1)
	}

	var (
		sessionsProcessed       int
		sessionsSkipped         int
		sessionsStaleEnded      int
		sessionsStaleDeleted    int
		sessionsInactiveEnded   int
		sessionsInactiveDeleted int
		sessionsErrored         int
		loadoutsSaved           int
		loadoutsFailed          int
		activitiesFoundTotal    int
		aggregatesAddedTotal    int
		errorsTotal             int
	)

	sessions, err := GetSessions(ctx, db)
	if err != nil {
		l.Error("failed to get sessions", "error", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		l.Info("no sessions to process")
	} else {
		l.Info("received sessions to process", "sessions", len(sessions))
		for i, session := range sessions {
			sessionStartTime := time.Now()
			isCompleted := session.Status != nil && *session.Status == SessionComplete

			membershipType, membershipID, err := GetMembershipType(ctx, db, session.UserID)
			if err != nil {
				sessionsErrored++
				errorsTotal++
				l.Error("failed to fetch membership type", "error", err, "sessionId", session.ID)
				continue
			}

			ll := l.With("session", session.ID, "count", i, "userId", session.UserID)

			if !config.SkipSave {
				if config.DryRun {
					ll.Info("[DRY-RUN] would save loadout")
				} else {
					ll.Info("starting to save loadout")
					saveStart := time.Now()
					_, err = Save(ctx, db, cli, session.UserID, membershipID, session.CharacterID)
					loadoutDuration := time.Since(saveStart)
					if err != nil {
						loadoutsFailed++
						errorsTotal++
						ll.Warn("failed to save loadout",
							"error", err,
							"loadoutDurationMs", loadoutDuration.Milliseconds(),
						)
						continue
					}
					loadoutsSaved++
					ll.Info("saved loadout",
						"loadoutDurationMs", loadoutDuration.Milliseconds(),
						"loadoutDurationSec", loadoutDuration.Seconds(),
					)
				}
			}

			ll.Info("starting to get pvp games")
			pvpStart := time.Now()
			activityHistories, err := GetAllPVP(
				ctx,
				cli,
				db,
				membershipID,
				membershipType,
				session.CharacterID,
				5,
				0,
			)
			pvpDuration := time.Since(pvpStart)
			if err != nil {
				sessionsErrored++
				errorsTotal++
				ll.Error("[SKIP]: failed to get activities",
					"error", err,
					"pvpDurationMs", pvpDuration.Milliseconds(),
				)
				continue
			}
			ll.Info("got pvp response",
				"pvpDurationMs", pvpDuration.Milliseconds(),
				"pvpDurationSec", pvpDuration.Seconds(),
				"activitiesFetched", len(activityHistories),
			)

			if !isCompleted && IsStaleSession(session) {
				if config.DryRun {
					if len(session.AggregateIDs) == 0 {
						sessionsStaleDeleted++
						ll.Info("[DRY-RUN] would delete stale session (no activities)")
					} else {
						sessionsStaleEnded++
						ll.Info("[DRY-RUN] would end stale session")
					}
				} else {
					err := EndOrDeleteSession(ctx, db, session)
					if err != nil {
						sessionsErrored++
						errorsTotal++
						ll.Error("failed to process stale session", "error", err)
						continue
					}
					if len(session.AggregateIDs) == 0 {
						sessionsStaleDeleted++
						ll.Info("session is stale with no activities. Deleted session")
					} else {
						sessionsStaleEnded++
						ll.Info("session is stale. Ending session")
					}
				}
				continue
			}

			if !isCompleted && session.LastSeenTimestamp == nil && IsInactiveSession(session) {
				if config.DryRun {
					sessionsInactiveDeleted++
					ll.Info("[DRY-RUN] would delete inactive session (no activities)")
				} else {
					err := EndOrDeleteSession(ctx, db, session)
					if err != nil {
						sessionsErrored++
						errorsTotal++
						ll.Error("failed to process inactive session", "error", err)
						continue
					}
					sessionsInactiveDeleted++
					ll.Info("session is inactive with no activities. Deleted session")
				}
				continue
			}

			if len(activityHistories) == 0 {
				sessionsSkipped++
				ll.Warn("[SKIP]: no history found for user",
					"sessionDurationMs", time.Since(sessionStartTime).Milliseconds(),
				)
				continue
			}

			latest := activityHistories[0]

			if session.LastSeenActivityID != nil && *session.LastSeenActivityID == latest.InstanceID {
				ll.Info("[SKIP]: No new activities since last check-in")
				sessionsSkipped++
				continue
			}

			IDs := make([]string, 0)
			histories := make([]ActivityHistory, 0)
			gracePeriod := session.StartedAt.Add(-15 * time.Minute)
			for _, activity := range activityHistories {
				if activity.Period.After(gracePeriod) {
					IDs = append(IDs, activity.InstanceID)
					histories = append(histories, activity)
				}
			}

			if len(IDs) == 0 {
				ll.Info("[SKIP]: No new activity to save")
				sessionsSkipped++
				continue
			}

			activitiesFoundTotal += len(IDs)
			ll.Info("Activities Found", "IDs", IDs, "newActivitiesCount", len(IDs))

			existingAggs, err := GetAggregatesByActivity(ctx, db, IDs)
			if err != nil {
				sessionsErrored++
				errorsTotal++
				ll.Error("failed to fetch aggregates by the provided IDs", "error", err, "activityIDs", IDs)
				continue
			}

			ll.Info("fetched existing aggregates", "count", len(existingAggs))

			existingAggMap := make(map[string]*Aggregate)
			for _, agg := range existingAggs {
				existingAggMap[agg.ActivityID] = &agg
			}

			aggIDs := make([]string, 0)
			for _, history := range histories {
				agg := existingAggMap[history.InstanceID]

				link := LookupLink(agg, session.CharacterID)
				if link != nil && link.SessionID != nil {
					ll.Info("Already linked to this activity", "activityId", history.InstanceID)
					continue
				}

				perfStart := time.Now()
				performances, err := GetPerformances(ctx, cli, db, history.InstanceID, session.CharacterID)
				perfDuration := time.Since(perfStart)
				if err != nil {
					errorsTotal++
					ll.Error("failed to fetch performances",
						"error", err,
						"activityId", history.InstanceID,
						"perfDurationMs", perfDuration.Milliseconds(),
					)
					continue
				}
				ll.Info("fetched performance",
					"activityId", history.InstanceID,
					"perfDurationMs", perfDuration.Milliseconds(),
				)

				performance, ok := performances[session.CharacterID]
				if !ok {
					ll.Warn("no performance found for member", "userId", session.UserID, "activityId", history.InstanceID)
					continue
				}

				if config.DryRun {
					ll.Info("[DRY-RUN] would set aggregate", "activityId", history.InstanceID)
					aggIDs = append(aggIDs, history.InstanceID)
					continue
				}

				aggStart := time.Now()
				a, err := SetAggregate(
					ctx,
					db,
					session.UserID,
					session.CharacterID,
					history,
					history.Period,
					performance,
					session.ID,
				)
				aggDuration := time.Since(aggStart)
				if err != nil {
					errorsTotal++
					ll.Error("failed to add data to aggregate",
						"error", err,
						"activityId", history.InstanceID,
						"aggDurationMs", aggDuration.Milliseconds(),
					)
					continue
				}
				ll.Info("set aggregate successfully",
					"activityId", history.InstanceID,
					"aggregateId", a.ID,
					"aggDurationMs", aggDuration.Milliseconds(),
				)
				aggIDs = append(aggIDs, a.ID)
			}

			if len(aggIDs) == 0 {
				sessionsSkipped++
				ll.Info("[SKIP]: All activities already linked. No session update needed",
					"sessionDurationMs", time.Since(sessionStartTime).Milliseconds(),
				)
				continue
			}

			if config.DryRun {
				sessionsProcessed++
				aggregatesAddedTotal += len(aggIDs)
				ll.Info("[DRY-RUN] would update session with last activity and aggregate IDs",
					"aggregateIds", aggIDs,
					"latestActivityId", latest.InstanceID,
					"sessionDurationMs", time.Since(sessionStartTime).Milliseconds(),
				)
				continue
			}

			if !isCompleted {
				err = SetLastActivity(ctx, db, session.ID, latest.InstanceID)
				if err != nil {
					errorsTotal++
					ll.Warn("failed to save last activity for session. Continuing on", "error", err)
				}
			}

			ll.Info("Aggregates to add", "aggregateIds", aggIDs)

			err = AddAggregateIDs(ctx, db, session.ID, aggIDs)
			if err != nil {
				sessionsErrored++
				errorsTotal++
				ll.Error("Failed to add aggregate IDs to session", "error", err)
				continue
			}
			ll.Info("Added aggregate IDs to session", "aggregates", aggIDs)

			session.AggregateIDs = append(session.AggregateIDs, aggIDs...)
			summaryStart := time.Now()
			summary, err := ComputeSessionSummary(ctx, db, session)
			summaryDuration := time.Since(summaryStart)
			if err != nil {
				errorsTotal++
				ll.Error("Failed to compute session summary after adding aggregates",
					"error", err,
					"summaryDurationMs", summaryDuration.Milliseconds(),
				)
			} else {
				err = UpdateSessionSummary(ctx, db, session.ID, summary)
				if err != nil {
					errorsTotal++
					ll.Error("Failed to update session summary after adding aggregates",
						"error", err,
						"summaryDurationMs", summaryDuration.Milliseconds(),
					)
				} else {
					ll.Info("Updated session summary",
						"sessionId", session.ID,
						"summaryDurationMs", summaryDuration.Milliseconds(),
					)
				}
			}

			sessionsProcessed++
			aggregatesAddedTotal += len(aggIDs)
			ll.Info("session_processed",
				"sessionId", session.ID,
				"userId", session.UserID,
				"characterId", session.CharacterID,
				"activitiesFound", len(IDs),
				"aggregatesAdded", len(aggIDs),
				"sessionDurationMs", time.Since(sessionStartTime).Milliseconds(),
			)
		}
		l.Info("finished going through all sessions")
	}

	cleanupStart := time.Now()
	cleaned, err := CleanupEmptyCompletedSessions(ctx, db, config.DryRun)
	cleanupDuration := time.Since(cleanupStart)
	if err != nil {
		errorsTotal++
		l.Error("failed to cleanup older empty completed sessions",
			"error", err,
			"cleanupDurationMs", cleanupDuration.Milliseconds(),
		)
	} else if cleaned > 0 {
		l.Info("completed cleanup of older empty sessions",
			"count", cleaned,
			"cleanupDurationMs", cleanupDuration.Milliseconds(),
		)
	}

	tickDuration := time.Since(tickStartTime)
	l.Info("server_tick_completed",
		slog.Int64("tickDurationMs", tickDuration.Milliseconds()),
		slog.Float64("tickDurationSec", tickDuration.Seconds()),
		slog.Int("totalSessions", len(sessions)),
		slog.Int("sessionsProcessed", sessionsProcessed),
		slog.Int("sessionsSkipped", sessionsSkipped),
		slog.Int("sessionsStaleEnded", sessionsStaleEnded),
		slog.Int("sessionsStaleDeleted", sessionsStaleDeleted),
		slog.Int("sessionsInactiveEnded", sessionsInactiveEnded),
		slog.Int("sessionsInactiveDeleted", sessionsInactiveDeleted),
		slog.Int("sessionsErrored", sessionsErrored),
		slog.Int("emptySessionsCleaned", cleaned),
		slog.Int("loadoutsSaved", loadoutsSaved),
		slog.Int("loadoutsFailed", loadoutsFailed),
		slog.Int("activitiesFoundTotal", activitiesFoundTotal),
		slog.Int("aggregatesAddedTotal", aggregatesAddedTotal),
		slog.Int("errorsTotal", errorsTotal),
		slog.Bool("dryRun", config.DryRun),
	)
}
