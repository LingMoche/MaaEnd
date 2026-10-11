package giftoperator

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCountTargetInitResetsInventoryAndKeepsRecipientCountIndependent(t *testing.T) {
	store := newSessionStore()
	attach := store.nodes[sessionNode]["attach"].(map[string]any)
	attach["target_rings"], attach["target_count"] = 1, 5
	count := 3
	if err := updateSession(store, sessionActionParam{Operation: "init", Count: &count}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := loadSession(store)
	if err != nil || s.TargetCount != 5 || s.TargetRings != 1 || s.Remaining != 3 {
		t.Fatalf("quantity option overwrote recipients or ring target: %+v, %v", s, err)
	}
	s.SelectedCount, s.ExpectedInventory = 5, []int{0, 4, 4}
	s.Pending, s.Prepared = "Perlica", true
	if err := saveSession(store, s); err != nil {
		t.Fatal(err)
	}
	store.nodes[sessionNode]["attach"].(map[string]any)["target_count"] = 0
	if err := initSession(store, 2); err != nil {
		t.Fatal(err)
	}
	s, err = loadSession(store)
	if err != nil || s.TargetCount != 0 || s.SelectedCount != 0 || s.ExpectedInventory != nil || s.Pending != "" || s.Prepared {
		t.Fatalf("switching to ring mode retained quantity evidence: %+v, %v", s, err)
	}
	s.ExpectedInventory, s.SelectedCount = []int{1, 2}, 2
	if err := s.reserve("Yvonne"); err != nil || s.ExpectedInventory != nil || s.SelectedCount != 0 {
		t.Fatalf("next recipient retained inventory evidence: %+v, %v", s, err)
	}
	for _, invalid := range []any{-1, "2", 1.5} {
		store := newSessionStore()
		store.nodes[sessionNode]["attach"].(map[string]any)["target_count"] = invalid
		if err := initSession(store, 1); err == nil {
			t.Fatalf("accepted invalid gift quantity target: %v", invalid)
		}
	}
}

func TestCountBeforeIgnoresRingTargetButStillRejectsActualCaps(t *testing.T) {
	for _, test := range []struct {
		trust, rings int
		prepared     bool
	}{{101, 1, true}, {101, 2, true}, {101, 3, false}, {200, 0, false}} {
		s := session{TargetRings: 1, TargetCount: 2, Remaining: 1, Pending: "Perlica", ExpectedInventory: []int{2}}
		if err := s.observeBefore(test.trust, test.rings); err != nil || s.Prepared != test.prepared || s.ExpectedInventory != nil {
			t.Fatalf("quantity eligibility used ring option or stale inventory: %+v, %v", s, err)
		}
		if !test.prepared {
			s.SelectedCount, s.ExpectedInventory = 2, []int{2}
			if ok, err := s.commitCountAfter(200, 3, []int{2}); err == nil || ok || s.Remaining != 1 {
				t.Fatal("already-capped recipient consumed quantity count")
			}
		}
	}
}

func TestCountTargetNeedsStockConsumptionWithoutTrustOrRingGain(t *testing.T) {
	s := preparedCountSession(t, 2)
	store := newSessionStore()
	input := []int{10, 2, 10}
	if err := applyGiftObservation(store, &s, "observe_selection", countPreview(101, 1, 2, nil)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.commitCountAfter(101, 1, []int{2, 10, 10}); err == nil || ok || s.Remaining != 2 {
		t.Fatal("target preview without frozen inventory confirmed consumption")
	}
	if err := applyGiftObservation(store, &s, "freeze_selection", countPreview(101, 1, 2, input)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, []int{10, 2, 10}) || !reflect.DeepEqual(s.ExpectedInventory, []int{2, 10, 10}) {
		t.Fatal("quantity evidence was not copied and sorted without changing input")
	}
	input[0] = 999
	if !reflect.DeepEqual(s.ExpectedInventory, []int{2, 10, 10}) {
		t.Fatal("preview input remained aliased to persisted inventory")
	}
	if store.nodes["GiftOperatorSendSelectionReady"]["enabled"] != true || store.nodes["GiftOperatorSendSelectionMore"]["enabled"] != false {
		t.Fatal("exact target did not enable confirmation")
	}
	if ok, err := s.commitAfter(101, 3); err == nil || ok || s.Remaining != 2 {
		t.Fatal("ring commit bypassed quantity inventory evidence")
	}
	actual := []int{10, 10, 2}
	if ok, err := s.commitCountAfter(101, 1, actual); err != nil || !ok || s.Remaining != 1 || !containsOperator(s.Completed, "Perlica") {
		t.Fatalf("actual consumption required integer trust/ring gain: %+v, %v", s, err)
	}
	if !reflect.DeepEqual(actual, []int{10, 10, 2}) {
		t.Fatal("verification sorted caller-owned inventory in place")
	}
	if ok, err := s.commitCountAfter(101, 1, actual); err != nil || ok || s.Remaining != 1 {
		t.Fatal("duplicate inventory confirmation counted twice")
	}
}

func TestCountInventoryMustMatchEveryVisibleQuantityAndMultiplicity(t *testing.T) {
	for _, inventory := range [][]int{nil, {}, {2, 10}, {2, 9, 11}, {2, 10, 10, 0}, {-1, 10, 13}} {
		s := preparedCountSession(t, 2)
		s.SelectedCount, s.ExpectedInventory = 2, []int{2, 10, 10}
		before := cloneCountSession(t, s)
		if ok, err := s.commitCountAfter(101, 1, inventory); err == nil || ok || !reflect.DeepEqual(s, before) {
			t.Fatalf("partial/wrong inventory counted or changed state: %v, %+v, %v", inventory, s, err)
		}
	}
	s := preparedCountSession(t, 2)
	s.SelectedCount, s.ExpectedInventory = 2, []int{2, 10, 10}
	for _, after := range []struct{ trust, rings int }{{100, 1}, {101, 0}} {
		before := cloneCountSession(t, s)
		if ok, err := s.commitCountAfter(after.trust, after.rings, []int{2, 10, 10}); err == nil || ok || !reflect.DeepEqual(s, before) {
			t.Fatal("inventory match bypassed decreasing actual progress")
		}
	}
}

func TestCountPartialSelectionNeedsRealCapAndMatchingInventory(t *testing.T) {
	for _, cap := range []struct{ trust, rings int }{{200, 1}, {101, 3}} {
		s := preparedCountSession(t, 3)
		preview := countPreview(cap.trust, cap.rings, 1, nil)
		if err := applyGiftObservation(newSessionStore(), &s, "observe_selection", preview); err != nil {
			t.Fatal(err)
		}
		preview.ExpectedInventory = []int{8, 10}
		if err := applyGiftObservation(newSessionStore(), &s, "freeze_selection", preview); err != nil {
			t.Fatal(err)
		}
		before := cloneCountSession(t, s)
		preview.Inventory = []int{8, 10}
		if err := applyGiftObservation(newSessionStore(), &s, "observe_after", preview); err == nil || !reflect.DeepEqual(s, before) {
			t.Fatal("preview cap confirmed delivery")
		}
		if ok, err := s.commitCountAfter(101, 1, []int{8, 10}); err == nil || ok || !reflect.DeepEqual(s, before) {
			t.Fatal("only preview cap allowed below-target actual delivery")
		}
		after := observation(cap.trust, cap.rings)
		after.Inventory = []int{10, 8}
		if cap.trust == 200 {
			after.CompletedRings = nil // Max total trust may hide its tier label.
		}
		if err := applyGiftObservation(newSessionStore(), &s, "observe_after", after); err != nil || s.Remaining != 1 {
			t.Fatalf("real cap with consumption did not count partial quantity: %+v, %v", s, err)
		}
	}
}

func TestCountCapCannotBypassPositiveSelectionOrCompleteInventory(t *testing.T) {
	for _, test := range []struct {
		selected         int
		expected, actual []int
	}{{0, []int{4}, []int{4}}, {1, nil, []int{4}}, {1, []int{4}, nil}, {1, []int{4}, []int{5}}, {4, []int{4}, []int{4}}} {
		s := preparedCountSession(t, 3)
		s.SelectedCount, s.ExpectedInventory = test.selected, test.expected
		before := cloneCountSession(t, s)
		if ok, err := s.commitCountAfter(200, 3, test.actual); err == nil || ok || !reflect.DeepEqual(s, before) {
			t.Fatalf("cap bypassed quantity/inventory proof: %+v, %v", test, err)
		}
	}
}

func TestCountSelectionInvalidEvidenceDoesNotChangeQuantityOrReadiness(t *testing.T) {
	s := preparedCountSession(t, 3)
	store := newSessionStore()
	if err := applyGiftObservation(store, &s, "observe_selection", countPreview(101, 1, 1, nil)); err != nil {
		t.Fatal(err)
	}
	before := cloneCountSession(t, s)
	flags, _ := json.Marshal(store.nodes)
	for _, preview := range []selectionStatus{
		countPreview(101, 1, 1, nil),
		countPreview(200, 3, 4, nil),
		countPreview(101, 1, 2, []int{}),
		countPreview(101, 1, 2, []int{7, 10}),
	} {
		if err := applyGiftObservation(store, &s, "observe_selection", preview); err == nil || !reflect.DeepEqual(s, before) {
			t.Fatal("invalid preview changed selected count or inventory")
		}
		current, _ := json.Marshal(store.nodes)
		if string(current) != string(flags) {
			t.Fatal("invalid preview changed confirmation flags")
		}
	}
}

func TestCountConfirmationKeepsUnknownRecipientIdentityGuards(t *testing.T) {
	s := preparedCountSession(t, 2)
	s.Generic, s.Pending = true, "portrait-1"
	s.Portraits = []portraitEntry{{Operator: "portrait-1", Name: "新干员"}}
	s.SelectedCount, s.ExpectedInventory = 2, []int{8, 10}
	after := observation(101, 1)
	after.Inventory, after.Name = []int{10, 8}, "新干员"
	before := cloneCountSession(t, s)
	after.IdentityMatch = false
	if err := applyGiftObservation(newSessionStore(), &s, "observe_after", after); err == nil || !reflect.DeepEqual(s, before) {
		t.Fatal("wrong portrait consumed quantity recipient")
	}
	after.IdentityMatch, after.Name = true, "另一人"
	if err := applyGiftObservation(newSessionStore(), &s, "observe_after", after); err == nil || !reflect.DeepEqual(s, before) {
		t.Fatal("changed UI name consumed quantity recipient")
	}
	after.Name = "新干员"
	if err := applyGiftObservation(newSessionStore(), &s, "observe_after", after); err != nil || s.Remaining != 1 {
		t.Fatalf("same verified identity and inventory was rejected: %+v, %v", s, err)
	}
}

func TestCountFreezeRefreshesLastInventoryWithoutSelectingAgain(t *testing.T) {
	s := preparedCountSession(t, 2)
	store := newSessionStore()
	if err := applyGiftObservation(store, &s, "freeze_selection", countPreview(101, 1, 2, []int{9, 10})); err != nil {
		t.Fatal(err)
	}
	fresh := countPreview(101, 1, 2, []int{10, 8})
	if err := applyGiftObservation(store, &s, "observe_selection", countPreview(101, 1, 2, nil)); err == nil {
		t.Fatal("normal selection accepted equal count")
	}
	if err := applyGiftObservation(store, &s, "freeze_selection", fresh); err != nil || !reflect.DeepEqual(s.ExpectedInventory, []int{8, 10}) || s.Remaining != 2 {
		t.Fatalf("final same-count frame did not refresh inventory: %+v, %v", s, err)
	}
	if store.nodes["GiftOperatorSendSelectionReady"]["enabled"] != true || store.nodes["GiftOperatorSendSelectionMore"]["enabled"] != false {
		t.Fatal("freeze did not lock confirmation readiness")
	}
	after := observation(101, 1)
	after.Inventory = []int{10, 8}
	if err := applyGiftObservation(store, &s, "observe_after", after); err != nil || s.Remaining != 1 {
		t.Fatalf("freshly frozen inventory consumption did not count: %+v, %v", s, err)
	}
}

func TestCountOrdinaryGrowthClearsInventoryAndOnlyFreezeRequiresIt(t *testing.T) {
	s := preparedCountSession(t, 3)
	s.SelectedCount, s.ExpectedInventory = 1, []int{8, 10}
	store := newSessionStore()
	if err := applyGiftObservation(store, &s, "observe_selection", countPreview(101, 1, 2, nil)); err != nil || s.ExpectedInventory != nil {
		t.Fatalf("ordinary growing preview required or retained inventory: %+v, %v", s, err)
	}
	before := cloneCountSession(t, s)
	flags, _ := json.Marshal(store.nodes)
	for _, inventory := range [][]int{nil, {}, {-1, 10}} {
		if err := applyGiftObservation(store, &s, "freeze_selection", countPreview(101, 1, 3, inventory)); err == nil || !reflect.DeepEqual(s, before) {
			t.Fatal("final freeze accepted missing/corrupt inventory or changed state")
		}
		current, _ := json.Marshal(store.nodes)
		if string(current) != string(flags) {
			t.Fatal("invalid freeze changed confirmation flags")
		}
	}
	if err := applyGiftObservation(store, &s, "freeze_selection", countPreview(101, 1, 3, []int{10, 7})); err != nil || !reflect.DeepEqual(s.ExpectedInventory, []int{7, 10}) {
		t.Fatalf("valid final freeze rejected complete inventory: %+v, %v", s, err)
	}
}

func TestFreezeRequiresPositiveNondecreasingReadyQuantity(t *testing.T) {
	for _, test := range []struct{ count, trust, rings int }{{0, 200, 3}, {1, 101, 1}, {1, 200, 3}, {4, 200, 3}} {
		s := preparedCountSession(t, 3)
		s.SelectedCount, s.ExpectedInventory = 2, []int{8, 10}
		before := cloneCountSession(t, s)
		preview := countPreview(test.trust, test.rings, test.count, []int{7, 10})
		if err := applyGiftObservation(newSessionStore(), &s, "freeze_selection", preview); err == nil || !reflect.DeepEqual(s, before) {
			t.Fatalf("unsafe freeze changed selection: %+v, %v", test, err)
		}
	}
	s := preparedCountSession(t, 3)
	s.SelectedCount, s.ExpectedInventory = 1, []int{8, 10}
	before := cloneCountSession(t, s)
	if err := applyGiftObservation(newSessionStore(), &s, "freeze_selection", countPreview(101, 1, 2, []int{7, 10})); err == nil || !reflect.DeepEqual(s, before) {
		t.Fatal("below-target freeze ignored count mode in favor of ring target")
	}
}

func TestRingFreezeDoesNotRequireQuantityInventory(t *testing.T) {
	s := session{TargetRings: 1, Remaining: 1, Pending: "Perlica", Prepared: true, BeforeTrust: 101, SelectedCount: 28}
	preview := countPreview(101, 1, 28, nil)
	if err := applyGiftObservation(newSessionStore(), &s, "freeze_selection", preview); err != nil || s.SelectedCount != 28 || s.Remaining != 1 {
		t.Fatalf("ring mode freeze incorrectly required count evidence: %+v, %v", s, err)
	}
}

func TestCountEvidencePersistsAcrossCallbacksAndStopsAtZero(t *testing.T) {
	store := newSessionStore()
	store.nodes[sessionNode]["attach"].(map[string]any)["target_count"] = 2
	if err := initSession(store, 1); err != nil {
		t.Fatal(err)
	}
	s, err := loadSession(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.reserve("Perlica"); err != nil {
		t.Fatal(err)
	}
	if err := s.observeBefore(101, 0); err != nil {
		t.Fatal(err)
	}
	if err := applyGiftObservation(store, &s, "freeze_selection", countPreview(101, 0, 2, []int{10, 8})); err != nil {
		t.Fatal(err)
	}
	if err := saveSession(store, s); err != nil {
		t.Fatal(err)
	}
	s, err = loadSession(store)
	if err != nil || s.TargetCount != 2 || s.SelectedCount != 2 || !reflect.DeepEqual(s.ExpectedInventory, []int{8, 10}) {
		t.Fatalf("frozen inventory did not survive the next callback: %+v, %v", s, err)
	}
	after := observation(101, 0)
	after.Inventory = []int{10, 8}
	if err := applyGiftObservation(store, &s, "observe_after", after); err != nil {
		t.Fatal(err)
	}
	if err := saveSession(store, s); err != nil {
		t.Fatal(err)
	}
	if store.nodes["GiftOperatorSendContinue"]["enabled"] != false || s.Remaining != 0 {
		t.Fatal("quantity success failed to terminate recipient loop")
	}
	if err := applyGiftObservation(store, &s, "observe_after", after); err != nil || s.Remaining != 0 || len(s.Completed) != 1 {
		t.Fatal("duplicate quantity confirmation changed persisted remaining recipients")
	}
	if err := s.reserve("Yvonne"); err == nil {
		t.Fatal("reserved after recipient goal completed")
	}
	if err := updateSession(store, sessionActionParam{Operation: "finish"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsCorruptCountInventoryState(t *testing.T) {
	for _, test := range []struct {
		target    int
		inventory []int
	}{{-1, nil}, {2, []int{-1, 1}}, {2, []int{2, 1}}} {
		store := newSessionStore()
		s := session{TargetRings: 3, TargetCount: test.target, Remaining: 1, ExpectedInventory: test.inventory}
		if err := saveSession(store, s); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSession(store); err == nil {
			t.Fatalf("loaded corrupt count evidence: %+v", test)
		}
	}
}

func preparedCountSession(t *testing.T, target int) session {
	t.Helper()
	s := session{TargetRings: 1, TargetCount: target, Remaining: 2, Pending: "Perlica"}
	if err := s.observeBefore(101, 1); err != nil || !s.Prepared {
		t.Fatalf("count session was not prepared: %+v, %v", s, err)
	}
	return s
}

func countPreview(trust, rings, count int, inventory []int) selectionStatus {
	status := observation(trust, rings)
	status.Preview, status.SelectedCount, status.ExpectedInventory = true, &count, inventory
	return status
}

func cloneCountSession(t *testing.T, s session) session {
	t.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var copy session
	if err := json.Unmarshal(data, &copy); err != nil {
		t.Fatal(err)
	}
	return copy
}
