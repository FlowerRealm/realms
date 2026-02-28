package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type AdoptConflict struct {
	ID      string   `json:"id"`
	Targets []Target `json:"targets"`
	Reason  string   `json:"reason"`
}

type AutoAdoptResult struct {
	Store        StoreV1         `json:"store"`
	AdoptedIDs   []string        `json:"adopted_ids"`
	Conflicts    []AdoptConflict `json:"conflicts,omitempty"`
	StoreChanged bool            `json:"store_changed"`
}

// AutoAdoptMissing scans targets and imports scan-only items into the store.
// It is conservative:
// - Only adds items missing in store.
// - Disables targets that are missing OR would be overwritten (render differs from existing content).
func AutoAdoptMissing(store StoreV1, scans map[Target]ScanTargetResult) (AutoAdoptResult, error) {
	store = store.Normalize()
	if err := store.Validate(); err != nil {
		return AutoAdoptResult{}, err
	}
	if store.Skills == nil {
		store.Skills = map[string]SkillV1{}
	}

	idsSet := map[string]struct{}{}
	for _, tr := range scans {
		for id := range tr.Skills {
			id = strings.TrimSpace(id)
			if id == "" || !IsSafeID(id) {
				continue
			}
			if _, ok := store.Skills[id]; ok {
				continue
			}
			idsSet[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(idsSet))
	for id := range idsSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := AutoAdoptResult{Store: store, AdoptedIDs: []string{}, Conflicts: []AdoptConflict{}, StoreChanged: false}
	if len(ids) == 0 {
		return out, nil
	}

	for _, id := range ids {
		// Gather raw contents for each target if present.
		type present struct {
			ok       bool
			path     string
			raw      string
			fileHash string
			title    string
			desc     *string
			prompt   string
		}
		p := map[Target]*present{
			TargetCodex:  {ok: false},
			TargetClaude: {ok: false},
			TargetGemini: {ok: false},
		}
		for _, t := range []Target{TargetCodex, TargetClaude, TargetGemini} {
			tr := scans[t]
			sk, ok := tr.Skills[id]
			if !ok {
				continue
			}
			rawBytes, err := os.ReadFile(sk.Path)
			if err != nil {
				continue
			}
			raw := strings.TrimSpace(string(rawBytes))
			if raw == "" {
				continue
			}
			sum := sha256.Sum256([]byte(raw))
			ps := p[t]
			ps.ok = true
			ps.path = sk.Path
			ps.raw = raw
			ps.fileHash = hex.EncodeToString(sum[:])
			if t == TargetGemini {
				var m map[string]any
				if err := toml.Unmarshal([]byte(raw), &m); err == nil {
					ps.title = strings.TrimSpace(stringFromAny(m["title"]))
					dd := strings.TrimSpace(stringFromAny(m["description"]))
					if dd != "" {
						ps.desc = &dd
					}
					ps.prompt = strings.TrimSpace(stringFromAny(m["prompt"]))
				}
				if ps.prompt == "" {
					ps.prompt = raw
				}
				if ps.title == "" {
					ps.title = id
				}
			}
		}

		// Choose canonical fields.
		title := id
		var desc *string
		prompt := ""
		if p[TargetCodex].ok {
			prompt = p[TargetCodex].raw
		} else if p[TargetClaude].ok {
			prompt = p[TargetClaude].raw
		} else if p[TargetGemini].ok {
			title = strings.TrimSpace(p[TargetGemini].title)
			if title == "" {
				title = id
			}
			desc = p[TargetGemini].desc
			prompt = p[TargetGemini].prompt
		}
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			// No usable source.
			continue
		}

		skill := SkillV1{ID: id, Title: title, Description: desc, Prompt: prompt}

		// Compute per-target enablement based on "would overwrite" + existence.
		confT := []Target{}
		per := &PerTargetV1{}
		disable := func(t Target) {
			b := false
			switch t {
			case TargetCodex:
				per.Codex = &TargetOptionsV1{Enabled: &b}
			case TargetClaude:
				per.Claude = &TargetOptionsV1{Enabled: &b}
			case TargetGemini:
				per.Gemini = &TargetOptionsV1{Enabled: &b}
			}
		}

		for _, t := range []Target{TargetCodex, TargetClaude, TargetGemini} {
			ps := p[t]
			if !ps.ok {
				disable(t)
				continue
			}
			desiredBytes, err := RenderForTarget(skill, t)
			if err != nil {
				disable(t)
				continue
			}
			desired := strings.TrimSpace(string(desiredBytes))
			existing := strings.TrimSpace(ps.raw)
			if desired != existing {
				disable(t)
				confT = append(confT, t)
			}
		}

		if per.Codex != nil || per.Claude != nil || per.Gemini != nil {
			skill.PerTarget = per
		}

		out.Store.Skills[id] = skill
		out.AdoptedIDs = append(out.AdoptedIDs, id)
		out.StoreChanged = true
		if len(confT) > 0 {
			out.Conflicts = append(out.Conflicts, AdoptConflict{ID: id, Targets: confT, Reason: "render differs from existing"})
		}
	}

	if out.StoreChanged {
		out.Store = out.Store.Normalize()
		if err := out.Store.Validate(); err != nil {
			return AutoAdoptResult{}, errors.New("adopt produced invalid store")
		}
	}
	return out, nil
}
