package giftoperator

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"reflect"
	"strconv"
	"testing"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func TestGiftInventoryReadsAllColumnsIncludingZeros(t *testing.T) {
	runner := newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609})
	stocks, hit, err := readGiftInventory(runner, inventoryTestImage())
	want := []int{0, 0, 0, 0, 3, 592, 609, 676, 732, 931, 1476}
	if err != nil || !hit || !reflect.DeepEqual(stocks, want) || runner.calls != 11 {
		t.Fatalf("inventory lost a column or zero: stocks=%v hit=%v calls=%d error=%v", stocks, hit, runner.calls, err)
	}
}

func TestExpectedGiftInventoryAssociatesBeforeSorting(t *testing.T) {
	runner := newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609})
	gifts := []selectedGift{{Box: maa.Rect{132, 522, 68, 38}, Count: 74}, {Box: maa.Rect{236, 522, 68, 38}, Count: 1}}
	stocks, hit, err := expectedGiftInventory(runner, inventoryTestImage(), gifts)
	want := []int{0, 0, 0, 0, 3, 592, 609, 658, 675, 931, 1476}
	if err != nil || !hit || !reflect.DeepEqual(stocks, want) {
		t.Fatalf("debit lost card association: stocks=%v hit=%v error=%v", stocks, hit, err)
	}
}

func TestGiftInventoryAllowsReorderingButNotOnlyMatchingSum(t *testing.T) {
	before := newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609})
	expected, hit, err := expectedGiftInventory(before, inventoryTestImage(), []selectedGift{{Box: maa.Rect{132, 522, 68, 38}, Count: 74}})
	if err != nil || !hit {
		t.Fatalf("baseline unavailable: hit=%v error=%v", hit, err)
	}
	for _, test := range []struct {
		name   string
		stocks []int
		match  bool
	}{
		{"reordered", []int{676, 658, 592, 3, 0, 0, 0, 0, 1476, 931, 609}, true},
		{"unchanged", []int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609}, false},
		{"same sum different multiset", []int{677, 657, 592, 3, 0, 0, 0, 0, 1476, 931, 609}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual, hit, err := readGiftInventory(newInventoryRunner(test.stocks), inventoryTestImage())
			if err != nil || !hit || reflect.DeepEqual(expected, actual) != test.match {
				t.Fatalf("unexpected debit evidence: expected=%v actual=%v hit=%v error=%v", expected, actual, hit, err)
			}
		})
	}
}

func TestExpectedGiftInventoryPreservesDuplicateAndExhaustedStock(t *testing.T) {
	for _, test := range []struct {
		name   string
		stocks []int
		gifts  []selectedGift
		want   []int
	}{
		{"duplicate quantities", []int{5, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			[]selectedGift{{Box: maa.Rect{132, 522, 68, 38}, Count: 1}, {Box: maa.Rect{233, 522, 68, 38}, Count: 2}},
			[]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 4}},
		{"last item becomes zero", []int{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			[]selectedGift{{Box: maa.Rect{132, 522, 68, 38}, Count: 1}},
			[]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual, hit, err := expectedGiftInventory(newInventoryRunner(test.stocks), inventoryTestImage(), test.gifts)
			if err != nil || !hit || !reflect.DeepEqual(actual, test.want) {
				t.Fatalf("multiplicity or zero lost: actual=%v hit=%v error=%v", actual, hit, err)
			}
		})
	}
}

func TestGiftInventoryRejectsIncompleteIndependentOCR(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*maa.RecognitionDetail)
	}{
		{"standalone miss", func(detail *maa.RecognitionDetail) { detail.Hit = false }},
		{"missing best", func(detail *maa.RecognitionDetail) { detail.DetailJson = `{"best":null}` }},
		{"wrong algorithm", func(detail *maa.RecognitionDetail) { detail.Algorithm = "ColorMatch" }},
		{"invalid payload", func(detail *maa.RecognitionDetail) { detail.DetailJson = `invalid` }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609})
			runner.mutate = func(column int, detail *maa.RecognitionDetail) {
				if column == 10 {
					test.mutate(detail)
				}
			}
			actual, hit, err := readGiftInventory(runner, inventoryTestImage())
			if err != nil || hit || actual != nil {
				t.Fatalf("partial inventory accepted: actual=%v hit=%v error=%v", actual, hit, err)
			}
		})
	}
	for _, invalid := range []string{"", "C", "。", "|", "_3", "1O", "01", "-1", "1 2", "3+", "999999999999999999999999999"} {
		t.Run(invalid, func(t *testing.T) {
			runner := newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609})
			runner.texts[10] = invalid
			actual, hit, err := readGiftInventory(runner, inventoryTestImage())
			if err != nil || hit || actual != nil {
				t.Fatalf("unreadable inventory accepted: input=%q actual=%v hit=%v error=%v", invalid, actual, hit, err)
			}
		})
	}
}

