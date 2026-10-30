package arenaofgames

import (
	"encoding/json"
	"testing"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func TestDecideNextRoutesStatus(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   string
	}{
		{"complete", nodeStop},
		{"rejected", nodeAbandon},
		{"continuing", nodeDrawCard},
		{"", nodeDrawCard},
	} {
		if got := decideNext(tc.status); got != tc.want {
			t.Fatalf("decideNext(%q) = %q; want %q", tc.status, got, tc.want)
		}
	}
}

func TestDrawCardOverrideTargetsNextSlot(t *testing.T) {
	for count := 0; count <= 4; count++ {
		override := drawCardOverride(count)
		draw, ok := override[nodeDrawCard].(map[string]any)
		if !ok {
			t.Fatalf("count=%d: draw card override missing", count)
		}
		param, ok := draw["custom_action_param"].(map[string]any)
		if !ok || param["wait_node"] != nodeBackSlotPrefix+string(rune('1'+count)) {
			t.Fatalf("count=%d: wait_node = %v", count, param["wait_node"])
		}
		if param["action"] != "Click" || param["interval_ms"] != 500 || param["repeat_count"] != 10 {
			t.Fatalf("count=%d: custom_action_param must be given in full: %v", count, param)
		}
		wait, ok := override[nodeDrawCardWait].(map[string]any)
		if !ok {
			t.Fatalf("count=%d: draw card success override missing", count)
		}
		recognition, ok := wait["recognition"].(map[string]any)
		if !ok {
			t.Fatalf("count=%d: recognition override missing", count)
		}
		raw, err := json.Marshal(recognition["param"])
		if err != nil {
			t.Fatal(err)
		}
		var reco struct {
			CustomRecognition      string `json:"custom_recognition"`
			CustomRecognitionParam struct {
				Condition string `json:"condition"`
				Count     int    `json:"count"`
			} `json:"custom_recognition_param"`
		}
		if err := json.Unmarshal(raw, &reco); err != nil {
			t.Fatal(err)
		}
		if reco.CustomRecognition != component || reco.CustomRecognitionParam.Condition != "recognized" || reco.CustomRecognitionParam.Count != count+1 {
			t.Fatalf("count=%d: recognition override = %+v", count, reco)
		}
	}
}

func TestUnwrapHandDetailAcceptsRawAndWrappedForms(t *testing.T) {
	detail := handDetail{Count: 3, Category: "pair"}
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string]string{
		"raw":     string(raw),
		"wrapped": `{"best":{"detail":` + string(raw) + `}}`,
	} {
		got, ok := unwrapHandDetail(&maa.RecognitionDetail{DetailJson: payload})
		if !ok || got.Count != detail.Count || got.Category != detail.Category {
			t.Fatalf("%s: unwrapHandDetail = %+v, %v", name, got, ok)
		}
	}
	for name, payload := range map[string]string{
		"empty":   "",
		"garbage": "{",
		"null":    "null",
	} {
		if got, ok := unwrapHandDetail(&maa.RecognitionDetail{DetailJson: payload}); ok {
			t.Fatalf("%s: unwrapHandDetail accepted %q: %+v", name, payload, got)
		}
	}
	if got, ok := unwrapHandDetail(nil); ok {
		t.Fatalf("unwrapHandDetail accepted nil detail: %+v", got)
	}
}
