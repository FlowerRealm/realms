package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ConflictAction string

const (
	ConflictKeep      ConflictAction = "keep"
	ConflictOverwrite ConflictAction = "overwrite"
	ConflictRename    ConflictAction = "rename"
)

type Conflict struct {
	ID          string `json:"id"`
	Target      Target `json:"target"`
	Path        string `json:"path"`
	ExistingSHA string `json:"existing_sha256,omitempty"`
	DesiredSHA  string `json:"desired_sha256,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type ConflictResolution struct {
	ID     string         `json:"id"`
	Target Target         `json:"target"`
	Action ConflictAction `json:"action"`
	Name   string         `json:"name,omitempty"` // for rename
}

type ApplyResult struct {
	ID      string `json:"id"`
	Target  Target `json:"target"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
	Changed bool   `json:"changed"`
	Exists  bool   `json:"exists"`
	Error   string `json:"error,omitempty"`
}

type ApplyOutput struct {
	Results      []ApplyResult `json:"apply_results"`
	Conflicts    []Conflict    `json:"conflicts,omitempty"`
	Store        StoreV1       `json:"store,omitempty"` // may be updated by rename resolutions
	StoreChanged bool          `json:"store_changed,omitempty"`
}

type ApplyOptions struct {
	Targets       []Target
	Force         bool
	RemoveIDs     []string
	Resolutions   []ConflictResolution
	TargetEnabled TargetEnabledV1
}

