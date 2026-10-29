package arenaofgames

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"strconv"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

const component = "ArenaOfGames.CheckStraightFlush"

var _ maa.CustomRecognitionRunner = &Recognition{}

// Recognition 识别五个手牌槽并检查同花顺条件，不执行点击或修改流程。
type Recognition struct{}

type recognitionParam struct {
	Condition string `json:"condition"`
	Count     *int   `json:"count,omitempty"`
}

type handSlot struct {
	Type  string  `json:"type"`
	Point int     `json:"point"`
	Score float64 `json:"score"`
}

type handDetail struct {
	Slots          [5]handSlot `json:"slots"`
	Count          int         `json:"count"`
	Classification string      `json:"classification"`
}

type recognitionSource interface {
	GetNodeJSON(string) (string, error)
	RunRecognition(string, image.Image, ...any) (*maa.RecognitionDetail, error)
}

// Run 从本次截图识别手牌，只有完整识别且满足 condition、count 时才命中。
func (r *Recognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	if ctx == nil || arg == nil || arg.Img == nil {
		log.Debug().Str("component", component).Msg("recognition context or image is unavailable")
		return nil, false
	}
	params, err := parseParams(arg.CustomRecognitionParam)
	if err != nil {
		log.Debug().Err(err).Str("component", component).Msg("invalid straight flush condition")
		return nil, false
	}
	detail, err := recognizeHand(ctx, arg.Img)
	if err != nil {
		log.Debug().Err(err).Str("component", component).Msg("hand could not be confirmed")
		return nil, false
	}
	if !params.matches(detail) {
		log.Debug().Str("component", component).Str("condition", params.Condition).
			Int("count", detail.Count).Str("classification", detail.Classification).
			Msg("hand does not match the requested condition")
		return nil, false
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		log.Debug().Err(err).Str("component", component).Msg("failed to encode recognized hand")
		return nil, false
	}
	log.Info().Str("component", component).Str("condition", params.Condition).
		Int("count", detail.Count).Str("classification", detail.Classification).
		Interface("slots", detail.Slots).Msg("hand condition matched")
	return &maa.CustomRecognitionResult{Box: arg.Roi, Detail: string(raw)}, true
}

func parseParams(raw string) (recognitionParam, error) {
	var params recognitionParam
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return params, err
	}
	switch params.Condition {
	case "recognized", "continuing", "rejected", "complete":
	default:
		return params, fmt.Errorf("unknown condition %q", params.Condition)
	}
	if params.Count != nil && (*params.Count < 0 || *params.Count > 5) {
		return params, fmt.Errorf("count must be between 0 and 5")
	}
	return params, nil
}

func (p recognitionParam) matches(detail handDetail) bool {
	return (p.Count == nil || *p.Count == detail.Count) &&
		(p.Condition == "recognized" || p.Condition == detail.Classification)
}

// recognizeHand 只在本次截图内汇总牌面和牌背，所有槽均明确后才进行分类。
func recognizeHand(src recognitionSource, img image.Image) (handDetail, error) {
	var handROIs, backROIs [5]maa.Rect
	for i := range handROIs {
		var err error
		handROIs[i], err = readROI(src, "ArenaOfGamesHandPosition"+strconv.Itoa(i+1))
		if err != nil {
			return handDetail{}, err
		}
		backROIs[i], err = readROI(src, "ArenaOfGamesBackPosition"+strconv.Itoa(i+1))
		if err != nil {
			return handDetail{}, err
		}
	}
	var candidates [5][]handSlot
	for _, suit := range []string{"Dot", "Bamboo", "Character"} {
		for point := 1; point <= 5; point++ {
			node := "ArenaOfGamesHand" + suit + strconv.Itoa(point)
			if err := collectMatches(src, img, node, handROIs, handSlot{Type: suit, Point: point}, &candidates); err != nil {
				return handDetail{}, err
			}
		}
	}
	if err := collectMatches(src, img, "ArenaOfGamesCardBack", backROIs, handSlot{Type: "back"}, &candidates); err != nil {
		return handDetail{}, err
	}
	return classifyHand(candidates)
}

