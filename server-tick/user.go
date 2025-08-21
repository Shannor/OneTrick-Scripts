package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const (
	userCollection = "users"
)

func GetMembershipType(ctx context.Context, db *firestore.Client, userID string) (int64, string, error) {
	u, err := GetUser(ctx, db, userID)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch user: %w", err)
	}
	membershipType := int64(0)
	for _, membership := range u.Memberships {
		if membership.ID == u.PrimaryMembershipID {
			membershipType = membership.Type
		}
	}
	return membershipType, u.PrimaryMembershipID, nil
}
func GetUser(ctx context.Context, db *firestore.Client, ID string) (*User, error) {
	user := User{}

	q1 := firestore.PropertyFilter{
		Path:     "id",
		Operator: "==",
		Value:    ID,
	}

	q2 := firestore.PropertyFilter{
		Path:     "memberId",
		Operator: "==",
		Value:    ID,
	}
	q3 := firestore.PropertyFilter{
		Path:     "primaryMembershipId",
		Operator: "==",
		Value:    ID,
	}
	orFilter := firestore.OrFilter{
		Filters: []firestore.EntityFilter{q1, q2, q3},
	}

	iter := db.Collection(userCollection).WhereEntity(orFilter).Documents(ctx)

	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		err = doc.DataTo(&user)
		if err != nil {
			return nil, err
		}
		return &user, nil
	}

	return nil, fmt.Errorf("not found")
}

type User struct {
	ID                  string       `json:"id" firestore:"id"`
	MemberID            string       `json:"memberId" firestore:"memberId"`
	PrimaryMembershipID string       `json:"primaryMembershipId" firestore:"primaryMembershipId"`
	UniqueName          string       `json:"uniqueName" firestore:"uniqueName"`
	DisplayName         string       `json:"displayName" firestore:"displayName"`
	Memberships         []Membership `json:"memberships" firestore:"memberships"`
	CreatedAt           time.Time    `json:"createdAt" firestore:"createdAt"`
	CharacterIDs        []string     `json:"characterIDs" firestore:"characterIds"`
}

type Membership struct {
	ID          string `json:"id" firestore:"id"`
	Type        int64  `json:"type" firestore:"type"`
	DisplayName string `json:"displayName" firestore:"displayName"`
}
