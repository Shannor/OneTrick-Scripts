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

func TestRunJobEmitsCanonicalSuccessWithMigrationMetadata(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "migration", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
			"taskIndex":   int64(2),
			"taskAttempt": "1",
		})
		clog.WithGroupFields(ctx, "migration", clog.Fields{
			"table":       "activityDefinition",
			"fromVersion": "old",
			"toVersion":   "new",
		})
		return nil
	})

	if code != 0 {
		t.Fatalf("runJob() exit code = %d, want 0", code)
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
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["taskIndex"] != float64(2) || job["taskAttempt"] != "1" {
		t.Errorf("event job metadata = %v, want taskIndex 2 and taskAttempt 1", event["jobMetadata"])
	}
	migration, ok := event["migration"].(map[string]any)
	if !ok || migration["table"] != "activityDefinition" || migration["fromVersion"] != "old" || migration["toVersion"] != "new" {
		t.Errorf("event migration metadata = %v, want table and version fields", event["migration"])
	}
}

func TestRunJobEmitsCanonicalFailureAndReturnsNonzero(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "migration", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{"taskIndex": int64(3)})
		return errors.New("test failure")
	})

	if code != 1 {
		t.Fatalf("runJob() exit code = %d, want 1", code)
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

func TestMigrationConfigurationFailureIsCanonicalAndNonzero(t *testing.T) {
	t.Setenv("CLOUD_RUN_TASK_INDEX", "invalid")
	t.Setenv("CLOUD_RUN_TASK_ATTEMPT", "4")
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "migration", runMigration)

	if code != 1 {
		t.Fatalf("runJob() exit code = %d, want 1", code)
	}
	event := decodeEvent(t, &output)
	if event["message"] != "migration-failure" || event["severity"] != "ERROR" {
		t.Errorf("configuration failure event = %v, want migration-failure with ERROR severity", event)
	}
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["taskIndex"] != "invalid" || job["taskAttempt"] != "4" {
		t.Errorf("event job metadata = %v, want raw task index and attempt", event["jobMetadata"])
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
			var code int
			output := captureConfiguredLogs(t, tt.cloudRun, tt.cloudRunJob, func() {
				ctx := clog.NewContextWithLogger(context.Background(), slog.Default())
				code = runJob(ctx, "migration", func(ctx context.Context) error {
					clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{"taskIndex": int64(7)})
					if tt.failure {
						return errors.New("test failure")
					}
					return nil
				})
			})

			wantCode, wantMessage, wantSeverity := 0, "migration-success", "INFO"
			if tt.failure {
				wantCode, wantMessage, wantSeverity = 1, "migration-failure", "ERROR"
			}
			if code != wantCode {
				t.Errorf("runJob() exit code = %d, want %d", code, wantCode)
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(output), &event); err != nil {
				t.Fatalf("failed to decode canonical event %q: %v", output, err)
			}
			if event["message"] != wantMessage || event["severity"] != wantSeverity || event["job"] != "migration" {
				t.Errorf("canonical event = %v, want message %q, severity %q, and job migration", event, wantMessage, wantSeverity)
			}
			labels, ok := event["logging.googleapis.com/labels"].(map[string]any)
			if !ok || labels["logger"] != "onetrick-migration" {
				t.Errorf("event labels = %v, want logger onetrick-migration", event["logging.googleapis.com/labels"])
			}
			job, ok := event["jobMetadata"].(map[string]any)
			if !ok || job["taskIndex"] != float64(7) {
				t.Errorf("event job metadata = %v, want taskIndex 7", event["jobMetadata"])
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
