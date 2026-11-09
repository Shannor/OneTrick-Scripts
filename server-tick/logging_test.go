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

func TestRunEmitsCanonicalSuccessEvent(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "request", clog.Fields{
			"taskIndex": int64(2),
		})
		clog.WithFields(ctx, clog.Fields{
			"sessionsProcessed": 3,
		})
		return nil
	})

	if err != nil {
		t.Fatalf("clog.Run() error = %v, want nil", err)
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
	req, ok := event["request"].(map[string]any)
	if !ok || req["taskIndex"] != float64(2) {
		t.Errorf("event request = %v, want taskIndex 2", event["request"])
	}
	if event["sessionsProcessed"] != float64(3) {
		t.Errorf("event sessionsProcessed = %v, want 3", event["sessionsProcessed"])
	}
}

func TestRunEmitsCanonicalFailureAndReturnsError(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		clog.WithGroupFields(ctx, "request", clog.Fields{"taskIndex": int64(4)})
		return errors.New("test failure")
	})

	if err == nil {
		t.Fatalf("clog.Run() error = nil, want error")
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

func TestServerTickConfigurationFailureIsCanonical(t *testing.T) {
	t.Setenv("CLOUD_RUN_TASK_INDEX", "17")
	t.Setenv("SKIP_SAVE", "invalid")
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "server-tick", runServerTick)

	if err == nil {
		t.Fatalf("clog.Run() error = nil, want configuration error")
	}
	events := decodeEvents(t, &output)
	if len(events) != 1 {
		t.Fatalf("configuration failure logs = %v, want one canonical record", events)
	}
	event := events[0]
	if event["message"] != "server-tick-failure" || event["severity"] != "ERROR" {
		t.Errorf("configuration failure event = %v, want server-tick-failure with ERROR severity", event)
	}
	req, ok := event["request"].(map[string]any)
	if !ok || req["taskIndex"] != "17" {
		t.Errorf("event request metadata = %v, want taskIndex 17", event["request"])
	}
	if event["errorsTotal"] != float64(1) {
		t.Errorf("errorsTotal = %v, want 1", event["errorsTotal"])
	}
}

func TestServerTickEmulatorQueryTimeoutIsCanonical(t *testing.T) {
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

			err := clog.Run(ctx, "server-tick", runServerTick)

			if err == nil {
				t.Fatalf("clog.Run() error = nil, want timeout error")
			}
			events := decodeEvents(t, &output)
			if len(events) != 1 {
				t.Fatalf("emulator query failure logs = %v, want one canonical event", events)
			}
			event := events[0]
			if event["message"] != "server-tick-failure" || event["severity"] != "ERROR" {
				t.Errorf("emulator query failure event = %v, want server-tick-failure with ERROR severity", event)
			}
			req, ok := event["request"].(map[string]any)
			if !ok || req["firestoreEmulatorHost"] != endpoint {
				t.Errorf("emulator request metadata = %v, want endpoint %q", event["request"], endpoint)
			}
			if event["errorsTotal"] != float64(1) {
				t.Errorf("emulator errorsTotal = %v, want 1", event["errorsTotal"])
			}
			dbGroup, ok := event["db"].(map[string]any)
			if !ok {
				t.Fatalf("event db group = %v, want nested db group", event["db"])
			}
			getSessions, ok := dbGroup["getSessions"].(map[string]any)
			if !ok || getSessions["collection"] != SessionCollection {
				t.Errorf("db.getSessions = %v, want collection %q", dbGroup["getSessions"], SessionCollection)
			}
			errorText := fmt.Sprint(event["error"])
			if !strings.Contains(errorText, endpoint) {
				t.Errorf("canonical error = %v, want endpoint %q", event["error"], endpoint)
			}
			if got := strings.Contains(errorText, "timed out after"); got != tt.wantEmulatorTimeout {
				t.Errorf("canonical error = %v, emulator timeout attribution = %t, want %t", event["error"], got, tt.wantEmulatorTimeout)
			}
			if tt.wantEmulatorTimeout && ctx.Err() != nil {
				t.Errorf("caller context error = %v, want nil when emulator deadline expires first", ctx.Err())
			} else if !tt.wantEmulatorTimeout && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("caller context error = %v, want deadline exceeded", ctx.Err())
			}
		})
	}
}

