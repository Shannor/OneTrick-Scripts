package main

import (
	"bytes"
	"context"
	"errors"
	"serverTick/utils"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	clog "github.com/Shannor/canonical-logger"
)

type fakeSnapshotStore struct {
	byHash           map[string]*CharacterSnapshot
	docs             map[string]*CharacterSnapshot
	historiesWritten []CharacterSnapshot
	createdDocs      []CharacterSnapshot
	clearedArchives  []string
	recentHistories  []History
	getByHashErr     error
	getDocErr        map[string]error
	createDocErr     error
	createHistoryErr error
	listHistoriesErr error
}

func newFakeSnapshotStore() *fakeSnapshotStore {
	return &fakeSnapshotStore{
		byHash:    make(map[string]*CharacterSnapshot),
		docs:      make(map[string]*CharacterSnapshot),
		getDocErr: make(map[string]error),
	}
}

func (f *fakeSnapshotStore) getByHash(_ context.Context, hash string) (*CharacterSnapshot, error) {
	if f.getByHashErr != nil {
		return nil, f.getByHashErr
	}
	snap, ok := f.byHash[hash]
	if !ok || snap == nil {
		return nil, nil
	}
	cloned := *snap
	return &cloned, nil
}

func (f *fakeSnapshotStore) getDoc(_ context.Context, snapshotID string) (*CharacterSnapshot, error) {
	if err, ok := f.getDocErr[snapshotID]; ok {
		return nil, err
	}
	snap, ok := f.docs[snapshotID]
	if !ok || snap == nil {
		return nil, ErrSnapshotNotFound
	}
	cloned := *snap
	return &cloned, nil
}

func (f *fakeSnapshotStore) createSnapshotDoc(_ context.Context, snapshot CharacterSnapshot) (CharacterSnapshot, error) {
	if f.createDocErr != nil {
		return CharacterSnapshot{}, f.createDocErr
	}
	if snapshot.ID == "" {
		snapshot.ID = "created-snap-id"
	}
	f.createdDocs = append(f.createdDocs, snapshot)
	f.docs[snapshot.ID] = &snapshot
	f.byHash[snapshot.Hash] = &snapshot
	return snapshot, nil
}

func (f *fakeSnapshotStore) createHistory(_ context.Context, og CharacterSnapshot) (*string, error) {
	if f.createHistoryErr != nil {
		return nil, f.createHistoryErr
	}
	f.historiesWritten = append(f.historiesWritten, og)
	id := og.ID
	return &id, nil
}

func (f *fakeSnapshotStore) clearMergeArchiveFields(_ context.Context, snapshotID string) error {
	f.clearedArchives = append(f.clearedArchives, snapshotID)
	if snap, ok := f.docs[snapshotID]; ok && snap != nil {
		snap.MergedIntoID = nil
		snap.MergedAt = nil
	}
	return nil
}

func (f *fakeSnapshotStore) listRecentHistories(_ context.Context, _, _ string, _, _ time.Time) ([]History, error) {
	if f.listHistoriesErr != nil {
		return nil, f.listHistoriesErr
	}
	return f.recentHistories, nil
}

func sampleLoadout() Loadout {
	return Loadout{
		strconv.Itoa(Kinetic): {
			InstanceID: "inst-kinetic",
			ItemHash:   111,
			ItemProperties: ItemProperties{
				BaseInfo: BaseItemInfo{InstanceId: "inst-kinetic"},
				Perks: []Perk{
					{Hash: 300},
					{Hash: 100},
					{Hash: 200},
				},
			},
		},
		strconv.Itoa(Energy): {
			InstanceID: "inst-energy",
			ItemHash:   222,
			ItemProperties: ItemProperties{
				BaseInfo: BaseItemInfo{InstanceId: "inst-energy"},
				Perks: []Perk{
					{Hash: 500},
					{Hash: 400},
				},
			},
		},
		strconv.Itoa(Power): {
			InstanceID: "inst-power",
			ItemHash:   333,
			ItemProperties: ItemProperties{
				BaseInfo: BaseItemInfo{InstanceId: "inst-power"},
				Perks:    []Perk{{Hash: 600}},
			},
		},
	}
}

