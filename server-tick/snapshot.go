package main

import (
	"context"
	"fmt"
	"log/slog"
	"serverTick/bungie"
	"serverTick/generator"
	"serverTick/utils"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/rs/zerolog/log"
)

const (
	snapshotCollection = "snapshots"
	historyCollection  = "histories"
)

func Save(ctx context.Context, db *firestore.Client, client *bungie.ClientWithResponses, userID, membershipID, characterID string) (*CharacterSnapshot, error) {
	data, err := generateSnapshot(ctx, db, client, userID, membershipID, characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to build data: %v", err)
	}
	if data == nil {
		return nil, fmt.Errorf("failed to generate snapshot")
	}
	id, err := create(ctx, db, userID, *data)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}
	data.ID = *id
	return data, nil
}
func generateSnapshot(ctx context.Context, db *firestore.Client, client *bungie.ClientWithResponses, userID, membershipID, characterID string) (*CharacterSnapshot, error) {

	membershipType, _, err := GetMembershipType(ctx, db, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch membership type: %w", err)
	}

	memID, err := strconv.ParseInt(membershipID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid membership id: %w", err)
	}

	loadout, stats, timestamp, err := GetLoadout(ctx, db, client, memID, membershipType, characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profile data: %w", err)
	}
	if timestamp == nil {
		return nil, fmt.Errorf("failed to fetch timestamp for profile data: %w", err)
	}

	return &CharacterSnapshot{
		UserID:      userID,
		CharacterID: characterID,
		Stats:       Of(stats),
		Loadout:     loadout,
	}, nil
}

func create(ctx context.Context, db *firestore.Client, userID string, snapshot CharacterSnapshot) (*string, error) {

	if snapshot.Hash == "" {
		hash, err := utils.HashMap(snapshot.Loadout)
		if err != nil {
			return nil, err
		}
		snapshot.Hash = hash
	}

	existingSnapshot, err := optionalGetByHash(db, ctx, snapshot.Hash)
	if err != nil {
		return nil, err
	}
	if existingSnapshot != nil {
		log.Info().Msg("Creating a history entry")
		return createHistoryEntry(ctx, db, *existingSnapshot)
	}

	snapshot.UserID = userID
	now := time.Now()
	snapshot.CreatedAt = now
	snapshot.UpdatedAt = now
	if snapshot.Name == "" {
		snapshot.Name = generator.PVPName()
	}
	ref := db.Collection(snapshotCollection).NewDoc()
	snapshot.ID = ref.ID
	_, err = ref.Set(ctx, snapshot)
	log.Info().Msg("Created original snapshot")
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Creating a history entry for original snapshot")
	return createHistoryEntry(ctx, db, snapshot)
}

func optionalGetByHash(db *firestore.Client, ctx context.Context, hash string) (*CharacterSnapshot, error) {
	og := CharacterSnapshot{}
	docs, err := db.Collection(snapshotCollection).
		Where("hash", "==", hash).
		Limit(1).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}
	err = docs[0].DataTo(&og)
	return &og, nil
}

func createHistoryEntry(ctx context.Context, db *firestore.Client, og CharacterSnapshot) (*string, error) {
	now := time.Now()
	history := History{
		ParentID:    og.ID,
		UserID:      og.UserID,
		CharacterID: og.CharacterID,
		Timestamp:   now,
		Meta: MetaData{
			KineticID: strconv.FormatInt(og.Loadout[strconv.Itoa(Kinetic)].ItemHash, 10),
			EnergyID:  strconv.FormatInt(og.Loadout[strconv.Itoa(Energy)].ItemHash, 10),
			PowerID:   strconv.FormatInt(og.Loadout[strconv.Itoa(Power)].ItemHash, 10),
		},
	}
	ref := db.Collection(snapshotCollection).Doc(og.ID).Collection(historyCollection).NewDoc()
	history.ID = ref.ID
	_, err := ref.Set(ctx, history)
	if err != nil {
		return nil, err
	}

	_, err = db.Collection(snapshotCollection).Doc(og.ID).Set(ctx, map[string]interface{}{
		"updatedAt": now,
	}, firestore.MergeAll)
	if err != nil {
		return nil, err
	}
	return &og.ID, nil
}

