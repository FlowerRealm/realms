package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const StoreVersionV1 = 1

type Target string

const (
	TargetCodex  Target = "codex"
	TargetClaude Target = "claude"
	TargetGemini Target = "gemini"
)

var safeIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func IsSafeID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if strings.Contains(id, "..") {
		return false
	}
	if strings.ContainsAny(id, `/\`) {
		return false
	}
	return safeIDRe.MatchString(id)
}

type StoreV1 struct {
	Version int                `json:"version"`
	Skills  map[string]SkillV1 `json:"skills"`
	Meta    map[string]any     `json:"meta,omitempty"`
	Raw     map[string]any     `json:"-"`
}

type SkillV1 struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Prompt      string  `json:"prompt"`

	InstallAs *InstallAsV1 `json:"install_as,omitempty"`
	PerTarget *PerTargetV1 `json:"per_target,omitempty"`
}

type InstallAsV1 struct {
	Codex  *string `json:"codex,omitempty"`
	Claude *string `json:"claude,omitempty"`
	Gemini *string `json:"gemini,omitempty"`
}

type PerTargetV1 struct {
	Codex  *TargetOptionsV1 `json:"codex,omitempty"`
	Claude *TargetOptionsV1 `json:"claude,omitempty"`
	Gemini *TargetOptionsV1 `json:"gemini,omitempty"`
}

type TargetOptionsV1 struct {
	Enabled *bool `json:"enabled,omitempty"`
}

func (s StoreV1) Normalize() StoreV1 {
	if s.Version == 0 {
		s.Version = StoreVersionV1
	}
	out := StoreV1{Version: s.Version}
	if s.Skills == nil {
		out.Skills = map[string]SkillV1{}
	} else {
		out.Skills = map[string]SkillV1{}
		for k, v := range s.Skills {
			id := strings.TrimSpace(v.ID)
			if id == "" {
				id = strings.TrimSpace(k)
			}
			v.ID = id
			v.Title = strings.TrimSpace(v.Title)
			if v.Title == "" {
				v.Title = v.ID
			}
			if v.Description != nil {
				d := strings.TrimSpace(*v.Description)
				if d == "" {
					v.Description = nil
				} else {
					v.Description = &d
				}
			}
			v.Prompt = strings.TrimSpace(v.Prompt)
			if v.InstallAs != nil {
				v.InstallAs.Codex = normalizeNamePtr(v.InstallAs.Codex)
				v.InstallAs.Claude = normalizeNamePtr(v.InstallAs.Claude)
				v.InstallAs.Gemini = normalizeNamePtr(v.InstallAs.Gemini)
				if v.InstallAs.Codex == nil && v.InstallAs.Claude == nil && v.InstallAs.Gemini == nil {
					v.InstallAs = nil
				}
			}
			if v.PerTarget != nil {
				v.PerTarget.Codex = normalizeTargetOptions(v.PerTarget.Codex)
				v.PerTarget.Claude = normalizeTargetOptions(v.PerTarget.Claude)
				v.PerTarget.Gemini = normalizeTargetOptions(v.PerTarget.Gemini)
				if v.PerTarget.Codex == nil && v.PerTarget.Claude == nil && v.PerTarget.Gemini == nil {
					v.PerTarget = nil
				}
			}
			out.Skills[v.ID] = v
		}
	}
	return out
}

func normalizeTargetOptions(o *TargetOptionsV1) *TargetOptionsV1 {
	if o == nil {
		return nil
	}
	if o.Enabled == nil {
		return nil
	}
	return o
}

func normalizeNamePtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func (s StoreV1) Validate() error {
	if s.Version != StoreVersionV1 {
		return fmt.Errorf("unsupported skills store version: %d", s.Version)
	}
	for id, sk := range s.Skills {
		key := strings.TrimSpace(id)
		if strings.TrimSpace(sk.ID) != "" && strings.TrimSpace(sk.ID) != key {
			return fmt.Errorf("skill id mismatch: key=%q id=%q", key, sk.ID)
		}
		if !IsSafeID(key) {
			return fmt.Errorf("invalid skill id: %q", key)
		}
		title := strings.TrimSpace(sk.Title)
		if title == "" {
			return fmt.Errorf("skill[%s] title is empty", key)
		}
		if strings.TrimSpace(sk.Prompt) == "" {
			return fmt.Errorf("skill[%s] prompt is empty", key)
		}
		for _, nm := range []struct {
			t   Target
			val *string
		}{
			{TargetCodex, nameForTarget(sk.InstallAs, TargetCodex)},
			{TargetClaude, nameForTarget(sk.InstallAs, TargetClaude)},
			{TargetGemini, nameForTarget(sk.InstallAs, TargetGemini)},
		} {
			if nm.val == nil {
				continue
			}
			if !IsSafeID(*nm.val) {
				return fmt.Errorf("skill[%s] invalid install_as.%s: %q", key, nm.t, *nm.val)
			}
		}
	}
	return nil
}

func nameForTarget(ia *InstallAsV1, t Target) *string {
	if ia == nil {
		return nil
	}
	switch t {
	case TargetCodex:
		return ia.Codex
	case TargetClaude:
		return ia.Claude
	case TargetGemini:
		return ia.Gemini
	default:
		return nil
	}
}

func enabledForTarget(sk SkillV1, t Target) bool {
	if sk.PerTarget == nil {
		return true
	}
	var o *TargetOptionsV1
	switch t {
	case TargetCodex:
		o = sk.PerTarget.Codex
	case TargetClaude:
		o = sk.PerTarget.Claude
	case TargetGemini:
		o = sk.PerTarget.Gemini
	}
	if o == nil || o.Enabled == nil {
		return true
	}
	return *o.Enabled
}

func (s StoreV1) IDs() []string {
	ids := make([]string, 0, len(s.Skills))
	for id := range s.Skills {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func ParseStoreV1JSON(raw string) (StoreV1, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return StoreV1{Version: StoreVersionV1, Skills: map[string]SkillV1{}}, nil
	}
	var anyRoot map[string]any
	if err := json.Unmarshal([]byte(raw), &anyRoot); err != nil {
		return StoreV1{}, err
	}
	var s StoreV1
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return StoreV1{}, err
	}
	s.Raw = anyRoot
	s = s.Normalize()
	if err := s.Validate(); err != nil {
		return StoreV1{}, err
	}
	return s, nil
}

func PrettyStoreV1JSON(s StoreV1) (string, error) {
	s = s.Normalize()
	if err := s.Validate(); err != nil {
		return "", err
	}
	type canon struct {
		Version int                `json:"version"`
		Skills  map[string]SkillV1 `json:"skills"`
	}
	b, err := json.MarshalIndent(canon{Version: s.Version, Skills: s.Skills}, "", "  ")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

type TargetEnabledV1 struct {
	Codex  *bool `json:"codex,omitempty"`
	Claude *bool `json:"claude,omitempty"`
	Gemini *bool `json:"gemini,omitempty"`
}

func (t TargetEnabledV1) Effective(target Target) bool {
	switch target {
	case TargetCodex:
		if t.Codex == nil {
			return true
		}
		return *t.Codex
	case TargetClaude:
		if t.Claude == nil {
			return true
		}
		return *t.Claude
	case TargetGemini:
		if t.Gemini == nil {
			return true
		}
		return *t.Gemini
	default:
		return true
	}
}

func ParseTargetEnabledV1JSON(raw string) (TargetEnabledV1, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TargetEnabledV1{}, nil
	}
	var t TargetEnabledV1
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return TargetEnabledV1{}, err
	}
	return t, nil
}

func PrettyTargetEnabledV1JSON(t TargetEnabledV1) (string, error) {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(string(b))
	if out == "null" || out == "" {
		return "{}", nil
	}
	return out, nil
}

var ErrConflicts = errors.New("conflicts detected")
