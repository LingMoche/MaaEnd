package arenaofgames

import (
	"fmt"
	"image"
	"math"
	"testing"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func TestClassifyHand(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cards    []handSlot
		count    int
		category string
	}{
		{"empty", nil, 0, ""},
		{"single", []handSlot{{Type: "Dot", Point: 5}}, 1, ""},
		{"pair", []handSlot{{Type: "Character", Point: 2}, {Type: "Dot", Point: 2}}, 2, "pair"},
		{"triplet", []handSlot{{Type: "Dot", Point: 3}, {Type: "Bamboo", Point: 3}, {Type: "Character", Point: 3}}, 3, "three_of_a_kind"},
		{"two pair", []handSlot{{Type: "Dot", Point: 3}, {Type: "Bamboo", Point: 3}, {Type: "Character", Point: 2}, {Type: "Bamboo", Point: 2}}, 4, "two_pair"},
		{"straight at four", []handSlot{{Type: "Bamboo", Point: 4}, {Type: "Character", Point: 1}, {Type: "Dot", Point: 3}, {Type: "Bamboo", Point: 2}}, 4, "straight"},
		{"flush at four same suit distinct", []handSlot{{Type: "Dot", Point: 1}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 4}, {Type: "Dot", Point: 5}}, 4, "flush"},
		{"flush at five same suit", []handSlot{{Type: "Dot", Point: 1}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 3}, {Type: "Dot", Point: 4}}, 5, "flush"},
		{"quad identical tiles", []handSlot{{Type: "Dot", Point: 1}, {Type: "Dot", Point: 1}, {Type: "Dot", Point: 1}, {Type: "Dot", Point: 1}}, 4, "four_of_a_kind"},
		{"quad same suit among five", []handSlot{{Type: "Bamboo", Point: 5}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 2}, {Type: "Dot", Point: 2}}, 5, "four_of_a_kind"},
		{"quad mixed suits", []handSlot{{Type: "Dot", Point: 1}, {Type: "Bamboo", Point: 1}, {Type: "Character", Point: 1}, {Type: "Dot", Point: 1}}, 4, "four_of_a_kind"},
		{"full house mixed suits", []handSlot{{Type: "Dot", Point: 3}, {Type: "Bamboo", Point: 3}, {Type: "Character", Point: 3}, {Type: "Dot", Point: 2}, {Type: "Bamboo", Point: 2}}, 5, "full_house"},
		{"same suit two points is flush not full house", []handSlot{{Type: "Dot", Point: 3}, {Type: "Dot", Point: 3}, {Type: "Dot", Point: 3}, {Type: "Dot", Point: 5}, {Type: "Dot", Point: 5}}, 5, "flush"},
		{"straight flush any order", []handSlot{{Type: "Bamboo", Point: 4}, {Type: "Bamboo", Point: 1}, {Type: "Bamboo", Point: 3}, {Type: "Bamboo", Point: 2}, {Type: "Bamboo", Point: 5}}, 5, "straight_flush"},
		{"straight covers pair", []handSlot{{Type: "Dot", Point: 1}, {Type: "Bamboo", Point: 1}, {Type: "Bamboo", Point: 2}, {Type: "Dot", Point: 3}, {Type: "Dot", Point: 4}}, 5, "straight"},
		{"quad covers pair", []handSlot{{Type: "Dot", Point: 1}, {Type: "Bamboo", Point: 1}, {Type: "Character", Point: 1}, {Type: "Dot", Point: 1}, {Type: "Dot", Point: 2}}, 5, "four_of_a_kind"},
		{"no pattern", []handSlot{{Type: "Dot", Point: 1}, {Type: "Bamboo", Point: 2}, {Type: "Character", Point: 4}}, 3, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detail, err := classifyHand(handCandidates(tc.cards...))
			if err != nil || detail.Count != tc.count || detail.Category != tc.category {
				t.Fatalf("classifyHand = %+v, %v; want count=%d category=%s", detail, err, tc.count, tc.category)
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
			if err != nil || detail.Count != 5 || detail.Category != "straight_flush" {
				t.Fatalf("suit=%s points=%v: detail=%+v error=%v", suit, points, detail, err)
			}
		})
	}
}

func TestUnconfirmedSlotsDoNotProduceAClassification(t *testing.T) {
	for _, name := range []string{"unknown", "ambiguous faces", "face and back", "non-contiguous", "invalid point", "invalid suit"} {
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
			}
			if detail, err := classifyHand(candidates); err == nil || detail != (handDetail{}) {
				t.Fatalf("unconfirmed hand produced %+v, %v", detail, err)
			}
		})
	}
}

