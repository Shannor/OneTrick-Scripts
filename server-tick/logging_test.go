package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clog "github.com/Shannor/canonical-logger"
)

func TestRunJobEmitsCanonicalSuccessEvent(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{
			"taskIndex":         int64(2),
			"sessionsProcessed": 3,
		})
		return nil
	})

	if code != 0 {
		t.Fatalf("runJob() exit code = %d, want 0", code)
	}
	event := decodeEvent(t, &output)
	if event["message"] != "server-tick-success" {
		t.Errorf("event message = %v, want server-tick-success", event["message"])
	}
	if event["job"] != "server-tick" {
		t.Errorf("event job = %v, want server-tick", event["job"])
	}
	if event["severity"] != "INFO" {
		t.Errorf("event severity = %v, want INFO", event["severity"])
	}
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["taskIndex"] != float64(2) || job["sessionsProcessed"] != float64(3) {
		t.Errorf("event job metadata = %v, want taskIndex 2 and sessionsProcessed 3", event["jobMetadata"])
	}
}

func TestRunJobEmitsCanonicalFailureAndReturnsNonzero(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{"taskIndex": int64(4)})
		return errors.New("test failure")
	})

	if code != 1 {
		t.Fatalf("runJob() exit code = %d, want 1", code)
	}
	event := decodeEvent(t, &output)
	if event["message"] != "server-tick-failure" {
		t.Errorf("event message = %v, want server-tick-failure", event["message"])
	}
	if event["severity"] != "ERROR" {
		t.Errorf("event severity = %v, want ERROR", event["severity"])
	}
	if _, ok := event["error"].(map[string]any); !ok {
		t.Errorf("event error group = %v, want an object", event["error"])
	}
}

func TestServerTickConfigurationFailureIsCanonicalAndNonzero(t *testing.T) {
	t.Setenv("CLOUD_RUN_TASK_INDEX", "17")
	t.Setenv("SKIP_SAVE", "invalid")
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "server-tick", runServerTick)

	if code != 1 {
		t.Fatalf("runJob() exit code = %d, want 1", code)
	}
	events := decodeEvents(t, &output)
	if len(events) != 1 {
		t.Fatalf("configuration failure logs = %v, want one canonical record", events)
	}
	event := events[0]
	if event["message"] != "server-tick-failure" || event["severity"] != "ERROR" {
		t.Errorf("configuration failure event = %v, want server-tick-failure with ERROR severity", event)
	}
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["taskIndex"] != "17" {
		t.Errorf("event job metadata = %v, want taskIndex 17", event["jobMetadata"])
	}
	if job["issueCount"] != float64(1) {
		t.Errorf("issue count = %v, want 1", job["issueCount"])
	}
	issue, ok := job["lastIssue"].(map[string]any)
	if !ok || issue["type"] != "configuration" || issue["message"] != "failed to get config" {
		t.Errorf("last issue = %v, want failed configuration details", job["lastIssue"])
	}
}

