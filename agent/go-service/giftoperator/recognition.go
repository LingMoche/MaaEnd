package giftoperator

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

var _ maa.CustomRecognitionRunner = &CandidateRecognition{}
var _ maa.CustomRecognitionRunner = &StatusRecognition{}

// CandidateRecognition 使用已有头像模板选择尚未成功送礼或排除的干员。
// 每次只返回一位干员；记录身份及点击由 Pipeline 的后续节点负责。
type CandidateRecognition struct{}

type candidate struct {
	Operator string   `json:"operator"`
	Names    []string `json:"names"`
	Box      maa.Rect `json:"-"`
}

// Run implements maa.CustomRecognitionRunner.
func (r *CandidateRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	if ctx == nil || arg == nil || arg.Img == nil {
		return nil, false
	}
	s, err := loadSession(ctx)
	if err != nil || s.Remaining <= 0 {
		log.Error().Err(err).Msg("gift session unavailable")
		return nil, false
	}
	raw, err := ctx.GetNodeJSON("GiftOperatorSendCandidate")
	if err != nil {
		log.Error().Err(err).Msg("read gift candidates failed")
		return nil, false
	}
	var config struct {
		Attach struct {
			Operators []string `json:"operators"`
			Templates []string `json:"templates"`
		} `json:"attach"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		log.Error().Err(err).Msg("parse gift candidates failed")
		return nil, false
	}
	var best *candidate
	for _, node := range config.Attach.Operators {
		info, template, err := readCandidate(ctx, node)
		if err != nil {
			log.Error().Err(err).Str("node", node).Msg("read gift operator failed")
			return nil, false
		}
		if slices.Contains(s.Completed, info.Operator) || slices.Contains(s.Excluded, info.Operator) {
			continue
		}
		if len(config.Attach.Templates) > 0 && !slices.Contains(config.Attach.Templates, template) {
			continue
		}
		// 记录身份后再次识别同一头像，避免使用上一帧的点击框。
		if arg.CurrentTaskName == "GiftOperatorSendClickCandidate" && info.Operator != s.Pending {
			continue
		}
		detail, err := ctx.RunRecognition("GiftOperatorSelectSpecifiedOp", arg.Img, map[string]any{
			"GiftOperatorSelectSpecifiedOp": map[string]any{"template": []string{template}},
		})
		if err != nil {
			log.Error().Err(err).Str("operator", info.Operator).Msg("match gift operator failed")
			return nil, false
		}
		if detail == nil || !detail.Hit {
			continue
		}
		info.Box = detail.Box
		if best == nil || info.Box[1] < best.Box[1] || (info.Box[1] == best.Box[1] && info.Box[0] < best.Box[0]) {
			best = &info
		}
	}
	if best == nil {
		return nil, false
	}
	encoded, err := json.Marshal(best)
	if err != nil {
		return nil, false
	}
	return &maa.CustomRecognitionResult{Box: best.Box, Detail: string(encoded)}, true
}

// readCandidate 复用收礼身份节点的模板和五语言姓名，不另建一份干员映射。
func readCandidate(store nodeStore, node string) (candidate, string, error) {
	raw, err := store.GetNodeJSON(node)
	if err != nil {
		return candidate{}, "", err
	}
	type identityPatch struct {
		Patch map[string]struct {
			Expected []string `json:"expected"`
		} `json:"patch"`
	}
	var metadata struct {
		Template          []string        `json:"template"`
		CustomActionParam identityPatch   `json:"custom_action_param"`
		Recognition       json.RawMessage `json:"recognition"`
		Action            json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return candidate{}, "", err
	}
	// 同时支持 V2 对象和扁平字段；扁平格式的 recognition/action 是字符串。
	if len(metadata.Recognition) > 0 && metadata.Recognition[0] == '{' {
		var recognition struct {
			Param struct {
				Template []string `json:"template"`
			} `json:"param"`
		}
		if err := json.Unmarshal(metadata.Recognition, &recognition); err != nil {
			return candidate{}, "", err
		}
		metadata.Template = recognition.Param.Template
	}
	if len(metadata.Action) > 0 && metadata.Action[0] == '{' {
		var action struct {
			Param struct {
				CustomActionParam identityPatch `json:"custom_action_param"`
			} `json:"param"`
		}
		if err := json.Unmarshal(metadata.Action, &action); err != nil {
			return candidate{}, "", err
		}
		metadata.CustomActionParam = action.Param.CustomActionParam
	}
	templates := metadata.Template
	names := metadata.CustomActionParam.Patch["GiftOperatorReceiveName"].Expected
	if len(templates) != 1 || len(names) == 0 {
		return candidate{}, "", fmt.Errorf("operator metadata missing in %s", node)
	}
	template := templates[0]
	filename := template[strings.LastIndex(template, "/")+1:]
	id := strings.TrimSuffix(filename, ".png")
	if id == "" {
		return candidate{}, "", fmt.Errorf("operator ID missing in %s", node)
	}
	return candidate{Operator: id, Names: names}, template, nil
}

// StatusRecognition 读取稳定送礼界面的信赖百分比及每日上限文字。
// 选礼物时的临时 toast 不参与成功判定。
type StatusRecognition struct{}

// Run implements maa.CustomRecognitionRunner.
func (r *StatusRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	if ctx == nil || arg == nil || arg.Img == nil {
		return nil, false
	}
	ui, err := ctx.RunRecognition("GiftOperatorGiftUI", arg.Img)
	if err != nil || ui == nil || !ui.Hit {
		return nil, false
	}
	detail, err := ctx.RunRecognition("GiftOperatorGiftTrust", arg.Img)
	if err != nil || detail == nil || !detail.Hit || detail.Results == nil || detail.Results.Best == nil {
		return nil, false
	}
	ocr, ok := detail.Results.Best.AsOCR()
	if !ok {
		return nil, false
	}
	trust, err := parseTrust(ocr.Text)
	if err != nil {
		log.Error().Err(err).Str("text", ocr.Text).Msg("parse gift trust failed")
		return nil, false
	}
	limit, err := ctx.RunRecognition("GiftOperatorDailyLimitText", arg.Img)
	if err != nil {
		log.Error().Err(err).Msg("recognize daily gift limit failed")
		return nil, false
	}
	encoded, err := json.Marshal(struct {
		Trust   int  `json:"trust"`
		Limited bool `json:"limited"`
	}{trust, limit != nil && limit.Hit})
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
