package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	clog "github.com/Shannor/canonical-logger"
)

const (
	SessionCollection = "sessions"
	// CutOffDuration is when we will automatically complete a session if no activity seen in the time span
	CutOffDuration = 1 * time.Hour
)

func IsStaleSession(s Session) bool {
	now := time.Now()
	if s.LastSeenTimestamp != nil {
		return now.Sub(*s.LastSeenTimestamp) >= CutOffDuration
	}
	if s.UpdatedAt != nil {
		return now.Sub(*s.UpdatedAt) >= CutOffDuration
	}
	return false
}

func IsInactiveSession(s Session) bool {
	now := time.Now()
	return now.Sub(s.StartedAt) >= CutOffDuration
}

func GetSessions(ctx context.Context, db *firestore.Client) ([]Session, error) {
	start := time.Now()
	now := time.Now()

	// Grace period for sessions that might have just completed.
	last15Minutes := now.Add(-15 * time.Minute)

	q1 := firestore.PropertyFilter{
		Path:     "status",
		Operator: "==",
		Value:    SessionPending,
	}

	q2 := firestore.PropertyFilter{
		Path:     "completedAt",
		Operator: ">=",
		Value:    last15Minutes,
	}

	orFilter := firestore.OrFilter{
		Filters: []firestore.EntityFilter{q1, q2},
	}

	docs, err := db.Collection(SessionCollection).
		WhereEntity(orFilter).
		Documents(ctx).
		GetAll()
	if err != nil {
		clog.WithGroupFields(ctx, "db.getSessions", clog.Fields{
			"collection": SessionCollection,
			"durationMs": time.Since(start).Milliseconds(),
		})
		return nil, err
	}

	sessions := make([]Session, 0, len(docs))
	for _, doc := range docs {
		s := Session{}
		err := doc.DataTo(&s)
		if err != nil {
			clog.WithGroupFields(ctx, "db.getSessions", clog.Fields{
				"collection": SessionCollection,
				"durationMs": time.Since(start).Milliseconds(),
			})
			return nil, err
		}
		sessions = append(sessions, s)
	}
	clog.WithGroupFields(ctx, "db.getSessions", clog.Fields{
		"collection":   SessionCollection,
		"rowsReturned": len(sessions),
		"durationMs":   time.Since(start).Milliseconds(),
	})
	return sessions, nil
}

func SetLastActivity(ctx context.Context, db *firestore.Client, ID, activityID string) error {
	start := time.Now()
	_, err := db.Collection(SessionCollection).Doc(ID).Update(ctx, []firestore.Update{
		{
			Path:  "lastSeenActivityId",
			Value: activityID,
		},
		{
			Path:  "lastSeenTimestamp",
			Value: time.Now(),
		},
		{
			Path:  "updatedAt",
			Value: time.Now(),
		},
	})
	clog.WithGroupFields(ctx, "db.setLastActivity", clog.Fields{
		"collection": SessionCollection,
		"sessionId":  ID,
		"activityId": activityID,
		"durationMs": time.Since(start).Milliseconds(),
	})
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

func DeleteSession(ctx context.Context, db *firestore.Client, ID string) error {
	start := time.Now()
	_, err := db.Collection(SessionCollection).Doc(ID).Delete(ctx)
	clog.WithGroupFields(ctx, "db.deleteSession", clog.Fields{
		"collection": SessionCollection,
		"sessionId":  ID,
		"durationMs": time.Since(start).Milliseconds(),
	})
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

func CleanupEmptyCompletedSessions(ctx context.Context, db *firestore.Client, dryRun bool) (int, error) {
	start := time.Now()
	docs, err := db.Collection(SessionCollection).
		Where("status", "==", SessionComplete).
		Documents(ctx).
		GetAll()
	if err != nil {
		clog.WithGroupFields(ctx, "sessionService.cleanupEmpty", clog.Fields{
			"collection": SessionCollection,
			"dryRun":     dryRun,
			"durationMs": time.Since(start).Milliseconds(),
		})
		return 0, fmt.Errorf("failed to fetch completed sessions for cleanup: %w", err)
	}

	deletedCount := 0
	_ = clog.LoopRollup(ctx, "sessionService.cleanupEmptyItem", docs, func(iterCtx context.Context, _ int, doc *firestore.DocumentSnapshot) error {
		clog.WithGroupField(iterCtx, "session", "docId", doc.Ref.ID)
		var s Session
		if err := doc.DataTo(&s); err != nil {
			return fmt.Errorf("failed to parse session %s for cleanup: %w", doc.Ref.ID, err)
		}

		if len(s.AggregateIDs) == 0 {
			if !dryRun {
				if _, err := doc.Ref.Delete(iterCtx); err != nil {
					clog.WithGroupField(iterCtx, "session", "sessionId", s.ID)
					return fmt.Errorf("failed to delete empty completed session %s: %w", s.ID, err)
				}
			}
			deletedCount++
		}
		return nil
	})
	clog.WithGroupFields(ctx, "sessionService.cleanupEmpty", clog.Fields{
		"collection":   SessionCollection,
		"rowsChecked":  len(docs),
		"deletedCount": deletedCount,
		"dryRun":       dryRun,
		"durationMs":   time.Since(start).Milliseconds(),
	})
	return deletedCount, nil
}

func EndOrDeleteSession(ctx context.Context, db *firestore.Client, s Session) error {
	if len(s.AggregateIDs) == 0 {
		return DeleteSession(ctx, db, s.ID)
	}
	return EndSession(ctx, db, s)
}

func EndSession(ctx context.Context, db *firestore.Client, s Session) error {
	start := time.Now()
	completedBy := AuditField{
		ID:       "system",
		Username: "system",
	}
	now := time.Now()

	summary, err := ComputeSessionSummary(ctx, db, s)
	if err != nil {
		clog.WithGroupField(ctx, "sessionService.endSession", "warning", clog.Fields{
			"type":      "ended_session_summary",
			"sessionId": s.ID,
		})
	}

	updates := []firestore.Update{
		{
			Path:  "completedBy",
			Value: completedBy,
		},
		{
			Path:  "status",
			Value: SessionComplete,
		},
		{
			Path:  "completedAt",
			Value: now,
		},
		{
			Path:  "updatedAt",
			Value: now,
		},
	}
	if summary != nil {
		updates = append(updates, firestore.Update{
			Path:  "sessionSummary",
			Value: summary,
		})
	}

	_, err = db.Collection(SessionCollection).Doc(s.ID).Update(ctx, updates)
	clog.WithGroupFields(ctx, "db.endSession", clog.Fields{
		"collection": SessionCollection,
		"sessionId":  s.ID,
		"durationMs": time.Since(start).Milliseconds(),
	})
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}
	return nil
}

func AddAggregateIDs(ctx context.Context, db *firestore.Client, sessionID string, aggregateIDs []string) error {
	start := time.Now()
	ids := make([]any, 0, len(aggregateIDs))
	for _, d := range aggregateIDs {
		ids = append(ids, d)
	}
	_, err := db.Collection(SessionCollection).Doc(sessionID).Update(ctx, []firestore.Update{
		{
			Path:  "aggregateIds",
			Value: firestore.ArrayUnion(ids...),
		},
	})
	clog.WithGroupFields(ctx, "db.addAggregateIds", clog.Fields{
		"collection":     SessionCollection,
		"sessionId":      sessionID,
		"aggregateCount": len(aggregateIDs),
		"durationMs":     time.Since(start).Milliseconds(),
	})
	if err != nil {
		return err
	}
	return nil
}
