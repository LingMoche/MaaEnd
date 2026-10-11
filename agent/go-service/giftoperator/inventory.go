package giftoperator

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

const (
	giftInventoryColumns   = 11
	giftInventoryCardLeft  = 132
	giftInventoryStride    = 101
	giftInventoryTolerance = 4
)

// giftInventoryROI covers only the stock number, excluding the card frame.
// Native 720p screenshots verified all eleven columns, including zero stock.
func giftInventoryROI(column int) maa.Rect {
	return maa.Rect{158 + column*giftInventoryStride, 592, 46, 27}
}

// expectedGiftInventory freezes a complete stock snapshot minus every selected
// card's quantity. Associate columns before sorting, so duplicate stocks and
// post-delivery reordering do not lose multiplicity or change the debit.
func expectedGiftInventory(runner recognitionRunner, img image.Image, gifts []selectedGift) ([]int, bool, error) {
	if img == nil || len(gifts) == 0 {
		return nil, false, nil
	}
	selected := make(map[int]int, len(gifts))
	for _, gift := range gifts {
		if gift.Count <= 0 || gift.Box[2] <= 0 || gift.Box[2] > giftInventoryStride || gift.Box[3] <= 0 ||
			!image.Rect(gift.Box[0], gift.Box[1], gift.Box[0]+gift.Box[2], gift.Box[1]+gift.Box[3]).In(img.Bounds()) {
			return nil, false, nil
		}
		column, ok := giftInventoryColumn(gift.Box[0])
		if !ok {
			return nil, false, nil
		}
		if _, duplicate := selected[column]; duplicate {
			return nil, false, nil
		}
		selected[column] = gift.Count
	}
	stocks, hit, err := readGiftInventoryColumns(runner, img)
	if err != nil || !hit {
		return nil, false, err
	}
	for column, count := range selected {
		if count > stocks[column] {
			return nil, false, nil
		}
		stocks[column] -= count
	}
	result, err := sortedGiftInventory(stocks)
	return result, err == nil, err
}

// readGiftInventory returns a complete sorted multiset, including every zero.
// Missing, unreadable, or independently unmatched stock invalidates the frame.
func readGiftInventory(runner recognitionRunner, img image.Image) ([]int, bool, error) {
	stocks, hit, err := readGiftInventoryColumns(runner, img)
	if err != nil || !hit {
		return nil, false, err
	}
	result, err := sortedGiftInventory(stocks)
	return result, err == nil, err
}

func readGiftInventoryColumns(runner recognitionRunner, img image.Image) ([]int, bool, error) {
	if runner == nil || img == nil {
		return nil, false, nil
	}
	stocks := make([]int, giftInventoryColumns)
	for column := range stocks {
		roi := giftInventoryROI(column)
		if !image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3]).In(img.Bounds()) {
			return nil, false, nil
		}
		detail, err := runner.RunRecognition("GiftOperatorGiftInventoryText", img, map[string]any{
			"GiftOperatorGiftInventoryText": map[string]any{
				"roi": roi, "roi_offset": maa.Rect{}, "only_rec": true, "expected": []string{`^[0-9]+$`},
			},
		})
		if err != nil {
			return nil, false, err
		}
		text, ok := bestOCRText(detail)
		if !ok {
			return nil, false, nil
		}
		stock, err := parseGiftStock(text)
		if err != nil {
			log.Warn().Err(err).Str("component", "GiftOperatorGiftInventory").
				Str("text", text).Interface("roi", roi).Msg("gift stock is unreadable")
			return nil, false, nil
		}
		stocks[column] = stock
	}
	return stocks, true, nil
}

// giftInventoryColumn requires one unambiguous card origin; selected labels can
// differ from the card edge by a few pixels, but cannot belong to another column.
func giftInventoryColumn(left int) (int, bool) {
	for column := range giftInventoryColumns {
		delta := left - (giftInventoryCardLeft + column*giftInventoryStride)
		if delta >= -giftInventoryTolerance && delta <= giftInventoryTolerance {
			return column, true
		}
	}
	return 0, false
}

func parseGiftStock(text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 1 && text[0] == '0' {
		return 0, fmt.Errorf("gift stock must be a nonnegative decimal integer")
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("invalid gift stock %q", text)
		}
	}
	stock, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("invalid gift stock: %w", err)
	}
	return stock, nil
}