func TestServerTickEmulatorQueryTimeoutIsCanonicalAndNonzero(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start unresponsive local endpoint: %v", err)
	}
	defer listener.Close()
	endpoint := listener.Addr().String()
	stopServer := make(chan struct{})
	defer close(stopServer)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				<-stopServer
			}()
		}
	}()

	t.Setenv("FIRESTORE_EMULATOR_HOST", endpoint)
	t.Setenv("FIRESTORE_PROJECT_ID", "server-tick-test")
	for _, tt := range []struct {
		name                string
		callerTimeout       time.Duration
		wantEmulatorTimeout bool
	}{
		{name: "emulator deadline", callerTimeout: 5 * time.Second, wantEmulatorTimeout: true},
		{name: "earlier caller deadline", callerTimeout: 500 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx, cancel := context.WithTimeout(testLoggingContext(&output), tt.callerTimeout)
			defer cancel()

			code := runJob(ctx, "server-tick", runServerTick)

			if code != 1 {
				t.Fatalf("runJob() exit code = %d, want 1", code)
			}
			events := decodeEvents(t, &output)
			if len(events) != 1 {
				t.Fatalf("emulator query failure logs = %v, want one canonical event", events)
			}
			event := events[0]
			if event["message"] != "server-tick-failure" || event["severity"] != "ERROR" {
				t.Errorf("emulator query failure event = %v, want server-tick-failure with ERROR severity", event)
			}
			job, ok := event["jobMetadata"].(map[string]any)
			if !ok || job["firestoreEmulatorHost"] != endpoint || job["errorsTotal"] != float64(1) {
				t.Errorf("emulator failure metadata = %v, want endpoint %q and errorsTotal 1", event["jobMetadata"], endpoint)
			}
			errorText := fmt.Sprint(event["error"])
			if !strings.Contains(errorText, endpoint) {
				t.Errorf("canonical error = %v, want endpoint %q", event["error"], endpoint)
			}
			if got := strings.Contains(errorText, "timed out after"); got != tt.wantEmulatorTimeout {
				t.Errorf("canonical error = %v, emulator timeout attribution = %t, want %t", event["error"], got, tt.wantEmulatorTimeout)
			}
			issue, ok := job["lastIssue"].(map[string]any)
			if !ok || issue["type"] != "session_query" || !strings.Contains(fmt.Sprint(issue["error"]), endpoint) {
				t.Errorf("emulator issue metadata = %v, want session query failure for %q", job["lastIssue"], endpoint)
			}
			if tt.wantEmulatorTimeout && ctx.Err() != nil {
				t.Errorf("caller context error = %v, want nil when emulator deadline expires first", ctx.Err())
			} else if !tt.wantEmulatorTimeout && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("caller context error = %v, want deadline exceeded", ctx.Err())
			}
		})
	}
}

func TestRunJobEmitsOneCanonicalEventWithDirectClogMetadata(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{"sessionsProcessed": 5})
		return nil
	})

	if code != 0 {
		t.Fatalf("runJob() exit code = %d, want 0", code)
	}
	events := decodeEvents(t, &output)
	if len(events) != 1 {
		t.Fatalf("job emitted %d records, want one: %v", len(events), events)
	}
	event := events[0]
	if event["message"] != "server-tick-success" {
		t.Errorf("event message = %v, want server-tick-success", event["message"])
	}
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["sessionsProcessed"] != float64(5) {
		t.Errorf("event job metadata = %v, want sessionsProcessed 5", event["jobMetadata"])
	}
	if _, ok := event["diagnosticLogs"]; ok {
		t.Errorf("canonical event contains wrapper diagnostic logs: %v", event["diagnosticLogs"])
	}
}

func TestJobMetadataRetainsOnlyLatestIssue(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "server-tick", func(ctx context.Context) error {
		ctx, state := withJobMetadataState(ctx)
		for i := range 250 {
			recordJobIssue(ctx, "test", "issue", clog.Fields{"index": i})
		}
		state.flush(ctx)
		return nil
	})

	if code != 0 {
		t.Fatalf("runJob() exit code = %d, want 0", code)
	}
	event := decodeEvent(t, &output)
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["issueCount"] != float64(250) {
		t.Fatalf("job metadata = %v, want all issue occurrences counted", event["jobMetadata"])
	}
	issue, ok := job["lastIssue"].(map[string]any)
	if !ok || issue["index"] != float64(249) {
		t.Errorf("last issue = %v, want only the latest issue details", job["lastIssue"])
	}
}

func TestConverterManifestWarningsAreAggregatedInCanonicalMetadata(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	code := runJob(ctx, "server-tick", func(ctx context.Context) error {
		ctx, state := withJobMetadataState(ctx)
		generateClassStats(ctx, map[string]StatDefinition{}, map[string]int32{"missing": 10})
		generateClassStats(ctx, map[string]StatDefinition{}, map[string]int32{"also_missing": 20})
		state.flush(ctx)
		return nil
	})

	if code != 0 {
		t.Fatalf("runJob() exit code = %d, want 0", code)
	}
	event := decodeEvent(t, &output)
	job, ok := event["jobMetadata"].(map[string]any)
	if !ok || job["manifestWarningCount"] != float64(2) {
		t.Fatalf("manifest warning metadata = %v, want count 2", event["jobMetadata"])
	}
	warning, ok := job["lastManifestWarning"].(map[string]any)
	if !ok || warning["type"] != "missing_stat_definition" || warning["statKey"] != "also_missing" {
		t.Errorf("last manifest warning = %v, want latest warning only", job["lastManifestWarning"])
	}
}