func TestGenerateHashParityAndDoesNotMutateCallerPerks(t *testing.T) {
	loadout := sampleLoadout()
	kineticKey := strconv.Itoa(Kinetic)
	origPerks := []int64{300, 100, 200}

	snap := CharacterSnapshot{
		UserID:      "user-1",
		CharacterID: "char-1",
		Loadout:     loadout,
	}

	gotHash, err := generateHash(snap)
	if err != nil {
		t.Fatalf("generateHash() error = %v, want nil", err)
	}

	// Verify caller Perks slice was not mutated in-place
	for i, wantPerkHash := range origPerks {
		if got := snap.Loadout[kineticKey].ItemProperties.Perks[i].Hash; got != wantPerkHash {
			t.Errorf("caller Perks[%d].Hash = %d, want %d (slice was mutated in-place)", i, got, wantPerkHash)
		}
	}

	// Verify exact parity with Shannor/OneTrick GenerateHash:
	// sorted bucket keys: "1498876634" (Kinetic), "2465295065" (Energy), "953998645" (Power)
	expectedHashing := []string{
		"inst-kinetic",
		"inst-kinetic-100",
		"inst-kinetic-200",
		"inst-kinetic-300",
		"inst-energy",
		"inst-energy-400",
		"inst-energy-500",
		"inst-power",
		"inst-power-600",
	}
	wantHash, err := utils.HashMap(expectedHashing)
	if err != nil {
		t.Fatalf("utils.HashMap() error = %v", err)
	}
	if gotHash != wantHash {
		t.Errorf("generateHash() = %q, want %q", gotHash, wantHash)
	}
}

func TestCreateWithActiveExistingSnapshotWritesHistoryToActiveSnapshot(t *testing.T) {
	store := newFakeSnapshotStore()
	snap := CharacterSnapshot{
		UserID:      "user-1",
		CharacterID: "char-1",
		Loadout:     sampleLoadout(),
	}
	hash, err := generateHash(snap)
	if err != nil {
		t.Fatalf("generateHash() error = %v", err)
	}

	activeSnap := snap
	activeSnap.ID = "active-snap-1"
	activeSnap.Hash = hash
	store.byHash[hash] = &activeSnap
	store.docs[activeSnap.ID] = &activeSnap

	gotID, err := createWithStore(context.Background(), store, "user-1", snap)
	if err != nil {
		t.Fatalf("createWithStore() error = %v, want nil", err)
	}
	if gotID == nil || *gotID != "active-snap-1" {
		t.Fatalf("createWithStore() id = %v, want active-snap-1", gotID)
	}
	if len(store.historiesWritten) != 1 || store.historiesWritten[0].ID != "active-snap-1" {
		t.Errorf("historiesWritten = %v, want 1 entry on active-snap-1", store.historiesWritten)
	}
	if len(store.createdDocs) != 0 {
		t.Errorf("createdDocs = %v, want 0 new snapshot docs", store.createdDocs)
	}
}