func ApplyStore(store StoreV1, opts ApplyOptions) (ApplyOutput, error) {
	store = store.Normalize()
	if err := store.Validate(); err != nil {
		return ApplyOutput{}, err
	}
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []Target{TargetCodex, TargetClaude, TargetGemini}
	}
	// normalize targets and dedupe
	seen := map[Target]bool{}
	normTargets := make([]Target, 0, 3)
	for _, t := range targets {
		switch t {
		case TargetCodex, TargetClaude, TargetGemini:
			if !seen[t] {
				seen[t] = true
				normTargets = append(normTargets, t)
			}
		}
	}
	targets = normTargets

	resByKey := map[string]ConflictResolution{}
	for _, r := range opts.Resolutions {
		id := strings.TrimSpace(r.ID)
		if id == "" {
			continue
		}
		key := string(r.Target) + ":" + id
		r.ID = id
		r.Name = strings.TrimSpace(r.Name)
		resByKey[key] = r
	}

	removeSet := map[string]bool{}
	for _, id := range opts.RemoveIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			removeSet[id] = true
		}
	}

	out := ApplyOutput{Results: []ApplyResult{}, Conflicts: []Conflict{}, Store: store, StoreChanged: false}

	for _, t := range targets {
		root, err := ResolveTargetDir(t)
		if err != nil {
			out.Results = append(out.Results, ApplyResult{Target: t, Enabled: false, Error: err.Error()})
			continue
		}
		root = strings.TrimSpace(root)
		if root == "" {
			out.Results = append(out.Results, ApplyResult{Target: t, Enabled: false, Error: "path is empty"})
			continue
		}

		enabledGlobal := opts.TargetEnabled.Effective(t)
		if !enabledGlobal {
			// Still allow removals even when disabled (explicit action).
			if len(removeSet) > 0 {
				out.Results = append(out.Results, applyRemovals(t, root, store, removeSet)...)
			}
			continue
		}

		if err := os.MkdirAll(root, 0o755); err != nil {
			out.Results = append(out.Results, ApplyResult{Target: t, Enabled: false, Error: err.Error()})
			continue
		}

		if len(removeSet) > 0 {
			out.Results = append(out.Results, applyRemovals(t, root, store, removeSet)...)
		}

		ids := store.IDs()
		for _, id := range ids {
			sk := store.Skills[id]
			sk.ID = id
			if !enabledForTarget(sk, t) {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: id, Enabled: false})
				continue
			}
			name := id
			if v := nameForTarget(sk.InstallAs, t); v != nil && strings.TrimSpace(*v) != "" {
				name = strings.TrimSpace(*v)
			}
			if !IsSafeID(name) {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Enabled: true, Error: "invalid name"})
				continue
			}
			dstPath, err := targetPath(t, root, name)
			if err != nil {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Enabled: true, Error: err.Error()})
				continue
			}
			desired, err := RenderForTargetInDir(sk, t, root)
			if err != nil {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Error: err.Error()})
				continue
			}
			desiredTrim := strings.TrimSpace(string(desired))
			desiredSum := sha256.Sum256([]byte(desiredTrim))
			desiredSHA := hex.EncodeToString(desiredSum[:])

			exists, same, existingSHA, reason, readErr := compareExisting(t, dstPath, desiredTrim)
			if readErr != nil {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: exists, Error: readErr.Error()})
				continue
			}

			if exists && !same {
				key := string(t) + ":" + id
				res, hasRes := resByKey[key]
				if opts.Force {
					hasRes = true
					res = ConflictResolution{ID: id, Target: t, Action: ConflictOverwrite}
				}
				if !hasRes {
					out.Conflicts = append(out.Conflicts, Conflict{
						ID:          id,
						Target:      t,
						Path:        dstPath,
						ExistingSHA: existingSHA,
						DesiredSHA:  desiredSHA,
						Reason:      reason,
					})
					out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: true, Changed: false, Error: "conflict"})
					continue
				}
				switch res.Action {
				case ConflictKeep:
					out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: true, Changed: false})
					continue
				case ConflictRename:
					newName := strings.TrimSpace(res.Name)
					if !IsSafeID(newName) {
						out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: true, Error: "invalid rename name"})
						continue
					}
					newPath, err := targetPath(t, root, newName)
					if err != nil {
						out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: newName, Enabled: true, Error: err.Error()})
						continue
					}
					// re-check new path conflict
					ex2, same2, ex2SHA, reason2, readErr2 := compareExisting(t, newPath, desiredTrim)
					if readErr2 != nil {
						out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: newName, Path: newPath, Enabled: true, Exists: ex2, Error: readErr2.Error()})
						continue
					}
					if ex2 && !same2 && !opts.Force {
						out.Conflicts = append(out.Conflicts, Conflict{
							ID:          id,
							Target:      t,
							Path:        newPath,
							ExistingSHA: ex2SHA,
							DesiredSHA:  desiredSHA,
							Reason:      "rename target conflict: " + reason2,
						})
						out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: newName, Path: newPath, Enabled: true, Exists: true, Error: "conflict"})
						continue
					}
					changed, err := writeTarget(t, newPath, []byte(desiredTrim+"\n"))
					if err != nil {
						out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: newName, Path: newPath, Enabled: true, Exists: ex2, Error: err.Error()})
						continue
					}
					out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: newName, Path: newPath, Enabled: true, Exists: ex2, Changed: changed})
					// update store install_as for this target
					store2 := out.Store
					sk2 := store2.Skills[id]
					if sk2.InstallAs == nil {
						sk2.InstallAs = &InstallAsV1{}
					}
					switch t {
					case TargetCodex:
						sk2.InstallAs.Codex = &newName
					case TargetClaude:
						sk2.InstallAs.Claude = &newName
					case TargetGemini:
						sk2.InstallAs.Gemini = &newName
					}
					store2.Skills[id] = sk2
					out.Store = store2.Normalize()
					out.StoreChanged = true
					continue
				case ConflictOverwrite:
					// fallthrough to write
				default:
					out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: true, Error: "unknown conflict action"})
					continue
				}
			}

			if exists && same {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: true, Changed: false})
				continue
			}

			changed, err := writeTarget(t, dstPath, []byte(desiredTrim+"\n"))
			if err != nil {
				out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: exists, Error: err.Error()})
				continue
			}
			out.Results = append(out.Results, ApplyResult{ID: id, Target: t, Name: name, Path: dstPath, Enabled: true, Exists: exists, Changed: changed})
		}
	}

	if len(out.Conflicts) > 0 {
		sort.Slice(out.Conflicts, func(i, j int) bool {
			if out.Conflicts[i].Target != out.Conflicts[j].Target {
				return out.Conflicts[i].Target < out.Conflicts[j].Target
			}
			return out.Conflicts[i].ID < out.Conflicts[j].ID
		})
		return out, ErrConflicts
	}
	return out, nil
}

func targetPath(t Target, root string, name string) (string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	name = strings.TrimSpace(name)
	if root == "" || name == "" {
		return "", errors.New("empty path")
	}
	var p string
	switch t {
	case TargetCodex:
		p = filepath.Join(root, name, "SKILL.md")
	case TargetClaude:
		if claudeUsesSkillsLayout(root) {
			p = filepath.Join(root, name, "SKILL.md")
		} else {
			p = filepath.Join(root, name+".md")
		}
	case TargetGemini:
		p = filepath.Join(root, name+".toml")
	default:
		return "", fmt.Errorf("unknown target: %s", t)
	}
	if !withinDir(root, p) {
		return "", errors.New("path traversal detected")
	}
	return p, nil
}