func TestRunJobEmitsCanonicalFailureBeforeRepanicking(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		runJob(ctx, "server-tick", func(context.Context) error {
			panic("test panic")
		})
	}()

	if recovered != "test panic" {
		t.Fatalf("recovered panic = %v, want test panic", recovered)
	}
	events := decodeEvents(t, &output)
	if len(events) != 1 || events[0]["message"] != "server-tick-failure" || events[0]["severity"] != "ERROR" {
		t.Errorf("panic output = %v, want one canonical failure event", events)
	}
}

func TestMetricDefinitionsUseCanonicalEventMetadata(t *testing.T) {
	definitions := map[string]string{
		"server_tick_activities_found.yaml":   "activitiesFoundTotal",
		"server_tick_errors_total.yaml":       "errorsTotal",
		"server_tick_execution_duration.yaml": "tickDurationMs",
		"server_tick_loadouts_failed.yaml":    "loadoutsFailed",
		"server_tick_loadouts_saved.yaml":     "loadoutsSaved",
		"server_tick_sessions_processed.yaml": "sessionsProcessed",
	}
	for filename, field := range definitions {
		t.Run(filename, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join("metrics", filename))
			if err != nil {
				t.Fatalf("failed to read metric definition: %v", err)
			}
			text := string(contents)
			if !strings.Contains(text, `jsonPayload.job="server-tick"`) || strings.Contains(text, `jsonPayload.message="server_tick_completed"`) {
				t.Errorf("metric filter does not select canonical job events: %s", text)
			}
			if !strings.Contains(text, "EXTRACT(jsonPayload.jobMetadata."+field+")") {
				t.Errorf("metric extractor does not read jobMetadata.%s: %s", field, text)
			}
		})
	}
	runCount, err := os.ReadFile(filepath.Join("metrics", "server_tick_run_count.yaml"))
	if err != nil {
		t.Fatalf("failed to read run-count metric definition: %v", err)
	}
	if !strings.Contains(string(runCount), `jsonPayload.message="server-tick-success"`) {
		t.Errorf("run-count metric should include successful canonical events: %s", runCount)
	}
	manifestWarnings, err := os.ReadFile(filepath.Join("metrics", "server_tick_manifest_warnings.yaml"))
	if err != nil {
		t.Fatalf("failed to read manifest warning metric definition: %v", err)
	}
	if !strings.Contains(string(manifestWarnings), `jsonPayload.job="server-tick"`) || !strings.Contains(string(manifestWarnings), "EXTRACT(jsonPayload.jobMetadata.manifestWarningCount)") {
		t.Errorf("manifest warning metric should extract the canonical warning total: %s", manifestWarnings)
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
		{name: "Cloud Run success", cloudRun: true, cloudRunJob: "server-tick"},
		{name: "Cloud Run failure", cloudRun: true, cloudRunJob: "server-tick", failure: true},
		{name: "Cloud Run with empty job value", cloudRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			output := captureConfiguredLogs(t, tt.cloudRun, tt.cloudRunJob, func() {
				ctx := clog.NewContextWithLogger(context.Background(), slog.Default())
				code = runJob(ctx, "server-tick", func(ctx context.Context) error {
					clog.WithGroupFields(ctx, "jobMetadata", clog.Fields{"taskIndex": int64(7)})
					if tt.failure {
						return errors.New("test failure")
					}
					return nil
				})
			})

			wantCode, wantMessage, wantSeverity := 0, "server-tick-success", "INFO"
			if tt.failure {
				wantCode, wantMessage, wantSeverity = 1, "server-tick-failure", "ERROR"
			}
			if code != wantCode {
				t.Errorf("runJob() exit code = %d, want %d", code, wantCode)
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(output), &event); err != nil {
				t.Fatalf("failed to decode canonical event %q: %v", output, err)
			}
			if event["message"] != wantMessage || event["severity"] != wantSeverity || event["job"] != "server-tick" {
				t.Errorf("canonical event = %v, want message %q, severity %q, and job server-tick", event, wantMessage, wantSeverity)
			}
			labels, ok := event["logging.googleapis.com/labels"].(map[string]any)
			if !ok || labels["logger"] != "onetrick-server-tick" {
				t.Errorf("event labels = %v, want logger onetrick-server-tick", event["logging.googleapis.com/labels"])
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

func decodeEvents(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	var events []map[string]any
	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Fatalf("failed to decode log events %q: %v", output.String(), err)
		}
		events = append(events, event)
	}
	return events
}