func TestExpectedGiftInventoryRejectsInvalidAssociation(t *testing.T) {
	for _, gifts := range [][]selectedGift{
		nil,
		{{Box: maa.Rect{132, 522, 68, 38}, Count: 0}},
		{{Box: maa.Rect{132, 522, 68, 38}, Count: -1}},
		{{Box: maa.Rect{132, 522, 68, 38}, Count: 733}},
		{{Box: maa.Rect{137, 522, 68, 38}, Count: 1}},
		{{Box: maa.Rect{31, 522, 68, 38}, Count: 1}},
		{{Box: maa.Rect{1243, 522, 68, 38}, Count: 1}},
		{{Box: maa.Rect{132, 522, 0, 38}, Count: 1}},
		{{Box: maa.Rect{132, 522, 68, 0}, Count: 1}},
		{{Box: maa.Rect{132, 522, 102, 38}, Count: 1}},
		{{Box: maa.Rect{132, 522, 68, 38}, Count: 1}, {Box: maa.Rect{134, 522, 68, 38}, Count: 1}},
		{{Box: maa.Rect{536, 522, 68, 38}, Count: 1}},
	} {
		actual, hit, err := expectedGiftInventory(newInventoryRunner([]int{732, 676, 592, 3, 0, 0, 0, 0, 1476, 931, 609}), inventoryTestImage(), gifts)
		if err != nil || hit || actual != nil {
			t.Fatalf("invalid selected card accepted: gifts=%+v actual=%v hit=%v error=%v", gifts, actual, hit, err)
		}
	}
}

func TestGiftInventoryPropagatesErrorAndRejectsClippedImage(t *testing.T) {
	runner := newInventoryRunner(make([]int, 11))
	runner.errColumn, runner.err = 10, errors.New("recognition failed")
	if actual, hit, err := readGiftInventory(runner, inventoryTestImage()); !errors.Is(err, runner.err) || hit || actual != nil {
		t.Fatalf("recognition error hidden: actual=%v hit=%v error=%v", actual, hit, err)
	}
	for _, img := range []image.Image{nil, image.NewRGBA(image.Rect(0, 0, 1200, 720)), image.NewRGBA(image.Rect(0, 0, 1280, 618))} {
		if actual, hit, err := readGiftInventory(newInventoryRunner(make([]int, 11)), img); err != nil || hit || actual != nil {
			t.Fatalf("clipped inventory accepted: actual=%v hit=%v error=%v", actual, hit, err)
		}
	}
}

func inventoryTestImage() image.Image { return image.NewRGBA(image.Rect(0, 0, 1280, 720)) }

type inventoryRunner struct {
	texts     []string
	calls     int
	errColumn int
	err       error
	mutate    func(int, *maa.RecognitionDetail)
}

func newInventoryRunner(stocks []int) *inventoryRunner {
	r := &inventoryRunner{errColumn: -1}
	for _, stock := range stocks {
		r.texts = append(r.texts, strconv.Itoa(stock))
	}
	return r
}

func (r *inventoryRunner) RunRecognition(entry string, _ image.Image, overrides ...any) (*maa.RecognitionDetail, error) {
	if entry != "GiftOperatorGiftInventoryText" || len(overrides) != 1 {
		return nil, fmt.Errorf("unexpected stock recognition %q", entry)
	}
	data, err := json.Marshal(overrides[0])
	if err != nil {
		return nil, err
	}
	var patch map[string]struct {
		ROI      maa.Rect `json:"roi"`
		Offset   maa.Rect `json:"roi_offset"`
		OnlyRec  bool     `json:"only_rec"`
		Expected []string `json:"expected"`
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, err
	}
	param := patch[entry]
	if param.Offset != (maa.Rect{}) || !param.OnlyRec || !reflect.DeepEqual(param.Expected, []string{`^[0-9]+$`}) {
		return nil, fmt.Errorf("stock OCR lacks independent decimal constraints")
	}
	column := -1
	for index := range giftInventoryColumns {
		if param.ROI == giftInventoryROI(index) {
			column = index
			break
		}
	}
	if column < 0 || column >= len(r.texts) {
		return nil, fmt.Errorf("unverified stock ROI: %v", param.ROI)
	}
	r.calls++
	if column == r.errColumn {
		return nil, r.err
	}
	detail := ocrDetail(r.texts[column])
	if r.mutate != nil {
		r.mutate(column, detail)
	}
	return detail, nil
}
