package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

// GetAggregatesByIDs fetches aggregate documents corresponding to the provided aggregate IDs in batches.
func GetAggregatesByIDs(ctx context.Context, db *firestore.Client, aggregateIDs []string) ([]Aggregate, error) {
	if len(aggregateIDs) == 0 {
		return nil, nil
	}
	const maxBatch = 30
	var out []Aggregate
	for i := 0; i < len(aggregateIDs); i += maxBatch {
		end := i + maxBatch
		if end > len(aggregateIDs) {
			end = len(aggregateIDs)
		}
		batch := aggregateIDs[i:end]
		refs := make([]*firestore.DocumentRef, 0, len(batch))
		for _, id := range batch {
			refs = append(refs, db.Collection(aggregateCollection).Doc(id))
		}
		docs, err := db.GetAll(ctx, refs)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			if !doc.Exists() {
				continue
			}
			var agg Aggregate
			if err := doc.DataTo(&agg); err != nil {
				continue
			}
			out = append(out, agg)
		}
	}
	return out, nil
}

// ComputeSessionSummary calculates performance statistics and loadout weapon summaries for a session.
func ComputeSessionSummary(ctx context.Context, db *firestore.Client, session Session) (*SessionSummary, error) {
	if len(session.AggregateIDs) == 0 {
		return &SessionSummary{
			TotalMatches: 0,
			Wins:         0,
			Losses:       0,
			WinRate:      0,
			Kills:        0,
			Deaths:       0,
			Assists:      0,
			KDRatio:      0,
			KDARatio:     0,
			ModesPlayed:  make([]string, 0),
			TopWeapons:   make([]SessionWeaponSummary, 0),
		}, nil
	}

	aggregates, err := GetAggregatesByIDs(ctx, db, session.AggregateIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch aggregates for session %s: %w", session.ID, err)
	}

	totalMatches := len(aggregates)
	wins := 0
	kills := 0
	deaths := 0
	assists := 0

	modesMap := make(map[string]struct{})
	var modesOrder []string

	type weaponInfo struct {
		hash         int64
		kills        int
		fallbackName string
		fallbackIcon string
	}
	weaponKillsMap := make(map[int64]*weaponInfo)

	var missingModeActivityHashes []int64

	for _, agg := range aggregates {
		if agg.ActivityDetails.Mode != nil && *agg.ActivityDetails.Mode != "" {
			modeName := *agg.ActivityDetails.Mode
			if _, exists := modesMap[modeName]; !exists {
				modesMap[modeName] = struct{}{}
				modesOrder = append(modesOrder, modeName)
			}
		} else if agg.ActivityDetails.ActivityHash != 0 {
			missingModeActivityHashes = append(missingModeActivityHashes, agg.ActivityDetails.ActivityHash)
		} else if agg.ActivityDetails.ReferenceID != 0 {
			missingModeActivityHashes = append(missingModeActivityHashes, agg.ActivityDetails.ReferenceID)
		}

		perf, ok := agg.Performance[session.CharacterID]
		if !ok {
			continue
		}

		// Victory check: Standing 0 indicates win
		if perf.PlayerStats.Standing != nil && perf.PlayerStats.Standing.Value != nil && *perf.PlayerStats.Standing.Value == 0 {
			wins++
		}

		if perf.PlayerStats.Kills != nil && perf.PlayerStats.Kills.Value != nil {
			kills += int(*perf.PlayerStats.Kills.Value)
		}
		if perf.PlayerStats.Deaths != nil && perf.PlayerStats.Deaths.Value != nil {
			deaths += int(*perf.PlayerStats.Deaths.Value)
		}
		if perf.PlayerStats.Assists != nil && perf.PlayerStats.Assists.Value != nil {
			assists += int(*perf.PlayerStats.Assists.Value)
		}

		if perf.Weapons != nil {
			for mapKey, weaponMetrics := range perf.Weapons {
				var refID int64
				if weaponMetrics.ReferenceID != nil && *weaponMetrics.ReferenceID != 0 {
					refID = *weaponMetrics.ReferenceID
				} else {
					parsed, err := strconv.ParseInt(mapKey, 10, 64)
					if err == nil {
						refID = parsed
					}
				}
				if refID == 0 {
					continue
				}

				wKills := 0
				if weaponMetrics.Stats != nil {
					for statKey, statVal := range *weaponMetrics.Stats {
						if statKey == "uniqueWeaponKills" || (statVal.Name != nil && *statVal.Name == "uniqueWeaponKills") {
							if statVal.Basic.Value != nil {
								wKills += int(*statVal.Basic.Value)
							}
						}
					}
				}

				info, exists := weaponKillsMap[refID]
				if !exists {
					info = &weaponInfo{hash: refID}
					weaponKillsMap[refID] = info
				}
				info.kills += wKills

				if weaponMetrics.Display != nil {
					if info.fallbackName == "" && weaponMetrics.Display.Name != "" {
						info.fallbackName = weaponMetrics.Display.Name
					}
					if info.fallbackIcon == "" && weaponMetrics.Display.Icon != nil {
						info.fallbackIcon = *weaponMetrics.Display.Icon
					}
				}
			}
		}
	}

	if len(missingModeActivityHashes) > 0 {
		actDefs, err := GetActivitiesByIDs(ctx, db, missingModeActivityHashes)
		if err == nil && len(actDefs) > 0 {
			var modeIDs []int64
			for _, def := range actDefs {
				if def.DirectActivityModeHash != 0 {
					modeIDs = append(modeIDs, int64(def.DirectActivityModeHash))
				}
				for _, mHash := range def.ActivityModeHashes {
					modeIDs = append(modeIDs, int64(mHash))
				}
			}
			if len(modeIDs) > 0 {
				modeDefs, err := GetActivityModesByIDs(ctx, db, modeIDs)
				if err == nil {
					for _, mDef := range modeDefs {
						name := mDef.DisplayProperties.Name
						if name != "" {
							if _, exists := modesMap[name]; !exists {
								modesMap[name] = struct{}{}
								modesOrder = append(modesOrder, name)
							}
						}
					}
				}
			}
		}
	}

	losses := totalMatches - wins
	var winRate, kdRatio, kdaRatio float64
	if totalMatches > 0 {
		winRate = math.Round((float64(wins)/float64(totalMatches))*100) / 100
		denom := float64(deaths)
		if denom < 1 {
			denom = 1
		}
		kdRatio = math.Round((float64(kills)/denom)*100) / 100
		kdaRatio = math.Round(((float64(kills)+float64(assists)*0.5)/denom)*100) / 100
	}

	var weaponHashes []int64
	for h := range weaponKillsMap {
		weaponHashes = append(weaponHashes, h)
	}

	itemDefs := make(map[string]ItemDefinition)
	if len(weaponHashes) > 0 {
		fetched, err := GetItemsByIDs(ctx, db, weaponHashes)
		if err == nil {
			itemDefs = fetched
		}
	}

	topWeapons := make([]SessionWeaponSummary, 0)
	for hash, info := range weaponKillsMap {
		name := info.fallbackName
		icon := info.fallbackIcon

		if def, found := itemDefs[strconv.FormatInt(hash, 10)]; found {
			if def.DisplayProperties.Name != "" {
				name = def.DisplayProperties.Name
			}
			if def.DisplayProperties.Icon != "" {
				icon = def.DisplayProperties.Icon
			}
		}

		if name == "" {
			name = "Unknown Weapon"
		}
		if icon != "" && !strings.HasPrefix(icon, "http") {
			icon = fmt.Sprintf("https://www.bungie.net%s", icon)
		}

		topWeapons = append(topWeapons, SessionWeaponSummary{
			Name:  name,
			Icon:  icon,
			Kills: info.kills,
		})
	}

	sort.Slice(topWeapons, func(i, j int) bool {
		return topWeapons[i].Kills > topWeapons[j].Kills
	})

	if len(topWeapons) > 5 {
		topWeapons = topWeapons[:5]
	}

	if modesOrder == nil {
		modesOrder = make([]string, 0)
	}

	return &SessionSummary{
		TotalMatches: totalMatches,
		Wins:         wins,
		Losses:       losses,
		WinRate:      winRate,
		Kills:        kills,
		Deaths:       deaths,
		Assists:      assists,
		KDRatio:      kdRatio,
		KDARatio:     kdaRatio,
		ModesPlayed:  modesOrder,
		TopWeapons:   topWeapons,
	}, nil
}

