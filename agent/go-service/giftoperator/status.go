package giftoperator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

var _ maa.CustomRecognitionRunner = &StatusRecognition{}

// StatusRecognition verifies the gift screen before reading trust and daily limit.
// Generic recipients must match the runtime portrait and the previously read name.
type StatusRecognition struct{}

type giftStatus struct {
	Trust         *int   `json:"trust"`
	Limited       *bool  `json:"limited"`
	Name          string `json:"name,omitempty"`
	IdentityMatch bool   `json:"identity_match"`
}

// Run validates recipient identity and returns stable gift-screen status.
func (r *StatusRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	if ctx == nil || arg == nil || arg.Img == nil {
		return nil, false
	}
	ui, err := ctx.RunRecognition("GiftOperatorGiftUI", arg.Img)
	if err != nil || ui == nil || !ui.Hit {
		return nil, false
	}
	s, err := loadSession(ctx)
	if err != nil {
		return nil, false
	}
	status := giftStatus{IdentityMatch: true}
	if s.Generic {
		portrait, ok := pendingPortrait(s)
		if !ok {
			return nil, false
		}
		if err := restorePortrait(ctx, portrait); err != nil {
			log.Error().Err(err).Str("component", "GiftOperatorGiftStatusRecognition").Msg("restore runtime portrait failed")
			return nil, false
		}
		name, err := ctx.RunRecognition("GiftOperatorGiftName", arg.Img)
		if err != nil || name == nil || !name.Hit || len(name.CombinedResult) != 2 {
			return nil, false
		}
		nameText := name.CombinedResult[1]
		if nameText == nil || nameText.Results == nil || nameText.Results.Best == nil {
			return nil, false
		}
		ocr, ok := nameText.Results.Best.AsOCR()
		if !ok || ocr == nil {
			return nil, false
		}
		status.Name = normalizeName(ocr.Text)
		if status.Name == "" {
			return nil, false
		}
		match, err := ctx.RunRecognitionDirect(maa.RecognitionTypeTemplateMatch, &maa.TemplateMatchParam{
			ROI:      maa.NewTargetRect(maa.Rect{1170, 155, 80, 75}),
			Template: giftPortraitTemplates(portrait.Template), Threshold: []float64{0.8},
			Method: maa.TemplateMatchMethodCCOEFF_NORMED,
		}, arg.Img)
		if err != nil {
			log.Error().Err(err).Str("component", "GiftOperatorGiftStatusRecognition").Msg("match runtime gift portrait failed")
			return nil, false
		}
		status.IdentityMatch = match != nil && match.Hit && (portrait.Name == "" || portrait.Name == status.Name)
		log.Info().Str("component", "GiftOperatorGiftStatusRecognition").Str("operator", s.Pending).
			Str("name", status.Name).Bool("identity_match", status.IdentityMatch).Msg("verified gift recipient identity")
		if !status.IdentityMatch {
			if arg.CurrentTaskName == "GiftOperatorSendVerify" {
				return nil, false
			}
			encoded, err := json.Marshal(status)
			if err != nil {
				return nil, false
			}
			return &maa.CustomRecognitionResult{Box: name.Box, Detail: string(encoded)}, true
		}
	}
	detail, err := ctx.RunRecognition("GiftOperatorGiftTrust", arg.Img)
	if err != nil || detail == nil || !detail.Hit || detail.Results == nil || detail.Results.Best == nil {
		return nil, false
	}
	ocr, ok := detail.Results.Best.AsOCR()
	if !ok || ocr == nil {
		return nil, false
	}
	trust, err := parseTrust(ocr.Text)
	if err != nil {
		log.Error().Err(err).Str("text", ocr.Text).Msg("parse gift trust failed")
		return nil, false
	}
	limit, err := ctx.RunRecognition("GiftOperatorDailyLimitText", arg.Img)
	if err != nil {
		return nil, false
	}
	limited := limit != nil && limit.Hit
	status.Trust, status.Limited = &trust, &limited
	encoded, err := json.Marshal(status)
	if err != nil {
		return nil, false
	}
	return &maa.CustomRecognitionResult{Box: detail.Box, Detail: string(encoded)}, true
}

func parseTrust(text string) (int, error) {
	text = strings.Join(strings.Fields(text), "")
	text = strings.ReplaceAll(text, "％", "%")
	if !strings.HasSuffix(text, "%") {
		return 0, fmt.Errorf("trust has no percent sign")
	}
	value, err := strconv.Atoi(strings.TrimSuffix(text, "%"))
	if err != nil || value < 0 || value > 200 {
		return 0, fmt.Errorf("invalid trust percentage %q", text)
	}
	return value, nil
}
