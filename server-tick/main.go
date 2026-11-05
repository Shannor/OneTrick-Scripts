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
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	clog "github.com/Shannor/canonical-logger"
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
	fs.StringVar(&emulatorHost, "emulator-host", "", "Firestore emulator host:port (e.g. 127.0.0.1:8081)")
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
	os.Exit(runJob(context.Background(), "server-tick", runServerTick))
}

type jobMetadataStateKey struct{}

type jobMetadataState struct {
	mu                   sync.Mutex
	issueCount           int
	manifestWarningCount int
	lastIssue            clog.Fields
	lastManifestWarning  clog.Fields
}

func withJobMetadataState(ctx context.Context) (context.Context, *jobMetadataState) {
	state := &jobMetadataState{}
	return context.WithValue(ctx, jobMetadataStateKey{}, state), state
}

func recordJobIssue(ctx context.Context, issueType, message string, fields clog.Fields) {
	details := makeJobDetails(issueType, message, fields)
	if state, ok := ctx.Value(jobMetadataStateKey{}).(*jobMetadataState); ok {
		state.record(details, false)
		return
	}
	clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{"issueCount": 1, "lastIssue": details})
}

func recordManifestWarning(ctx context.Context, warningType string, fields clog.Fields) {
	details := makeJobDetails(warningType, warningType, fields)
	if state, ok := ctx.Value(jobMetadataStateKey{}).(*jobMetadataState); ok {
		state.record(details, true)
		return
	}
	clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
		"issueCount":           1,
		"lastIssue":            details,
		"manifestWarningCount": 1,
		"lastManifestWarning":  details,
	})
}

func makeJobDetails(detailType, message string, fields clog.Fields) clog.Fields {
	details := clog.Fields{"type": detailType, "message": limitJobDetail(message)}
	for key, value := range fields {
		switch value := value.(type) {
		case error:
			details[key] = limitJobDetail(value.Error())
		case string:
			details[key] = limitJobDetail(value)
		default:
			details[key] = value
		}
	}
	return details
}

func limitJobDetail(value string) string {
	const maxRunes = 1024
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func (s *jobMetadataState) record(details clog.Fields, manifestWarning bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.issueCount++
	s.lastIssue = details
	if manifestWarning {
		s.manifestWarningCount++
		s.lastManifestWarning = details
	}
}

func (s *jobMetadataState) flush(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields := clog.Fields{
		"issueCount":           s.issueCount,
		"manifestWarningCount": s.manifestWarningCount,
	}
	if s.lastIssue != nil {
		fields["lastIssue"] = s.lastIssue
	}
	if s.lastManifestWarning != nil {
		fields["lastManifestWarning"] = s.lastManifestWarning
	}
	clog.WithGroupFields(ctx, "jobMetadata", fields)
}

func runJob(ctx context.Context, event string, execute func(context.Context) error) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := clog.Run(ctx, event, execute); err != nil {
		return 1
	}
	return 0
}