func TestCreateWithArchivedSnapshotAutoForwardsIncludingOneHopChain(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(store *fakeSnapshotStore, hash string)
		wantID   string
		wantFrom string
	}{
		{
			name: "direct mergedIntoId target",
			setup: func(store *fakeSnapshotStore, hash string) {
				targetID := "target-snap-1"
				archived := CharacterSnapshot{
					ID:           "archived-snap-1",
					UserID:       "user-1",
					CharacterID:  "char-1",
					Hash:         hash,
					MergedIntoID: &targetID,
					Loadout:      sampleLoadout(),
				}
				target := CharacterSnapshot{
					ID:          targetID,
					UserID:      "user-1",
					CharacterID: "char-1",
					Loadout:     sampleLoadout(),
				}
				store.byHash[hash] = &archived
				store.docs[archived.ID] = &archived
				store.docs[target.ID] = &target
			},
			wantID:   "target-snap-1",
			wantFrom: "archived-snap-1",
		},
		{
			name: "1-hop chain A -> B -> C",
			setup: func(store *fakeSnapshotStore, hash string) {
				midID := "mid-snap-B"
				finalID := "final-snap-C"
				snapA := CharacterSnapshot{
					ID:           "archived-snap-A",
					UserID:       "user-1",
					CharacterID:  "char-1",
					Hash:         hash,
					MergedIntoID: &midID,
					Loadout:      sampleLoadout(),
				}
				snapB := CharacterSnapshot{
					ID:           midID,
					UserID:       "user-1",
					CharacterID:  "char-1",
					MergedIntoID: &finalID,
					Loadout:      sampleLoadout(),
				}
				snapC := CharacterSnapshot{
					ID:          finalID,
					UserID:      "user-1",
					CharacterID: "char-1",
					Loadout:     sampleLoadout(),
				}
				store.byHash[hash] = &snapA
				store.docs[snapA.ID] = &snapA
				store.docs[snapB.ID] = &snapB
				store.docs[snapC.ID] = &snapC
			},
			wantID:   "final-snap-C",
			wantFrom: "archived-snap-A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeSnapshotStore()
			snap := CharacterSnapshot{
				UserID:      "user-1",
				CharacterID: "char-1",
				Loadout:     sampleLoadout(),
			}
			hash, err := generateHash(snap)
			if err != nil {
				t.Fatalf("generateHash() error = %v", err)
			}
			tt.setup(store, hash)

			var output bytes.Buffer
			ctx := testLoggingContext(&output)

			var gotID *string
			runErr := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
				var err error
				gotID, err = createWithStore(ctx, store, "user-1", snap)
				return err
			})
			if runErr != nil {
				t.Fatalf("createWithStore() error = %v, want nil", runErr)
			}
			if gotID == nil || *gotID != tt.wantID {
				t.Fatalf("createWithStore() id = %v, want %q", gotID, tt.wantID)
			}
			if len(store.historiesWritten) != 1 || store.historiesWritten[0].ID != tt.wantID {
				t.Errorf("historiesWritten = %v, want 1 entry on %q", store.historiesWritten, tt.wantID)
			}
			if len(store.clearedArchives) != 0 {
				t.Errorf("clearedArchives = %v, want none", store.clearedArchives)
			}
			event := decodeEvent(t, &output)
			snapSvc, ok := event["snapshotService"].(map[string]any)
			if !ok {
				t.Fatalf("snapshotService group = %v, want object", event["snapshotService"])
			}
			createGroup, ok := snapSvc["create"].(map[string]any)
			if !ok {
				t.Fatalf("snapshotService.create = %v, want object", snapSvc["create"])
			}
			if createGroup["autoForwardedFrom"] != tt.wantFrom || createGroup["snapshotId"] != tt.wantID {
				t.Errorf("snapshotService.create = %v, want autoForwardedFrom=%q, snapshotId=%q", createGroup, tt.wantFrom, tt.wantID)
			}
		})
	}
}

