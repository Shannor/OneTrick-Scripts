package generator

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// SessionName generates a Destiny 2 themed name by combining randomly selected elements.
func SessionName() string {
	prefixes := []string{
		"Sweat'", "Flawless'", "Focus'", "Speed'", "Clutch'",
		"React'", "Sharp'", "Quick'", "Tactical'", "Primed'",
		"Elite'", "Pro'", "Peak'", "Optimal'", "Perfect'",
	}

	suffixes := []string{
		"Goes Brrr", "Has Entered the Chat", "Intensifies", "404",
		"Not Found", "Over 9000", "To the Moon", "Stonks",
		"Yeet", "POV", "No Cap", "Chad", "This Is Fine",
		"Sus", "Poggers", "Big Brain Time", "Chief", "Literally Me",
		"In 4K", "Speedrun", "Any%", "Hack", "Doom",
		"Unhinged", "Vibes", "Energy", "UwU", "Loading...",
	}

	nouns := []string{
		"Operation", "Protocol", "Project", "Mission", "Initiative", "Task",
		"Objective", "Endeavor", "Campaign", "Program",
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	usePrefix := r.Float64() < 0.7
	useSuffix := r.Float64() < 0.8

	noun := nouns[r.Intn(len(nouns))]
	// Apply prefix/suffix modifications
	if usePrefix {
		prefix := prefixes[r.Intn(len(prefixes))]
		noun = prefix + noun
	}

	if useSuffix {
		suffix := suffixes[r.Intn(len(suffixes))]
		noun = noun + suffix
	}

	return fmt.Sprintf("%s", noun)
}

// PVPName generates a Destiny 2 PvP loadout name with meme flair
func PVPName() string {
	// PvP build types and playstyles
	buildTypes := []string{
		"Aggressive", "Defensive", "Rush", "Anchor", "Precision",
		"Flanking", "Support", "Slayer", "Lockdown", "Roaming",
		"Shutdown", "Denial", "Counter", "Zone", "Reactive",
		"Passive", "Punish", "Pressure", "Control", "Tempo",
	}

	// PvP playstyle descriptors
	playstyles := []string{
		"Silent", "Swift", "Methodical", "Calculated", "Ruthless",
		"Tactical", "Coordinated", "Disciplined", "Unpredictable", "Patient",
		"Aggressive", "Precise", "Rapid", "Strategic", "Dominant",
		"Disruptive", "Elusive", "Mobile", "Defensive", "Persistent",
	}

	// Destiny 2 PvP meme terms
	memeTerms := []string{
		"Main Character", "Touch Grass", "Crayon Eater", "Monkey Brain", "W Key",
		"Skill Issue", "Tilt Proof", "Keyboard Warrior", "Chair Camper", "Sweat Lord",
		"Tryhard Andy", "No Life", "Dad Build", "Bot Lobby", "Grief Master",
		"Rage Quit", "Skill Gap", "Gamer Mode", "Touched Solar", "Zero Chill",
	}

	// PvP-focused prefixes
	prefixes := []string{
		"Sweat'", "Flawless'", "Focus'", "Speed'", "Clutch'",
		"React'", "Sharp'", "Quick'", "Tactical'", "Primed'",
		"Elite'", "Pro'", "Peak'", "Optimal'", "Perfect'",
	}

	// Create a new random number generator
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Random chance modifiers
	usePrefix := r.Float64() < 0.4    // 40% chance
	usePlaystyle := r.Float64() < 0.7 // 70% chance
	useMeme := r.Float64() < 0.6      // 60% chance

	// Build the name
	var parts []string

	if usePrefix {
		prefix := prefixes[r.Intn(len(prefixes))]
		parts = append(parts, prefix)
	}

	// Always include a build type
	buildType := buildTypes[r.Intn(len(buildTypes))]
	parts = append(parts, buildType)

	if usePlaystyle {
		playstyle := playstyles[r.Intn(len(playstyles))]
		parts = append(parts, playstyle)
	}

	if useMeme {
		meme := memeTerms[r.Intn(len(memeTerms))]
		parts = append(parts, meme)
	}

	result := strings.Join(parts, " ")

	// Clean up double spaces
	result = strings.ReplaceAll(result, "  ", " ")
	return result
}
