package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type ScannedSkill struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ScanTargetResult struct {
	Target     Target                  `json:"target"`
	Path       string                  `json:"path"`
	Exists     bool                    `json:"exists"`
	ParseError string                  `json:"parse_error,omitempty"`
	SkillCount int                     `json:"skill_count"`
	Skills     map[string]ScannedSkill `json:"skills,omitempty"`
}

func ScanTarget(t Target, root string) ScanTargetResult {
	root = strings.TrimSpace(root)
	out := ScanTargetResult{
		Target: t,
		Path:   root,
		Skills: map[string]ScannedSkill{},
	}
	if root == "" {
		out.ParseError = "path is empty"
		return out
	}
	st, err := os.Stat(root)
	if err != nil || st == nil || !st.IsDir() {
		out.Exists = false
		return out
	}
	out.Exists = true

	switch t {
	case TargetCodex:
		entries, err := os.ReadDir(root)
		if err != nil {
			out.ParseError = err.Error()
			return out
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := strings.TrimSpace(e.Name())
			if name == "" || !IsSafeID(name) {
				continue
			}
			p := filepath.Join(root, name, "SKILL.md")
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			sum := sha256.Sum256([]byte(strings.TrimSpace(string(raw))))
			out.Skills[name] = ScannedSkill{Name: name, Path: p, SHA256: hex.EncodeToString(sum[:])}
		}
	case TargetClaude:
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			name = strings.TrimSpace(name)
			if name == "" || !IsSafeID(name) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256([]byte(strings.TrimSpace(string(raw))))
			out.Skills[name] = ScannedSkill{Name: name, Path: p, SHA256: hex.EncodeToString(sum[:])}
			return nil
		})
	case TargetGemini:
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".toml") {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			name = strings.TrimSpace(name)
			if name == "" || !IsSafeID(name) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			trim := strings.TrimSpace(string(raw))
			if trim == "" {
				return nil
			}
			// Parse best-effort: if invalid TOML, record target parse error but still hash the raw content.
			var m map[string]any
			if err := toml.Unmarshal([]byte(trim), &m); err != nil {
				if out.ParseError == "" {
					out.ParseError = "invalid toml detected"
				}
			}
			sum := sha256.Sum256([]byte(trim))
			out.Skills[name] = ScannedSkill{Name: name, Path: p, SHA256: hex.EncodeToString(sum[:])}
			return nil
		})
	default:
		out.ParseError = "unknown target"
		return out
	}
	out.SkillCount = len(out.Skills)
	return out
}

func ScanAllTargets() (map[Target]ScanTargetResult, error) {
	out := map[Target]ScanTargetResult{}
	for _, t := range []Target{TargetCodex, TargetClaude, TargetGemini} {
		dir, err := ResolveTargetDir(t)
		if err != nil {
			return nil, err
		}
		out[t] = ScanTarget(t, dir)
	}
	return out, nil
}

func ImportFromTarget(t Target, root string) (StoreV1, string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return StoreV1{}, "", errors.New("path is empty")
	}
	st, err := os.Stat(root)
	if err != nil || st == nil || !st.IsDir() {
		return StoreV1{}, "", errors.New("path does not exist")
	}
	s := StoreV1{Version: StoreVersionV1, Skills: map[string]SkillV1{}}

	switch t {
	case TargetCodex:
		entries, err := os.ReadDir(root)
		if err != nil {
			return StoreV1{}, "", err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			id := strings.TrimSpace(e.Name())
			if !IsSafeID(id) {
				continue
			}
			p := filepath.Join(root, id, "SKILL.md")
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(raw))
			if content == "" {
				continue
			}
			s.Skills[id] = SkillV1{ID: id, Title: id, Prompt: content}
		}
	case TargetClaude:
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			id := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			id = strings.TrimSpace(id)
			if !IsSafeID(id) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			content := strings.TrimSpace(string(raw))
			if content == "" {
				return nil
			}
			s.Skills[id] = SkillV1{ID: id, Title: id, Prompt: content}
			return nil
		})
		if err != nil {
			return StoreV1{}, "", err
		}
	case TargetGemini:
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".toml") {
				return nil
			}
			id := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			id = strings.TrimSpace(id)
			if !IsSafeID(id) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			content := strings.TrimSpace(string(raw))
			if content == "" {
				return nil
			}
			var m map[string]any
			if err := toml.Unmarshal([]byte(content), &m); err != nil {
				// fallback: keep raw content as prompt
				s.Skills[id] = SkillV1{ID: id, Title: id, Prompt: content}
				return nil
			}
			title := strings.TrimSpace(stringFromAny(m["title"]))
			if title == "" {
				title = id
			}
			desc := strings.TrimSpace(stringFromAny(m["description"]))
			prompt := strings.TrimSpace(stringFromAny(m["prompt"]))
			if prompt == "" {
				prompt = content
			}
			var descPtr *string
			if desc != "" {
				descPtr = &desc
			}
			s.Skills[id] = SkillV1{ID: id, Title: title, Description: descPtr, Prompt: prompt}
			return nil
		})
		if err != nil {
			return StoreV1{}, "", err
		}
	default:
		return StoreV1{}, "", errors.New("unknown target")
	}

	s = s.Normalize()
	if err := s.Validate(); err != nil {
		return StoreV1{}, "", err
	}
	pretty, err := PrettyStoreV1JSON(s)
	if err != nil {
		return StoreV1{}, "", err
	}
	return s, pretty, nil
}

func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}
