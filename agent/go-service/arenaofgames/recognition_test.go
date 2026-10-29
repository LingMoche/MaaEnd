package arenaofgames

import (
	"fmt"
	"math"
	"testing"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func TestClassifyHand(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cards          []handSlot
		classification string
	}{
		{"empty", nil, "continuing"},
		{"single", []handSlot{{Type: "Dot", Point: 5}}, "continuing"},
		{"partial unordered", []handSlot{{Type: "Bamboo", Point: 4}, {Type: "Bamboo", Point: 1}, {Type: "Bamboo", Point: 3}}, "continuing"},
		{"duplicate", []handSlot{{Type: "Character", Point: 2}, {Type: "Character", Point: 2}}, "rejected"},
		{"mixed suits", []handSlot{{Type: "Dot", Point: 1}, {Type: "Bamboo", Point: 2}}, "rejected"},
		{"mixed full", []handSlot{{Type: "Dot", Point: 1}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 3}, {Type: "Dot", Point: 4}, {Type: "Character", Point: 5}}, "rejected"},
		{"duplicate full", []handSlot{{Type: "Dot", Point: 1}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 3}, {Type: "Dot", Point: 4}, {Type: "Dot", Point: 4}}, "rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detail, err := classifyHand(handCandidates(tc.cards...))
			if err != nil || detail.Count != len(tc.cards) || detail.Classification != tc.classification {
				t.Fatalf("classifyHand = %+v, %v; want count=%d classification=%s", detail, err, len(tc.cards), tc.classification)
			}
			for i, card := range tc.cards {
				if detail.Slots[i].Type != card.Type || detail.Slots[i].Point != card.Point {
					t.Fatalf("slot %d lost card identity: %+v", i+1, detail.Slots[i])
				}
			}
		})
	}
}

func TestCompleteStraightFlushAllowsEveryOrderAndSuit(t *testing.T) {
	var visit func([]int, []int, func([]int))
	visit = func(prefix, remaining []int, accept func([]int)) {
		if len(remaining) == 0 {
			accept(prefix)
			return
		}
		for i, point := range remaining {
			next := append([]int(nil), remaining[:i]...)
			next = append(next, remaining[i+1:]...)
			visit(append(append([]int(nil), prefix...), point), next, accept)
		}
	}
	for _, suit := range []string{"Dot", "Bamboo", "Character"} {
		visit(nil, []int{1, 2, 3, 4, 5}, func(points []int) {
			var cards []handSlot
			for _, point := range points {
				cards = append(cards, handSlot{Type: suit, Point: point})
			}
			detail, err := classifyHand(handCandidates(cards...))
			if err != nil || detail.Count != 5 || detail.Classification != "complete" {
				t.Fatalf("suit=%s points=%v: detail=%+v error=%v", suit, points, detail, err)
			}
		})
	}
}

func TestUnconfirmedSlotsDoNotProduceAClassification(t *testing.T) {
	for _, name := range []string{"unknown", "ambiguous faces", "face and back", "non-contiguous", "invalid point", "invalid suit", "incomplete rejected"} {
		t.Run(name, func(t *testing.T) {
			candidates := handCandidates(handSlot{Type: "Dot", Point: 1})
			switch name {
			case "unknown":
				candidates[1] = nil
			case "ambiguous faces":
				candidates[0] = append(candidates[0], handSlot{Type: "Bamboo", Point: 1, Score: 0.99})
			case "face and back":
				candidates[0] = append(candidates[0], handSlot{Type: "back"})
			case "non-contiguous":
				candidates[2] = []handSlot{{Type: "Dot", Point: 3}}
			case "invalid point":
				candidates[0][0].Point = 6
			case "invalid suit":
				candidates[0][0].Type = "unknown"
			case "incomplete rejected":
				candidates[1] = []handSlot{{Type: "Bamboo", Point: 2}}
				candidates[4] = nil
			}
			if detail, err := classifyHand(candidates); err == nil || detail != (handDetail{}) {
				t.Fatalf("unconfirmed hand produced %+v, %v", detail, err)
			}
		})
	}
}

func TestParseParamsAndMatchConditions(t *testing.T) {
	for _, condition := range []string{"recognized", "continuing", "rejected", "complete"} {
		params, err := parseParams(fmt.Sprintf(`{"condition":%q}`, condition))
		if err != nil {
			t.Fatal(err)
		}
		for _, classification := range []string{"continuing", "rejected", "complete"} {
			want := condition == "recognized" || condition == classification
			if got := params.matches(handDetail{Count: 3, Classification: classification}); got != want {
				t.Fatalf("condition=%s classification=%s: matched=%v want=%v", condition, classification, got, want)
			}
		}
	}
	for count := 0; count <= 5; count++ {
		params, err := parseParams(fmt.Sprintf(`{"condition":"recognized","count":%d}`, count))
		if err != nil || !params.matches(handDetail{Count: count}) || params.matches(handDetail{Count: count + 1}) {
			t.Fatalf("count gate %d failed: %+v, %v", count, params, err)
		}
	}
	for _, raw := range []string{
		``, `{`, `null`, `{}`, `[]`, `{"condition":"draw"}`, `{"condition":1}`,
		`{"condition":"recognized","count":-1}`, `{"condition":"recognized","count":6}`,
		`{"condition":"recognized","count":1.5}`, `{"condition":"recognized","count":"1"}`,
	} {
		if _, err := parseParams(raw); err == nil {
			t.Fatalf("accepted invalid params %s", raw)
		}
	}
}

