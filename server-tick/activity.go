package main

import (
	"context"
	"fmt"
	"net/http"
	"serverTick/bungie"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/rs/zerolog/log"
)

func GetAllPVP(ctx context.Context, client *bungie.ClientWithResponses, db *firestore.Client, membershipID string, membershipType int64, characterID string, count int64, page int64) (
	[]ActivityHistory,
	error,
) {
	cID, err := strconv.ParseInt(characterID, 10, 64)
	if err != nil {
		return nil, err
	}
	mID, err := strconv.ParseInt(membershipID, 10, 64)
	if err != nil {
		return nil, err
	}
	resp, err := client.Destiny2GetActivityHistoryWithResponse(
		ctx,
		int32(membershipType),
		mID,
		cID,
		&bungie.Destiny2GetActivityHistoryParams{
			Count: Of(int32(count)),
			Mode:  Of(int32(5)), // ALL PVP
			Page:  Of(int32(page)),
		},
	)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("failed to get activity history")
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("no response found")
	}
	if resp.JSON200.Response == nil {
		return nil, fmt.Errorf("no response found")
	}
	if resp.JSON200.Response.Activities == nil {
		return nil, fmt.Errorf("no definitions found")
	}

	source := *resp.JSON200.Response.Activities
	var (
		hashes         = make([]int64, 0)
		directorHashes = make([]int64, 0)
		modeIDs        = make(map[int64]bool)
	)
	for _, period := range source {
		hashes = append(hashes, int64(*period.ActivityDetails.ReferenceId))
		directorHashes = append(directorHashes, int64(*period.ActivityDetails.DirectorActivityHash))
	}

	definitions, err := GetActivitiesByIDs(ctx, db, hashes)
	if err != nil {
		return nil, err
	}
	directorDefinitions, err := GetActivitiesByIDs(ctx, db, directorHashes)
	if err != nil {
		return nil, err
	}

	for _, definition := range directorDefinitions {
		modeIDs[int64(definition.DirectActivityModeHash)] = true
	}
	var ids []int64
	for ID := range modeIDs {
		ids = append(ids, ID)
	}

	modes, err := GetActivityModesByIDs(ctx, db, ids)
	if err != nil {
		return nil, err
	}

	return TransformPeriodGroups(*resp.JSON200.Response.Activities, definitions, directorDefinitions, modes), nil
}

type ActivityHistory struct {
	Activity string `firestore:"activity" json:"activity"`

	// ActivityHash Hash id of the type of activity: Strike, Competitive, QuickPlay, etc.
	ActivityHash int64 `firestore:"activityHash" json:"activityHash"`

	// ActivityIcon URL to the icon for the type of activity, IB, Crucible, etc.
	ActivityIcon string `firestore:"activityIcon" json:"activityIcon"`
	Description  string `firestore:"description" json:"description"`

	// ImageURL URL for the image of the destination activity
	ImageURL string `firestore:"imageUrl" json:"imageUrl"`

	// InstanceID Id to get more details about the particular game
	InstanceID string `firestore:"instanceId" json:"instanceId"`
	IsPrivate  *bool  `firestore:"isPrivate" json:"isPrivate,omitempty"`
	Location   string `firestore:"location" json:"location"`

	// Mode Name
	Mode        *string   `firestore:"mode" json:"mode,omitempty"`
	Period      time.Time `firestore:"period" json:"period"`
	ReferenceID int64     `firestore:"referenceId" json:"referenceId"`
}

func Of[T any](value T) *T {
	return &value
}

func setBaseBungieURL(value *string) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%s%s", "https://www.bungie.net", *value)
}

func GetLoadout(ctx context.Context, db *firestore.Client, client *bungie.ClientWithResponses, membershipID int64, membershipType int64, characterID string) (Loadout, map[string]ClassStat, *time.Time, error) {
	var components []int32
	components = append(components, CharactersEquipment, CharactersCode)
	params := &bungie.Destiny2GetProfileParams{
		Components: &components,
	}
	test, err := client.Destiny2GetProfileWithResponse(ctx, int32(membershipType), membershipID, params)
	if err != nil {
		return nil, nil, nil, err
	}

	// TODO: Migrate snapshot to include the guns information as it is now, since mods and perks could change on the same gun.

	if test.JSON200 == nil {
		return nil, nil, nil, fmt.Errorf("no response found")
	}

	timeStamp := test.JSON200.Response.ResponseMintedTimestamp

	results := make([]bungie.ItemComponent, 0)
	if test.JSON200.Response.CharacterEquipment.Data != nil {
		equipment := *test.JSON200.Response.CharacterEquipment.Data
		for ID, equ := range equipment {
			if characterID == ID {
				if equ.Items == nil {
					continue
				}
				buckets := map[uint32]bool{
					HelmetArmor:    true,
					GauntletsArmor: true,
					ChestArmor:     true,
					LegArmor:       true,
					ClassArmor:     true,
					KineticBucket:  true,
					EnergyBucket:   true,
					PowerBucket:    true,
					SubClass:       true,
				}

				for _, item := range *equ.Items {
					if item.BucketHash == nil {
						continue
					}
					if buckets[*item.BucketHash] {
						results = append(results, item)
					}
				}

			}
		}

	}

	statDefinitions, err := GetStats(ctx, db)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get statDefinitions but still will generate stats")
	}
	stats := make(map[string]ClassStat)
	if test.JSON200.Response.Characters.Data != nil {
		characters := *test.JSON200.Response.Characters.Data
		for ID, character := range characters {
			if characterID == ID && character.Stats != nil {
				stats = generateClassStats(statDefinitions, *character.Stats)
			}
		}

	}
	loadout, err := buildLoadout(ctx, db, client, membershipID, membershipType, results, statDefinitions)
	if err != nil {
		log.Error().Err(err).Msg("couldn't build the loadout")
		return nil, nil, nil, err
	}
	return loadout, stats, timeStamp, nil
}