func TestCreateWhenMergedIntoTargetDeletedClearsArchiveAndRestoresSnapshot(t *testing.T) {
	store := newFakeSnapshotStore()
	snap := CharacterSnapshot{
		UserID:      "user-1",
		CharacterID: "char-1",
		Loadout:     sampleLoadout(),
	}
	hash, err := generateHash(snap)
	if err != nil {
		t.Fatalf("generateHash() error = %v", err)
	}

	deletedTargetID := "deleted-target-snap"
	now := time.Now()
	archived := CharacterSnapshot{
		ID:           "archived-snap-1",
		UserID:       "user-1",
		CharacterID:  "char-1",
		Hash:         hash,
		MergedIntoID: &deletedTargetID,
		MergedAt:     &now,
		Loadout:      sampleLoadout(),
	}
	store.byHash[hash] = &archived
	store.docs[archived.ID] = &archived
	// deletedTargetID is not in store.docs, so getDoc returns ErrSnapshotNotFound

	var output bytes.Buffer
	ctx := testLoggingContext(&output)
	var gotID *string
	runErr := clog.Run(ctx, "server-tick", func(ctx context.Context) error {
		var err error
		gotID, err = createWithStore(ctx, store, "user-1", snap)
		return err
	})
	if runErr != nil {
		t.Fatalf("createWithStore() error = %v, want nil", runErr)
	}
	if gotID == nil || *gotID != "archived-snap-1" {
		t.Fatalf("createWithStore() id = %v, want archived-snap-1", gotID)
	}
	if len(store.clearedArchives) != 1 || store.clearedArchives[0] != "archived-snap-1" {
		t.Errorf("clearedArchives = %v, want [archived-snap-1]", store.clearedArchives)
	}
	if len(store.historiesWritten) != 1 || store.historiesWritten[0].ID != "archived-snap-1" {
		t.Errorf("historiesWritten = %v, want 1 entry on restored archived-snap-1", store.historiesWritten)
	}
	event := decodeEvent(t, &output)
	if event["severity"] != "WARNING" {
		t.Errorf("event severity = %v, want WARNING when merge target was deleted", event["severity"])
	}
}

func TestFindBestFitWhenMergeTargetDeletedUnarchivesAndKeepsSourceSnapshot(t *testing.T) {
	store := newFakeSnapshotStore()
	now := time.Now()
	deletedTargetID := "deleted-target-snap"

	archivedParent := CharacterSnapshot{
		ID:           "source-snap-1",
		UserID:       "user-1",
		CharacterID:  "char-1",
		MergedIntoID: &deletedTargetID,
		Loadout:      sampleLoadout(),
	}
	store.docs[archivedParent.ID] = &archivedParent
	store.recentHistories = []History{
		{
			ID:          "hist-1",
			UserID:      "user-1",
			CharacterID: "char-1",
			ParentID:    archivedParent.ID,
			Timestamp:   now.Add(-5 * time.Minute),
			Meta:        MetaData{KineticID: "111", EnergyID: "222"},
		},
	}

	kRef := int64(111)
	eRef := int64(222)
	weapons := map[string]WeaponInstanceMetrics{
		"111": {ReferenceID: &kRef},
		"222": {ReferenceID: &eRef},
	}

	gotSnap, gotLink, err := findBestFitWithStore(context.Background(), store, "user-1", "char-1", now, weapons)
	if err != nil {
		t.Fatalf("findBestFitWithStore() error = %v, want nil", err)
	}
	if gotSnap == nil || gotSnap.ID != archivedParent.ID {
		t.Fatalf("gotSnap = %v, want restored source-snap-1", gotSnap)
	}
	if gotLink == nil || gotLink.SnapshotID == nil || *gotLink.SnapshotID != archivedParent.ID {
		t.Errorf("gotLink.SnapshotID = %v, want %q", gotLink, archivedParent.ID)
	}
	if gotLink.OriginalSnapshotID != nil {
		t.Errorf("gotLink.OriginalSnapshotID = %v, want nil when target was deleted", gotLink.OriginalSnapshotID)
	}
	if len(store.clearedArchives) != 1 || store.clearedArchives[0] != archivedParent.ID {
		t.Errorf("clearedArchives = %v, want [%s]", store.clearedArchives, archivedParent.ID)
	}
}