func TestEvaluateHand(t *testing.T) {
	targets := func(cats ...category) map[category]bool {
		set := make(map[category]bool)
		for _, c := range cats {
			set[c] = true
		}
		return set
	}
	hand := func(cards ...handSlot) ([15]int, int) {
		detail, err := classifyHand(handCandidates(cards...))
		if err != nil {
			t.Fatalf("classifyHand: %v", err)
		}
		return handCounts(detail), detail.Count
	}
	D, B, C := "Dot", "Bamboo", "Character"
	for _, tc := range []struct {
		name    string
		cards   []handSlot
		targets map[category]bool
		status  string
	}{
		{"straight flush empty keeps going", nil, targets(catStraightFlush), "continuing"},
		{"straight flush single keeps going", []handSlot{{Type: B, Point: 4}}, targets(catStraightFlush), "continuing"},
		{"straight flush duplicate point rejected", []handSlot{{Type: D, Point: 2}, {Type: D, Point: 2}}, targets(catStraightFlush), "rejected"},
		{"straight flush mixed suit rejected", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}}, targets(catStraightFlush), "rejected"},
		{"straight flush complete", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 3}, {Type: D, Point: 4}, {Type: D, Point: 5}}, targets(catStraightFlush), "complete"},
		{"straight flush partial unordered", []handSlot{{Type: D, Point: 4}, {Type: D, Point: 1}, {Type: D, Point: 3}}, targets(catStraightFlush), "continuing"},
		{"flush early stop with four of the same suit", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 4}, {Type: D, Point: 5}}, targets(catFlush), "complete"},
		// 日志 2026-10-30 复现：4 索 + 1 筒不是同花，同花要求整手牌全部同花色。
		{"logged hand with four of one suit and an off-suit fifth rejected", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 5}, {Type: B, Point: 5}, {Type: B, Point: 2}, {Type: B, Point: 5}}, targets(catTwoPair, catFlush, catFullHouse, catFourOfAKind), "rejected"},
		{"flush continuing at two identical", []handSlot{{Type: D, Point: 5}, {Type: D, Point: 5}}, targets(catFlush), "continuing"},
		{"flush continuing at three identical", []handSlot{{Type: D, Point: 5}, {Type: D, Point: 5}, {Type: D, Point: 5}}, targets(catFlush), "continuing"},
		{"flush continuing at three distinct same-suit tiles", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 3}}, targets(catFlush), "continuing"},
		{"flush rejected at three mixed suits", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}}, targets(catFlush), "rejected"},
		{"flush rejected at two mixed suits", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}}, targets(catFlush), "rejected"},
		{"quad early stop at four tiles", []handSlot{{Type: D, Point: 2}, {Type: B, Point: 2}, {Type: C, Point: 2}, {Type: D, Point: 2}}, targets(catFourOfAKind), "complete"},
		{"quad continuing at pair", []handSlot{{Type: D, Point: 2}, {Type: B, Point: 2}}, targets(catFourOfAKind), "continuing"},
		{"quad rejected at three singles", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}}, targets(catFourOfAKind), "rejected"},
		{"quad continuing at triplet plus single", []handSlot{{Type: D, Point: 2}, {Type: B, Point: 2}, {Type: C, Point: 2}, {Type: D, Point: 5}}, targets(catFourOfAKind), "continuing"},
		{"full house complete", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 3}, {Type: D, Point: 5}, {Type: B, Point: 5}}, targets(catFullHouse), "complete"},
		{"full house continuing at triplet plus single", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 3}, {Type: D, Point: 5}}, targets(catFullHouse), "continuing"},
		{"full house continuing at two singles", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}}, targets(catFullHouse), "continuing"},
		{"full house rejected at quad", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 3}, {Type: D, Point: 3}}, targets(catFullHouse), "rejected"},
		{"full house rejected at three distinct points", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}}, targets(catFullHouse), "rejected"},
		{"full house two pairs continuing", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 5}, {Type: D, Point: 5}}, targets(catFullHouse), "continuing"},
		{"pair early stop at pair", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}}, targets(catPair), "complete"},
		{"pair burned by triplet rejected", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 3}}, targets(catPair), "rejected"},
		{"pair continuing at three distinct", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}}, targets(catPair), "continuing"},
		{"pair continuing when no straight window", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}, {Type: D, Point: 5}}, targets(catPair), "continuing"},
		{"pair rejected when straight window locked", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}, {Type: D, Point: 4}}, targets(catPair), "rejected"},
		{"pair burned by two pair rejected", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 5}, {Type: D, Point: 5}}, targets(catPair), "rejected"},
		{"triplet early stop", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 3}}, targets(catThreeOfAKind), "complete"},
		{"triplet continuing at pair", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}}, targets(catThreeOfAKind), "continuing"},
		{"triplet rejected at four distinct points", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}, {Type: D, Point: 4}}, targets(catThreeOfAKind), "rejected"},
		{"two pair complete", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 5}, {Type: D, Point: 5}}, targets(catTwoPair), "complete"},
		{"two pair continuing at single pair", []handSlot{{Type: D, Point: 3}, {Type: B, Point: 3}, {Type: C, Point: 5}}, targets(catTwoPair), "continuing"},
		{"two pair rejected at four distinct", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}, {Type: D, Point: 4}}, targets(catTwoPair), "rejected"},
		{"straight complete at four consecutive", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 3}, {Type: D, Point: 4}}, targets(catStraight), "complete"},
		{"straight continuing at gap", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 2}, {Type: C, Point: 4}}, targets(catStraight), "continuing"},
		{"straight rejected at both windows blocked", []handSlot{{Type: D, Point: 1}, {Type: B, Point: 1}, {Type: C, Point: 5}}, targets(catStraight), "rejected"},
		{"mixed targets keep straight flush attempt", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 3}, {Type: D, Point: 4}}, targets(catFlush, catStraightFlush), "complete"},
		{"straight flush only keeps drawing at four of the same suit", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 3}, {Type: D, Point: 4}}, targets(catStraightFlush), "continuing"},
		{"straight flush only rejects off suit fifth", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 3}, {Type: D, Point: 4}, {Type: B, Point: 2}}, targets(catStraightFlush), "rejected"},
		{"flush and four both dead rejected", []handSlot{{Type: D, Point: 1}, {Type: D, Point: 2}, {Type: D, Point: 3}, {Type: B, Point: 2}}, targets(catFlush, catFourOfAKind), "rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts, drawn := hand(tc.cards...)
			if status := evaluateHand(counts, drawn, tc.targets); status != tc.status {
				t.Fatalf("evaluateHand = %q; want %q", status, tc.status)
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
		for _, status := range []string{"continuing", "rejected", "complete"} {
			want := condition == "recognized" || condition == status
			if got := params.matches(handDetail{Count: 3}, status); got != want {
				t.Fatalf("condition=%s status=%s: matched=%v want=%v", condition, status, got, want)
			}
		}
	}
	for count := 0; count <= 5; count++ {
		params, err := parseParams(fmt.Sprintf(`{"condition":"recognized","count":%d}`, count))
		if err != nil || !params.matches(handDetail{Count: count}, "rejected") || params.matches(handDetail{Count: count + 1}, "rejected") {
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

func TestParseTargetAttachAndResolveTargets(t *testing.T) {
	dailyHand.reset()
	t.Cleanup(dailyHand.reset)
	valid := `{"attach":{"hand_of_today":true,"flush":true}}`
	attach, err := parseTargetAttach(valid)
	if err != nil || !attach.HandOfToday || !attach.Flush || attach.StraightFlush || attach.FullHouse || attach.FourOfAKind {
		t.Fatalf("parseTargetAttach = %+v, %v", attach, err)
	}
	store := nodeStore{
		targetNode:    `{"attach":{"straight_flush":true}}`,
		dailyHandNode: dailyHandNodeJSON,
	}
	targets, err := resolveTargets(store, nil)
	if err != nil || len(targets) != 1 || !targets[catStraightFlush] {
		t.Fatalf("resolveTargets = %v, %v; want straight flush only", targets, err)
	}
	store[targetNode] = `{"attach":{"flush":true,"four_of_a_kind":true}}`
	targets, err = resolveTargets(store, nil)
	if err != nil || len(targets) != 2 || !targets[catFlush] || !targets[catFourOfAKind] {
		t.Fatalf("resolveTargets = %v, %v; want flush and quad", targets, err)
	}
	// 勾选今日好运但 OCR 不可用：有静态目标则降级沿用，没有则报错。
	store[targetNode] = `{"attach":{"hand_of_today":true,"flush":true}}`
	targets, err = resolveTargets(store, nil)
	if err != nil || len(targets) != 1 || !targets[catFlush] {
		t.Fatalf("resolveTargets = %v, %v; want degraded flush only", targets, err)
	}
	store[targetNode] = `{"attach":{"hand_of_today":true}}`
	if targets, err := resolveTargets(store, nil); err == nil || targets != nil {
		t.Fatalf("resolveTargets accepted an unreadable daily hand: %v, %v", targets, err)
	}
	// 缓存命中后不再依赖 OCR 来源。
	dailyHand.store(catPair)
	store[targetNode] = `{"attach":{"hand_of_today":true}}`
	targets, err = resolveTargets(store, nil)
	if err != nil || len(targets) != 1 || !targets[catPair] {
		t.Fatalf("resolveTargets = %v, %v; want cached daily pair", targets, err)
	}
	// 跨日的缓存即使已 set 也读不到，必须重新 OCR。
	dailyHand = &dailyHandState{category: catPair, day: localDay() - 1, set: true}
	store[targetNode] = `{"attach":{"hand_of_today":true}}`
	if targets, err := resolveTargets(store, nil); err == nil || targets != nil {
		t.Fatalf("resolveTargets used a stale daily cache: %v, %v", targets, err)
	}
	dailyHand.reset()
	store[targetNode] = `{"attach":{}}`
	if targets, err := resolveTargets(store, nil); err == nil || targets != nil {
		t.Fatalf("resolveTargets accepted empty selection: %v, %v", targets, err)
	}
	store[targetNode] = `{`
	if _, err := resolveTargets(store, nil); err == nil {
		t.Fatal("resolveTargets accepted invalid attach json")
	}
}

func TestMatchDailyCategoryPrefersLongestName(t *testing.T) {
	names, err := dailyHandNames(nodeStore{dailyHandNode: dailyHandNodeJSON})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text string
		want category
	}{
		{"同花顺", catStraightFlush},
		{"今日好运：同花顺", catStraightFlush},
		{"同花", catFlush},
		{"Straight Flush", catStraightFlush},
		{"straight", catStraight},
		{"Two Pair", catTwoPair},
		{"ストレートフラッシュ", catStraightFlush},
		{"四条两对", catFourOfAKind},
		{"Quad", catFourOfAKind},
		{"Triplet", catThreeOfAKind},
		{"Hand of Today: Quad", catFourOfAKind},
		{"투 페어", catTwoPair},
		{"원 페어", catPair},
		{"스이트레이트", catNone},
		{"", catNone},
		{"高牌", catNone},
	} {
		got, err := matchDailyCategory(tc.text, names)
		if tc.want == catNone {
			if err == nil || got != catNone {
				t.Fatalf("matchDailyCategory(%q) = %v, %v; want error", tc.text, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("matchDailyCategory(%q) = %v, %v; want %v", tc.text, got, err, tc.want)
		}
	}
}

func TestDailyHandNamesValidation(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"attach":{}}`, `{"attach":{"pair":["对子"]}}`, `{`,
		`{"attach":{"pair":[]}}`,
	} {
		if _, err := dailyHandNames(nodeStore{dailyHandNode: raw}); err == nil {
			t.Fatalf("dailyHandNames accepted %s", raw)
		}
	}
	if _, err := dailyHandNames(nodeStore{}); err == nil {
		t.Fatal("dailyHandNames accepted missing node")
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

// dailyHandNodeJSON 是「今日好运」OCR 节点 attach 的最小样例，名字与键均齐全。
// 名单与 ArenaOfGamesDailyHandText 的 attach 保持同步：按截图校准词表时两处一起改。
const dailyHandNodeJSON = `{"attach":{` +
	`"straight_flush":["同花顺","同花順","Straight Flush","ストレートフラッシュ","스트레이트 플러시"],` +
	`"four_of_a_kind":["四条","四條","Quad","フォーカード","포카드"],` +
	`"flush":["同花","Flush","フラッシュ","플러시"],` +
	`"full_house":["葫芦","葫蘆","Full House","フルハウス","풀하우스"],` +
	`"straight":["顺子","順子","Straight","ストレート","스트레이트"],` +
	`"two_pair":["两对","兩對","Two Pair","ツーペア","투 페어"],` +
	`"three_of_a_kind":["三条","三條","Triplet","スリーカード","트리플"],` +
	`"pair":["对子","對子","Pair","ワンペア","원 페어"]}}`

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

func (s nodeStore) RunRecognition(string, image.Image, ...any) (*maa.RecognitionDetail, error) {
	return nil, fmt.Errorf("recognition is unavailable in nodeStore")
}
