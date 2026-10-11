package giftoperator

import (
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

// SelectionRecognition reads cumulative selected quantities and preview progress.
// It cannot confirm delivery; Pipeline performs every click and confirmation.
type SelectionRecognition struct{}

var _ maa.CustomRecognitionRunner = &SelectionRecognition{}

type selectionStatus struct {
	giftStatus
	SelectedCount     *int  `json:"selected_count,omitempty"`
	ExpectedInventory []int `json:"expected_inventory,omitempty"`
}

// Run requires growing selection except for the final read-only submission snapshot.
func (r *SelectionRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	if ctx == nil || arg == nil || arg.Img == nil {
		return nil, false
	}
	s, err := loadSession(ctx)
	if err != nil {
		return nil, false
	}
	status, box, err := recognizeGiftSelectionFrame(ctx, arg.Img, s, arg.CurrentTaskName == "GiftOperatorSendFreezeSelection")
	if err != nil {
		log.Error().Err(err).Str("component", "GiftOperatorGiftSelectionRecognition").Msg("read selected gift preview failed")
		return nil, false
	}
	if status == nil {
		return nil, false
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		return nil, false
	}
	return &maa.CustomRecognitionResult{Box: box, Detail: string(encoded)}, true
}

func recognizeGiftSelection(runner portraitRunner, img image.Image, s session) (*selectionStatus, maa.Rect, error) {
	return recognizeGiftSelectionFrame(runner, img, s, false)
}

func recognizeGiftSelectionFrame(runner portraitRunner, img image.Image, s session, final bool) (*selectionStatus, maa.Rect, error) {
	if !s.Prepared || s.Pending == "" || s.Remaining <= 0 {
		return nil, maa.Rect{}, nil
	}
	status, box, err := readGiftStatus(runner, img, s, true, true)
	if err != nil || status == nil {
		return nil, maa.Rect{}, err
	}
	gifts, err := readSelectedGifts(runner, img)
	if err != nil || len(gifts) == 0 {
		return nil, maa.Rect{}, err
	}
	count, err := selectedGiftTotal(gifts)
	if err != nil {
		return nil, maa.Rect{}, err
	}
	if count < s.SelectedCount || (!final && count == s.SelectedCount) {
		return nil, maa.Rect{}, nil
	}
	preview := &selectionStatus{giftStatus: *status, SelectedCount: &count}
	if final && s.TargetCount > 0 {
		expected, hit, err := expectedGiftInventory(runner, img, gifts)
		if err != nil || !hit {
			return nil, maa.Rect{}, err
		}
		preview.ExpectedInventory = expected
	}
	preview.Preview = true
	return preview, box, nil
}

type selectedGift struct {
	Box   maa.Rect
	Count int
}

// readSelectedCount sums every visible selected label across gift categories.
func readSelectedCount(runner recognitionRunner, img image.Image) (int, bool, error) {
	gifts, err := readSelectedGifts(runner, img)
	if err != nil {
		return 0, false, err
	}
	count, err := selectedGiftTotal(gifts)
	return count, count > 0 && err == nil, err
}

func selectedGiftTotal(gifts []selectedGift) (int, error) {
	total := 0
	for _, gift := range gifts {
		if gift.Count <= 0 || total > int(^uint(0)>>1)-gift.Count {
			return 0, fmt.Errorf("invalid or overflowing selected gift quantity")
		}
		total += gift.Count
	}
	return total, nil
}

// readSelectedGifts rejects the entire frame if any selected quantity is unreadable.
func readSelectedGifts(runner recognitionRunner, img image.Image) ([]selectedGift, error) {
	detail, err := runner.RunRecognition("GiftOperatorGiftSelectedLabelColor", img)
	if err != nil || detail == nil || !detail.Hit {
		return nil, err
	}
	if detail.Algorithm != "ColorMatch" {
		return nil, fmt.Errorf("selected labels must use ColorMatch")
	}
	var labels struct {
		Filtered []maa.ColorMatchResult `json:"filtered"`
	}
	if err := json.Unmarshal([]byte(detail.DetailJson), &labels); err != nil {
		return nil, err
	}
	if len(labels.Filtered) == 0 {
		return nil, nil
	}
	gifts := make([]selectedGift, 0, len(labels.Filtered))
	seen := make(map[maa.Rect]bool)
	for _, label := range labels.Filtered {
		if seen[label.Box] {
			continue
		}
		seen[label.Box] = true
		roi, ok := selectedTextROI(label.Box)
		if !ok || !image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3]).In(img.Bounds()) {
			return nil, nil
		}
		number, err := runner.RunRecognition("GiftOperatorGiftSelectedText", img, map[string]any{
			"GiftOperatorGiftSelectedText": map[string]any{
				"roi": roi, "roi_offset": maa.Rect{}, "only_rec": true, "expected": []string{`^[1-9][0-9]*$`},
			},
		})
		if err != nil {
			return nil, err
		}
		text, ok := bestOCRText(number)
		if !ok {
			return nil, nil
		}
		value, err := parseSelectedCount(text)
		if err != nil {
			log.Warn().Err(err).Str("component", "GiftOperatorGiftSelectionRecognition").Str("text", text).Interface("roi", roi).Msg("selected quantity is unreadable")
			return nil, nil
		}
		gifts = append(gifts, selectedGift{Box: label.Box, Count: value})
	}
	return gifts, nil
}

// selectedTextROI excludes the icon and the yellow frame below the numeric label.
// Win32 native screenshots verified [132,522,68,38] -> [163,522,37,20].
func selectedTextROI(box maa.Rect) (maa.Rect, bool) {
	if box[2] <= 31 || box[3] <= 0 {
		return maa.Rect{}, false
	}
	return maa.Rect{box[0] + 31, box[1], box[2] - 31, min(box[3], 20)}, true
}

func parseSelectedCount(text string) (int, error) {
	text = strings.Join(strings.Fields(text), "")
	if text == "" || text[0] < '1' || text[0] > '9' {
		return 0, fmt.Errorf("selected quantity must be a positive decimal integer")
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("invalid selected quantity %q", text)
		}
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("invalid selected quantity: %w", err)
	}
	return value, nil
}
