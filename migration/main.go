package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	DryRun     bool
}

func SetBaseUrl(value *string) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%s%s", "https://www.bungie.net", *value)
}

func configFromEnv() (Config, error) {
	taskNum, err := stringToInt(os.Getenv("CLOUD_RUN_TASK_INDEX"))
	if err != nil {
		return Config{}, err
	}

	attemptNum := os.Getenv("CLOUD_RUN_TASK_ATTEMPT")
	dryRun, err := stringToInt(os.Getenv("DRY_RUN"))
	if err != nil {
		return Config{}, err
	}

	config := Config{
		taskNum:    taskNum,
		attemptNum: attemptNum,
		DryRun:     dryRun == 1,
	}

	fs := flag.NewFlagSet("migration", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		dryRunFlag  bool
		taskNumFlag int64
	)
	fs.BoolVar(&dryRunFlag, "dry-run", false, "Run without performing any writes")
	fs.Int64Var(&taskNumFlag, "task-index", -1, "Override CLOUD_RUN_TASK_INDEX (0-13)")

	_ = fs.Parse(os.Args[1:])

	if dryRunFlag {
		config.DryRun = true
	}
	if taskNumFlag >= 0 {
		config.taskNum = taskNumFlag
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
	setupLogging()
	if err := clog.Run(context.Background(), "migration", runMigration); err != nil {
		os.Exit(1)
	}
}

func runMigration(ctx context.Context) error {
	startTime := time.Now()
	var (
		itemsMigrated int
		upToDate      bool
		errorsTotal   int
	)
	clog.WithGroupFields(ctx, "request", clog.Fields{
		"taskIndex":   os.Getenv("CLOUD_RUN_TASK_INDEX"),
		"taskAttempt": os.Getenv("CLOUD_RUN_TASK_ATTEMPT"),
	})
	defer func() {
		duration := time.Since(startTime)
		clog.WithFields(ctx, clog.Fields{
			"durationMs":    duration.Milliseconds(),
			"itemsMigrated": itemsMigrated,
			"upToDate":      upToDate,
			"errorsTotal":   errorsTotal,
		})
	}()

	config, err := configFromEnv()
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to get config: %w", err)
	}
	clog.WithGroupFields(ctx, "request", clog.Fields{
		"taskIndex":   config.taskNum,
		"taskAttempt": config.attemptNum,
		"dryRun":      config.DryRun,
		"projectId":   projectID,
	})

	db, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}
	defer func(db *firestore.Client) {
		if err := db.Close(); err != nil {
			errorsTotal++
		}
	}(db)

	configStart := time.Now()
	snapshot, err := db.Collection(ConfigurationCollection).Doc(DestinyDocument).Get(ctx)
	if err != nil {
		errorsTotal++
		clog.WithGroupFields(ctx, "db.getConfiguration", clog.Fields{
			"collection": ConfigurationCollection,
			"docId":      DestinyDocument,
			"durationMs": time.Since(configStart).Milliseconds(),
		})
		return fmt.Errorf("failed to get db information: %w", err)
	}

	var data Configuration
	err = snapshot.DataTo(&data)
	clog.WithGroupFields(ctx, "db.getConfiguration", clog.Fields{
		"collection": ConfigurationCollection,
		"docId":      DestinyDocument,
		"durationMs": time.Since(configStart).Milliseconds(),
	})
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to read into configuration: %w", err)
	}

	currentVersion, ok := GetVersionByIndex(data, config.taskNum)
	if !ok {
		errorsTotal++
		return fmt.Errorf("unknown index for version: %d", config.taskNum)
	}
	table, ok := GetConfigKeyByIndex(config.taskNum)
	if !ok {
		errorsTotal++
		return fmt.Errorf("unknown task index: %d", config.taskNum)
	}
	clog.WithGroupFields(ctx, "migration", clog.Fields{
		"table":       table,
		"fromVersion": currentVersion,
	})

	manifestResponse, err := requestManifestInformation(ctx)
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to get manifest from Bungie: %w", err)
	}

	version := manifestResponse.Response.Version
	clog.WithGroupFields(ctx, "migration", clog.Fields{
		"toVersion": version,
		"upToDate":  version == currentVersion,
	})
	if version == currentVersion {
		upToDate = true
		return nil
	}

	path := manifestResponse.Response.JsonWorldContentPaths.EN
	manifestURL := SetBaseUrl(&path)

	manifest, err := downloadManifest(ctx, manifestURL)
	if err != nil {
		errorsTotal++
		return err
	}

	migrated, err := performMigration(ctx, db, *manifest, config.taskNum, config.DryRun)
	itemsMigrated = migrated
	if err != nil {
		errorsTotal++
		return fmt.Errorf("failed to perform migration: %w", err)
	}

	if !config.DryRun {
		err = updateManifestVersion(ctx, db, table, version)
		if err != nil {
			errorsTotal++
			return fmt.Errorf("failed to update version: %w", err)
		}
	}

	return nil
}