func TestConverterManifestWarningsEscalateSeverityToWarning(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		generateClassStats(ctx, map[string]StatDefinition{}, map[string]int32{"missing": 10})
		return nil
	})

	if err != nil {
		t.Fatalf("clog.Run() error = %v, want nil", err)
	}
	event := decodeEvent(t, &output)
	if event["severity"] != "WARNING" {
		t.Errorf("event severity = %v, want WARNING when manifest definition is missing", event["severity"])
	}
	converterSvc, ok := event["converterService"].(map[string]any)
	if !ok {
		t.Fatalf("converterService group = %v, want object", event["converterService"])
	}
	classStats, ok := converterSvc["generateClassStats"].(map[string]any)
	if !ok {
		t.Fatalf("converterService.generateClassStats = %v, want object", converterSvc["generateClassStats"])
	}
	warning, ok := classStats["warning"].(map[string]any)
	if !ok || warning["type"] != "missing_stat_definition" || warning["statKey"] != "missing" {
		t.Errorf("warning = %v, want missing_stat_definition for statKey missing", classStats["warning"])
	}
}

func TestEstimateGameModeClassifiesProductionCombinations(t *testing.T) {
	tests := []struct {
		mode     string
		activity string
		want     GameMode
	}{
		// Competitive matches
		{mode: "Clash Quickplay", activity: "Competitive: Matchmade", want: GameModeCompetitive},
		{mode: "Zone Control", activity: "Competitive: Matchmade", want: GameModeCompetitive},
		{mode: "Clash Quickplay", activity: "Competitive", want: GameModeCompetitive},
		{mode: "Rift", activity: "Competitive", want: GameModeCompetitive},
		{mode: "Survival", activity: "Competitive", want: GameModeCompetitive},
		{mode: "Control Competitive", activity: "Competitive", want: GameModeCompetitive},
		{mode: "Clash Competitive", activity: "Competitive", want: GameModeCompetitive},
		// Quickplay matches
		{mode: "Control Quickplay", activity: "Control: Matchmade", want: GameModeQuickplay},
		{mode: "Control Quickplay", activity: "Control", want: GameModeQuickplay},
		{mode: "Clash Quickplay", activity: "Clash 3v3: Matchmade", want: GameModeQuickplay},
		{mode: "Zone Control", activity: "Zone Control: Matchmade", want: GameModeQuickplay},
		{mode: "Zone Control", activity: "Cutting Edge: Zone Control: Matchmade", want: GameModeQuickplay},
		// Iron Banner matches
		{mode: "Iron Banner Control", activity: "Control: Matchmade", want: GameModeIronBanner},
		{mode: "Iron Banner Control", activity: "Iron Banner: Control", want: GameModeIronBanner},
		{mode: "Iron Banner Supremacy", activity: "Iron Banner: Tribute", want: GameModeIronBanner},
		{mode: "Iron Banner Clash", activity: "Eruption: Matchmade", want: GameModeIronBanner},
		{mode: "Iron Banner Clash", activity: "Iron Banner: Eruption", want: GameModeIronBanner},
		// Trials of Osiris matches
		{mode: "Trials of Osiris", activity: "Trials of Osiris: Matchmade", want: GameModeTrials},
		{mode: "Trials of Osiris", activity: "Trials of Osiris", want: GameModeTrials},
		{mode: "Trials Survival", activity: "Trials of Osiris", want: GameModeTrials},
	}

	for _, tt := range tests {
		t.Run(tt.mode+"|"+tt.activity, func(t *testing.T) {
			mode := tt.mode
			got := EstimateGameMode(&mode, tt.activity)
			if got == nil || *got != tt.want {
				t.Errorf("EstimateGameMode(%q, %q) = %v, want %q", tt.mode, tt.activity, got, tt.want)
			}
		})
	}
}

