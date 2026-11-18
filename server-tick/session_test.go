package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	clog "github.com/Shannor/canonical-logger"
)

func TestSessionCompTrackingDeserializationAndEndSessionUpdates(t *testing.T) {
	t.Run("JSON deserializes compTracking.enabled and compTracking.end", func(t *testing.T) {
		rawPending := `{
			"id": "sess-comp-1",
			"userId": "user-1",
			"characterId": "char-1",
			"aggregateIds": ["agg-1"],
			"compTracking": {
				"enabled": true,
				"start": {"points": 5400, "divisionName": "Platinum III"}
			}
		}`
		var pending Session
		if err := json.Unmarshal([]byte(rawPending), &pending); err != nil {
			t.Fatalf("failed to unmarshal session with compTracking.start: %v", err)
		}
		if pending.CompTracking == nil || !pending.CompTracking.Enabled {
			t.Fatalf("expected CompTracking.Enabled = true, got %+v", pending.CompTracking)
		}
		if pending.CompTracking.End != nil {
			t.Fatalf("expected CompTracking.End = nil, got %+v", pending.CompTracking.End)
		}

		rawFinalized := `{
			"id": "sess-comp-2",
			"userId": "user-1",
			"characterId": "char-1",
			"aggregateIds": ["agg-1"],
			"compTracking": {
				"enabled": true,
				"start": {"points": 5400, "divisionName": "Platinum III"},
				"end": {"points": 5525, "divisionName": "Platinum II"},
				"pointDelta": 125
			}
		}`
		var finalized Session
		if err := json.Unmarshal([]byte(rawFinalized), &finalized); err != nil {
			t.Fatalf("failed to unmarshal session with compTracking.end: %v", err)
		}
		if finalized.CompTracking == nil || !finalized.CompTracking.Enabled || finalized.CompTracking.End == nil {
			t.Fatalf("expected CompTracking.Enabled = true and End != nil, got %+v", finalized.CompTracking)
		}
	})

	t.Run("Firestore struct tags match compTracking.enabled and compTracking.end", func(t *testing.T) {
		sessionType := reflect.TypeOf(Session{})
		ctField, ok := sessionType.FieldByName("CompTracking")
		if !ok {
			t.Fatal("Session struct is missing CompTracking field")
		}
		if got := ctField.Tag.Get("firestore"); got != "compTracking,omitempty" {
			t.Errorf("Session.CompTracking firestore tag = %q, want %q", got, "compTracking,omitempty")
		}

		compType := reflect.TypeOf(CompTracking{})
		enabledField, ok := compType.FieldByName("Enabled")
		if !ok || enabledField.Tag.Get("firestore") != "enabled" {
			t.Errorf("CompTracking.Enabled firestore tag = %q, want %q", enabledField.Tag.Get("firestore"), "enabled")
		}
		endField, ok := compType.FieldByName("End")
		if !ok || endField.Tag.Get("firestore") != "end,omitempty" {
			t.Errorf("CompTracking.End firestore tag = %q, want %q", endField.Tag.Get("firestore"), "end,omitempty")
		}
	})

	t.Run("buildEndSessionUpdates does not touch compTracking field", func(t *testing.T) {
		now := time.Now()
		completedBy := AuditField{ID: "system", Username: "system"}
		summary := &SessionSummary{TotalMatches: 3, Wins: 2, Losses: 1}

		updates := buildEndSessionUpdates(completedBy, now, summary)
		for _, u := range updates {
			if strings.HasPrefix(u.Path, "compTracking") {
				t.Fatalf("EndSession update path %q must not overwrite compTracking", u.Path)
			}
		}
		if len(updates) != 5 {
			t.Fatalf("len(updates) = %d, want 5 explicit field updates", len(updates))
		}
	})
}

