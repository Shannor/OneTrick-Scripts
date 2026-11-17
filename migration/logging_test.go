package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	clog "github.com/Shannor/canonical-logger"
)

func TestRunEmitsCanonicalSuccessWithMigrationMetadata(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "migration", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "request", clog.Fields{
			"taskIndex":   int64(2),
			"taskAttempt": "1",
		})
		clog.WithGroupFields(ctx, "migration", clog.Fields{
			"table":       "activityDefinition",
			"fromVersion": "old",
			"toVersion":   "new",
		})
		clog.WithFields(ctx, clog.Fields{
			"itemsMigrated": 42,
			"upToDate":      false,
		})
		return nil
	})

	if err != nil {
		t.Fatalf("clog.Run() error = %v, want nil", err)
	}
	event := decodeEvent(t, &output)
	if event["message"] != "migration-success" {
		t.Errorf("event message = %v, want migration-success", event["message"])
	}
	if event["job"] != "migration" {
		t.Errorf("event job = %v, want migration", event["job"])
	}
	if event["severity"] != "INFO" {
		t.Errorf("event severity = %v, want INFO", event["severity"])
	}
	req, ok := event["request"].(map[string]any)
	if !ok || req["taskIndex"] != float64(2) || req["taskAttempt"] != "1" {
		t.Errorf("event request metadata = %v, want taskIndex 2 and taskAttempt 1", event["request"])
	}
	migration, ok := event["migration"].(map[string]any)
	if !ok || migration["table"] != "activityDefinition" || migration["fromVersion"] != "old" || migration["toVersion"] != "new" {
		t.Errorf("event migration metadata = %v, want table and version fields", event["migration"])
	}
	if event["itemsMigrated"] != float64(42) || event["upToDate"] != false {
		t.Errorf("event root counters = %v, want itemsMigrated 42 and upToDate false", event)
	}
}

func TestRunEmitsCanonicalFailureAndReturnsError(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "migration", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "request", clog.Fields{"taskIndex": int64(3)})
		return errors.New("test failure")
	})

	if err == nil {
		t.Fatalf("clog.Run() error = nil, want error")
	}
	event := decodeEvent(t, &output)
	if event["message"] != "migration-failure" {
		t.Errorf("event message = %v, want migration-failure", event["message"])
	}
	if event["severity"] != "ERROR" {
		t.Errorf("event severity = %v, want ERROR", event["severity"])
	}
	if _, ok := event["error"].(map[string]any); !ok {
		t.Errorf("event error group = %v, want an object", event["error"])
	}
}

func TestMigrationConfigurationFailureIsCanonical(t *testing.T) {
	t.Setenv("CLOUD_RUN_TASK_INDEX", "invalid")
	t.Setenv("CLOUD_RUN_TASK_ATTEMPT", "4")
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "migration", runMigration)

	if err == nil {
		t.Fatalf("clog.Run() error = nil, want configuration error")
	}
	event := decodeEvent(t, &output)
	if event["message"] != "migration-failure" || event["severity"] != "ERROR" {
		t.Errorf("configuration failure event = %v, want migration-failure with ERROR severity", event)
	}
	req, ok := event["request"].(map[string]any)
	if !ok || req["taskIndex"] != "invalid" || req["taskAttempt"] != "4" {
		t.Errorf("event request metadata = %v, want raw task index and attempt", event["request"])
	}
	if event["errorsTotal"] != float64(1) {
		t.Errorf("errorsTotal = %v, want 1", event["errorsTotal"])
	}
}