// UpdateSessionSummary updates the sessionSummary field and updatedAt timestamp on a session document in Firestore.
func UpdateSessionSummary(ctx context.Context, db *firestore.Client, sessionID string, summary *SessionSummary) error {
	_, err := db.Collection(SessionCollection).Doc(sessionID).Update(ctx, []firestore.Update{
		{
			Path:  "sessionSummary",
			Value: summary,
		},
		{
			Path:  "updatedAt",
			Value: time.Now(),
		},
	})
	return err
}

// BackfillSessionSummaries iterates over completed sessions missing sessionSummary, computes their summary, and updates Firestore.
func BackfillSessionSummaries(ctx context.Context, db *firestore.Client, dryRun bool) (int, error) {
	slog.Info("starting session summary backfill search", "collection", SessionCollection)
	docs, err := db.Collection(SessionCollection).
		Where("status", "==", SessionComplete).
		Documents(ctx).
		GetAll()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch completed sessions for backfill: %w", err)
	}

	totalCompleted := len(docs)
	slog.Info("fetched completed sessions for backfill evaluation", "totalCompletedSessions", totalCompleted)

	backfilledCount := 0
	alreadyHasSummaryCount := 0
	noAggregatesCount := 0
	errorCount := 0

	for i, doc := range docs {
		var s Session
		if err := doc.DataTo(&s); err != nil {
			slog.Error("failed to parse session for backfill", "docId", doc.Ref.ID, "error", err)
			errorCount++
			continue
		}

		if s.Summary != nil {
			alreadyHasSummaryCount++
			continue
		}

		if len(s.AggregateIDs) == 0 {
			noAggregatesCount++
			slog.Debug("skipping empty completed session during backfill", "sessionId", s.ID)
			continue
		}

		l := slog.With(
			"progress", fmt.Sprintf("%d/%d", i+1, totalCompleted),
			"sessionId", s.ID,
			"characterId", s.CharacterID,
			"aggregateCount", len(s.AggregateIDs),
		)

		summary, err := ComputeSessionSummary(ctx, db, s)
		if err != nil {
			l.Error("failed to compute summary for backfill", "error", err)
			errorCount++
			continue
		}

		summaryDetails := slog.Group("summary",
			"matches", summary.TotalMatches,
			"wins", summary.Wins,
			"losses", summary.Losses,
			"winRate", summary.WinRate,
			"kills", summary.Kills,
			"deaths", summary.Deaths,
			"assists", summary.Assists,
			"kdRatio", summary.KDRatio,
			"kdaRatio", summary.KDARatio,
			"modesPlayed", summary.ModesPlayed,
			"topWeaponsCount", len(summary.TopWeapons),
		)

		if dryRun {
			l.Info("[DRY-RUN] would backfill session summary", summaryDetails)
		} else {
			if err := UpdateSessionSummary(ctx, db, s.ID, summary); err != nil {
				l.Error("failed to update session summary during backfill", "error", err)
				errorCount++
				continue
			}
			l.Info("successfully backfilled session summary", summaryDetails)
		}
		backfilledCount++
	}

	slog.Info("session summary backfill finished",
		"totalCompletedInspected", totalCompleted,
		"backfilled", backfilledCount,
		"alreadyHasSummary", alreadyHasSummaryCount,
		"noAggregates", noAggregatesCount,
		"errors", errorCount,
		"dryRun", dryRun,
	)

	return backfilledCount, nil
}
