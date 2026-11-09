package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"serverTick/bungie"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	clog "github.com/Shannor/canonical-logger"
)

type Config struct {
	taskNum           int64
	attemptNum        string
	DestinyAPIKey     string
	SkipSave          bool
	DryRun            bool
	BackfillGameModes bool
	ProjectID         string
	EmulatorHost      string
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
		useEmulator           bool
		emulatorHost          string
		projIDFlag            string
		dryRunFlag            bool
		backfillGameModesFlag bool
	)
	fs.BoolVar(&useEmulator, "use-emulator", false, "Use local Firestore emulator/docker DB")
	fs.StringVar(&emulatorHost, "emulator-host", "", "Firestore emulator host:port (e.g. 127.0.0.1:8081)")
	fs.StringVar(&projIDFlag, "project-id", "", "Google Cloud / Firestore project ID (defaults to gruntt-destiny)")
	fs.BoolVar(&dryRunFlag, "dry-run", false, "Run without performing any writes")
	fs.BoolVar(&backfillGameModesFlag, "backfill-game-modes", false, "Backfill activityHistory.estimatedGameMode on all aggregates and exit")

	_ = fs.Parse(os.Args[1:])

	if dryRunFlag {
		config.DryRun = true
	}
	backfillEnv, _ := stringToInt(os.Getenv("BACKFILL_GAME_MODES"))
	if backfillGameModesFlag || backfillEnv == 1 {
		config.BackfillGameModes = true
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
		config.EmulatorHost = "127.0.0.1:8081"
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
	defaultProjectID            = "gruntt-destiny"
	emulatorSessionQueryTimeout = 3 * time.Second
)

func main() {
	setupLogging()
	if err := clog.Run(context.Background(), "server-tick", runServerTick); err != nil {
		os.Exit(1)
	}
}