func FindBestFit(ctx context.Context, db *firestore.Client, userID string, characterID string, activityPeriod time.Time, weapons map[string]WeaponInstanceMetrics) (*CharacterSnapshot, *SnapshotLink, error) {

	minTime := activityPeriod.Add(time.Duration(-12) * time.Hour)
	// A game can last about 8 minutes over the starting time
	maxTime := activityPeriod.Add(time.Duration(15) * time.Minute)
	l := slog.With(
		"activityPeriod", activityPeriod,
		"minTime", minTime,
		"maxTime", maxTime,
		"userId", userID,
		"characterId", characterID,
	)
	docs, err := db.CollectionGroup(historyCollection).
		Where("userId", "==", userID).
		Where("characterId", "==", characterID).
		Where("timestamp", ">=", minTime).
		Where("timestamp", "<=", maxTime).
		OrderBy("timestamp", firestore.Desc).
		Documents(ctx).GetAll()
	if err != nil {
		l.Error("failed to get histories", "error", err.Error())
		return nil, nil, err
	}

	if docs == nil || len(docs) == 0 {
		link := SnapshotLink{
			CharacterID:      characterID,
			ConfidenceLevel:  NotFoundConfidenceLevel,
			ConfidenceSource: SystemConfidenceSource,
			CreatedAt:        time.Now(),
		}
		return nil, &link, nil
	}

	var (
		bestFit      *History
		bestFitScore = 0
	)
	histories, err := utils.GetAllToStructs[History](docs)
	if err != nil {
		l.Error("failed to get all histories", "error", err.Error())
		return nil, nil, err
	}

	weaponSet := make(map[string]bool)
	for _, weapon := range weapons {
		if weapon.ReferenceID != nil {
			weaponSet[strconv.FormatInt(*weapon.ReferenceID, 10)] = true
		}
	}
	for _, h := range histories {
		matches := 0
		if weaponSet[h.Meta.KineticID] {
			matches += 2
		}
		if weaponSet[h.Meta.EnergyID] {
			matches += 2
		}
		if weaponSet[h.Meta.PowerID] {
			matches++
		}

		if bestFit == nil && matches >= 1 {
			bestFit = &h
			bestFitScore = matches
			continue
		}
		if matches > bestFitScore {
			bestFit = &h
			bestFitScore = matches
		}
	}

	if bestFit == nil {
		link := SnapshotLink{
			CharacterID:      characterID,
			ConfidenceLevel:  NoMatchConfidenceLevel,
			ConfidenceSource: SystemConfidenceSource,
			CreatedAt:        time.Now(),
		}
		return nil, &link, nil
	}
	level := LowConfidenceLevel
	if bestFitScore >= 4 {
		level = HighConfidenceLevel
	} else if bestFitScore >= 2 {
		level = MediumConfidenceLevel
	}

	link := SnapshotLink{
		CharacterID:      characterID,
		ConfidenceLevel:  level,
		ConfidenceSource: SystemConfidenceSource,
		CreatedAt:        time.Now(),
		SnapshotID:       &bestFit.ParentID,
	}

	snap, err := Get(ctx, db, bestFit.ParentID)
	if err != nil {
		l.Error("failed to get snapshot", "error", err.Error())
		return nil, nil, err
	}
	return snap, &link, nil
}
func Get(ctx context.Context, db *firestore.Client, snapshotID string) (*CharacterSnapshot, error) {
	var result *CharacterSnapshot
	data, err := db.Collection(snapshotCollection).Doc(snapshotID).Get(ctx)
	if err != nil {
		return nil, err
	}
	err = data.DataTo(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func EnrichInstancePerformance(snapshot *CharacterSnapshot, performance InstancePerformance) (*InstancePerformance, error) {
	result := &InstancePerformance{
		Extra:       performance.Extra,
		PlayerStats: performance.PlayerStats,
		Weapons:     performance.Weapons,
	}
	if snapshot == nil {
		log.Debug().Msg("No provided snapshot to perform enrichment on")
		return result, nil
	}

	if len(performance.Weapons) == 0 {
		log.Debug().Msg("No metrics provided to enrich")
		return result, nil
	}
	if snapshot.Loadout == nil {
		log.Debug().Msg("No loadout provided to enrich")
		return result, nil
	}

	mapping := map[int64]ItemProperties{}
	for _, component := range snapshot.Loadout {
		mapping[component.ItemHash] = component.ItemProperties
	}

	results := make(map[string]WeaponInstanceMetrics)
	for _, metric := range performance.Weapons {
		result := WeaponInstanceMetrics{}
		if metric.ReferenceID == nil {
			continue
		}
		result.ReferenceID = metric.ReferenceID
		result.Stats = metric.Stats

		properties, ok := mapping[*metric.ReferenceID]
		if ok {
			result.ItemProperties = &properties
		}
		results[strconv.FormatInt(*metric.ReferenceID, 10)] = result
	}
	result.Weapons = results
	return result, nil
}

type CharacterSnapshot struct {
	// CharacterID Id of the character being recorded
	CharacterID string `firestore:"characterId" json:"characterId"`

	// CreatedAt Timestamp for when the snapshot was first created
	CreatedAt time.Time `firestore:"createdAt" json:"createdAt"`

	// Hash Hash of all the items to give us a unique key
	Hash string `firestore:"hash" json:"hash"`

	// ID Id of the snapshot
	ID string `firestore:"id" json:"id"`

	// Loadout All buckets that we currently care about, Kinetic, Energy, Heavy and Class for now. Each will be a key in the items.
	Loadout Loadout `firestore:"loadout" json:"loadout"`

	// Name Name of the snapshot, will probably be generated by default by the system but can be changed by a user
	Name  string                `firestore:"name" json:"name"`
	Stats *map[string]ClassStat `firestore:"stats" json:"stats,omitempty"`

	// UpdatedAt Timestamp for when the snapshot was last updated or when a history entry was made for it.
	UpdatedAt time.Time `firestore:"updatedAt" json:"updatedAt"`

	// UserID Id of the user it belongs to
	UserID string `firestore:"userId" json:"userId"`
}
type Loadout map[string]ItemSnapshot

type ItemSnapshot struct {
	// BucketHash Hash of which bucket this item can be equipped in
	BucketHash *int64 `firestore:"bucketHash" json:"bucketHash,omitempty"`

	// ItemProperties The response object for retrieving an individual instanced item. None of these components are relevant for an item that doesn't have an "itemInstanceId": for those, get your information from the DestinyInventoryDefinition.
	ItemProperties ItemProperties `firestore:"itemProperties" json:"details"`

	// InstanceID Specific instance id for the item
	InstanceID string `firestore:"instanceId" json:"instanceId"`

	// ItemHash Id used to find the definition of the item
	ItemHash int64 `firestore:"itemHash" json:"itemHash"`

	// Name Name of the particular item
	Name string `firestore:"name" json:"name"`
}
type ClassStat struct {
	AggregationType int    `firestore:"description" json:"aggregationType"`
	Description     string `firestore:"description" json:"description"`
	HasIcon         bool   `firestore:"hasIcon" json:"hasIcon"`
	Icon            string `firestore:"icon" json:"icon"`
	Name            string `firestore:"name" json:"name"`
	StatCategory    int    `firestore:"description" json:"statCategory"`
	Value           int32  `firestore:"value" json:"value"`
}

const (
	HighConfidenceLevel     ConfidenceLevel = "high"
	LowConfidenceLevel      ConfidenceLevel = "low"
	MediumConfidenceLevel   ConfidenceLevel = "medium"
	NoMatchConfidenceLevel  ConfidenceLevel = "noMatch"
	NotFoundConfidenceLevel ConfidenceLevel = "notFound"
)

// Defines values for ConfidenceSource.
const (
	SystemConfidenceSource ConfidenceSource = "system"
	UserConfidenceSource   ConfidenceSource = "user"
)

type History struct {
	ID          string    `json:"id" firestore:"id"`
	UserID      string    `json:"userId" firestore:"userId"`
	CharacterID string    `json:"characterId" firestore:"characterId"`
	ParentID    string    `json:"parentId" firestore:"parentId"`
	Timestamp   time.Time `json:"timestamp" firestore:"timestamp"`
	Meta        MetaData  `json:"meta" firestore:"meta"`
}

type MetaData struct {
	KineticID string `json:"kineticId" firestore:"kineticId"`
	EnergyID  string `json:"energyId" firestore:"energyId"`
	PowerID   string `json:"powerId" firestore:"powerId"`
}
