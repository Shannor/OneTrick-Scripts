package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestGenerateArmorSetBonusesZeroTwoAndFourPieceCombinations(t *testing.T) {
	itemSets := map[string]EquipableItemSetDefinition{
		"2618364783": {
			Hash: 2618364783,
			DisplayProperties: ItemDisplayProperties{
				Name: "Per Audacia",
			},
			SetItems: []int64{479136420, 43450237, 3913444611, 2367602335, 2812267642},
			SetPerks: []ItemSetPerkDefinition{
				{RequiredSetCount: 4, SandboxPerkHash: 3091068917},
				{RequiredSetCount: 2, SandboxPerkHash: 3091068916},
			},
		},
		"1223381128": {
			Hash: 1223381128,
			DisplayProperties: ItemDisplayProperties{
				Name: "AION Renewal",
			},
			SetItems: []int64{94775422, 3389206211},
			SetPerks: []ItemSetPerkDefinition{
				{RequiredSetCount: 2, SandboxPerkHash: 2140508055},
				{RequiredSetCount: 4, SandboxPerkHash: 2140508054},
			},
		},
	}

	perks := map[string]PerkDefinition{
		"3091068916": {
			Hash: 3091068916,
			DisplayProperties: DamageTypeDisplayProperties{
				Name:        "Primary Chain",
				Description: "Primary ammo weapon final blows grant a stacking bonus.",
				Icon:        "/common/destiny2_content/icons/primary_chain.png",
				HasIcon:     true,
			},
			IsDisplayable: true,
		},
		"3091068917": {
			Hash: 3091068917,
			DisplayProperties: DamageTypeDisplayProperties{
				Name:        "Sublime Transit",
				Description: "Increases mobility and grants enhanced sprint speed.",
				Icon:        "/common/destiny2_content/icons/sublime_transit.png",
				HasIcon:     true,
			},
			IsDisplayable: true,
		},
		"2140508055": {
			Hash: 2140508055,
			DisplayProperties: DamageTypeDisplayProperties{
				Name:        "Force Converter",
				Description: "Sprint for a short time to gain Speed Booster.",
				Icon:        "/common/destiny2_content/icons/force_converter.png",
				HasIcon:     true,
			},
			IsDisplayable: true,
		},
	}

	helmetKey := strconv.FormatUint(uint64(HelmetArmor), 10)
	gauntletsKey := strconv.FormatUint(uint64(GauntletsArmor), 10)
	chestKey := strconv.FormatUint(uint64(ChestArmor), 10)
	legKey := strconv.FormatUint(uint64(LegArmor), 10)
	classKey := strconv.FormatUint(uint64(ClassArmor), 10)

	t.Run("0-piece or 1-piece activates no set bonus", func(t *testing.T) {
		loadout := Loadout{
			helmetKey: {ItemHash: 479136420}, // 1 Per Audacia
			legKey:    {ItemHash: 1709794165}, // Exotic legs
		}
		got := generateArmorSetBonuses(context.Background(), loadout, itemSets, perks)
		if len(got) != 0 {
			t.Fatalf("generateArmorSetBonuses() = %+v, want empty slice for 1-piece", got)
		}
	})

	t.Run("2-piece activates 2pc bonus only", func(t *testing.T) {
		loadout := Loadout{
			helmetKey:    {ItemHash: 479136420},
			gauntletsKey: {ItemHash: 43450237},
			chestKey:     {ItemHash: 999999999},
		}
		got := generateArmorSetBonuses(context.Background(), loadout, itemSets, perks)
		if len(got) != 1 {
			t.Fatalf("len(generateArmorSetBonuses()) = %d, want 1", len(got))
		}
		if got[0].SetHash != 2618364783 || got[0].SetName != "Per Audacia" || got[0].ActiveCount != 2 {
			t.Errorf("got[0] = %+v, want Per Audacia activeCount=2", got[0])
		}
		if len(got[0].ActivePerks) != 1 || got[0].ActivePerks[0].Hash != 3091068916 || got[0].ActivePerks[0].RequiredSetCount != 2 {
			t.Errorf("got[0].ActivePerks = %+v, want [Primary Chain (2pc)]", got[0].ActivePerks)
		}
	})

	t.Run("4-piece activates both 2pc and 4pc bonuses sorted by requiredSetCount", func(t *testing.T) {
		loadout := Loadout{
			helmetKey:    {ItemHash: 479136420},
			gauntletsKey: {ItemHash: 43450237},
			chestKey:     {ItemHash: 3913444611},
			legKey:       {ItemHash: 1709794165}, // Exotic legs
			classKey:     {ItemHash: 2812267642},
		}
		got := generateArmorSetBonuses(context.Background(), loadout, itemSets, perks)
		if len(got) != 1 {
			t.Fatalf("len(generateArmorSetBonuses()) = %d, want 1", len(got))
		}
		if got[0].ActiveCount != 4 || len(got[0].ActivePerks) != 2 {
			t.Fatalf("got[0] = %+v, want activeCount=4 and 2 activePerks", got[0])
		}
		if got[0].ActivePerks[0].RequiredSetCount != 2 || got[0].ActivePerks[0].Name != "Primary Chain" {
			t.Errorf("got[0].ActivePerks[0] = %+v, want 2pc Primary Chain first", got[0].ActivePerks[0])
		}
		if got[0].ActivePerks[1].RequiredSetCount != 4 || got[0].ActivePerks[1].Name != "Sublime Transit" {
			t.Errorf("got[0].ActivePerks[1] = %+v, want 4pc Sublime Transit second", got[0].ActivePerks[1])
		}
	})

	t.Run("split 2-piece + 2-piece activates both 2pc set bonuses sorted by setHash", func(t *testing.T) {
		loadout := Loadout{
			helmetKey:    {ItemHash: 479136420},  // Per Audacia
			gauntletsKey: {ItemHash: 43450237},   // Per Audacia
			chestKey:     {ItemHash: 94775422},   // AION Renewal
			legKey:       {ItemHash: 3389206211}, // AION Renewal
		}
		got := generateArmorSetBonuses(context.Background(), loadout, itemSets, perks)
		if len(got) != 2 {
			t.Fatalf("len(generateArmorSetBonuses()) = %d, want 2", len(got))
		}
		if got[0].SetHash != 1223381128 || got[1].SetHash != 2618364783 {
			t.Errorf("got sets = [%d, %d], want sorted [1223381128, 2618364783]", got[0].SetHash, got[1].SetHash)
		}
	})
}

