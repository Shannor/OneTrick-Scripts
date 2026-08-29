package utils

import (
	"fmt"

	"cloud.google.com/go/firestore"
)

func GetAllToStructs[T any](docs []*firestore.DocumentSnapshot) ([]T, error) {
	result := make([]T, 0, len(docs))
	for _, doc := range docs {
		if !doc.Exists() {
			continue
		}
		var item T
		if err := doc.DataTo(&item); err != nil {
			return nil, fmt.Errorf("failed to convert doc %s: %w", doc.Ref.ID, err)
		}
		result = append(result, item)
	}
	return result, nil
}

func ToMap[T any, K comparable](items []T, keySelector func(T) K) (map[K]T, error) {
	if items == nil {
		return nil, fmt.Errorf("items slice cannot be nil")
	}

	result := make(map[K]T)
	for _, item := range items {
		key := keySelector(item)
		result[key] = item
	}
	return result, nil
}
