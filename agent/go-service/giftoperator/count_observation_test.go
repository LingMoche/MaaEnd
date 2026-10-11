package giftoperator

import (
	"image"
	"reflect"
	"testing"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func TestCountSelectionReadsStockOnlyAtFinalSnapshot(t *testing.T) {
	runner := newCountObservationRunner()
	s := genericGiftStatusSession(t)
	s.TargetCount = 1
	img := inventoryTestImage()
	status, _, err := recognizeGiftSelectionFrame(runner, img, s, false)
	if err != nil || status == nil || *status.SelectedCount != 1 || status.ExpectedInventory != nil || runner.stock.calls != 0 {
		t.Fatalf("ordinary selection read inventory: status=%+v calls=%d error=%v", status, runner.stock.calls, err)
	}
	s.SelectedCount = 1
	if status, _, err := recognizeGiftSelectionFrame(runner, img, s, false); err != nil || status != nil {
		t.Fatalf("unchanged ordinary preview accepted: status=%+v error=%v", status, err)
	}
	status, _, err = recognizeGiftSelectionFrame(runner, img, s, true)
	want := []int{0, 0, 0, 0, 3, 592, 609, 676, 731, 931, 1476}
	if err != nil || status == nil || !reflect.DeepEqual(status.ExpectedInventory, want) || runner.stock.calls != 11 {
		t.Fatalf("final equal count lost debit snapshot: status=%+v calls=%d error=%v", status, runner.stock.calls, err)
	}
}

func TestFinalCountSnapshotRejectsDecreasedOrUnreadableSelection(t *testing.T) {
	for _, test := range []struct {
		name     string
		previous int
		badStock bool
	}{
		{"count decreased", 2, false},
		{"last stock unreadable", 1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := newCountObservationRunner()
			if test.badStock {
				runner.stock.texts[10] = "C"
			}
			s := genericGiftStatusSession(t)
			s.TargetCount, s.SelectedCount = 2, test.previous
			status, _, err := recognizeGiftSelectionFrame(runner, inventoryTestImage(), s, true)
			if err != nil || status != nil {
				t.Fatalf("unsafe final snapshot accepted: status=%+v error=%v", status, err)
			}
		})
	}
}

func TestCountActualStatusRequiresSameIdentityAndNoSelection(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*countObservationRunner)
	}{
		{"selected preview", func(r *countObservationRunner) {}},
		{"wrong name", func(r *countObservationRunner) { r.labels = nil; r.name = "另一人" }},
		{"wrong portrait", func(r *countObservationRunner) { r.labels = nil; r.portraitMatch = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := newCountObservationRunner()
			test.mutate(runner)
			s := genericGiftStatusSession(t)
			s.TargetCount = 1
			status, _, err := readGiftStatus(runner, inventoryTestImage(), s, true, false)
			if err != nil || status != nil || runner.stock.calls != 0 {
				t.Fatalf("unsafe actual frame accepted: status=%+v calls=%d error=%v", status, runner.stock.calls, err)
			}
		})
	}
}

func TestCountActualStockConfirmsOneGiftWithoutTrustOrRingChange(t *testing.T) {
	for _, test := range []struct {
		name   string
		stocks []int
		valid  bool
	}{
		{"inventory consumed and reordered", []int{676, 731, 592, 3, 0, 0, 0, 0, 1476, 931, 609}, true},
		{"inventory unchanged", []int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := newCountObservationRunner()
			s := genericGiftStatusSession(t)
			s.TargetCount, s.SelectedCount, s.BeforeTrust = 1, 1, 110
			preview, _, err := recognizeGiftSelectionFrame(runner, inventoryTestImage(), s, true)
			if err != nil || preview == nil {
				t.Fatalf("missing final snapshot: status=%+v error=%v", preview, err)
			}
			s.ExpectedInventory = preview.ExpectedInventory
			runner.labels = nil
			runner.stock = newInventoryRunner(test.stocks)
			status, _, err := readGiftStatus(runner, inventoryTestImage(), s, true, false)
			if err != nil || status == nil || *status.Trust != 110 || *status.CompletedRings != 0 {
				t.Fatalf("actual observation lost unchanged progress: status=%+v error=%v", status, err)
			}
			changed, err := s.commitCountAfter(*status.Trust, *status.CompletedRings, status.Inventory)
			if changed != test.valid || (err == nil) != test.valid {
				t.Fatalf("wrong completion outcome: changed=%v error=%v session=%+v", changed, err, s)
			}
			if !test.valid && (s.Remaining != 1 || len(s.Completed) != 0) {
				t.Fatalf("failed delivery consumed recipient: %+v", s)
			}
			if test.valid {
				changed, err = s.commitCountAfter(*status.Trust, *status.CompletedRings, status.Inventory)
				if err != nil || changed || s.Remaining != 0 || len(s.Completed) != 1 {
					t.Fatalf("delivery counted twice: changed=%v error=%v session=%+v", changed, err, s)
				}
			}
		})
	}
}

type countObservationRunner struct {
	*giftStatusRunner
	stock *inventoryRunner
}

func newCountObservationRunner() *countObservationRunner {
	r := &countObservationRunner{
		giftStatusRunner: newGiftStatusRunner("高", "110%"),
		stock:            newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609}),
	}
	r.labels = []maa.Rect{{132, 522, 68, 38}}
	r.quantities = map[maa.Rect]string{{163, 522, 37, 20}: "1"}
	return r
}

func (r *countObservationRunner) RunRecognition(entry string, img image.Image, overrides ...any) (*maa.RecognitionDetail, error) {
	if entry == "GiftOperatorGiftInventoryText" {
		return r.stock.RunRecognition(entry, img, overrides...)
	}
	return r.giftStatusRunner.RunRecognition(entry, img, overrides...)
}