func TestLiveFirestoreEquipableItemSetsAndSnapshotBonusResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Firestore verification in short mode")
	}
	t.Setenv("FIRESTORE_EMULATOR_HOST", "")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := firestore.NewClient(ctx, defaultProjectID)
	if err != nil {
		t.Skipf("skipping live Firestore test: %v", err)
	}
	defer db.Close()

	itemSets, err := GetEquipableItemSets(ctx, db)
	if err != nil {
		t.Skipf("skipping live Firestore test (could not read d2EquipableItemSets): %v", err)
	}
	if len(itemSets) == 0 {
		t.Fatalf("GetEquipableItemSets() returned 0 sets, want > 0")
	}

	var perkHashes []int64
	for _, setDef := range itemSets {
		for _, sp := range setDef.SetPerks {
			if sp.SandboxPerkHash != 0 {
				perkHashes = append(perkHashes, sp.SandboxPerkHash)
			}
		}
	}
	perks, err := GetPerksByIDs(ctx, db, perkHashes)
	if err != nil {
		t.Fatalf("GetPerksByIDs() error = %v", err)
	}

	snapDocs, err := db.Collection(snapshotCollection).Documents(ctx).GetAll()
	if err != nil {
		t.Fatalf("failed to fetch snapshots: %v", err)
	}

	var (
		withBonuses     int
		twoPieceOnly    int
		fourPiece       int
		splitTwoPlusTwo int
	)
	for _, doc := range snapDocs {
		var snap CharacterSnapshot
		if err := doc.DataTo(&snap); err != nil {
			continue
		}
		bonuses := generateArmorSetBonuses(ctx, snap.Loadout, itemSets, perks)
		if len(bonuses) > 0 {
			withBonuses++
			if len(bonuses) >= 2 {
				splitTwoPlusTwo++
			} else if bonuses[0].ActiveCount >= 4 {
				fourPiece++
			} else {
				twoPieceOnly++
			}
		}
	}
	t.Logf("Evaluated %d existing snapshots in Firestore: %d have active Armor Set Bonuses (2-piece only: %d, 4-piece: %d, split 2+2-piece: %d)",
		len(snapDocs), withBonuses, twoPieceOnly, fourPiece, splitTwoPlusTwo)
	if withBonuses == 0 {
		t.Errorf("expected at least 1 production snapshot to resolve active Armor Set Bonuses, got 0")
	}
}