func TestShouldFinalizeCompExcludesEmptyDisabledAndAlreadyFinalizedSessions(t *testing.T) {
	tests := []struct {
		name    string
		session Session
		want    bool
	}{
		{
			name: "non-empty stale session with compTracking.enabled=true and end=nil",
			session: Session{
				ID:           "sess-1",
				AggregateIDs: []string{"agg-1", "agg-2"},
				CompTracking: &CompTracking{Enabled: true, End: nil},
			},
			want: true,
		},
		{
			name: "empty/deleted session with len(AggregateIDs)==0 is excluded",
			session: Session{
				ID:           "sess-empty",
				AggregateIDs: nil,
				CompTracking: &CompTracking{Enabled: true, End: nil},
			},
			want: false,
		},
		{
			name: "session without compTracking is excluded",
			session: Session{
				ID:           "sess-no-comp",
				AggregateIDs: []string{"agg-1"},
				CompTracking: nil,
			},
			want: false,
		},
		{
			name: "session with compTracking.enabled=false is excluded",
			session: Session{
				ID:           "sess-disabled",
				AggregateIDs: []string{"agg-1"},
				CompTracking: &CompTracking{Enabled: false},
			},
			want: false,
		},
		{
			name: "session with compTracking.end already populated is excluded",
			session: Session{
				ID:           "sess-already-ended",
				AggregateIDs: []string{"agg-1"},
				CompTracking: &CompTracking{
					Enabled: true,
					End:     map[string]any{"points": 5500},
				},
			},
			want: false,
		},
		{
			name: "session with empty ID is excluded",
			session: Session{
				ID:           "",
				AggregateIDs: []string{"agg-1"},
				CompTracking: &CompTracking{Enabled: true},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldFinalizeComp(tt.session); got != tt.want {
				t.Errorf("ShouldFinalizeComp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFinalizeCompSessionsHTTPCallbackAndDryRun(t *testing.T) {
	t.Run("204 No Content sends POST with sessionIds and X-Internal-API-Key", func(t *testing.T) {
		var (
			gotMethod  string
			gotPath    string
			gotAPIKey  string
			gotCT      string
			gotPayload finalizeCompRequest
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAPIKey = r.Header.Get("X-Internal-API-Key")
			gotCT = r.Header.Get("Content-Type")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed to read request body: %v", err)
			}
			if err := json.Unmarshal(body, &gotPayload); err != nil {
				t.Errorf("failed to decode request body %q: %v", string(body), err)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		var output bytes.Buffer
		ctx := testLoggingContext(&output)

		err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
			return FinalizeCompSessions(
				ctx,
				srv.Client(),
				srv.URL+"/",
				"secret-internal-key",
				[]string{"sess-1", "sess-2"},
				false,
			)
		})
		if err != nil {
			t.Fatalf("FinalizeCompSessions() error = %v, want nil", err)
		}
		if gotMethod != http.MethodPost {
			t.Errorf("request method = %q, want POST", gotMethod)
		}
		if gotPath != "/internal/sessions/finalize-comp" {
			t.Errorf("request path = %q, want /internal/sessions/finalize-comp", gotPath)
		}
		if gotAPIKey != "secret-internal-key" {
			t.Errorf("X-Internal-API-Key = %q, want %q", gotAPIKey, "secret-internal-key")
		}
		if gotCT != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", gotCT)
		}
		if !reflect.DeepEqual(gotPayload.SessionIDs, []string{"sess-1", "sess-2"}) {
			t.Errorf("sessionIds payload = %v, want [sess-1 sess-2]", gotPayload.SessionIDs)
		}

		event := decodeEvent(t, &output)
		backendClient, ok := event["backendClient"].(map[string]any)
		if !ok {
			t.Fatalf("backendClient group = %v, want object", event["backendClient"])
		}
		finalizeComp, ok := backendClient["finalizeComp"].(map[string]any)
		if !ok {
			t.Fatalf("backendClient.finalizeComp = %v, want object", backendClient["finalizeComp"])
		}
		if finalizeComp["sessionCount"] != float64(2) {
			t.Errorf("sessionCount = %v, want 2", finalizeComp["sessionCount"])
		}
		if finalizeComp["statusCode"] != float64(http.StatusNoContent) {
			t.Errorf("statusCode = %v, want 204", finalizeComp["statusCode"])
		}
	})

	t.Run("200 OK with JSON summary records updated and failed counts", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"updated":2,"skipped":1,"failed":0}`))
		}))
		defer srv.Close()

		var output bytes.Buffer
		ctx := testLoggingContext(&output)

		err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
			return FinalizeCompSessions(ctx, srv.Client(), srv.URL, "secret-key", []string{"s1", "s2", "s3"}, false)
		})
		if err != nil {
			t.Fatalf("FinalizeCompSessions() error = %v, want nil", err)
		}

		event := decodeEvent(t, &output)
		finalizeComp := event["backendClient"].(map[string]any)["finalizeComp"].(map[string]any)
		if finalizeComp["statusCode"] != float64(http.StatusOK) ||
			finalizeComp["updated"] != float64(2) ||
			finalizeComp["skipped"] != float64(1) ||
			finalizeComp["failed"] != float64(0) {
			t.Errorf("backendClient.finalizeComp = %v, want statusCode=200, updated=2, skipped=1, failed=0", finalizeComp)
		}
	})

	t.Run("dry-run skips HTTP call and logs sessionCount with dryRun=true", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		var output bytes.Buffer
		ctx := testLoggingContext(&output)

		err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
			return FinalizeCompSessions(ctx, srv.Client(), srv.URL, "secret-key", []string{"sess-dry-1", "sess-dry-2"}, true)
		})
		if err != nil {
			t.Fatalf("FinalizeCompSessions(dryRun=true) error = %v, want nil", err)
		}
		if called {
			t.Fatal("expected HTTP server not to be called when dryRun=true")
		}

		event := decodeEvent(t, &output)
		backendClient, ok := event["backendClient"].(map[string]any)
		if !ok {
			t.Fatalf("backendClient group = %v, want object", event["backendClient"])
		}
		finalizeComp, ok := backendClient["finalizeComp"].(map[string]any)
		if !ok {
			t.Fatalf("backendClient.finalizeComp = %v, want object", backendClient["finalizeComp"])
		}
		if finalizeComp["sessionCount"] != float64(2) || finalizeComp["dryRun"] != true {
			t.Errorf("backendClient.finalizeComp = %v, want sessionCount=2 and dryRun=true", finalizeComp)
		}
	})

	t.Run("empty sessionIDs slice skips HTTP call and emits no backendClient telemetry", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		var output bytes.Buffer
		ctx := testLoggingContext(&output)

		err := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
			return FinalizeCompSessions(ctx, srv.Client(), srv.URL, "secret-key", nil, false)
		})
		if err != nil {
			t.Fatalf("FinalizeCompSessions(empty) error = %v, want nil", err)
		}
		if called {
			t.Fatal("expected HTTP server not to be called when sessionIDs is empty")
		}

		event := decodeEvent(t, &output)
		if _, exists := event["backendClient"]; exists {
			t.Errorf("expected no backendClient telemetry when sessionIDs is empty, got %v", event["backendClient"])
		}
	})

	t.Run("non-2xx response returns error and records statusCode and error in telemetry", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
		}))
		defer srv.Close()

		var output bytes.Buffer
		ctx := testLoggingContext(&output)

		var callbackErr error
		_ = clog.Run(ctx, "server-tick", func(ctx context.Context) error {
			callbackErr = FinalizeCompSessions(ctx, srv.Client(), srv.URL, "wrong-key", []string{"sess-1"}, false)
			return nil
		})
		if callbackErr == nil {
			t.Fatal("FinalizeCompSessions() error = nil, want error on 401")
		}

		event := decodeEvent(t, &output)
		finalizeComp := event["backendClient"].(map[string]any)["finalizeComp"].(map[string]any)
		if finalizeComp["statusCode"] != float64(http.StatusUnauthorized) {
			t.Errorf("statusCode = %v, want 401", finalizeComp["statusCode"])
		}
		if _, hasErr := finalizeComp["error"].(string); !hasErr {
			t.Errorf("expected error string in backendClient.finalizeComp, got %v", finalizeComp)
		}
	})
}

func TestConfigFromEnvReadsOneTrickAPIURLAndInternalAPIKey(t *testing.T) {
	t.Setenv("ONE_TRICK_API_URL", "http://localhost:8080")
	t.Setenv("INTERNAL_API_KEY", "test-internal-secret")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv() error = %v, want nil", err)
	}
	if cfg.OneTrickAPIURL != "http://localhost:8080" {
		t.Errorf("cfg.OneTrickAPIURL = %q, want %q", cfg.OneTrickAPIURL, "http://localhost:8080")
	}
	if cfg.InternalAPIKey != "test-internal-secret" {
		t.Errorf("cfg.InternalAPIKey = %q, want %q", cfg.InternalAPIKey, "test-internal-secret")
	}
}
