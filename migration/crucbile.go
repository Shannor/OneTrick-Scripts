package main

import (
	"strings"
)

func BuildCrucibleMaps(items map[string]ActivityDefinition) (any, error) {
	maps := make(map[string]CrucibleMap)
	for _, definition := range items {
		// Combine with skip function
		if definition.IsPvP && !definition.IsPlaylist {
			name := format(definition.DisplayProperties.Name)
			if skip(name) {
				continue
			}
			existing, ok := maps[name]
			if ok {
				existing.Hashes = append(existing.Hashes, definition.Hash)
				existing.ActivityModeHashes = append(existing.Hashes, definition.ActivityModeHashes...)
				existing.ActivityModeTypes = append(existing.Hashes, definition.ActivityModeTypes...)
				if !existing.DisplayProperties.HasIcon {
					existing.DisplayProperties.Icon = definition.DisplayProperties.Icon
					existing.DisplayProperties.HasIcon = definition.DisplayProperties.HasIcon
				}
				maps[name] = existing
			} else {
				maps[name] = CrucibleMap{
					DisplayProperties:  definition.DisplayProperties,
					Key:                name,
					Hashes:             []int{definition.Hash},
					ActivityModeHashes: definition.ActivityModeHashes,
					ActivityModeTypes:  definition.ActivityModeTypes,
				}
			}
		}
	}
	return maps, nil
}

func format(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}

func skip(value string) bool {
	if strings.Contains(value, "private") {
		return true
	}
	if strings.Contains(value, "control") {
		return true
	}
	if strings.Contains(value, "clash") {
		return true
	}
	if strings.Contains(value, "elimination") {
		return true
	}
	if strings.Contains(value, "iron-banner") {
		return true
	}
	if strings.Contains(value, "relic") {
		return true
	}
	if strings.Contains(value, "rumble") {
		return true
	}
	if value == "" {
		return true
	}
	return false
}

type CrucibleMap struct {
	DisplayProperties  ActivityDisplayProperties `firestore:"displayProperties"`
	Key                string                    `firestore:"key"`
	Hashes             []int                     `firestore:"hashes"`
	ActivityModeHashes []int                     `firestore:"activityModeHashes"`
	ActivityModeTypes  []int                     `firestore:"activityModeTypes"`
	Categories         []string                  `firestore:"categories"`
}