func TestLoopRollupWithStopOnErrorRetainsFailedSnapshot(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	items := []string{"doc-1", "doc-2", "doc-3"}
	err := clog.Run(ctx, "migration", func(ctx context.Context) error {
		return clog.LoopRollupWithOptions(
			ctx,
			"migrationService.migrateItem",
			items,
			clog.LoopOptions{StopOnError: true},
			func(iterCtx context.Context, _ int, docID string) error {
				clog.WithGroupFields(iterCtx, "definition", clog.Fields{
					"collection": "d2ActivityDefinitions",
					"docId":      docID,
				})
				if docID == "doc-2" {
					return errors.New("firestore write failed")
				}
				return nil
			},
		)
	})

	if err == nil {
		t.Fatalf("clog.Run() error = nil, want error")
	}
	event := decodeEvent(t, &output)
	migrationSvc, ok := event["migrationService"].(map[string]any)
	if !ok {
		t.Fatalf("migrationService group = %v, want object", event["migrationService"])
	}
	rollup, ok := migrationSvc["migrateItem"].(map[string]any)
	if !ok {
		t.Fatalf("migrationService.migrateItem = %v, want object", migrationSvc["migrateItem"])
	}
	if rollup["total"] != float64(2) || rollup["succeeded"] != float64(1) || rollup["failed"] != float64(1) {
		t.Errorf("rollup counts = %v, want total=2, succeeded=1, failed=1", rollup)
	}
	failures, ok := rollup["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("rollup failures = %v, want 1 failed snapshot", rollup["failures"])
	}
	failedSnap, ok := failures[0].(map[string]any)
	if !ok {
		t.Fatalf("failed snapshot = %v, want object", failures[0])
	}
	defGroup, ok := failedSnap["definition"].(map[string]any)
	if !ok || defGroup["docId"] != "doc-2" {
		t.Errorf("failed snapshot definition = %v, want docId doc-2", failedSnap["definition"])
	}
}

func TestGCPHandlerEmitsCanonicalSeverityAndMessage(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(newGCPHandlerWithWriter(&output, slog.LevelDebug))
	logger.Warn("migration starting", "table", "activityDefinition")
	event := decodeEvent(t, &output)

	if event["message"] != "migration starting" || event["severity"] != "WARNING" {
		t.Errorf("GCP log schema = %v, want message and WARNING severity", event)
	}
	if event["table"] != "activityDefinition" {
		t.Errorf("GCP log fields = %v, want table metadata", event)
	}
}

func TestSetupLoggingSelectsExpectedHandlerAndPreservesCanonicalEvents(t *testing.T) {
	tests := []struct {
		name        string
		cloudRun    bool
		cloudRunJob string
		failure     bool
		wantPretty  bool
	}{
		{name: "local success", wantPretty: true},
		{name: "local failure", failure: true, wantPretty: true},
		{name: "Cloud Run success", cloudRun: true, cloudRunJob: "migration"},
		{name: "Cloud Run failure", cloudRun: true, cloudRunJob: "migration", failure: true},
		{name: "Cloud Run with empty job value", cloudRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runErr error
			output := captureConfiguredLogs(t, tt.cloudRun, tt.cloudRunJob, func() {
				ctx := clog.NewContextWithLogger(context.Background(), slog.Default())
				runErr = clog.Run(ctx, "migration", func(ctx context.Context) error {
					clog.WithGroupFields(ctx, "request", clog.Fields{"taskIndex": int64(7)})
					if tt.failure {
						return errors.New("test failure")
					}
					return nil
				})
			})

			wantMessage, wantSeverity := "migration-success", "INFO"
			if tt.failure {
				wantMessage, wantSeverity = "migration-failure", "ERROR"
			}
			if (runErr != nil) != tt.failure {
				t.Errorf("clog.Run() error = %v, want failure=%t", runErr, tt.failure)
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(output), &event); err != nil {
				t.Fatalf("failed to decode canonical event %q: %v", output, err)
			}
			if event["message"] != wantMessage || event["severity"] != wantSeverity || event["job"] != "migration" {
				t.Errorf("canonical event = %v, want message %q, severity %q, and job migration", event, wantMessage, wantSeverity)
			}
			req, ok := event["request"].(map[string]any)
			if !ok || req["taskIndex"] != float64(7) {
				t.Errorf("event request metadata = %v, want taskIndex 7", event["request"])
			}
			if tt.failure {
				if _, ok := event["error"].(map[string]any); !ok {
					t.Errorf("failure event error = %v, want an object", event["error"])
				}
			}
			lineCount := strings.Count(strings.TrimSpace(output), "\n") + 1
			if tt.wantPretty && lineCount < 2 {
				t.Errorf("local log output is not pretty-printed: %q", output)
			}
			if !tt.wantPretty && lineCount != 1 {
				t.Errorf("Cloud Run log output should be compact JSON, got %d lines", lineCount)
			}
		})
	}
}

