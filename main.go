package main

import (
	"cloud.google.com/go/firestore"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"
)

type Config struct {
	taskNum    int64
	attemptNum string
}

func SetBaseUrl(value *string) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%s%s", "https://www.bungie.net", *value)
}

func configFromEnv() (Config, error) {
	taskNum, err := stringToInt(os.Getenv("CLOUD_RUN_TASK_INDEX"))
	attemptNum := os.Getenv("CLOUD_RUN_TASK_ATTEMPT")

	if err != nil {
		return Config{}, err
	}

	config := Config{
		taskNum:    taskNum,
		attemptNum: attemptNum,
	}
	return config, nil
}

func stringToInt(s string) (int64, error) {
	sleepMs, err := strconv.ParseInt(s, 10, 64)
	return sleepMs, err
}

const (
	projectID = "gruntt-destiny"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	config, err := configFromEnv()
	if err != nil {
		log.Fatal().Err(err)
	}
	ctx := context.Background()

	db, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to create client: %v", err)
	}

	snapshot, err := db.Collection(ConfigurationCollection).Doc(DestinyDocument).Get(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get db information")
	}

	var data Configuration
	err = snapshot.DataTo(&data)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read into configuration")
	}
	currentVersion, ok := GetVersionByIndex(data, config.taskNum)
	if !ok {
		log.Info().Msg("No matching version found")
		return
	}
	log.Debug().Int64("taskNum", config.taskNum).Msgf("Configuration Version by Task: %s\n", currentVersion)

	manifestResponse, err := requestManifestInformation(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get manifest from bungie")
	}

	version := manifestResponse.Response.Version
	if version == currentVersion {
		log.Info().Msg("data is up to date")
		return
	}

	path := manifestResponse.Response.JsonWorldContentPaths.EN
	manifestURL := SetBaseUrl(&path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create request")
	}

	// Add headers that might be necessary for the request
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "oneTrick")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to download file:")
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		log.Fatal().Msgf("bad response status: %s (code: %d)", resp.Status, resp.StatusCode)
	}

	log.Info().
		Str("url", manifestURL).
		Msg("downloaded file from source")
	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		log.Fatal().Err(err).Msg("failed to decode manifest data")
	}

	log.Debug().Int64("index", config.taskNum).Msg("Decoded JSON successfully")

	err = performMigration(ctx, db, manifest, config.taskNum)
	if err != nil {
		log.Fatal().Int64("taskNum", config.taskNum).Err(err).Msg("failed to perform migration")
	}
	table, ok := GetConfigKeyByIndex(config.taskNum)
	if !ok {
		log.Fatal().Msg("unknown task index")
	}
	err = updateManifestVersion(ctx, db, table, currentVersion)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to update version")
	}
}

func requestManifestInformation(ctx context.Context) (*ManifestResponse, error) {
	// Create a request to the Bungie.net manifest endpoint
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "https://www.bungie.net/Platform/Destiny2/Manifest/", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("building request failed: %w", err)
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", "oneTrick")

	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot get manifest because of http failure: %w", err)
	}
	defer resp.Body.Close()

	// Check for success
	if resp.StatusCode != http.StatusOK {
		log.
			Error().
			Any("value", resp).
			Str("status", resp.Status).
			Int("statusCode", resp.StatusCode).
			Msg("issue with reaching destiny api")
		return nil, fmt.Errorf("failed to retrieve manifest")
	}

	var manifestResponse ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&manifestResponse); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	log.
		Info().
		Str("version", manifestResponse.Response.Version).
		Msg("Successfully downloaded manifest")

	return &manifestResponse, nil
}

// GetVersionByIndex returns the version string for the given collection index
// Returns the version string and a boolean indicating if the index was valid
func GetVersionByIndex(configuration Configuration, index int64) (string, bool) {
	switch index {
	case 0:
		return configuration.InventoryBucketVersion, true
	case 1:
		return configuration.ClassVersion, true
	case 2:
		return configuration.PlaceVersion, true
	case 3:
		return configuration.DamageVersion, true
	case 4:
		return configuration.ActivityModeVersion, true
	case 5:
		return configuration.ActivityVersion, true
	case 6:
		return configuration.ItemCategoryVersion, true
	case 7:
		return configuration.ItemDefinitionVersion, true
	case 8:
		return configuration.StatDefinitionVersion, true
	case 9:
		return configuration.RaceVersion, true
	case 10:
		return configuration.SandboxPerkVersion, true
	case 11:
		return configuration.RecordDefinitionVersion, true
	default:
		return "", false
	}
}