func TestLoopRollupSummarizesSessionsAndRetainsOnlyFailedSnapshots(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)

	sessions := []Session{
		{ID: "sess-1", UserID: "user-1", CharacterID: "char-1"},
		{ID: "sess-2", UserID: "user-2", CharacterID: "char-2"},
		{ID: "sess-3", UserID: "user-3", CharacterID: "char-3"},
	}

	err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		_ = clog.LoopRollup(ctx, "sessionService.processSession", sessions, func(iterCtx context.Context, _ int, s Session) error {
			clog.WithGroupFields(iterCtx, "session", clog.Fields{
				"sessionId":   s.ID,
				"userId":      s.UserID,
				"characterId": s.CharacterID,
			})
			clog.WithGroupFields(iterCtx, "db.getUser", clog.Fields{
				"collection": "users",
				"userId":     s.UserID,
			})
			if s.ID == "sess-2" {
				return errors.New("bungie API timeout")
			}
			return nil
		})
		return nil
	})

	if err != nil {
		t.Fatalf("clog.Run() error = %v, want nil", err)
	}
	event := decodeEvent(t, &output)
	if event["message"] != "server-tick-success" {
		t.Errorf("event message = %v, want server-tick-success", event["message"])
	}
	// Successful iteration db.getUser should NOT leak onto the parent root
	if _, leaked := event["db"]; leaked {
		t.Errorf("expected iteration db group not to leak onto parent root, got %v", event["db"])
	}

	sessionSvc, ok := event["sessionService"].(map[string]any)
	if !ok {
		t.Fatalf("sessionService group = %v, want object", event["sessionService"])
	}
	rollup, ok := sessionSvc["processSession"].(map[string]any)
	if !ok {
		t.Fatalf("sessionService.processSession = %v, want object", sessionSvc["processSession"])
	}
	if rollup["total"] != float64(3) || rollup["succeeded"] != float64(2) || rollup["failed"] != float64(1) {
		t.Errorf("rollup counts = %v, want total=3, succeeded=2, failed=1", rollup)
	}
	failures, ok := rollup["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("rollup failures = %v, want 1 failed snapshot", rollup["failures"])
	}
	failedSnap, ok := failures[0].(map[string]any)
	if !ok {
		t.Fatalf("failed snapshot = %v, want object", failures[0])
	}
	sessGroup, ok := failedSnap["session"].(map[string]any)
	if !ok || sessGroup["sessionId"] != "sess-2" {
		t.Errorf("failed snapshot session = %v, want sessionId sess-2", failedSnap["session"])
	}
	dbGroup, ok := failedSnap["db"].(map[string]any)
	if !ok {
		t.Fatalf("failed snapshot db = %v, want nested db group", failedSnap["db"])
	}
	getUser, ok := dbGroup["getUser"].(map[string]any)
	if !ok || getUser["userId"] != "user-2" {
		t.Errorf("failed snapshot db.getUser = %v, want userId user-2", dbGroup["getUser"])
	}
}

func TestRunEmitsCanonicalFailureBeforeRepanicking(t *testing.T) {
	var output bytes.Buffer
	ctx := testLoggingContext(&output)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = clog.Run(ctx, "server-tick", func(context.Context) error {
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
			if !strings.Contains(text, "EXTRACT(jsonPayload."+field+")") {
				t.Errorf("metric extractor does not read root jsonPayload.%s: %s", field, text)
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
	if !strings.Contains(string(manifestWarnings), `jsonPayload.job="server-tick"`) || !strings.Contains(string(manifestWarnings), `severity="WARNING"`) {
		t.Errorf("manifest warning metric should filter canonical WARNING events: %s", manifestWarnings)
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
			var runErr error
			output := captureConfiguredLogs(t, tt.cloudRun, tt.cloudRunJob, func() {
				ctx := clog.NewContextWithLogger(context.Background(), slog.Default())
				runErr = clog.Run(ctx, "server-tick", func(ctx context.Context) error {
					clog.WithGroupFields(ctx, "request", clog.Fields{"taskIndex": int64(7)})
					if tt.failure {
						return errors.New("test failure")
					}
					return nil
				})
			})

			wantMessage, wantSeverity := "server-tick-success", "INFO"
			if tt.failure {
				wantMessage, wantSeverity = "server-tick-failure", "ERROR"
			}
			if (runErr != nil) != tt.failure {
				t.Errorf("clog.Run() error = %v, want failure = %t", runErr, tt.failure)
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(output), &event); err != nil {
				t.Fatalf("failed to decode canonical event %q: %v", output, err)
			}
			if event["message"] != wantMessage || event["severity"] != wantSeverity || event["job"] != "server-tick" {
				t.Errorf("canonical event = %v, want message %q, severity %q, and job server-tick", event, wantMessage, wantSeverity)
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