func runServerTick(ctx context.Context) error {
	tickStartTime := time.Now()
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
		totalSessions           int
		emptySessionsCleaned    int
	)
	clog.WithGroupFields(ctx, "request", clog.Fields{
		"taskIndex":   os.Getenv("CLOUD_RUN_TASK_INDEX"),
		"taskAttempt": os.Getenv("CLOUD_RUN_TASK_ATTEMPT"),
	})
	defer func() {
		tickDuration := time.Since(tickStartTime)
		clog.WithFields(ctx, clog.Fields{
			"durationMs":              tickDuration.Milliseconds(),
			"tickDurationMs":          tickDuration.Milliseconds(),
			"tickDurationSec":         tickDuration.Seconds(),
			"totalSessions":           totalSessions,
			"sessionsProcessed":       sessionsProcessed,
			"sessionsSkipped":         sessionsSkipped,
			"sessionsStaleEnded":      sessionsStaleEnded,
			"sessionsStaleDeleted":    sessionsStaleDeleted,
			"sessionsInactiveEnded":   sessionsInactiveEnded,
			"sessionsInactiveDeleted": sessionsInactiveDeleted,
			"sessionsErrored":         sessionsErrored,
			"loadoutsSaved":           loadoutsSaved,
			"loadoutsFailed":          loadoutsFailed,
			"activitiesFoundTotal":    activitiesFoundTotal,
			"aggregatesAddedTotal":    aggregatesAddedTotal,
			"errorsTotal":             errorsTotal,
			"emptySessionsCleaned":    emptySessionsCleaned,
		})
	}()

	config, err := configFromEnv()
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to get config: %w", err)
	}
	clog.WithGroupFields(ctx, "request", clog.Fields{
		"taskIndex":         config.taskNum,
		"taskAttempt":       config.attemptNum,
		"dryRun":            config.DryRun,
		"skipSave":          config.SkipSave,
		"backfillGameModes": config.BackfillGameModes,
		"projectId":         config.ProjectID,
	})
	if config.EmulatorHost != "" {
		clog.WithGroupFields(ctx, "request", clog.Fields{
			"firestoreEmulatorHost": config.EmulatorHost,
		})
	}
	if config.EmulatorHost != "" {
		os.Setenv("FIRESTORE_EMULATOR_HOST", config.EmulatorHost)
	}

	db, err := firestore.NewClient(ctx, config.ProjectID)
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}

	defer func(db *firestore.Client) {
		err := db.Close()
		if err != nil {
			errorsTotal++
		}
	}(db)

	if config.BackfillGameModes {
		_, err := BackfillEstimatedGameModes(ctx, db, config.DryRun)
		if err != nil {
			errorsTotal++
			return err
		}
		return nil
	}

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
		errorsTotal++
		return fmt.Errorf("failed to start Destiny client: %w", err)
	}

	queryCtx := ctx
	if config.EmulatorHost != "" {
		var cancel context.CancelFunc
		queryCtx, cancel = context.WithTimeout(ctx, emulatorSessionQueryTimeout)
		defer cancel()
	}
	sessions, err := GetSessions(queryCtx, db)
	if err != nil {
		errorsTotal++
		if config.EmulatorHost != "" {
			if isEmulatorSessionQueryTimeout(ctx, queryCtx) {
				err = fmt.Errorf("firestore emulator at %s timed out after %s while fetching sessions: %w", config.EmulatorHost, emulatorSessionQueryTimeout, err)
			} else {
				err = fmt.Errorf("failed to fetch sessions from Firestore emulator at %s: %w", config.EmulatorHost, err)
			}
		}
		return fmt.Errorf("failed to get sessions: %w", err)
	}
	totalSessions = len(sessions)

	_ = clog.LoopRollup(ctx, "sessionService.processSession", sessions, func(iterCtx context.Context, _ int, session Session) error {
		defer func() {
			if bag := clog.BagFromContext(iterCtx); bag != nil && bag.HasWarning() {
				clog.WithGroupField(ctx, "sessionService.processSession", "warning", true)
			}
		}()
		clog.WithGroupFields(iterCtx, "session", clog.Fields{
			"sessionId":   session.ID,
			"userId":      session.UserID,
			"characterId": session.CharacterID,
		})
		isCompleted := session.Status != nil && *session.Status == SessionComplete

		membershipType, membershipID, err := GetMembershipType(iterCtx, db, session.UserID)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			return fmt.Errorf("failed to fetch membership type for session %s: %w", session.ID, err)
		}

		if !config.SkipSave && !config.DryRun {
			_, err = Save(iterCtx, db, cli, session.UserID, membershipID, session.CharacterID)
			if err != nil {
				loadoutsFailed++
				errorsTotal++
				return fmt.Errorf("failed to save loadout for session %s: %w", session.ID, err)
			}
			loadoutsSaved++
		}

		activityHistories, err := GetAllPVP(
			iterCtx,
			cli,
			db,
			membershipID,
			membershipType,
			session.CharacterID,
			5,
			0,
		)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			return fmt.Errorf("failed to get activities for session %s: %w", session.ID, err)
		}

		if !isCompleted && IsStaleSession(session) {
			if config.DryRun {
				if len(session.AggregateIDs) == 0 {
					sessionsStaleDeleted++
				} else {
					sessionsStaleEnded++
				}
			} else {
				err := EndOrDeleteSession(iterCtx, db, session)
				if err != nil {
					sessionsErrored++
					errorsTotal++
					return fmt.Errorf("failed to process stale session %s: %w", session.ID, err)
				}
				if len(session.AggregateIDs) == 0 {
					sessionsStaleDeleted++
				} else {
					sessionsStaleEnded++
				}
			}
			return nil
		}

		if !isCompleted && session.LastSeenTimestamp == nil && IsInactiveSession(session) {
			if config.DryRun {
				sessionsInactiveDeleted++
			} else {
				err := EndOrDeleteSession(iterCtx, db, session)
				if err != nil {
					sessionsErrored++
					errorsTotal++
					return fmt.Errorf("failed to process inactive session %s: %w", session.ID, err)
				}
				sessionsInactiveDeleted++
			}
			return nil
		}

		if len(activityHistories) == 0 {
			sessionsSkipped++
			return nil
		}

		latest := activityHistories[0]

		if session.LastSeenActivityID != nil && *session.LastSeenActivityID == latest.InstanceID {
			sessionsSkipped++
			return nil
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
			sessionsSkipped++
			return nil
		}

		activitiesFoundTotal += len(IDs)

		existingAggs, err := GetAggregatesByActivity(iterCtx, db, IDs)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			return fmt.Errorf("failed to fetch aggregates for session %s: %w", session.ID, err)
		}

		existingAggMap := make(map[string]*Aggregate)
		for _, agg := range existingAggs {
			existingAggMap[agg.ActivityID] = &agg
		}

		aggIDs := make([]string, 0)
		_ = clog.LoopRollup(iterCtx, "aggregateService.processActivity", histories, func(actCtx context.Context, _ int, history ActivityHistory) error {
			defer func() {
				if bag := clog.BagFromContext(actCtx); bag != nil && bag.HasWarning() {
					clog.WithGroupField(iterCtx, "aggregateService.processActivity", "warning", true)
				}
			}()
			clog.WithGroupField(actCtx, "activity", "activityId", history.InstanceID)
			agg := existingAggMap[history.InstanceID]

			link := LookupLink(agg, session.CharacterID)
			if link != nil && link.SessionID != nil {
				return nil
			}

			performances, err := GetPerformances(actCtx, cli, db, history.InstanceID, session.CharacterID)
			if err != nil {
				errorsTotal++
				return fmt.Errorf("failed to fetch performances for activity %s: %w", history.InstanceID, err)
			}

			performance, ok := performances[session.CharacterID]
			if !ok {
				clog.WithGroupField(actCtx, "activity", "warning", clog.Fields{
					"type":       "missing_performance",
					"userId":     session.UserID,
					"activityId": history.InstanceID,
				})
				return nil
			}

			if config.DryRun {
				aggIDs = append(aggIDs, history.InstanceID)
				return nil
			}

			a, err := SetAggregate(
				actCtx,
				db,
				session.UserID,
				session.CharacterID,
				history,
				history.Period,
				performance,
				session.ID,
			)
			if err != nil {
				errorsTotal++
				return fmt.Errorf("failed to add data to aggregate for activity %s: %w", history.InstanceID, err)
			}
			aggIDs = append(aggIDs, a.ID)
			return nil
		})

		if len(aggIDs) == 0 {
			sessionsSkipped++
			return nil
		}

		if config.DryRun {
			sessionsProcessed++
			aggregatesAddedTotal += len(aggIDs)
			return nil
		}

		var postErrs []error
		if !isCompleted {
			err = SetLastActivity(iterCtx, db, session.ID, latest.InstanceID)
			if err != nil {
				errorsTotal++
				postErrs = append(postErrs, fmt.Errorf("failed to save last activity for session %s: %w", session.ID, err))
			}
		}

		err = AddAggregateIDs(iterCtx, db, session.ID, aggIDs)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			return errors.Join(append(postErrs, fmt.Errorf("failed to add aggregate IDs to session %s: %w", session.ID, err))...)
		}

		session.AggregateIDs = append(session.AggregateIDs, aggIDs...)
		summary, err := ComputeSessionSummary(iterCtx, db, session)
		if err != nil {
			errorsTotal++
			postErrs = append(postErrs, fmt.Errorf("failed to compute session summary for session %s: %w", session.ID, err))
		} else {
			err = UpdateSessionSummary(iterCtx, db, session.ID, summary)
			if err != nil {
				errorsTotal++
				postErrs = append(postErrs, fmt.Errorf("failed to update session summary for session %s: %w", session.ID, err))
			}
		}

		sessionsProcessed++
		aggregatesAddedTotal += len(aggIDs)
		return errors.Join(postErrs...)
	})

	cleaned, err := CleanupEmptyCompletedSessions(ctx, db, config.DryRun)
	emptySessionsCleaned = cleaned
	if err != nil {
		errorsTotal++
	}

	return nil
}

func isEmulatorSessionQueryTimeout(parentCtx, queryCtx context.Context) bool {
	if !errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
		return false
	}
	queryDeadline, hasQueryDeadline := queryCtx.Deadline()
	if !hasQueryDeadline {
		return false
	}
	parentDeadline, hasParentDeadline := parentCtx.Deadline()
	return !hasParentDeadline || queryDeadline.Before(parentDeadline)
}