func migrateCollection(
	ctx context.Context,
	db *firestore.Client,
	collectionName string,
	itemsToMigrate interface{},
	getDocID func(item interface{}) string,
) error {
	loopStartTime := time.Now()

	// Use reflection to iterate over the items since they're of different types
	val := reflect.ValueOf(itemsToMigrate)

	if val.Kind() != reflect.Map && val.Kind() != reflect.Slice {
		return fmt.Errorf("itemsToMigrate must be a map or slice, got %s", val.Kind())
	}

	count := 0
	var keys []reflect.Value

	if val.Kind() == reflect.Map {
		keys = val.MapKeys()
	}

	// For maps, iterate over the keys
	// For slices, iterate over the indices
	for i := 0; i < val.Len(); i++ {
		var item interface{}

		if val.Kind() == reflect.Map {
			if i >= len(keys) {
				break
			}
			item = val.MapIndex(keys[i]).Interface()
		} else {
			item = val.Index(i).Interface()
		}

		// Get document ID for this item
		docID := getDocID(item)

		// Save to Firestore
		_, err := db.Collection(collectionName).Doc(docID).Set(ctx, item)
		if err != nil {
			log.Error().Str("docID", docID).Err(err).Msg("failed to save definition")
			return err
		}

		count++
	}

	log.Info().Str("collection", collectionName).Int("count", count).Dur("duration", time.Since(loopStartTime)).Msg("finished migrating collection")
	return nil
}

// to performMigration uses the generic migrateCollection function to handle various definition types
func performMigration(ctx context.Context, db *firestore.Client, manifest Manifest, index int64) error {
	switch index {
	case 0:
		return migrateCollection(
			ctx,
			db,
			string(InventoryBucketCollection),
			manifest.InventoryBucketDefinition,
			func(item interface{}) string {
				definition := item.(InventoryBucketDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 1:
		return migrateCollection(
			ctx,
			db,
			string(ClassCollection),
			manifest.ClassDefinition,
			func(item interface{}) string {
				definition := item.(ClassDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 2:
		return migrateCollection(
			ctx,
			db,
			string(PlaceCollection),
			manifest.PlaceDefinition,
			func(item interface{}) string {
				definition := item.(PlaceDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 3:
		return migrateCollection(
			ctx,
			db,
			string(DamageCollection),
			manifest.DamageTypeDefinition,
			func(item interface{}) string {
				definition := item.(DamageType)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 4:
		return migrateCollection(
			ctx,
			db,
			string(ActivityModeCollection),
			manifest.ActivityModeDefinition,
			func(item interface{}) string {
				definition := item.(ActivityModeDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)
	case 5:
		return migrateCollection(
			ctx,
			db,
			string(ActivityCollection),
			manifest.ActivityDefinition,
			func(item interface{}) string {
				definition := item.(ActivityDefinition)
				return strconv.FormatInt(int64(definition.Hash), 10)
			},
		)

	case 6:
		return migrateCollection(
			ctx,
			db,
			string(ItemCategoryCollection),
			manifest.ItemCategoryDefinition,
			func(item interface{}) string {
				definition := item.(ItemCategory)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 7:
		return migrateCollection(
			ctx,
			db,
			string(ItemDefinitionCollection),
			manifest.InventoryItemDefinition,
			func(item interface{}) string {
				definition := item.(ItemDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 8:
		return migrateCollection(
			ctx,
			db,
			string(StatDefinitionCollection),
			manifest.StatDefinition,
			func(item interface{}) string {
				definition := item.(StatDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 9:
		return migrateCollection(
			ctx,
			db,
			string(RaceCollection),
			manifest.RaceDefinition,
			func(item interface{}) string {
				definition := item.(RaceDefinition)
				return strconv.FormatInt(int64(definition.Hash), 10)
			},
		)

	case 10:
		return migrateCollection(
			ctx,
			db,
			string(SandboxPerkCollection),
			manifest.SandboxPerkDefinition,
			func(item interface{}) string {
				definition := item.(PerkDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)

	case 11:
		return migrateCollection(
			ctx,
			db,
			string(RecordDefinitionCollection),
			manifest.RecordDefinition,
			func(item interface{}) string {
				definition := item.(RecordDefinition)
				return strconv.FormatInt(int64(definition.Hash), 10)
			},
		)

	default:
		log.Info().Int64("index", index).Msg("Unsupported index")
		return nil
	}
}

// GetConfigKeyByIndex returns the configuration key name as a string for a given index
// Returns the key name and a boolean indicating if the index was valid
func GetConfigKeyByIndex(index int64) (string, bool) {
	switch index {
	case 0:
		return "InventoryBucketVersion", true
	case 1:
		return "ClassVersion", true
	case 2:
		return "PlaceVersion", true
	case 3:
		return "DamageVersion", true
	case 4:
		return "ActivityModeVersion", true
	case 5:
		return "ActivityVersion", true
	case 6:
		return "ItemCategoryVersion", true
	case 7:
		return "ItemDefinitionVersion", true
	case 8:
		return "StatDefinitionVersion", true
	case 9:
		return "RaceVersion", true
	case 10:
		return "SandboxPerkVersion", true
	case 11:
		return "RecordDefinitionVersion", true
	default:
		return "", false
	}
}
func updateManifestVersion(ctx context.Context, db *firestore.Client, table, version string) error {
	_, err := db.
		Collection(ConfigurationCollection).
		Doc(DestinyDocument).
		Set(
			ctx, map[string]interface{}{
				table: version,
			}, firestore.MergeAll,
		)
	if err != nil {
		log.Error().Err(err).Msg("failed to update config")
		return err
	}
	return nil
}
