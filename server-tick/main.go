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
	Backfill      bool
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
		backfillFlag bool
		useEmulator  bool
		emulatorHost string
		projIDFlag   string
		dryRunFlag   bool
	)
	fs.BoolVar(&backfillFlag, "backfill-summaries", false, "Calculate and backfill SessionSummary for completed sessions")
	fs.BoolVar(&useEmulator, "use-emulator", false, "Use local Firestore emulator/docker DB")
	fs.StringVar(&emulatorHost, "emulator-host", "", "Firestore emulator host:port (e.g. 0.0.0.0:8081)")
	fs.StringVar(&projIDFlag, "project-id", "", "Google Cloud / Firestore project ID (defaults to gruntt-destiny)")
	fs.BoolVar(&dryRunFlag, "dry-run", false, "Run without performing any writes")

	_ = fs.Parse(os.Args[1:])

	if dryRunFlag {
		config.DryRun = true
	}

	backfillEnv, _ := stringToInt(os.Getenv("BACKFILL_SUMMARIES"))
	if backfillFlag || backfillEnv == 1 {
		config.Backfill = true
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
			isCompleted := session.Status != nil && *session.Status == SessionComplete

			membershipType, membershipID, err := GetMembershipType(ctx, db, session.UserID)
			if err != nil {
				l.Error("failed to fetch membership type", "error", err)
				continue
			}

			// This could be moved to something else in the future maybe. It's not super necessary
			// that it is done here before the rest of the logic. Just that it is done
			ll := l.With("session", session.ID, "count", i)
			if !config.SkipSave {
				if config.DryRun {
					ll.Info("[DRY-RUN] would save loadout")
				} else {
					ll.Info("starting to save loadout")
					startTime := time.Now()
					_, err = Save(ctx, db, cli, session.UserID, membershipID, session.CharacterID)
					if err != nil {
						ll.Warn("failed to save loadout", "error", err)
						continue
					}
					ll.Info("saved loadout", "loadoutDuration", time.Since(startTime))
				}
			}
			ll.Info("starting to get pvp games")
			startTime := time.Now()
			// Activity history should be shared
			activityHistories, err := GetAllPVP(
				ctx,
				cli,
				db,
				membershipID,
				membershipType,
				session.CharacterID,
				2,
				0,
			)
			if err != nil {
				ll.Error("[SKIP]: failed to get activities", "error", err)
				continue
			}
			ll.Info("got pvp response", "pvpDuration", time.Since(startTime))

			if len(activityHistories) == 0 {
				ll.Warn("[SKIP]: no history found for user")
				continue
			}

			latest := activityHistories[0]

			if session.LastSeenActivityID != nil && *session.LastSeenActivityID == latest.InstanceID {
				ll.Info("[SKIP]: No new activities since last check-in")
				if !isCompleted && IsStaleSession(session) {
					if config.DryRun {
						if len(session.AggregateIDs) == 0 {
							ll.Info("[DRY-RUN] would delete stale session (no activities)")
						} else {
							ll.Info("[DRY-RUN] would end stale session")
						}
					} else {
						err := EndOrDeleteSession(ctx, db, session)
						if err != nil {
							ll.Error("failed to process stale session", "error", err)
							continue
						}
						if len(session.AggregateIDs) == 0 {
							ll.Info("session is stale with no activities. Deleted session")
						} else {
							ll.Info("session is stale. Ending session")
						}
					}
					continue
				}
				continue
			}

			IDs := make([]string, 0)
			histories := make([]ActivityHistory, 0)
			// Only choose activities that happened after starting the session
			gracePeriod := session.StartedAt.Add(-15 * time.Minute)
			for _, activity := range activityHistories {
				if activity.Period.After(gracePeriod) {
					IDs = append(IDs, activity.InstanceID)
					histories = append(histories, activity)
				}
			}

			if len(IDs) == 0 {
				ll.Info("[SKIP]: No new activity to save. Checking if Inactive")
				if !isCompleted && IsInactiveSession(session) {
					if config.DryRun {
						if len(session.AggregateIDs) == 0 {
							ll.Info("[DRY-RUN] would delete inactive session (no activities)")
						} else {
							ll.Info("[DRY-RUN] would end inactive session")
						}
					} else {
						err := EndOrDeleteSession(ctx, db, session)
						if err != nil {
							ll.Error("failed to process inactive session", "error", err)
							continue
						}
						if len(session.AggregateIDs) == 0 {
							ll.Info("session is inactive with no activities. Deleted session")
						} else {
							ll.Info("session is inactive. Ending session")
						}
					}
					continue
				}
				continue
			}

			ll.Info("Activities Found", "IDs", IDs)

			existingAggs, err := GetAggregatesByActivity(ctx, db, IDs)
			if err != nil {
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
				// Already attempted to link this character to this activity so we can skip it
				if link != nil && link.SessionID != nil {
					ll.Info("Already linked to this activity", "activityId", history.InstanceID)
					continue
				}

				performances, err := GetPerformances(ctx, cli, db, history.InstanceID, session.CharacterID)
				if err != nil {
					ll.Error("failed to fetch performances", "error", err)
					continue
				}
				performance, ok := performances[session.CharacterID]
				if !ok {
					ll.Warn("no performance found for member", "userId", session.UserID)
					continue
				}

				if config.DryRun {
					ll.Info("[DRY-RUN] would set aggregate", "activityId", history.InstanceID)
					aggIDs = append(aggIDs, history.InstanceID)
					continue
				}

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
				if err != nil {
					ll.Error("failed to add data to aggregate", "error", err)
					continue
				}
				aggIDs = append(aggIDs, a.ID)
			}

			// Only update the session if we actually processed new activities
			if len(aggIDs) == 0 {
				ll.Info("[SKIP]: All activities already linked. No session update needed")
				continue
			}

			if config.DryRun {
				ll.Info("[DRY-RUN] would update session with last activity and aggregate IDs",
					"aggregateIds", aggIDs,
					"latestActivityId", latest.InstanceID,
				)
				continue
			}

			if !isCompleted {
				err = SetLastActivity(ctx, db, session.ID, latest.InstanceID)
				if err != nil {
					ll.Warn("failed to save last activity for session. Continuing on", "error", err)
				}
			}

			ll.Info("Aggregates to add", "aggregateIds", aggIDs)

			err = AddAggregateIDs(ctx, db, session.ID, aggIDs)
			if err != nil {
				ll.Error("Failed to add aggregate IDs to session", "error", err)
				continue
			}
			ll.Info("Added aggregate IDs to session", "aggregates", aggIDs)

			session.AggregateIDs = append(session.AggregateIDs, aggIDs...)
			summary, err := ComputeSessionSummary(ctx, db, session)
			if err != nil {
				ll.Error("Failed to compute session summary after adding aggregates", "error", err)
			} else {
				err = UpdateSessionSummary(ctx, db, session.ID, summary)
				if err != nil {
					ll.Error("Failed to update session summary after adding aggregates", "error", err)
				} else {
					ll.Info("Updated session summary", "sessionId", session.ID)
				}
			}
		}
		l.Info("finished going through all sessions")
	}

	if config.Backfill {
		l.Info("starting session summary backfill")
		backfilled, err := BackfillSessionSummaries(ctx, db, config.DryRun)
		if err != nil {
			l.Error("failed to backfill session summaries", "error", err)
		} else {
			l.Info("completed session summary backfill", "count", backfilled)
		}
	}

	cleaned, err := CleanupEmptyCompletedSessions(ctx, db, config.DryRun)
	if err != nil {
		l.Error("failed to cleanup older empty completed sessions", "error", err)
	} else if cleaned > 0 {
		l.Info("completed cleanup of older empty sessions", "count", cleaned)
	}
}