func requestManifestInformation(ctx context.Context) (*ManifestResponse, error) {
	apiStart := time.Now()
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
		clog.WithGroupFields(ctx, "bungieClient.getManifest", clog.Fields{
			"durationMs": time.Since(apiStart).Milliseconds(),
		})
		return nil, fmt.Errorf("cannot get manifest because of http failure: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		clog.WithGroupFields(ctx, "bungieClient.getManifest", clog.Fields{
			"statusCode": resp.StatusCode,
			"durationMs": time.Since(apiStart).Milliseconds(),
		})
		return nil, fmt.Errorf("failed to retrieve manifest: HTTP %s", resp.Status)
	}

	var manifestResponse ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&manifestResponse); err != nil {
		clog.WithGroupFields(ctx, "bungieClient.getManifest", clog.Fields{
			"statusCode": resp.StatusCode,
			"durationMs": time.Since(apiStart).Milliseconds(),
		})
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	clog.WithGroupFields(ctx, "bungieClient.getManifest", clog.Fields{
		"statusCode": resp.StatusCode,
		"version":    manifestResponse.Response.Version,
		"durationMs": time.Since(apiStart).Milliseconds(),
	})

	return &manifestResponse, nil
}

func downloadManifest(ctx context.Context, manifestURL string) (*Manifest, error) {
	downloadStart := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest download request: %w", err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "oneTrick")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		clog.WithGroupFields(ctx, "bungieClient.downloadManifest", clog.Fields{
			"url":        manifestURL,
			"durationMs": time.Since(downloadStart).Milliseconds(),
		})
		return nil, fmt.Errorf("failed to download manifest file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		clog.WithGroupFields(ctx, "bungieClient.downloadManifest", clog.Fields{
			"url":        manifestURL,
			"statusCode": resp.StatusCode,
			"durationMs": time.Since(downloadStart).Milliseconds(),
		})
		return nil, fmt.Errorf("bad manifest response status: %s (code: %d)", resp.Status, resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		clog.WithGroupFields(ctx, "bungieClient.downloadManifest", clog.Fields{
			"url":        manifestURL,
			"statusCode": resp.StatusCode,
			"durationMs": time.Since(downloadStart).Milliseconds(),
		})
		return nil, fmt.Errorf("failed to decode manifest data: %w", err)
	}

	clog.WithGroupFields(ctx, "bungieClient.downloadManifest", clog.Fields{
		"url":        manifestURL,
		"statusCode": resp.StatusCode,
		"durationMs": time.Since(downloadStart).Milliseconds(),
	})
	return &manifest, nil
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
	case 13:
		return configuration.EquipableItemSetVersion, true
	default:
		return "", false
	}
}

