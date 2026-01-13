package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	SessionCollection = "sessions"
	// CutOffHours is when we will automatically complete a session if no activity seen in the time span
	CutOffHours = 2
)

func IsStaleSession(s Session) bool {
	now := time.Now()
	if s.LastSeenTimestamp != nil {
		duration := s.LastSeenTimestamp.Sub(now)
		return duration.Abs().Hours() >= CutOffHours
	}
	if s.UpdatedAt != nil {
		duration := s.UpdatedAt.Sub(now)
		return duration.Abs().Hours() >= CutOffHours
	}
	return false
}

func IsInactiveSession(s Session) bool {
	now := time.Now()
	hours := s.StartedAt.Sub(now).Abs().Hours()
	return hours >= CutOffHours
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
