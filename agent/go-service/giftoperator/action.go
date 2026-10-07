package giftoperator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

// SessionAction records recipient selection and confirmed gift results.
// Pipeline remains responsible for every UI action and transition.
type SessionAction struct{}

var _ maa.CustomActionRunner = &SessionAction{}

type sessionActionParam struct {
	Operation string `json:"operation"`
	Count     *int   `json:"count,omitempty"`
}

// Run updates the current task's gift session without operating the game UI.
func (a *SessionAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	if ctx == nil || arg == nil {
		log.Error().Str("component", "GiftOperatorSessionAction").Msg("nil context or action argument")
		return false
	}
	var param sessionActionParam
	if err := json.Unmarshal([]byte(arg.CustomActionParam), &param); err != nil {
		log.Error().Err(err).Str("component", "GiftOperatorSessionAction").Msg("failed to parse action parameters")
		return false
	}
	if err := updateSession(ctx, param, arg.RecognitionDetail); err != nil {
		log.Error().Err(err).Str("component", "GiftOperatorSessionAction").Str("operation", param.Operation).Msg("failed to update gift session")
		return false
	}
	return true
}

func updateSession(store nodeStore, param sessionActionParam, detail *maa.RecognitionDetail) error {
	if param.Operation == "init" {
		count := 1
		if param.Count != nil {
			count = *param.Count
		}
		return initSession(store, count)
	}
	if param.Count != nil {
		return fmt.Errorf("count is only supported for init")
	}
	s, err := loadSession(store)
	if err != nil {
		return err
	}
	switch param.Operation {
	case "reserve":
		var candidate struct {
			Operator string   `json:"operator"`
			Names    []string `json:"names"`
		}
		if err := decodeCustomDetail(detail, &candidate); err != nil {
			return err
		}
		expected := make([]string, 0, len(candidate.Names))
		for _, name := range candidate.Names {
			if name = strings.TrimSpace(name); name != "" {
				expected = append(expected, "^"+regexp.QuoteMeta(name)+"$")
			}
		}
		if len(expected) == 0 {
			return fmt.Errorf("candidate operator names are required")
		}
		if err := s.reserve(candidate.Operator); err != nil {
			return err
		}
		if err := store.OverridePipeline(map[string]any{
			"GiftOperatorName": map[string]any{"expected": expected},
		}); err != nil {
			return fmt.Errorf("restrict dialogue to reserved operator: %w", err)
		}
	case "observe_before", "observe_after":
		var status struct {
			Trust   *int  `json:"trust"`
			Limited *bool `json:"limited"`
		}
		if err := decodeCustomDetail(detail, &status); err != nil {
			return err
		}
		if status.Trust == nil || status.Limited == nil {
			return fmt.Errorf("gift status must include trust and limited")
		}
		if param.Operation == "observe_before" {
			if err := s.observeBefore(*status.Trust, *status.Limited); err != nil {
				return err
			}
			if err := store.OverridePipeline(map[string]any{
				"GiftOperatorSendCanGift":     map[string]any{"enabled": s.Prepared},
				"GiftOperatorSendAlreadyFull": map[string]any{"enabled": !s.Prepared},
			}); err != nil {
				return fmt.Errorf("update gift eligibility: %w", err)
			}
		} else {
			committed, err := s.commitAfter(*status.Trust, *status.Limited)
			if err != nil {
				return err
			}
			log.Info().Str("component", "GiftOperatorSessionAction").Str("operator", s.Pending).
				Int("remaining", s.Remaining).Bool("committed", committed).Msg("confirmed gift result")
		}
	case "reject":
		if err := s.reject(); err != nil {
			return err
		}
	case "finish":
		log.Info().Str("component", "GiftOperatorSessionAction").
			Int("completed", len(s.Completed)).Int("remaining", s.Remaining).
			Strs("completed_operators", s.Completed).Strs("excluded_operators", s.Excluded).
			Msg("gift recipient session finished")
		if s.Remaining > 0 {
			return fmt.Errorf("gift candidates exhausted with %d recipients remaining", s.Remaining)
		}
	default:
		return fmt.Errorf("unsupported operation %q", param.Operation)
	}
	return saveSession(store, s)
}

func decodeCustomDetail(detail *maa.RecognitionDetail, target any) error {
	if detail == nil || detail.Results == nil || detail.Results.Best == nil {
		return fmt.Errorf("custom recognition detail is missing")
	}
	custom, ok := detail.Results.Best.AsCustom()
	if !ok || custom == nil {
		return fmt.Errorf("expected a custom recognition result")
	}
	if err := json.Unmarshal([]byte(custom.Detail), target); err != nil {
		return fmt.Errorf("parse custom recognition detail: %w", err)
	}
	return nil
}