func migrateCollection(
	ctx context.Context,
	db *firestore.Client,
	collectionName string,
	itemsToMigrate any,
	dryRun bool,
	getDocID func(item any) string,
) (int, error) {
	loopStartTime := time.Now()

	// Use reflection to iterate over the items since they're of different types
	val := reflect.ValueOf(itemsToMigrate)

	if val.Kind() != reflect.Map && val.Kind() != reflect.Slice {
		return 0, fmt.Errorf("itemsToMigrate must be a map or slice, got %s", val.Kind())
	}

	items := make([]any, 0, val.Len())
	if val.Kind() == reflect.Map {
		for _, key := range val.MapKeys() {
			items = append(items, val.MapIndex(key).Interface())
		}
	} else {
		for i := 0; i < val.Len(); i++ {
			items = append(items, val.Index(i).Interface())
		}
	}

	count := 0
	err := clog.LoopRollupWithOptions(
		ctx,
		"migrationService.migrateItem",
		items,
		clog.LoopOptions{StopOnError: true},
		func(iterCtx context.Context, _ int, item any) error {
			docID := getDocID(item)
			clog.WithGroupFields(iterCtx, "definition", clog.Fields{
				"collection": collectionName,
				"docId":      docID,
			})

			if !dryRun {
				itemStart := time.Now()
				_, err := db.Collection(collectionName).Doc(docID).Set(iterCtx, item)
				clog.WithGroupFields(iterCtx, "db.setDefinition", clog.Fields{
					"collection": collectionName,
					"docId":      docID,
					"durationMs": time.Since(itemStart).Milliseconds(),
				})
				if err != nil {
					return fmt.Errorf("failed to save definition %s: %w", docID, err)
				}
			}

			count++
			return nil
		},
	)

	clog.WithGroupFields(ctx, "migrationService.migrateCollection", clog.Fields{
		"collection": collectionName,
		"totalItems": len(items),
		"migrated":   count,
		"dryRun":     dryRun,
		"durationMs": time.Since(loopStartTime).Milliseconds(),
	})
	if err != nil {
		return count, err
	}
	return count, nil
}

// performMigration uses the generic migrateCollection function to handle various definition types
func performMigration(ctx context.Context, db *firestore.Client, manifest Manifest, index int64, dryRun bool) (int, error) {
	switch index {
	case 0:
		return migrateCollection(
			ctx,
			db,
			string(InventoryBucketCollection),
			manifest.InventoryBucketDefinition,
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
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
			dryRun,
			func(item any) string {
				definition := item.(RecordDefinition)
				return strconv.FormatInt(int64(definition.Hash), 10)
			},
		)
	case 12:
		{
			maps, err := BuildCrucibleMaps(manifest.ActivityDefinition)
			if err != nil {
				return 0, err
			}
			return migrateCollection(
				ctx,
				db,
				string(CrucibleMapCollection),
				maps,
				dryRun,
				func(item any) string {
					definition := item.(CrucibleMap)
					return definition.Key
				},
			)

		}
	case 13:
		return migrateCollection(
			ctx,
			db,
			string(EquipableItemSetCollection),
			manifest.EquipableItemSetDefinition,
			dryRun,
			func(item any) string {
				definition := item.(EquipableItemSetDefinition)
				return strconv.FormatInt(definition.Hash, 10)
			},
		)
	default:
		clog.WithGroupField(ctx, "migrationService.performMigration", "warning", clog.Fields{
			"type":  "unsupported_index",
			"index": index,
		})
		return 0, nil
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
	case 13:
		return "equipableItemSetVersion", true
	default:
		return "", false
	}
}

func updateManifestVersion(ctx context.Context, db *firestore.Client, table, version string) error {
	start := time.Now()
	_, err := db.
		Collection(ConfigurationCollection).
		Doc(DestinyDocument).
		Set(
			ctx, map[string]any{
				table: version,
			}, firestore.MergeAll,
		)
	clog.WithGroupFields(ctx, "db.updateManifestVersion", clog.Fields{
		"collection": ConfigurationCollection,
		"docId":      DestinyDocument,
		"table":      table,
		"version":    version,
		"durationMs": time.Since(start).Milliseconds(),
	})
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}
	return nil
}
