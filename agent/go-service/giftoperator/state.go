package giftoperator

import (
	"encoding/json"
	"fmt"
	"strings"
)

const sessionNode = "GiftOperatorSendMain"

// nodeStore keeps bookkeeping in the current Pipeline context, not process state.
type nodeStore interface {
	GetNodeJSON(string) (string, error)
	OverridePipeline(any) error
}

// portraitEntry links a runtime avatar to names read from the current UI.
// Neither name nor template comes from the maintained operator catalog.
type portraitEntry struct {
	Operator     string `json:"operator"`
	Template     string `json:"template"`
	Image        string `json:"image,omitempty"`
	Name         string `json:"name,omitempty"`
	DialogueName string `json:"dialogue_name,omitempty"`
}

type session struct {
	Generic         bool            `json:"generic"`
	Portraits       []portraitEntry `json:"portraits,omitempty"`
	EncounteredName string          `json:"encountered_name,omitempty"`
	AvoidNames      []string        `json:"avoid_names,omitempty"`
	Remaining       int             `json:"remaining"`
	Completed       []string        `json:"completed"`
	Excluded        []string        `json:"excluded"`
	Pending         string          `json:"pending"`
	BeforeTrust     int             `json:"before_trust"`
	BeforeLimited   bool            `json:"before_limited"`
	Prepared        bool            `json:"prepared"`
}

func initSession(store nodeStore, count int) error {
	if count <= 0 {
		return fmt.Errorf("count must be a positive integer")
	}
	return saveSession(store, session{Remaining: count, Completed: []string{}, Excluded: []string{}})
}

func loadSession(store nodeStore) (session, error) {
	raw, err := store.GetNodeJSON(sessionNode)
	if err != nil {
		return session{}, fmt.Errorf("read gift session: %w", err)
	}
	var node struct {
		Attach struct {
			Session *session `json:"gift_session"`
		} `json:"attach"`
	}
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		return session{}, fmt.Errorf("parse gift session: %w", err)
	}
	if node.Attach.Session == nil {
		return session{}, fmt.Errorf("gift session has not been initialized")
	}
	s := *node.Attach.Session
	if s.Remaining < 0 || s.BeforeTrust < 0 || s.BeforeTrust > 200 {
		return session{}, fmt.Errorf("invalid gift session values")
	}
	return s, nil
}

func saveSession(store nodeStore, s session) error {
	raw, err := store.GetNodeJSON(sessionNode)
	if err != nil {
		return fmt.Errorf("read session node attach: %w", err)
	}
	var node struct {
		Attach map[string]any `json:"attach"`
	}
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		return fmt.Errorf("parse session node attach: %w", err)
	}
	if node.Attach == nil {
		node.Attach = map[string]any{}
	}
	node.Attach["gift_session"] = s
	return store.OverridePipeline(map[string]any{
		sessionNode:                map[string]any{"attach": node.Attach},
		"GiftOperatorSendContinue": map[string]any{"enabled": s.Remaining > 0},
	})
}

func (s *session) reserve(operator string) error {
	operator = strings.TrimSpace(operator)
	if operator == "" {
		return fmt.Errorf("operator is required")
	}
	if s.Remaining == 0 {
		return fmt.Errorf("requested gift recipients have been completed")
	}
	if containsOperator(s.Completed, operator) || containsOperator(s.Excluded, operator) {
		return fmt.Errorf("operator %q has already been completed or excluded", operator)
	}
	s.Generic = false
	s.EncounteredName = ""
	s.AvoidNames = nil
	s.Pending = operator
	s.BeforeTrust = 0
	s.BeforeLimited = false
	s.Prepared = false
	return nil
}

func (s *session) observeBefore(trust int, limited bool) error {
	if s.Pending == "" {
		return fmt.Errorf("no operator is reserved")
	}
	if trust < 0 || trust > 200 {
		return fmt.Errorf("trust must be between 0 and 200")
	}
	s.BeforeTrust = trust
	s.BeforeLimited = limited
	s.Prepared = trust < 200 && !limited
	return nil
}

// commitAfter counts an operator only once and only after trust or daily-limit
// evidence changes from the state observed before submitting the gift.
func (s *session) commitAfter(trust int, limited bool) (bool, error) {
	if trust < 0 || trust > 200 {
		return false, fmt.Errorf("trust must be between 0 and 200")
	}
	if s.Pending == "" {
		return false, fmt.Errorf("no operator is reserved")
	}
	if containsOperator(s.Completed, s.Pending) {
		return false, nil
	}
	if !s.Prepared || s.Remaining == 0 {
		return false, fmt.Errorf("operator was not prepared for gifting")
	}
	if trust < s.BeforeTrust {
		return false, fmt.Errorf("trust decreased between gift observations")
	}
	if trust <= s.BeforeTrust && (s.BeforeLimited || !limited) {
		return false, fmt.Errorf("gift success was not confirmed by trust or daily-limit change")
	}
	s.Completed = append(s.Completed, s.Pending)
	s.Remaining--
	s.Prepared = false
	return true, nil
}

func (s *session) reject() error {
	if s.Pending == "" {
		return fmt.Errorf("no operator is reserved")
	}
	if !containsOperator(s.Completed, s.Pending) && !containsOperator(s.Excluded, s.Pending) {
		s.Excluded = append(s.Excluded, s.Pending)
	}
	s.Prepared = false
	return nil
}

func containsOperator(operators []string, operator string) bool {
	for _, candidate := range operators {
		if candidate == operator {
			return true
		}
	}
	return false
}
