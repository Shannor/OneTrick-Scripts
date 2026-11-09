package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	clog "github.com/Shannor/canonical-logger"
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
	setupLogging()
	os.Exit(runJob(context.Background(), "migration", runMigration))
}

func runJob(ctx context.Context, event string, execute func(context.Context) error) int {
	if err := clog.Run(ctx, event, execute); err != nil {
		return 1
	}
	return 0
}

func runMigration(ctx context.Context) error {
	startTime := time.Now()
	clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
		"taskIndex":   os.Getenv("CLOUD_RUN_TASK_INDEX"),
		"taskAttempt": os.Getenv("CLOUD_RUN_TASK_ATTEMPT"),
	})
	defer func() {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
			"durationMs": time.Since(startTime).Milliseconds(),
		})
	}()

	config, err := configFromEnv()
	if err != nil {
		slog.Error("failed to get config", "error", err)
		return fmt.Errorf("failed to get config: %w", err)
	}
	clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
		"taskIndex":   config.taskNum,
		"taskAttempt": config.attemptNum,
	})
	l := slog.With("taskNum", config.taskNum)

	db, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		l.Error("Failed to create client", "error", err)
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			l.Error("failed to close database client", "error", err)
		}
	}()

	snapshot, err := db.Collection(ConfigurationCollection).Doc(DestinyDocument).Get(ctx)
	if err != nil {
		l.Error("failed to get db information", "error", err)
		return fmt.Errorf("failed to get db information: %w", err)
	}

	var data Configuration
	err = snapshot.DataTo(&data)
	if err != nil {
		l.Error("failed to read into configuration", "error", err)
		return fmt.Errorf("failed to read into configuration: %w", err)
	}
	currentVersion, ok := GetVersionByIndex(data, config.taskNum)
	if !ok {
		return fmt.Errorf("unknown index for version: %d", config.taskNum)
	}
	table, ok := GetConfigKeyByIndex(config.taskNum)
	if !ok {
		return fmt.Errorf("unknown task index: %d", config.taskNum)
	}
	clog.WithGroupFields(ctx, "migration", clog.Fields{
		"table":       table,
		"fromVersion": currentVersion,
	})
	l = l.With("table", table, "lastVersion", currentVersion)

	manifestResponse, err := requestManifestInformation(ctx)
	if err != nil {
		l.Error("failed to get manifest from bungie", "error", err)
		return fmt.Errorf("failed to get manifest from Bungie: %w", err)
	}

	version := manifestResponse.Response.Version
	clog.WithGroupFields(ctx, "migration", clog.Fields{"toVersion": version})
	if version == currentVersion {
		l.Info("data is up to date")
		return nil
	}

	path := manifestResponse.Response.JsonWorldContentPaths.EN
	manifestURL := SetBaseUrl(&path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create manifest download request: %w", err)
	}

	// Add headers that might be necessary for the request
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "oneTrick")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download manifest file: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad manifest response status: %s (code: %d)", resp.Status, resp.StatusCode)
	}

	l.Info("downloaded file from source", "url", manifestURL)
	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return fmt.Errorf("failed to decode manifest data: %w", err)
	}

	l.Debug("Decoded JSON successfully")

	err = performMigration(ctx, db, manifest, config.taskNum)
	if err != nil {
		l.Error("failed to perform migration", "error", err)
		return fmt.Errorf("failed to perform migration: %w", err)
	}
	l.Info("migration completed")
	err = updateManifestVersion(ctx, db, table, version)
	if err != nil {
		l.Error("failed to update version", "error", err)
		return fmt.Errorf("failed to update version: %w", err)
	}
	l.Info("updated version")
	l.Info("work done")
	return nil
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
		slog.Error("issue with reaching destiny api", "status", resp.Status, "statusCode", resp.StatusCode)
		return nil, fmt.Errorf("failed to retrieve manifest: HTTP %s", resp.Status)
	}

	var manifestResponse ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&manifestResponse); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	slog.Info("Successfully downloaded manifest", "version", manifestResponse.Response.Version)

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
	case 12:
		return configuration.CrucibleMapVersion, true
	default:
		return "", false
	}
}

func migrateCollection(
	ctx context.Context,
	db *firestore.Client,
	collectionName string,
	itemsToMigrate any,
	getDocID func(item any) string,
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
		var item any

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
			slog.Error("failed to save definition", "docId", docID, "error", err)
			return fmt.Errorf("failed to save definition %s: %w", docID, err)
		}

		count++
	}

	slog.Info("finished migrating collection",
		"collection", collectionName,
		"count", count,
		"durationMs", time.Since(loopStartTime).Milliseconds(),
	)
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
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
			func(item any) string {
				definition := item.(RecordDefinition)
				return strconv.FormatInt(int64(definition.Hash), 10)
			},
		)
	case 12:
		{
			maps, err := BuildCrucibleMaps(manifest.ActivityDefinition)
			if err != nil {
				return err
			}
			return migrateCollection(
				ctx,
				db,
				string(CrucibleMapCollection),
				maps,
				func(item any) string {
					definition := item.(CrucibleMap)
					return definition.Key
				},
			)

		}
	default:
		slog.Info("Unsupported index", "index", index)
		return nil
	}
}

// GetConfigKeyByIndex returns the configuration key name as a string for a given index
// Returns the key name and a boolean indicating if the index was valid
func GetConfigKeyByIndex(index int64) (string, bool) {
	switch index {
	case 0:
		return "inventoryBucketVersion", true
	case 1:
		return "classVersion", true
	case 2:
		return "placeVersion", true
	case 3:
		return "damageVersion", true
	case 4:
		return "activityModeVersion", true
	case 5:
		return "activityVersion", true
	case 6:
		return "itemCategoryVersion", true
	case 7:
		return "itemDefinitionVersion", true
	case 8:
		return "statDefinitionVersion", true
	case 9:
		return "raceVersion", true
	case 10:
		return "sandboxPerkVersion", true
	case 11:
		return "recordDefinitionVersion", true
	case 12:
		return "crucibleMapVersion", true
	default:
		return "", false
	}
}
func updateManifestVersion(ctx context.Context, db *firestore.Client, table, version string) error {
	_, err := db.
		Collection(ConfigurationCollection).
		Doc(DestinyDocument).
		Set(
			ctx, map[string]any{
				table: version,
			}, firestore.MergeAll,
		)
	if err != nil {
		slog.Error("failed to update config", "error", err)
		return fmt.Errorf("failed to update config: %w", err)
	}
	return nil
}