func collectMatches(src recognitionSource, img image.Image, node string, rois [5]maa.Rect, card handSlot, candidates *[5][]handSlot) error {
	detail, err := src.RunRecognition(node, img)
	if err != nil {
		return fmt.Errorf("recognize %s: %w", node, err)
	}
	if detail == nil {
		return fmt.Errorf("recognize %s returned no detail", node)
	}
	if !detail.Hit {
		return nil
	}
	if detail.Results == nil || len(detail.Results.Filtered) == 0 {
		return fmt.Errorf("recognize %s returned no filtered template matches", node)
	}
	for _, result := range detail.Results.Filtered {
		if result == nil {
			return fmt.Errorf("recognize %s returned an empty template match", node)
		}
		match, ok := result.AsTemplateMatch()
		if !ok || match == nil {
			return fmt.Errorf("recognize %s returned a non-template result", node)
		}
		card.Score = match.Score
		if err := addMatch(candidates, rois, match.Box, card); err != nil {
			return fmt.Errorf("assign %s: %w", node, err)
		}
	}
	return nil
}

// addMatch 将同一模板的重复匹配合并，保留不同牌型候选供分类时判定歧义。
func addMatch(candidates *[5][]handSlot, rois [5]maa.Rect, box maa.Rect, card handSlot) error {
	if box.Width() <= 0 || box.Height() <= 0 || math.IsNaN(card.Score) || math.IsInf(card.Score, 0) {
		return fmt.Errorf("invalid template match")
	}
	slot := -1
	for i, roi := range rois {
		if box.X() >= roi.X() && box.Y() >= roi.Y() &&
			box.X()+box.Width() <= roi.X()+roi.Width() &&
			box.Y()+box.Height() <= roi.Y()+roi.Height() {
			if slot >= 0 {
				return fmt.Errorf("template match belongs to multiple slots")
			}
			slot = i
		}
	}
	if slot < 0 {
		return nil
	}
	for i, existing := range candidates[slot] {
		if existing.Type == card.Type && existing.Point == card.Point {
			if card.Score > existing.Score {
				candidates[slot][i] = card
			}
			return nil
		}
	}
	candidates[slot] = append(candidates[slot], card)
	return nil
}

func classifyHand(candidates [5][]handSlot) (handDetail, error) {
	detail := handDetail{Classification: "continuing"}
	var suit string
	var points [5]bool
	emptySeen := false
	for i, matches := range candidates {
		if len(matches) != 1 {
			return handDetail{}, fmt.Errorf("slot %d has %d candidates", i+1, len(matches))
		}
		card := matches[0]
		detail.Slots[i] = card
		if card.Type == "back" && card.Point == 0 {
			emptySeen = true
			continue
		}
		if card.Point < 1 || card.Point > 5 ||
			(card.Type != "Dot" && card.Type != "Bamboo" && card.Type != "Character") {
			return handDetail{}, fmt.Errorf("slot %d has an invalid card", i+1)
		}
		if emptySeen {
			return handDetail{}, fmt.Errorf("slot %d follows an empty slot", i+1)
		}
		detail.Count++
		if suit == "" {
			suit = card.Type
		}
		if suit != card.Type || points[card.Point-1] {
			detail.Classification = "rejected"
		}
		points[card.Point-1] = true
	}
	if detail.Count == 5 && detail.Classification != "rejected" {
		detail.Classification = "complete"
	}
	return detail, nil
}

func readROI(src interface{ GetNodeJSON(string) (string, error) }, node string) (maa.Rect, error) {
	raw, err := src.GetNodeJSON(node)
	if err != nil {
		return maa.Rect{}, fmt.Errorf("read %s: %w", node, err)
	}
	// GetNodeJSON 可能返回资源的 v2 格式，也可能返回框架归一化后的扁平格式。
	var definition struct {
		ROI         *maa.Rect       `json:"roi"`
		Recognition json.RawMessage `json:"recognition"`
	}
	if err := json.Unmarshal([]byte(raw), &definition); err != nil {
		return maa.Rect{}, fmt.Errorf("parse %s: %w", node, err)
	}
	if definition.ROI == nil {
		var recognition struct {
			Param struct {
				ROI *maa.Rect `json:"roi"`
			} `json:"param"`
		}
		if err := json.Unmarshal(definition.Recognition, &recognition); err != nil {
			return maa.Rect{}, fmt.Errorf("parse %s recognition: %w", node, err)
		}
		definition.ROI = recognition.Param.ROI
	}
	if definition.ROI == nil {
		return maa.Rect{}, fmt.Errorf("node %s has no slot ROI", node)
	}
	roi := *definition.ROI
	if roi.X() < 0 || roi.Y() < 0 || roi.Width() <= 0 || roi.Height() <= 0 ||
		roi.X()+roi.Width() > 1280 || roi.Y()+roi.Height() > 720 {
		return maa.Rect{}, fmt.Errorf("node %s has invalid 720p ROI %v", node, roi)
	}
	return roi, nil
}
