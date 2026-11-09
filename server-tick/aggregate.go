package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"serverTick/bungie"
	"serverTick/utils"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	clog "github.com/Shannor/canonical-logger"
	"google.golang.org/api/iterator"
)

const aggregateCollection = "aggregates"

func GetAggregatesByActivity(ctx context.Context, db *firestore.Client, activityIDs []string) ([]Aggregate, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	start := time.Now()
	docs, err := db.
		Collection(aggregateCollection).
		Where("activityId", "in", activityIDs).
		Documents(ctx).GetAll()
	if err != nil {
		clog.WithGroupFields(ctx, "db.getAggregatesByActivity", clog.Fields{
			"collection":       aggregateCollection,
			"activityIdsCount": len(activityIDs),
			"durationMs":       time.Since(start).Milliseconds(),
		})
		return nil, err
	}
	results, err := utils.GetAllToStructs[Aggregate](docs)
	if err != nil {
		return nil, err
	}
	clog.WithGroupFields(ctx, "db.getAggregatesByActivity", clog.Fields{
		"collection":       aggregateCollection,
		"activityIdsCount": len(activityIDs),
		"rowsReturned":     len(results),
		"durationMs":       time.Since(start).Milliseconds(),
	})
	return results, nil
}

func GetPerformances(ctx context.Context, client *bungie.ClientWithResponses, db *firestore.Client, activityID string, characterID string) (map[string]InstancePerformance, error) {
	start := time.Now()
	id, err := strconv.ParseInt(activityID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid activity ID: %w", err)
	}

	apiStart := time.Now()
	resp, err := client.Destiny2GetPostGameCarnageReportWithResponse(ctx, id)
	if err != nil {
		clog.WithGroupFields(ctx, "bungieClient.getPostGameCarnageReport", clog.Fields{
			"activityId": activityID,
			"durationMs": time.Since(apiStart).Milliseconds(),
		})
		return nil, err
	}
	clog.WithGroupFields(ctx, "bungieClient.getPostGameCarnageReport", clog.Fields{
		"activityId": activityID,
		"statusCode": resp.StatusCode(),
		"durationMs": time.Since(apiStart).Milliseconds(),
	})
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to get carnage report (status %d)", resp.StatusCode())
	}
	data := resp.JSON200.PostGameCarnageReportData
	if data.Entries == nil || data.ActivityDetails == nil {
		return nil, fmt.Errorf("nil data response")
	}

	performances := make(map[string]InstancePerformance)
	items := buildItemsSet(ctx, db, data, characterID)
	for _, entry := range *data.Entries {
		if entry.CharacterId == nil {
			continue
		}
		if characterID == *entry.CharacterId {
			p := CarnageEntryToInstancePerformance(&entry, items)
			if p == nil {
				continue
			}
			performances[*entry.CharacterId] = *p
		}
	}

	clog.WithGroupFields(ctx, "aggregateService.getPerformances", clog.Fields{
		"activityId":          activityID,
		"characterId":         characterID,
		"performancesMatched": len(performances),
		"durationMs":          time.Since(start).Milliseconds(),
	})
	return performances, nil
}

func SetAggregate(ctx context.Context, db *firestore.Client, userID string, characterID string, activity ActivityHistory, period time.Time, performance InstancePerformance, sessionID string) (*Aggregate, error) {
	start := time.Now()
	snap, link, err := FindBestFit(ctx, db, userID, characterID, period, performance.Weapons)
	if err != nil {
		return nil, err
	}

	enrichedPerformance, err := EnrichInstancePerformance(snap, performance)
	if err != nil {
		return nil, fmt.Errorf("failed to enrich performance instance: %w", err)
	}

	link.SessionID = &sessionID

	agg, err := AddAggregate(ctx, db, characterID, activity, *link, *enrichedPerformance)
	if err != nil {
		return nil, err
	}
	clog.WithGroupFields(ctx, "aggregateService.setAggregate", clog.Fields{
		"aggregateId": agg.ID,
		"activityId":  activity.InstanceID,
		"sessionId":   sessionID,
		"durationMs":  time.Since(start).Milliseconds(),
	})
	return agg, nil
}