func captureConfiguredLogs(t *testing.T, cloudRun bool, cloudRunJob string, execute func()) string {
	t.Helper()
	originalStdout := os.Stdout
	originalLogger := slog.Default()
	originalCloudRunJob, hadCloudRunJob := os.LookupEnv("CLOUD_RUN_JOB")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create log pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
		_ = reader.Close()
		os.Stdout = originalStdout
		slog.SetDefault(originalLogger)
		if hadCloudRunJob {
			_ = os.Setenv("CLOUD_RUN_JOB", originalCloudRunJob)
		} else {
			_ = os.Unsetenv("CLOUD_RUN_JOB")
		}
	})

	if cloudRun {
		if err := os.Setenv("CLOUD_RUN_JOB", cloudRunJob); err != nil {
			t.Fatalf("failed to set CLOUD_RUN_JOB: %v", err)
		}
	} else if err := os.Unsetenv("CLOUD_RUN_JOB"); err != nil {
		t.Fatalf("failed to unset CLOUD_RUN_JOB: %v", err)
	}
	os.Stdout = writer
	setupLogging()
	execute()
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close log pipe: %v", err)
	}
	os.Stdout = originalStdout
	slog.SetDefault(originalLogger)

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read captured logs: %v", err)
	}
	return string(output)
}

func testLoggingContext(output *bytes.Buffer) context.Context {
	logger := slog.New(newGCPHandlerWithWriter(output, slog.LevelDebug))
	return clog.NewContextWithLogger(context.Background(), logger)
}

func decodeEvent(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("failed to decode log event %q: %v", output.String(), err)
	}
	return event
}

func TestEquipableItemSetMigrationAndManifestDecoding(t *testing.T) {
	cfg := Configuration{
		EquipableItemSetVersion: "v1.2.3",
	}
	gotVersion, ok := GetVersionByIndex(cfg, 13)
	if !ok || gotVersion != "v1.2.3" {
		t.Fatalf("GetVersionByIndex(cfg, 13) = (%q, %v), want (\"v1.2.3\", true)", gotVersion, ok)
	}
	gotKey, ok := GetConfigKeyByIndex(13)
	if !ok || gotKey != "equipableItemSetVersion" {
		t.Fatalf("GetConfigKeyByIndex(13) = (%q, %v), want (\"equipableItemSetVersion\", true)", gotKey, ok)
	}

	rawManifest := `{
		"DestinyInventoryItemDefinition": {
			"479136420": {
				"hash": 479136420,
				"displayProperties": {"name": "Per Audacia Casque"},
				"equippingBlock": {
					"uniqueLabelHash": 0,
					"equipmentSlotTypeHash": 3448274439,
					"equipableItemSetHash": 2618364783
				}
			}
		},
		"DestinyEquipableItemSetDefinition": {
			"2618364783": {
				"hash": 2618364783,
				"displayProperties": {"name": "Per Audacia"},
				"setItems": [479136420, 43450237],
				"setPerks": [
					{"requiredSetCount": 2, "sandboxPerkHash": 3091068916},
					{"requiredSetCount": 4, "sandboxPerkHash": 3091068917}
				]
			}
		}
	}`
	var manifest Manifest
	if err := json.Unmarshal([]byte(rawManifest), &manifest); err != nil {
		t.Fatalf("failed to unmarshal manifest JSON: %v", err)
	}

	itemDef, ok := manifest.InventoryItemDefinition["479136420"]
	if !ok || itemDef.EquippingBlock.EquipableItemSetHash == nil || *itemDef.EquippingBlock.EquipableItemSetHash != 2618364783 {
		t.Fatalf("itemDef.EquippingBlock.EquipableItemSetHash = %v, want 2618364783", itemDef.EquippingBlock.EquipableItemSetHash)
	}

	setDef, ok := manifest.EquipableItemSetDefinition["2618364783"]
	if !ok || setDef.DisplayProperties.Name != "Per Audacia" || len(setDef.SetPerks) != 2 {
		t.Fatalf("setDef = %+v, want Per Audacia with 2 setPerks", setDef)
	}

	var output bytes.Buffer
	ctx := testLoggingContext(&output)
	migrated, err := performMigration(ctx, nil, manifest, 13, true)
	if err != nil {
		t.Fatalf("performMigration(index=13, dryRun=true) error = %v", err)
	}
	if migrated != 1 {
		t.Errorf("performMigration(index=13, dryRun=true) migrated = %d, want 1", migrated)
	}
}