func claudeUsesSkillsLayout(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return false
	}
	switch strings.ToLower(filepath.Base(root)) {
	case "skills":
		return true
	case "commands":
		return false
	default:
		// Keep backward-compatible default.
		return false
	}
}

func compareExisting(t Target, path string, desiredTrim string) (exists bool, same bool, existingSHA string, reason string, err error) {
	st, statErr := os.Stat(path)
	if statErr != nil || st == nil {
		return false, false, "", "", nil
	}
	if st.IsDir() {
		return true, false, "", "existing is a directory", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return true, false, "", "", err
	}
	exTrim := strings.TrimSpace(string(raw))
	sum := sha256.Sum256([]byte(exTrim))
	existingSHA = hex.EncodeToString(sum[:])
	if exTrim == desiredTrim {
		return true, true, existingSHA, "", nil
	}
	return true, false, existingSHA, "content differs", nil
}

func writeTarget(t Target, path string, data []byte) (changed bool, err error) {
	before, ok, err := readFileIfExists(path)
	if err != nil {
		return false, err
	}
	if ok && strings.TrimSpace(string(before)) == strings.TrimSpace(string(data)) {
		return false, nil
	}
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func readFileIfExists(path string) ([]byte, bool, error) {
	st, err := os.Stat(path)
	if err != nil || st == nil {
		return nil, false, nil
	}
	if st.IsDir() {
		return nil, true, errors.New("path is a directory")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, true, err
	}
	return b, true, nil
}

func targetUsesSkillsDirLayout(t Target, root string) bool {
	return t == TargetCodex || (t == TargetClaude && claudeUsesSkillsLayout(root))
}

func applyRemovals(t Target, root string, store StoreV1, removeSet map[string]bool) []ApplyResult {
	out := []ApplyResult{}
	ids := make([]string, 0, len(removeSet))
	for id := range removeSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		sk, ok := store.Skills[id]
		if ok {
			name := id
			if v := nameForTarget(sk.InstallAs, t); v != nil && strings.TrimSpace(*v) != "" {
				name = strings.TrimSpace(*v)
			}
			if !IsSafeID(name) {
				out = append(out, ApplyResult{ID: id, Target: t, Name: name, Enabled: false, Error: "invalid name"})
				continue
			}
			if targetUsesSkillsDirLayout(t, root) {
				dir := filepath.Join(root, name)
				if !withinDir(root, dir) {
					out = append(out, ApplyResult{ID: id, Target: t, Name: name, Enabled: false, Error: "path traversal detected"})
					continue
				}
				if err := os.RemoveAll(dir); err != nil {
					out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: dir, Enabled: false, Exists: true, Error: err.Error()})
				} else {
					out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: dir, Enabled: false, Changed: true})
				}
				continue
			}
			p, err := targetPath(t, root, name)
			if err != nil {
				out = append(out, ApplyResult{ID: id, Target: t, Name: name, Enabled: false, Error: err.Error()})
				continue
			}
			if err := os.Remove(p); err != nil {
				if os.IsNotExist(err) {
					out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: p, Enabled: false, Exists: false})
				} else {
					out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: p, Enabled: false, Exists: true, Error: err.Error()})
				}
			} else {
				out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: p, Enabled: false, Exists: true, Changed: true})
			}
		} else {
			// best-effort: try removing by id name
			name := id
			if !IsSafeID(name) {
				out = append(out, ApplyResult{ID: id, Target: t, Name: name, Enabled: false, Error: "invalid id"})
				continue
			}
			if targetUsesSkillsDirLayout(t, root) {
				dir := filepath.Join(root, name)
				_ = os.RemoveAll(dir)
				out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: dir, Enabled: false, Changed: true})
				continue
			}
			p, err := targetPath(t, root, name)
			if err != nil {
				out = append(out, ApplyResult{ID: id, Target: t, Name: name, Enabled: false, Error: err.Error()})
				continue
			}
			_ = os.Remove(p)
			out = append(out, ApplyResult{ID: id, Target: t, Name: name, Path: p, Enabled: false, Changed: true})
		}
	}
	return out
}