func AddAggregate(ctx context.Context, db *firestore.Client, characterID string, history ActivityHistory, snapshotLink SnapshotLink, performance InstancePerformance) (*Aggregate, error) {
	start := time.Now()
	now := time.Now()
	sessionIDs := make([]string, 0)
	snapshotIDs := make([]string, 0)
	characterIDs := make([]string, 0)

	if snapshotLink.SessionID != nil {
		sessionIDs = append(sessionIDs, *snapshotLink.SessionID)
	}
	if snapshotLink.SnapshotID != nil {
		snapshotIDs = append(snapshotIDs, *snapshotLink.SnapshotID)
	}
	characterIDs = append(characterIDs, characterID)
	aggregate := Aggregate{
		ActivityID:      history.InstanceID,
		ActivityDetails: history,
		SnapshotLinks: map[string]SnapshotLink{
			characterID: snapshotLink,
		},
		Performance: map[string]InstancePerformance{
			characterID: performance,
		},
		SessionIDs:   sessionIDs,
		SnapshotIDs:  snapshotIDs,
		CharacterIDs: characterIDs,
		CreatedAt:    now,
	}

	iter := db.Collection(aggregateCollection).
		Where("activityId", "==", history.InstanceID).
		Limit(1).
		Documents(ctx)
	var (
		existingAggregate *Aggregate
	)
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		err = doc.DataTo(&existingAggregate)
		if err != nil {
			return nil, err
		}
	}
	if existingAggregate != nil {
		// Partial update, adding the new data
		updates := map[string]any{
			"snapshotLinks": map[string]any{
				characterID: snapshotLink,
			},
			"performance": map[string]any{
				characterID: performance,
			},
			"sessionIds":   firestore.ArrayUnion(toInterfaceSlice(sessionIDs)...),
			"snapshotIds":  firestore.ArrayUnion(toInterfaceSlice(snapshotIDs)...),
			"characterIds": firestore.ArrayUnion(toInterfaceSlice(characterIDs)...),
		}
		if history.EstimatedGameMode != nil && existingAggregate.ActivityDetails.EstimatedGameMode == nil {
			updates["activityHistory"] = map[string]any{
				"estimatedGameMode": *history.EstimatedGameMode,
			}
			existingAggregate.ActivityDetails.EstimatedGameMode = history.EstimatedGameMode
		}
		_, err := db.Collection(aggregateCollection).Doc(existingAggregate.ID).Set(ctx, updates, firestore.MergeAll)
		if err != nil {
			return nil, err
		}
		existingAggregate.SnapshotLinks[characterID] = snapshotLink
		existingAggregate.Performance[characterID] = performance
		clog.WithGroupFields(ctx, "db.addAggregate", clog.Fields{
			"collection":      aggregateCollection,
			"aggregateId":     existingAggregate.ID,
			"activityId":      history.InstanceID,
			"updatedExisting": true,
			"durationMs":      time.Since(start).Milliseconds(),
		})
		return existingAggregate, nil
	} else {
		// Create new Doc and return object
		ref := db.Collection(aggregateCollection).NewDoc()
		aggregate.ID = ref.ID
		_, err := ref.Set(ctx, aggregate)
		if err != nil {
			return nil, err
		}
		clog.WithGroupFields(ctx, "db.addAggregate", clog.Fields{
			"collection":      aggregateCollection,
			"aggregateId":     aggregate.ID,
			"activityId":      history.InstanceID,
			"updatedExisting": false,
			"durationMs":      time.Since(start).Milliseconds(),
		})
		return &aggregate, nil
	}
}