func TestAssignMatchesKeepsDifferentCandidatesAndBestScore(t *testing.T) {
	rois := [5]maa.Rect{{10, 10, 20, 20}, {50, 10, 20, 20}, {90, 10, 20, 20}, {130, 10, 20, 20}, {170, 10, 20, 20}}
	var candidates [5][]handSlot
	for _, score := range []float64{0.91, 0.99, 0.95} {
		if err := addMatch(&candidates, rois, maa.Rect{10, 10, 20, 20}, handSlot{Type: "Dot", Point: 1, Score: score}); err != nil {
			t.Fatal(err)
		}
	}
	if len(candidates[0]) != 1 || candidates[0][0].Score != 0.99 {
		t.Fatalf("same-template matches were not merged: %+v", candidates[0])
	}
	if err := addMatch(&candidates, rois, maa.Rect{11, 11, 18, 18}, handSlot{Type: "Bamboo", Point: 1, Score: 0.92}); err != nil || len(candidates[0]) != 2 {
		t.Fatalf("different suit candidate was discarded: candidates=%+v error=%v", candidates[0], err)
	}
	if err := addMatch(&candidates, rois, maa.Rect{50, 10, 20, 20}, handSlot{Type: "Dot", Point: 1, Score: 0.95}); err != nil || len(candidates[1]) != 1 {
		t.Fatalf("same card in second slot was discarded: candidates=%+v error=%v", candidates[1], err)
	}
	if err := addMatch(&candidates, rois, maa.Rect{30, 10, 20, 20}, handSlot{Type: "Dot", Point: 2, Score: 0.99}); err != nil || len(candidates[0]) != 2 || len(candidates[1]) != 1 {
		t.Fatalf("gap match entered a slot: candidates=%+v error=%v", candidates, err)
	}
	if err := addMatch(&candidates, rois, maa.Rect{10, 10, 0, 20}, handSlot{Type: "Dot", Point: 2, Score: 0.99}); err == nil {
		t.Fatal("accepted zero-width match")
	}
	if err := addMatch(&candidates, rois, maa.Rect{10, 10, 20, 20}, handSlot{Type: "Dot", Point: 2, Score: math.NaN()}); err == nil {
		t.Fatal("accepted non-finite score")
	}
	rois[1] = rois[0]
	if err := addMatch(&candidates, rois, maa.Rect{10, 10, 20, 20}, handSlot{Type: "Dot", Point: 2, Score: 0.99}); err == nil {
		t.Fatal("accepted a match in overlapping slots")
	}
}

func TestReadROIHandlesResourceAndNormalizedNodes(t *testing.T) {
	want := maa.Rect{233, 164, 38, 52}
	for _, raw := range []string{
		`{"recognition":{"type":"DirectHit","param":{"roi":[233,164,38,52]}}}`,
		`{"recognition":"DirectHit","roi":[233,164,38,52]}`,
	} {
		got, err := readROI(nodeStore{"slot": raw}, "slot")
		if err != nil || got != want {
			t.Fatalf("readROI = %v, %v; want %v", got, err, want)
		}
	}
	for _, raw := range []string{
		`{`, `{}`, `{"recognition":{"type":"DirectHit","param":{}}}`,
		`{"roi":[-1,164,38,52]}`, `{"roi":[233,164,0,52]}`,
		`{"roi":[1270,164,38,52]}`, `{"roi":[233,700,38,52]}`,
	} {
		if _, err := readROI(nodeStore{"slot": raw}, "slot"); err == nil {
			t.Fatalf("accepted invalid ROI %s", raw)
		}
	}
	if _, err := readROI(nodeStore{}, "missing"); err == nil {
		t.Fatal("accepted missing ROI node")
	}
}

func handCandidates(cards ...handSlot) [5][]handSlot {
	var candidates [5][]handSlot
	for i := range candidates {
		card := handSlot{Type: "back", Score: 0.97}
		if i < len(cards) {
			card = cards[i]
			card.Score = 0.98
		}
		candidates[i] = []handSlot{card}
	}
	return candidates
}

type nodeStore map[string]string

func (s nodeStore) GetNodeJSON(node string) (string, error) {
	if raw, ok := s[node]; ok {
		return raw, nil
	}
	return "", fmt.Errorf("missing node %s", node)
}