func TestFindBestFitResolvesArchivedParentToTargetSnapshot(t *testing.T) {
	store := newFakeSnapshotStore()
	now := time.Now()
	targetID := "target-snap-99"

	archivedParent := CharacterSnapshot{
		ID:           "source-snap-1",
		UserID:       "user-1",
		CharacterID:  "char-1",
		MergedIntoID: &targetID,
		Loadout:      sampleLoadout(),
	}
	targetLoadout := sampleLoadout()
	targetLoadout[strconv.Itoa(Kinetic)] = ItemSnapshot{
		InstanceID: "inst-kinetic-target",
		ItemHash:   111,
		Name:       "Target Kinetic",
		ItemProperties: ItemProperties{
			BaseInfo: BaseItemInfo{InstanceId: "inst-kinetic-target", Name: "Target Kinetic"},
		},
	}
	targetSnap := CharacterSnapshot{
		ID:          targetID,
		UserID:      "user-1",
		CharacterID: "char-1",
		Name:        "Target Loadout",
		Loadout:     targetLoadout,
	}

	store.docs[archivedParent.ID] = &archivedParent
	store.docs[targetSnap.ID] = &targetSnap
	store.recentHistories = []History{
		{
			ID:          "hist-1",
			UserID:      "user-1",
			CharacterID: "char-1",
			ParentID:    archivedParent.ID,
			Timestamp:   now.Add(-5 * time.Minute),
			Meta: MetaData{
				KineticID: "111",
				EnergyID:  "222",
				PowerID:   "333",
			},
		},
	}

	kRef := int64(111)
	eRef := int64(222)
	weapons := map[string]WeaponInstanceMetrics{
		"111": {ReferenceID: &kRef},
		"222": {ReferenceID: &eRef},
	}

	gotSnap, gotLink, err := findBestFitWithStore(context.Background(), store, "user-1", "char-1", now, weapons)
	if err != nil {
		t.Fatalf("findBestFitWithStore() error = %v, want nil", err)
	}
	if gotSnap == nil || gotSnap.ID != targetID {
		t.Fatalf("gotSnap = %v, want targetSnap with ID %q", gotSnap, targetID)
	}
	if gotLink == nil {
		t.Fatalf("gotLink = nil, want non-nil")
	}
	if gotLink.SnapshotID == nil || *gotLink.SnapshotID != targetID {
		t.Errorf("link.SnapshotID = %v, want %q", gotLink.SnapshotID, targetID)
	}
	if gotLink.OriginalSnapshotID == nil || *gotLink.OriginalSnapshotID != archivedParent.ID {
		t.Errorf("link.OriginalSnapshotID = %v, want %q", gotLink.OriginalSnapshotID, archivedParent.ID)
	}
	if gotLink.ConfidenceSource != SystemConfidenceSource {
		t.Errorf("link.ConfidenceSource = %q, want %q", gotLink.ConfidenceSource, SystemConfidenceSource)
	}
	if gotLink.ConfidenceLevel != HighConfidenceLevel {
		t.Errorf("link.ConfidenceLevel = %q, want %q", gotLink.ConfidenceLevel, HighConfidenceLevel)
	}

	// Verify EnrichInstancePerformance uses targetSnap's item properties
	enriched, err := EnrichInstancePerformance(gotSnap, InstancePerformance{Weapons: weapons})
	if err != nil {
		t.Fatalf("EnrichInstancePerformance() error = %v", err)
	}
	if enriched.Weapons["111"].ItemProperties == nil || enriched.Weapons["111"].ItemProperties.BaseInfo.InstanceId != "inst-kinetic-target" {
		t.Errorf("enriched weapon 111 = %+v, want targetSnap item properties", enriched.Weapons["111"])
	}
}

func TestFindBestFitWhenParentHardDeletedDoesNotCrashSessionTick(t *testing.T) {
	store := newFakeSnapshotStore()
	now := time.Now()

	// History references a parent snapshot that was hard-deleted (not in store.docs)
	store.recentHistories = []History{
		{
			ID:          "hist-deleted",
			UserID:      "user-1",
			CharacterID: "char-1",
			ParentID:    "hard-deleted-snap",
			Timestamp:   now.Add(-5 * time.Minute),
			Meta: MetaData{
				KineticID: "111",
				EnergyID:  "222",
				PowerID:   "333",
			},
		},
	}

	kRef := int64(111)
	weapons := map[string]WeaponInstanceMetrics{
		"111": {ReferenceID: &kRef},
	}

	gotSnap, gotLink, err := findBestFitWithStore(context.Background(), store, "user-1", "char-1", now, weapons)
	if err != nil {
		t.Fatalf("findBestFitWithStore() error = %v, want nil (graceful handling of ErrSnapshotNotFound)", err)
	}
	if gotSnap != nil {
		t.Errorf("gotSnap = %v, want nil when parent snapshot was hard-deleted", gotSnap)
	}
	if gotLink == nil {
		t.Fatalf("gotLink = nil, want non-nil NotFoundConfidenceLevel link")
	}
	if gotLink.ConfidenceLevel != NotFoundConfidenceLevel {
		t.Errorf("gotLink.ConfidenceLevel = %q, want %q", gotLink.ConfidenceLevel, NotFoundConfidenceLevel)
	}
	if gotLink.SnapshotID != nil {
		t.Errorf("gotLink.SnapshotID = %v, want nil", gotLink.SnapshotID)
	}
}

