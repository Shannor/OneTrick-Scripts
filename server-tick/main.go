package main

import (
	"context"
	"net/http"
	"os"
	"serverTick/bungie"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Config struct {
	taskNum       int64
	attemptNum    string
	DestinyAPIKey string
	SkipSave      bool
	DryRun        bool
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
	return config, nil
}

func stringToInt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

const (
	projectID = "gruntt-destiny"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	config, err := configFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get config")
	}
	l := log.With().Int64("taskNum", config.taskNum).Bool("dryRun", config.DryRun).Logger()
	if config.DryRun {
		l.Info().Msg("running in dry-run mode, no writes will be performed")
	}
	ctx := context.Background()

	db, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		l.Fatal().Err(err).Msgf("Failed to create client: %v", err)
	}

	defer func(db *firestore.Client) {
		err := db.Close()
		if err != nil {
			l.Fatal().Err(err).Msg("failed to close db")
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
		l.Fatal().Err(err).Msg("failed to start destiny client")
	}

	sessions, err := GetSessions(ctx, db)
	if err != nil {
		l.Fatal().Err(err).Msg("failed to get sessions")
	}

	if len(sessions) == 0 {
		l.Info().Msg("no sessions to process")
		return
	}

	l.Info().Int("sessions", len(sessions)).Msg("received sessions to process")
	for i, session := range sessions {

		membershipType, membershipID, err := GetMembershipType(ctx, db, session.UserID)
		if err != nil {
			l.Error().Err(err).Msg("failed to fetch membership type")
			continue
		}

		// This could be moved to something else in the future maybe. It's not super necessary
		// that it is done here before the rest of the logic. Just that it is done
		ll := l.With().Str("session", session.ID).Int("count", i).Logger()
		if !config.SkipSave {
			if config.DryRun {
				ll.Info().Msg("[DRY-RUN] would save loadout")
			} else {
				ll.Info().Msg("starting to save loadout")
				startTime := time.Now()
				_, err = Save(ctx, db, cli, session.UserID, membershipID, session.CharacterID)
				if err != nil {
					ll.Warn().Err(err).Msg("failed to save loadout")
					continue
				}
				ll.Info().
					TimeDiff("loadoutDuration", time.Now(), startTime).
					Msg("saved loadout")
			}
		}
		ll.Info().Msg("starting to get pvp games")
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
			ll.Error().Err(err).Msg("[SKIP]: failed to get activities")
			continue
		}
		ll.Info().
			TimeDiff("pvpDuration", time.Now(), startTime).
			Msg("got pvp response")

		if len(activityHistories) == 0 {
			ll.Warn().Msg("[SKIP]: no history found for user")
			continue
		}

		latest := activityHistories[0]

		if session.LastSeenActivityID != nil && *session.LastSeenActivityID == latest.InstanceID {
			ll.Info().Msg("[SKIP]: No new activities since last check-in")
			if IsStaleSession(session) {
				if config.DryRun {
					ll.Info().Msg("[DRY-RUN] would end stale session")
				} else {
					err := EndSession(ctx, db, session.ID)
					if err != nil {
						ll.Error().Err(err).Msg("failed to end session")
						continue
					}
					ll.Info().Msg("session is stale. Ending session")
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
			l.Info().Msg("[SKIP]: No new activity to save. Checking if Inactive")
			if IsInactiveSession(session) {
				if config.DryRun {
					ll.Info().Msg("[DRY-RUN] would end inactive session")
				} else {
					err := EndSession(ctx, db, session.ID)
					if err != nil {
						ll.Error().Err(err).Msg("failed to end session")
						continue
					}
					ll.Info().Msg("session is inactive. Ending session")
				}
				continue
			}
			continue
		}

		ll.Info().Strs("IDs", IDs).Msg("Activities Found")

		existingAggs, err := GetAggregatesByActivity(ctx, db, IDs)
		if err != nil {
			ll.Error().
				Err(err).
				Strs("activityIDs", IDs).Msg("failed to fetch aggregates by the provided IDs")
			continue
		}

		ll.Info().Msgf("Length of existing Aggs: %d", len(existingAggs))

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
				ll.Info().Str("activityId", history.InstanceID).Msg("Already linked to this activity")
				continue
			}

			performances, err := GetPerformances(ctx, cli, db, history.InstanceID, session.CharacterID)
			if err != nil {
				l.Error().Err(err).Msg("failed to fetch performances")
				continue
			}
			performance, ok := performances[session.CharacterID]
			if !ok {
				ll.Warn().Str("userId", session.UserID).Msg("no performance found for member")
				continue
			}

			if config.DryRun {
				ll.Info().Str("activityId", history.InstanceID).Msg("[DRY-RUN] would set aggregate")
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
				l.Error().Err(err).Msg("failed to add data to aggregate")
				continue
			}
			aggIDs = append(aggIDs, a.ID)
		}

		// Only update the session if we actually processed new activities
		if len(aggIDs) == 0 {
			ll.Info().Msg("[SKIP]: All activities already linked. No session update needed")
			continue
		}

		if config.DryRun {
			ll.Info().
				Strs("aggregateIds", aggIDs).
				Str("latestActivityId", latest.InstanceID).
				Msg("[DRY-RUN] would update session with last activity and aggregate IDs")
			continue
		}

		err = SetLastActivity(ctx, db, session.ID, latest.InstanceID)
		if err != nil {
			l.Warn().Err(err).Msg("failed to save last activity for session. Continuing on")
		}

		l.Info().Strs("aggregateIds", aggIDs).Msgf("Aggregates to add")

		err = AddAggregateIDs(ctx, db, session.ID, aggIDs)
		if err != nil {
			l.Error().Err(err).Msg("Failed to add aggregate IDs to session")
			continue
		}
		l.Info().Strs("aggregates", aggIDs).Msg("Added aggregate IDs to session")
	}
	l.Info().Msg("finished going through all sessions")
}
