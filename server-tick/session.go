package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
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
	now := time.Now()

	// Grace period for sessions that might have just completed.
	last15Minutes := now.Add(-15 * time.Minute)

	q1 := firestore.PropertyFilter{
		Path:     "status",
		Operator: "==",
		Value:    "pending",
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
		return nil, err
	}

	sessions := make([]Session, 0)
	for _, doc := range docs {
		s := Session{}
		err := doc.DataTo(&s)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func SetLastActivity(ctx context.Context, db *firestore.Client, ID, activityID string) error {
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
	if err != nil {
		return fmt.Errorf("failed to update session: %v", err)
	}
	return nil
}

func DeleteSession(ctx context.Context, db *firestore.Client, ID string) error {
	_, err := db.Collection(SessionCollection).Doc(ID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete session: %v", err)
	}
	return nil
}

func CleanupEmptyCompletedSessions(ctx context.Context, db *firestore.Client, dryRun bool) (int, error) {
	docs, err := db.Collection(SessionCollection).
		Where("status", "==", SessionComplete).
		Documents(ctx).
		GetAll()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch completed sessions for cleanup: %w", err)
	}

	deletedCount := 0
	for _, doc := range docs {
		var s Session
		if err := doc.DataTo(&s); err != nil {
			slog.Error("failed to parse session for cleanup", "docId", doc.Ref.ID, "error", err)
			continue
		}

		if len(s.AggregateIDs) == 0 {
			if dryRun {
				slog.Info("[DRY-RUN] would delete older empty completed session", "sessionId", s.ID)
			} else {
				if _, err := doc.Ref.Delete(ctx); err != nil {
					slog.Error("failed to delete empty completed session", "sessionId", s.ID, "error", err)
					continue
				}
				slog.Info("deleted older empty completed session", "sessionId", s.ID)
			}
			deletedCount++
		}
	}
	return deletedCount, nil
}

func EndOrDeleteSession(ctx context.Context, db *firestore.Client, s Session) error {
	if len(s.AggregateIDs) == 0 {
		return DeleteSession(ctx, db, s.ID)
	}
	return EndSession(ctx, db, s.ID)
}

func EndSession(ctx context.Context, db *firestore.Client, ID string) error {
	completedBy := AuditField{
		ID:       "system",
		Username: "system",
	}
	now := time.Now()
	_, err := db.Collection(SessionCollection).Doc(ID).Update(ctx, []firestore.Update{
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
	})
	if err != nil {
		return fmt.Errorf("failed to end session: %v", err)
	}
	return nil
}

func AddAggregateIDs(ctx context.Context, db *firestore.Client, sessionID string, aggregateIDs []string) error {
	ids := make([]any, 0)
	for _, d := range aggregateIDs {
		ids = append(ids, d)
	}
	_, err := db.Collection(SessionCollection).Doc(sessionID).Update(ctx, []firestore.Update{
		{
			Path:  "aggregateIds",
			Value: firestore.ArrayUnion(ids...),
		},
	})
	if err != nil {
		return err
	}
	return nil
}