// Helper function to convert any slice to []interface{}
func toInterfaceSlice[T any](slice []T) []any {
	result := make([]any, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

func LookupLink(agg *Aggregate, characterID string) *SnapshotLink {
	if agg == nil {
		return nil
	}
	link, ok := agg.SnapshotLinks[characterID]
	if !ok {
		return nil
	}
	return &link
}

func buildItemsSet(ctx context.Context, db *firestore.Client, data *bungie.PostGameCarnageReportData, characterID string) map[string]ItemDefinition {
	items := make(map[string]ItemDefinition)
	for _, entry := range *data.Entries {
		if entry.CharacterId == nil {
			continue
		}
		if characterID == *entry.CharacterId {
			if entry.Extended.Weapons != nil {
				for _, stats := range *entry.Extended.Weapons {
					if stats.ReferenceId != nil {
						id := *stats.ReferenceId
						item, err := GetItem(ctx, db, int64(id))
						if err != nil {
							continue
						}
						if item != nil {
							items[strconv.FormatInt(item.Hash, 10)] = *item
						}
					}
				}
			}
		}
	}
	return items
}

// BackfillEstimatedGameModes iterates over all aggregate documents in Firestore,
// computes estimatedGameMode from activityHistory.mode and activityHistory.activity,
// and persists it on any document where it is missing or differs.
func BackfillEstimatedGameModes(ctx context.Context, db *firestore.Client, dryRun bool) (int, error) {
	start := time.Now()
	docs, err := db.Collection(aggregateCollection).Documents(ctx).GetAll()
	if err != nil {
		clog.WithGroupFields(ctx, "aggregateService.backfillGameModes", clog.Fields{
			"collection": aggregateCollection,
			"dryRun":     dryRun,
			"durationMs": time.Since(start).Milliseconds(),
		})
		return 0, fmt.Errorf("failed to fetch aggregates for game mode backfill: %w", err)
	}

	var (
		updatedCount      int
		alreadySetCount   int
		unclassifiedCount int
		byGameModeCounts  = make(map[string]int)
	)

	_ = clog.LoopRollup(ctx, "aggregateService.backfillGameModeItem", docs, func(iterCtx context.Context, _ int, doc *firestore.DocumentSnapshot) error {
		defer func() {
			if bag := clog.BagFromContext(iterCtx); bag != nil && bag.HasWarning() {
				clog.WithGroupField(ctx, "aggregateService.backfillGameModeItem", "warning", true)
			}
		}()
		clog.WithGroupField(iterCtx, "aggregate", "docId", doc.Ref.ID)

		var agg Aggregate
		if err := doc.DataTo(&agg); err != nil {
			return fmt.Errorf("failed to parse aggregate %s: %w", doc.Ref.ID, err)
		}

		estimated := EstimateGameMode(agg.ActivityDetails.Mode, agg.ActivityDetails.Activity)
		if estimated == nil {
			unclassifiedCount++
			modeVal := ""
			if agg.ActivityDetails.Mode != nil {
				modeVal = *agg.ActivityDetails.Mode
			}
			clog.WithGroupField(ctx, "aggregateService.backfillGameModes", "warning", clog.Fields{
				"type":     "unclassified_game_mode",
				"docId":    doc.Ref.ID,
				"mode":     modeVal,
				"activity": agg.ActivityDetails.Activity,
			})
			return nil
		}

		byGameModeCounts[string(*estimated)]++

		if agg.ActivityDetails.EstimatedGameMode != nil && *agg.ActivityDetails.EstimatedGameMode == *estimated {
			alreadySetCount++
			return nil
		}

		if !dryRun {
			_, err := doc.Ref.Update(iterCtx, []firestore.Update{
				{
					Path:  "activityHistory.estimatedGameMode",
					Value: *estimated,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to update estimatedGameMode on aggregate %s: %w", doc.Ref.ID, err)
			}
		}
		updatedCount++
		return nil
	})

	clog.WithGroupFields(ctx, "aggregateService.backfillGameModes", clog.Fields{
		"collection":        aggregateCollection,
		"totalInspected":    len(docs),
		"updatedCount":      updatedCount,
		"alreadySetCount":   alreadySetCount,
		"unclassifiedCount": unclassifiedCount,
		"byGameMode":        byGameModeCounts,
		"dryRun":            dryRun,
		"durationMs":        time.Since(start).Milliseconds(),
	})

	return updatedCount, nil
}

type Aggregate struct {
	ActivityDetails ActivityHistory                `firestore:"activityHistory" json:"activityDetails"`
	ActivityID      string                         `firestore:"activityId" json:"activityId"`
	CreatedAt       time.Time                      `firestore:"createdAt" json:"createdAt"`
	ID              string                         `firestore:"id" json:"id"`
	Performance     map[string]InstancePerformance `firestore:"performance" json:"performance"`
	SnapshotLinks   map[string]SnapshotLink        `firestore:"snapshotLinks" json:"snapshotLinks"`
	SnapshotIDs     []string                       `firestore:"snapshotIds" json:"snapshotIds"`
	SessionIDs      []string                       `firestore:"sessionIds" json:"sessionIds"`
	CharacterIDs    []string                       `firestore:"characterIds" json:"characterIds"`
}
type InstancePerformance struct {
	Extra *map[string]UniqueStatValue `firestore:"extra" json:"extra,omitempty"`

	// PlayerStats All Player Stats from a match that we currently care about
	PlayerStats PlayerStats                      `firestore:"playerStats" json:"playerStats"`
	Weapons     map[string]WeaponInstanceMetrics `firestore:"weapons" json:"weapons"`
}
type WeaponInstanceMetrics struct {
	Display *Display `firestore:"display" json:"display,omitempty"`

	// ItemProperties The response object for retrieving an individual instanced item. None of these components are relevant for an item that doesn't have an "itemInstanceId": for those, get your information from the DestinyInventoryDefinition.
	ItemProperties *ItemProperties `firestore:"itemProperties" json:"properties,omitempty"`

	// ReferenceID The hash ID of the item definition that describes the weapon.
	ReferenceID *int64                      `firestore:"referenceId" json:"referenceId,omitempty"`
	Stats       *map[string]UniqueStatValue `firestore:"stats" json:"stats,omitempty"`
}
type ItemProperties struct {
	BaseInfo BaseItemInfo `firestore:"baseItemInfo" json:"baseInfo"`

	// CharacterId If the item is on a character, this will return the ID of the character that is holding the item.
	CharacterId *string `firestore:"characterId" json:"characterId"`

	// Perks Information specifically about the perks currently active on the item. COMPONENT TYPE: ItemPerks
	Perks []Perk `firestore:"perks" json:"perks"`

	// Sockets Information about the sockets of the item: which are currently active, what potential sockets you could have and the stats/abilities/perks you can gain from them. COMPONENT TYPE: ItemSockets
	Sockets *[]Socket `firestore:"sockets" json:"sockets,omitempty"`

	// Stats Information about the computed stats of the item: power, defense, etc... COMPONENT TYPE: ItemStats
	Stats Stats `firestore:"stats" json:"stats"`
}
type BaseItemInfo struct {
	BucketHash                 int64         `firestore:"bucketHash" json:"bucketHash"`
	Damage                     *DamageInfo   `firestore:"damageInfo" json:"damage,omitempty"`
	InstanceId                 string        `firestore:"instanceId" json:"instanceId"`
	ItemHash                   int64         `firestore:"itemHash" json:"itemHash"`
	Name                       string        `firestore:"name" json:"name"`
	Icon                       string        `firestore:"icon" json:"icon"`
	ItemTypeAndTierDisplayName string        `firestore:"itemTypeAndTierDisplayName" json:"itemTypeAndTierDisplayName"`
	ItemTypeDisplayName        string        `firestore:"itemTypeDisplayName" json:"itemTypeDisplayName"`
	TierTypeName               string        `firestore:"tierTypeName" json:"tierTypeName"`
	TierType                   int           `firestore:"tierType" json:"tierType"`
	StyleBasicInfo             *BaseItemInfo `firestore:"styleBasicInfo" json:"styleBasicInfo"`
}
type DamageInfo struct {
	Color           Color  `firestore:"color" json:"color"`
	DamageIcon      string `firestore:"damageIcon" json:"damageIcon"`
	DamageType      string `firestore:"damageType" json:"damageType"`
	TransparentIcon string `firestore:"transparentIcon" json:"transparentIcon"`
}
type Color struct {
	Alpha int `firestore:"alpha" json:"alpha"`
	Blue  int `firestore:"blue" json:"blue"`
	Green int `firestore:"green" json:"green"`
	Red   int `firestore:"red" json:"red"`
}
type Perk struct {
	Description *string `firestore:"description" json:"description,omitempty"`

	// Hash The hash ID of the perk
	Hash int64 `firestore:"hash" json:"hash"`

	// IconPath link to icon
	IconPath *string `firestore:"iconPath" json:"iconPath,omitempty"`
	Name     string  `firestore:"name" json:"name"`
}
type Socket struct {
	Description string  `firestore:"description" json:"description"`
	Icon        *string `firestore:"icon" json:"icon,omitempty"`

	// IsEnabled Whether the socket plug is enabled or not.
	IsEnabled *bool `firestore:"isEnabled" json:"isEnabled,omitempty"`

	// IsVisible Whether the socket plug is visible or not.
	IsVisible                 *bool   `firestore:"isVisible" json:"isVisible,omitempty"`
	ItemTypeDisplayName       *string `firestore:"itemTypeDisplayName" json:"itemTypeDisplayName,omitempty"`
	ItemTypeTieredDisplayName *string `firestore:"itemTypeTieredDisplayName" json:"itemTypeTieredDisplayName,omitempty"`
	Name                      string  `firestore:"name" json:"name"`

	// PlugHash The hash ID of the socket plug.
	PlugHash int `firestore:"plugHash" json:"plugHash"`
}

type Stats map[string]GunStat
type GunStat struct {
	Description string `firestore:"description" json:"description"`

	// Hash The hash ID of the stat.
	Hash int64  `firestore:"hash" json:"hash"`
	Name string `firestore:"name" json:"name"`

	// Value The value of the stat.
	Value int64 `firestore:"value" json:"value"`
}
type UniqueStatValue struct {
	// ActivityID When a stat represents the best, most, longest, fastest or some other personal best, the actual activity ID where that personal best was established is available on this property.
	ActivityID *int64 `firestore:"activityId" json:"activityId"`

	// Basic Basic stat value.
	Basic StatsValuePair `firestore:"basic" json:"basic"`
	Name  *string        `firestore:"name" json:"name,omitempty"`

	// Pga Per game average for the statistic, if applicable
	Pga *StatsValuePair `firestore:"pga" json:"pga,omitempty"`

	// Weighted Weighted value of the stat if a weight greater than 1 has been assigned.
	Weighted *StatsValuePair `firestore:"weighted" json:"weighted,omitempty"`
}

type StatsValuePair struct {
	// DisplayValue Localized formatted version of the value.
	DisplayValue *string `firestore:"displayValue" json:"displayValue,omitempty"`

	// Value Raw value of the statistic
	Value *float64 `firestore:"value" json:"value,omitempty"`
}
type Display struct {
	Description string  `firestore:"description" json:"description"`
	HasIcon     bool    `firestore:"hasIcon" json:"hasIcon"`
	Icon        *string `firestore:"icon" json:"icon,omitempty"`
	Name        string  `firestore:"name" json:"name"`
}
type PlayerStats struct {
	// Assists Number of assists done in the match
	Assists *StatsValuePair `firestore:"assists" json:"assists,omitempty"`

	// Deaths Number of deaths done in the match
	Deaths *StatsValuePair `firestore:"deaths" json:"deaths,omitempty"`

	// FireTeamID ID for the fireteam player was on. If the same as another player then they were together
	FireTeamID *StatsValuePair `firestore:"fireTeamId" json:"fireTeamId,omitempty"`

	// Kd ratio of kill / deaths in the match
	Kd *StatsValuePair `firestore:"kd" json:"kd,omitempty"`

	// Kda ratio of kills + assists/ deaths in the match
	Kda *StatsValuePair `firestore:"kda" json:"kda,omitempty"`

	// Kills Number of kills done in the match
	Kills *StatsValuePair `firestore:"kills" json:"kills,omitempty"`

	// Standing Win or lose in the match
	Standing *StatsValuePair `firestore:"standing" json:"standing,omitempty"`

	// Team Id for the team the player was on this match
	Team *StatsValuePair `firestore:"team" json:"team,omitempty"`

	// TimePlayed Time in seconds the player was in the match
	TimePlayed *StatsValuePair `firestore:"timePlayed" json:"timePlayed,omitempty"`
}
type SnapshotLink struct {
	CharacterID      string           `firestore:"characterId" json:"characterId"`
	ConfidenceLevel  ConfidenceLevel  `firestore:"confidenceLevel" json:"confidenceLevel"`
	ConfidenceSource ConfidenceSource `firestore:"confidenceSource" json:"confidenceSource"`
	CreatedAt        time.Time        `firestore:"createdAt" json:"createdAt"`

	// SessionID Optional ID of a session if this Snapshot link was added by a session check-in. Will be null in the case, where the link is added after the fact
	SessionID *string `firestore:"sessionId" json:"sessionId,omitempty"`

	// SnapshotID ID of the snapshot for the particular player
	SnapshotID *string `firestore:"snapshotId" json:"snapshotId,omitempty"`
}
type ConfidenceLevel string

// ConfidenceSource defines model for ConfidenceSource.
type ConfidenceSource string