func TestFindBestFitPropagatesNonNotFoundFirestoreErrors(t *testing.T) {
	store := newFakeSnapshotStore()
	now := time.Now()
	transientErr := errors.New("firestore unavailable")
	store.getDocErr["snap-1"] = transientErr
	store.recentHistories = []History{
		{
			ID:          "hist-1",
			UserID:      "user-1",
			CharacterID: "char-1",
			ParentID:    "snap-1",
			Timestamp:   now.Add(-5 * time.Minute),
			Meta:        MetaData{KineticID: "111"},
		},
	}
	kRef := int64(111)
	weapons := map[string]WeaponInstanceMetrics{
		"111": {ReferenceID: &kRef},
	}

	_, _, err := findBestFitWithStore(context.Background(), store, "user-1", "char-1", now, weapons)
	if !errors.Is(err, transientErr) {
		t.Fatalf("findBestFitWithStore() error = %v, want %v", err, transientErr)
	}
}

func TestFirestoreEmulatorAutoForwardAndFindBestFit(t *testing.T) {
	emulatorHost := "127.0.0.1:8081"
	t.Setenv("FIRESTORE_EMULATOR_HOST", emulatorHost)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := firestore.NewClient(ctx, "server-tick-emulator-test")
	if err != nil {
		t.Skipf("skipping emulator integration test (failed to create client): %v", err)
	}
	defer db.Close()

	// Probe emulator connectivity
	if _, err := db.Collection(snapshotCollection).Limit(1).Documents(ctx).GetAll(); err != nil {
		t.Skipf("skipping emulator integration test (emulator not reachable at %s): %v", emulatorHost, err)
	}

	userID := "emu-test-user-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	charID := "emu-test-char-1"
	now := time.Now()

	loadout := sampleLoadout()
	hash, err := generateHash(CharacterSnapshot{Loadout: loadout})
	if err != nil {
		t.Fatalf("generateHash() error = %v", err)
	}

	targetSnapRef := db.Collection(snapshotCollection).NewDoc()
	sourceSnapRef := db.Collection(snapshotCollection).NewDoc()
	deletedRefSnapRef := db.Collection(snapshotCollection).NewDoc()
	defer func() {
		_, _ = targetSnapRef.Delete(context.Background())
		_, _ = sourceSnapRef.Delete(context.Background())
		_, _ = deletedRefSnapRef.Delete(context.Background())
	}()

	targetLoadout := sampleLoadout()
	targetLoadout[strconv.Itoa(Kinetic)] = ItemSnapshot{
		InstanceID: "inst-kinetic-canonical",
		ItemHash:   111,
		Name:       "Canonical Kinetic",
		ItemProperties: ItemProperties{
			BaseInfo: BaseItemInfo{InstanceId: "inst-kinetic-canonical", Name: "Canonical Kinetic"},
		},
	}

	targetSnap := CharacterSnapshot{
		ID:          targetSnapRef.ID,
		UserID:      userID,
		CharacterID: charID,
		Name:        "Canonical Kept Loadout",
		Hash:        "target-hash-" + targetSnapRef.ID,
		CreatedAt:   now.Add(-20 * time.Minute),
		UpdatedAt:   now.Add(-20 * time.Minute),
		Loadout:     targetLoadout,
	}
	mergedAt := now.Add(-2 * time.Minute)
	sourceSnap := CharacterSnapshot{
		ID:           sourceSnapRef.ID,
		UserID:       userID,
		CharacterID:  charID,
		Name:         "Archived Source Loadout",
		Hash:         hash,
		MergedIntoID: &targetSnap.ID,
		MergedAt:     &mergedAt,
		CreatedAt:    now.Add(-30 * time.Minute),
		UpdatedAt:    now.Add(-10 * time.Minute),
		Loadout:      loadout,
	}

	if _, err := targetSnapRef.Set(ctx, targetSnap); err != nil {
		t.Fatalf("failed to seed targetSnap: %v", err)
	}
	if _, err := sourceSnapRef.Set(ctx, sourceSnap); err != nil {
		t.Fatalf("failed to seed sourceSnap: %v", err)
	}

	// Seed a <1h history entry under sourceSnap (simulating mid-session merge)
	preMergeHistRef := sourceSnapRef.Collection(historyCollection).NewDoc()
	defer func() { _, _ = preMergeHistRef.Delete(context.Background()) }()
	preMergeHist := History{
		ID:          preMergeHistRef.ID,
		ParentID:    sourceSnap.ID,
		UserID:      userID,
		CharacterID: charID,
		Timestamp:   now.Add(-5 * time.Minute),
		Meta: MetaData{
			KineticID: "111",
			EnergyID:  "222",
			PowerID:   "333",
		},
	}
	if _, err := preMergeHistRef.Set(ctx, preMergeHist); err != nil {
		t.Fatalf("failed to seed pre-merge history: %v", err)
	}

	// 1. Verify FindBestFit resolves archived sourceSnap -> targetSnap and sets OriginalSnapshotID
	kRef := int64(111)
	eRef := int64(222)
	weapons := map[string]WeaponInstanceMetrics{
		"111": {ReferenceID: &kRef},
		"222": {ReferenceID: &eRef},
	}
	bestSnap, link, err := FindBestFit(ctx, db, userID, charID, now, weapons)
	if err != nil {
		t.Fatalf("FindBestFit() error = %v", err)
	}
	if bestSnap == nil || bestSnap.ID != targetSnap.ID {
		t.Fatalf("FindBestFit() snap = %v, want targetSnap %s", bestSnap, targetSnap.ID)
	}
	if link == nil || link.SnapshotID == nil || *link.SnapshotID != targetSnap.ID {
		t.Fatalf("FindBestFit() link.SnapshotID = %v, want %s", link, targetSnap.ID)
	}
	if link.OriginalSnapshotID == nil || *link.OriginalSnapshotID != sourceSnap.ID {
		t.Fatalf("FindBestFit() link.OriginalSnapshotID = %v, want %s", link.OriginalSnapshotID, sourceSnap.ID)
	}

	// Verify SetAggregate persists targetSnap.ID into snapshotIds and originalSnapshotId on snapshotLinks
	agg, err := SetAggregate(ctx, db, userID, charID, ActivityHistory{
		InstanceID: "emu-act-" + targetSnap.ID,
		Period:     now,
	}, now, InstancePerformance{Weapons: weapons}, "emu-session-1")
	if err != nil {
		t.Fatalf("SetAggregate() error = %v", err)
	}
	defer func() {
		_, _ = db.Collection(aggregateCollection).Doc(agg.ID).Delete(context.Background())
	}()
	persistedAggDoc, err := db.Collection(aggregateCollection).Doc(agg.ID).Get(ctx)
	if err != nil {
		t.Fatalf("failed to fetch persisted aggregate: %v", err)
	}
	var persistedAgg Aggregate
	if err := persistedAggDoc.DataTo(&persistedAgg); err != nil {
		t.Fatalf("failed to decode persisted aggregate: %v", err)
	}
	if len(persistedAgg.SnapshotIDs) != 1 || persistedAgg.SnapshotIDs[0] != targetSnap.ID {
		t.Errorf("persistedAgg.SnapshotIDs = %v, want [%s]", persistedAgg.SnapshotIDs, targetSnap.ID)
	}
	persistedLink := persistedAgg.SnapshotLinks[charID]
	if persistedLink.SnapshotID == nil || *persistedLink.SnapshotID != targetSnap.ID {
		t.Errorf("persistedLink.SnapshotID = %v, want %s", persistedLink.SnapshotID, targetSnap.ID)
	}
	if persistedLink.OriginalSnapshotID == nil || *persistedLink.OriginalSnapshotID != sourceSnap.ID {
		t.Errorf("persistedLink.OriginalSnapshotID = %v, want %s", persistedLink.OriginalSnapshotID, sourceSnap.ID)
	}
	if persistedAgg.Performance[charID].Weapons["111"].ItemProperties == nil ||
		persistedAgg.Performance[charID].Weapons["111"].ItemProperties.BaseInfo.InstanceId != "inst-kinetic-canonical" {
		t.Errorf("persistedAgg enriched weapon = %+v, want canonical targetSnap item properties", persistedAgg.Performance[charID].Weapons["111"])
	}

	// 2. Verify create() with matching hash auto-forwards history write to targetSnap in Firestore
	gotID, err := create(ctx, db, userID, CharacterSnapshot{
		UserID:      userID,
		CharacterID: charID,
		Loadout:     loadout,
	})
	if err != nil {
		t.Fatalf("create() error = %v", err)
	}
	if gotID == nil || *gotID != targetSnap.ID {
		t.Fatalf("create() returned ID = %v, want %s", gotID, targetSnap.ID)
	}
	targetHistDocs, err := targetSnapRef.Collection(historyCollection).Documents(ctx).GetAll()
	if err != nil {
		t.Fatalf("failed to read targetSnap histories: %v", err)
	}
	defer func() {
		for _, d := range targetHistDocs {
			_, _ = d.Ref.Delete(context.Background())
		}
	}()
	if len(targetHistDocs) != 1 {
		t.Fatalf("targetSnap histories count = %d, want 1", len(targetHistDocs))
	}

	// 3. Verify deleted merge target fallback clears mergedIntoId & mergedAt in Firestore
	nonExistentTargetID := "missing-target-doc-id"
	deletedTargetSource := CharacterSnapshot{
		ID:           deletedRefSnapRef.ID,
		UserID:       userID,
		CharacterID:  charID,
		Name:         "Orphaned Archived Loadout",
		Hash:         "orphan-hash-" + deletedRefSnapRef.ID,
		MergedIntoID: &nonExistentTargetID,
		MergedAt:     &mergedAt,
		CreatedAt:    now.Add(-15 * time.Minute),
		UpdatedAt:    now.Add(-15 * time.Minute),
		Loadout:      loadout,
	}
	if _, err := deletedRefSnapRef.Set(ctx, deletedTargetSource); err != nil {
		t.Fatalf("failed to seed deletedTargetSource: %v", err)
	}
	restoredID, err := create(ctx, db, userID, CharacterSnapshot{
		UserID:      userID,
		CharacterID: charID,
		Hash:        deletedTargetSource.Hash,
		Loadout:     loadout,
	})
	if err != nil {
		t.Fatalf("create() with deleted target error = %v", err)
	}
	if restoredID == nil || *restoredID != deletedRefSnapRef.ID {
		t.Fatalf("create() restoredID = %v, want %s", restoredID, deletedRefSnapRef.ID)
	}
	restoredSnap, err := Get(ctx, db, deletedRefSnapRef.ID)
	if err != nil {
		t.Fatalf("Get() restored snapshot error = %v", err)
	}
	if restoredSnap.MergedIntoID != nil || restoredSnap.MergedAt != nil {
		t.Errorf("restoredSnap MergedIntoID=%v, MergedAt=%v; want both nil (deleted from Firestore)", restoredSnap.MergedIntoID, restoredSnap.MergedAt)
	}
	orphanHistDocs, _ := deletedRefSnapRef.Collection(historyCollection).Documents(ctx).GetAll()
	for _, d := range orphanHistDocs {
		_, _ = d.Ref.Delete(context.Background())
	}
}

