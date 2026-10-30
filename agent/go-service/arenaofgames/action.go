package arenaofgames

import (
	"encoding/json"
	"image"
	"strconv"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

// —— Decide 动作（借鉴 trialofswordmancy/action.go 的单步决策模式）——

var _ maa.CustomActionRunner = &DecideAction{}

// DecideAction 消费 ArenaOfGames.CheckHand 识别产出的手牌明细（识别每步只跑一次，本动作不重复读牌），
// 对照勾选目标算出三态并用 OverrideNext 路由：凑成即结束（早停）、无法凑出转放弃链、否则转抽牌。
// 流程仍归 pipeline：点击与等动画由执行节点（DrawCard / Abandon 链）完成，Go 只做判定与路由。
type DecideAction struct{}

// 路由与执行节点名（定义在 ArenaOfGames.json / ArenaOfGamesCommon.json）。
const (
	decideComponent    = "ArenaOfGames.Decide"
	nodeDecide         = "ArenaOfGamesDecide"
	nodeStop           = "ArenaOfGamesStop"
	nodeDrawCard       = "ArenaOfGamesDrawCard"
	nodeDrawCardWait   = "ArenaOfGamesDrawCardSuccess"
	nodeAbandon        = "ArenaOfGamesAbandon"
	nodeBackSlotPrefix = "ArenaOfGamesBackSlot" // + "1".."5"：空槽背面，抽牌落位的等待锚点
)

// Run 解析手牌明细、判定三态并路由。
func (a *DecideAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	if ctx == nil || arg == nil {
		log.Error().Str("component", component).Msg("decide action context or arg is nil")
		return false
	}
	detail, ok := unwrapHandDetail(arg.RecognitionDetail)
	if !ok {
		log.Error().Str("component", component).Msg("decide action got no hand detail")
		return false
	}
	img := cachedImage(ctx)
	if img == nil {
		log.Error().Str("component", component).Msg("decide action got no cached image")
		return false
	}
	targets, err := resolveTargets(ctx, img)
	if err != nil {
		log.Warn().Err(err).Str("component", component).Msg("target hands could not be resolved")
		return false
	}
	status := evaluateHand(handCounts(detail), detail.Count, targets)

	if status == "continuing" {
		// 先覆盖抽牌锚点再路由：两节点每次必经本动作，执行时必为最新定义。
		if err := ctx.OverridePipeline(drawCardOverride(detail.Count)); err != nil {
			log.Error().Err(err).Str("component", component).Msg("override draw card nodes failed")
			return false
		}
	}
	if err := ctx.OverrideNext(nodeDecide, []maa.NextItem{{Name: decideNext(status)}}); err != nil {
		log.Error().Err(err).Str("component", component).Str("status", status).Msg("failed to route decision")
		return false
	}
	log.Info().
		Str("component", component).
		Int("count", detail.Count).
		Str("category", detail.Category).
		Str("status", status).
		Str("next", decideNext(status)).
		Msg("decision made")
	return true
}

// decideNext 把三态映射到执行节点：complete → 结束；rejected → 放弃链；continuing → 抽牌。
func decideNext(status string) string {
	switch status {
	case "complete":
		return nodeStop
	case "rejected":
		return nodeAbandon
	default:
		return nodeDrawCard
	}
}

// drawCardOverride 按当前手牌数覆盖抽牌的两个锚点：
//   - DrawCard.custom_action_param.wait_node → BackSlot<手牌数+1>（该槽背面消失 = 新牌已抽出）
//   - DrawCardSuccess 的 recognized count → 手牌数+1（确认新牌完整可读）
//
// 注：override 为字段级浅合并，custom_action_param 与 recognition 必须全量给出，
// 与 ArenaOfGames.json 的 DrawCard / DrawCardSuccess 定义同步维护。
func drawCardOverride(count int) map[string]any {
	return map[string]any{
		nodeDrawCard: map[string]any{
			"custom_action_param": map[string]any{
				"action":       "Click",
				"interval_ms":  500,
				"repeat_count": 10,
				"wait_node":    nodeBackSlotPrefix + strconv.Itoa(count+1),
			},
		},
		nodeDrawCardWait: map[string]any{
			"recognition": map[string]any{
				"type": "Custom",
				"param": map[string]any{
					"custom_recognition": component,
					"custom_recognition_param": map[string]any{
						"condition": "recognized",
						"count":     count + 1,
					},
				},
			},
		},
	}
}

// cachedImage 取框架缓存的截图——Decide 的识别刚在同一帧上完成，无需再截。
func cachedImage(ctx *maa.Context) image.Image {
	tasker := ctx.GetTasker()
	if tasker == nil {
		return nil
	}
	ctrl := tasker.GetController()
	if ctrl == nil {
		return nil
	}
	img, err := ctrl.CacheImage()
	if err != nil || img == nil {
		return nil
	}
	return img
}

// unwrapHandDetail 从 CheckHand 识别的 DetailJson 中取回手牌明细。
// 框架可能把 Custom 识别的 detail 包成 {"best":{"detail": <raw>}}，两种形态都兼容。
func unwrapHandDetail(detail *maa.RecognitionDetail) (handDetail, bool) {
	if detail == nil || detail.DetailJson == "" {
		return handDetail{}, false
	}
	raw := json.RawMessage(detail.DetailJson)
	var wrapped struct {
		Best struct {
			Detail json.RawMessage `json:"detail"`
		} `json:"best"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Best.Detail) > 0 {
		raw = wrapped.Best.Detail
	}
	var parsed *handDetail
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed == nil {
		return handDetail{}, false
	}
	return *parsed, true
}