func TestProgressionDefinitionMigrationAndManifestDecoding(t *testing.T) {
	cfg := Configuration{
		ProgressionDefinitionVersion: "v2.0.0",
	}
	gotVersion, ok := GetVersionByIndex(cfg, 14)
	if !ok || gotVersion != "v2.0.0" {
		t.Fatalf("GetVersionByIndex(cfg, 14) = (%q, %v), want (\"v2.0.0\", true)", gotVersion, ok)
	}
	gotKey, ok := GetConfigKeyByIndex(14)
	if !ok || gotKey != "progressionDefinitionVersion" {
		t.Fatalf("GetConfigKeyByIndex(14) = (%q, %v), want (\"progressionDefinitionVersion\", true)", gotKey, ok)
	}

	rawManifest := `{
		"DestinyProgressionDefinition": {
			"3696598664": {
				"hash": 3696598664,
				"index": 42,
				"redacted": false,
				"blacklisted": false,
				"displayProperties": {
					"name": "Competitive Division",
					"description": "Your current Competitive Crucible division.",
					"icon": "/common/destiny2_content/icons/comp.png",
					"hasIcon": true
				},
				"steps": [
					{"stepName": "Unranked", "progressTotal": 5, "icon": "/common/destiny2_content/icons/unranked.png"},
					{"stepName": "Copper III", "progressTotal": 300, "icon": "/common/destiny2_content/icons/copper3.png"}
				]
			}
		}
	}`
	var manifest Manifest
	if err := json.Unmarshal([]byte(rawManifest), &manifest); err != nil {
		t.Fatalf("failed to unmarshal manifest JSON: %v", err)
	}

	progDef, ok := manifest.ProgressionDefinition["3696598664"]
	if !ok {
		t.Fatalf("expected progression 3696598664 in manifest")
	}
	if progDef.Hash != 3696598664 || progDef.Index != 42 || progDef.Redacted || progDef.Blacklisted {
		t.Fatalf("progDef metadata = %+v, want hash=3696598664, index=42, redacted=false, blacklisted=false", progDef)
	}
	if progDef.DisplayProperties.Name != "Competitive Division" || !progDef.DisplayProperties.HasIcon {
		t.Fatalf("progDef.DisplayProperties = %+v, want Competitive Division with icon", progDef.DisplayProperties)
	}
	if len(progDef.Steps) != 2 || progDef.Steps[1].StepName != "Copper III" || progDef.Steps[1].ProgressTotal != 300 {
		t.Fatalf("progDef.Steps = %+v, want 2 steps with Copper III (300)", progDef.Steps)
	}

	var output bytes.Buffer
	ctx := testLoggingContext(&output)
	migrated, err := performMigration(ctx, nil, manifest, 14, true)
	if err != nil {
		t.Fatalf("performMigration(index=14, dryRun=true) error = %v", err)
	}
	if migrated != 1 {
		t.Errorf("performMigration(index=14, dryRun=true) migrated = %d, want 1", migrated)
	}
}


