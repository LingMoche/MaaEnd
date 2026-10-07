package giftoperator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/jsonclean"
)

// The resource is V2 while GetNodeJSON may return its normalized flat form.
// Both must retain the same operator ID, avatar and multilingual identity.
func TestReadCandidateSupportsResourceAndNormalizedNode(t *testing.T) {
	path := filepath.Join("..", "..", "..", "assets", "resource", "pipeline", "GiftOperator", "Operator", "Operator.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var resources nodeJSONStore
	if err := json.Unmarshal(jsonclean.Clean(raw), &resources); err != nil {
		t.Fatal(err)
	}
	if len(resources) == 0 {
		t.Fatal("operator resource is empty")
	}
	for node, raw := range resources {
		t.Run(node, func(t *testing.T) {
			original, template, err := readCandidate(resources, node)
			if err != nil {
				t.Fatal(err)
			}
			if original.Operator != strings.TrimPrefix(node, "GiftOperatorSelect_") || len(original.Names) == 0 {
				t.Fatalf("operator metadata does not match resource node: %+v", original)
			}
			var v2 struct {
				Recognition struct {
					Param map[string]json.RawMessage `json:"param"`
				} `json:"recognition"`
				Action struct {
					Param map[string]json.RawMessage `json:"param"`
				} `json:"action"`
			}
			if err := json.Unmarshal(raw, &v2); err != nil {
				t.Fatal(err)
			}
			flat := map[string]json.RawMessage{
				"recognition": json.RawMessage(`"TemplateMatch"`),
				"action":      json.RawMessage(`"Custom"`),
			}
			for key, value := range v2.Recognition.Param {
				flat[key] = value
			}
			for key, value := range v2.Action.Param {
				flat[key] = value
			}
			normalized, err := json.Marshal(flat)
			if err != nil {
				t.Fatal(err)
			}
			actual, actualTemplate, err := readCandidate(nodeJSONStore{node: normalized}, node)
			if err != nil || actualTemplate != template || !reflect.DeepEqual(actual, original) {
				t.Fatalf("flat node changed identity: candidate=%+v template=%q error=%v", actual, actualTemplate, err)
			}
		})
	}
}

func TestParseTrustRequiresValidPercentage(t *testing.T) {
	for text, want := range map[string]int{"0%": 0, "101%": 101, "199%": 199, "200%": 200, " 101 ％ ": 101} {
		value, err := parseTrust(text)
		if err != nil || value != want {
			t.Fatalf("parseTrust(%q) = %d, %v; want %d", text, value, err, want)
		}
	}
	for _, text := range []string{"", "101", "201%", "-1%", "1O1%", "101.5%"} {
		if _, err := parseTrust(text); err == nil {
			t.Fatalf("accepted invalid trust %q", text)
		}
	}
}

type nodeJSONStore map[string]json.RawMessage

func (s nodeJSONStore) GetNodeJSON(name string) (string, error) {
	raw, ok := s[name]
	if !ok {
		return "", fmt.Errorf("unknown node %q", name)
	}
	return string(raw), nil
}

func (s nodeJSONStore) OverridePipeline(any) error {
	return fmt.Errorf("readCandidate must not modify the pipeline")
}