func runServerTick(ctx context.Context) error {
	ctx, metadataState := withJobMetadataState(ctx)
	defer metadataState.flush(ctx)
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
	clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
		"taskIndex":   os.Getenv("CLOUD_RUN_TASK_INDEX"),
		"taskAttempt": os.Getenv("CLOUD_RUN_TASK_ATTEMPT"),
	})
	defer func() {
		tickDuration := time.Since(tickStartTime)
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
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
		recordJobIssue(ctx, "configuration", "failed to get config", clog.Fields{"error": err})
		return fmt.Errorf("failed to get config: %w", err)
	}
	clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
		"taskIndex":   config.taskNum,
		"taskAttempt": config.attemptNum,
		"dryRun":      config.DryRun,
		"skipSave":    config.SkipSave,
		"projectId":   config.ProjectID,
	})
	if config.EmulatorHost != "" {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
			"firestoreEmulatorHost": config.EmulatorHost,
		})
	}
	if config.EmulatorHost != "" {
		os.Setenv("FIRESTORE_EMULATOR_HOST", config.EmulatorHost)
	}

	db, err := firestore.NewClient(ctx, config.ProjectID)
	if err != nil {
		recordJobIssue(ctx, "firestore_client", "failed to create client", clog.Fields{"error": err})
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}

	defer func(db *firestore.Client) {
		err := db.Close()
		if err != nil {
			errorsTotal++
			recordJobIssue(ctx, "firestore_close", "failed to close db", clog.Fields{"error": err})
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
		recordJobIssue(ctx, "bungie_client", "failed to start destiny client", clog.Fields{"error": err})
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
				err = fmt.Errorf("Firestore emulator at %s timed out after %s while fetching sessions: %w", config.EmulatorHost, emulatorSessionQueryTimeout, err)
			} else {
				err = fmt.Errorf("failed to fetch sessions from Firestore emulator at %s: %w", config.EmulatorHost, err)
			}
		}
		recordJobIssue(ctx, "session_query", "failed to get sessions", clog.Fields{"error": err})
		return fmt.Errorf("failed to get sessions: %w", err)
	}
	totalSessions = len(sessions)

	for _, session := range sessions {
		isCompleted := session.Status != nil && *session.Status == SessionComplete

		membershipType, membershipID, err := GetMembershipType(ctx, db, session.UserID)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			recordJobIssue(ctx, "membership_type", "failed to fetch membership type", clog.Fields{"error": err, "sessionId": session.ID})
			continue
		}

		if !config.SkipSave && !config.DryRun {
			saveStart := time.Now()
			_, err = Save(ctx, db, cli, session.UserID, membershipID, session.CharacterID)
			loadoutDuration := time.Since(saveStart)
			if err != nil {
				loadoutsFailed++
				errorsTotal++
				recordJobIssue(ctx, "loadout_save", "failed to save loadout", clog.Fields{
					"error": err, "loadoutDurationMs": loadoutDuration.Milliseconds(), "sessionId": session.ID,
				})
				continue
			}
			loadoutsSaved++
		}

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
			recordJobIssue(ctx, "activity_history", "failed to get activities", clog.Fields{
				"error": err, "pvpDurationMs": pvpDuration.Milliseconds(), "sessionId": session.ID,
			})
			continue
		}

		if !isCompleted && IsStaleSession(session) {
			if config.DryRun {
				if len(session.AggregateIDs) == 0 {
					sessionsStaleDeleted++
				} else {
					sessionsStaleEnded++
				}
			} else {
				err := EndOrDeleteSession(ctx, db, session)
				if err != nil {
					sessionsErrored++
					errorsTotal++
					recordJobIssue(ctx, "stale_session", "failed to process stale session", clog.Fields{"error": err, "sessionId": session.ID})
					continue
				}
				if len(session.AggregateIDs) == 0 {
					sessionsStaleDeleted++
				} else {
					sessionsStaleEnded++
				}
			}
			continue
		}

		if !isCompleted && session.LastSeenTimestamp == nil && IsInactiveSession(session) {
			if config.DryRun {
				sessionsInactiveDeleted++
			} else {
				err := EndOrDeleteSession(ctx, db, session)
				if err != nil {
					sessionsErrored++
					errorsTotal++
					recordJobIssue(ctx, "inactive_session", "failed to process inactive session", clog.Fields{"error": err, "sessionId": session.ID})
					continue
				}
				sessionsInactiveDeleted++
			}
			continue
		}

		if len(activityHistories) == 0 {
			sessionsSkipped++
			recordJobIssue(ctx, "no_activity_history", "no history found for user", clog.Fields{"sessionId": session.ID})
			continue
		}

		latest := activityHistories[0]

		if session.LastSeenActivityID != nil && *session.LastSeenActivityID == latest.InstanceID {
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
			sessionsSkipped++
			continue
		}

		activitiesFoundTotal += len(IDs)

		existingAggs, err := GetAggregatesByActivity(ctx, db, IDs)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			recordJobIssue(ctx, "aggregate_query", "failed to fetch aggregates", clog.Fields{"error": err, "sessionId": session.ID})
			continue
		}

		existingAggMap := make(map[string]*Aggregate)
		for _, agg := range existingAggs {
			existingAggMap[agg.ActivityID] = &agg
		}

		aggIDs := make([]string, 0)
		for _, history := range histories {
			agg := existingAggMap[history.InstanceID]

			link := LookupLink(agg, session.CharacterID)
			if link != nil && link.SessionID != nil {
				continue
			}

			perfStart := time.Now()
			performances, err := GetPerformances(ctx, cli, db, history.InstanceID, session.CharacterID)
			perfDuration := time.Since(perfStart)
			if err != nil {
				errorsTotal++
				recordJobIssue(ctx, "performance_query", "failed to fetch performances", clog.Fields{
					"error": err, "activityId": history.InstanceID, "perfDurationMs": perfDuration.Milliseconds(), "sessionId": session.ID,
				})
				continue
			}

			performance, ok := performances[session.CharacterID]
			if !ok {
				recordJobIssue(ctx, "missing_performance", "no performance found for member", clog.Fields{"userId": session.UserID, "activityId": history.InstanceID})
				continue
			}

			if config.DryRun {
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
				recordJobIssue(ctx, "aggregate_write", "failed to add data to aggregate", clog.Fields{
					"error": err, "activityId": history.InstanceID, "aggDurationMs": aggDuration.Milliseconds(), "sessionId": session.ID,
				})
				continue
			}
			aggIDs = append(aggIDs, a.ID)
		}

		if len(aggIDs) == 0 {
			sessionsSkipped++
			continue
		}

		if config.DryRun {
			sessionsProcessed++
			aggregatesAddedTotal += len(aggIDs)
			continue
		}

		if !isCompleted {
			err = SetLastActivity(ctx, db, session.ID, latest.InstanceID)
			if err != nil {
				errorsTotal++
				recordJobIssue(ctx, "last_activity_write", "failed to save last activity for session", clog.Fields{"error": err, "sessionId": session.ID})
			}
		}

		err = AddAggregateIDs(ctx, db, session.ID, aggIDs)
		if err != nil {
			sessionsErrored++
			errorsTotal++
			recordJobIssue(ctx, "aggregate_ids_write", "failed to add aggregate IDs to session", clog.Fields{"error": err, "sessionId": session.ID})
			continue
		}

		session.AggregateIDs = append(session.AggregateIDs, aggIDs...)
		summaryStart := time.Now()
		summary, err := ComputeSessionSummary(ctx, db, session)
		summaryDuration := time.Since(summaryStart)
		if err != nil {
			errorsTotal++
			recordJobIssue(ctx, "session_summary", "failed to compute session summary", clog.Fields{
				"error": err, "summaryDurationMs": summaryDuration.Milliseconds(), "sessionId": session.ID,
			})
		} else {
			err = UpdateSessionSummary(ctx, db, session.ID, summary)
			if err != nil {
				errorsTotal++
				recordJobIssue(ctx, "session_summary_update", "failed to update session summary", clog.Fields{
					"error": err, "summaryDurationMs": summaryDuration.Milliseconds(), "sessionId": session.ID,
				})
			}
		}

		sessionsProcessed++
		aggregatesAddedTotal += len(aggIDs)
	}

	cleanupStart := time.Now()
	cleaned, err := CleanupEmptyCompletedSessions(ctx, db, config.DryRun)
	emptySessionsCleaned = cleaned
	cleanupDuration := time.Since(cleanupStart)
	if err != nil {
		errorsTotal++
		recordJobIssue(ctx, "empty_session_cleanup", "failed to cleanup older empty completed sessions", clog.Fields{
			"error": err, "cleanupDurationMs": cleanupDuration.Milliseconds(),
		})
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
