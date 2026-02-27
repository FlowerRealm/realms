package mcp

import "testing"

func boolPtr(v bool) *bool { return &v }

func TestServerV2_EnabledFor_DefaultsTrue(t *testing.T) {
	sv := ServerV2{Transport: "stdio", Stdio: &StdioV2{Command: "echo"}}
	if !sv.EnabledFor(TargetCodex) || !sv.EnabledFor(TargetClaude) || !sv.EnabledFor(TargetGemini) {
		t.Fatalf("expected enabled by default")
	}
}

func TestServerV2_EnabledFor_ExplicitFalse(t *testing.T) {
	sv := ServerV2{
		Transport: "stdio",
		Stdio:     &StdioV2{Command: "echo"},
		Targets:   &ServerTargetsV2{Gemini: boolPtr(false)},
	}
	if !sv.EnabledFor(TargetCodex) || !sv.EnabledFor(TargetClaude) {
		t.Fatalf("expected codex/claude enabled")
	}
	if sv.EnabledFor(TargetGemini) {
		t.Fatalf("expected gemini disabled")
	}
}

func TestStoreV2_Normalize_CleansRedundantTargets(t *testing.T) {
	s := StoreV2{
		Version: 2,
		Servers: map[string]ServerV2{
			"s1": {
				Transport: "stdio",
				Stdio:     &StdioV2{Command: "echo"},
				Targets: &ServerTargetsV2{
					Codex:  boolPtr(true),
					Claude: boolPtr(true),
					Gemini: boolPtr(true),
				},
			},
		},
	}
	n := s.Normalize()
	if n.Servers["s1"].Targets != nil {
		t.Fatalf("expected targets normalized to nil when all true")
	}
}

func TestStoreV2ToRegistryForTarget_FiltersDisabled(t *testing.T) {
	s := StoreV2{
		Version: 2,
		Servers: map[string]ServerV2{
			"s1": {Transport: "stdio", Stdio: &StdioV2{Command: "echo"}},
			"s2": {Transport: "stdio", Stdio: &StdioV2{Command: "echo"}, Targets: &ServerTargetsV2{Claude: boolPtr(false)}},
		},
	}
	regClaude := StoreV2ToRegistryForTarget(s, TargetClaude)
	if _, ok := regClaude["s1"]; !ok {
		t.Fatalf("expected s1 present")
	}
	if _, ok := regClaude["s2"]; ok {
		t.Fatalf("expected s2 filtered for claude")
	}
	regCodex := StoreV2ToRegistryForTarget(s, TargetCodex)
	if _, ok := regCodex["s2"]; !ok {
		t.Fatalf("expected s2 present for codex")
	}
}

func TestDisabledServerIDsForTarget(t *testing.T) {
	s := StoreV2{
		Version: 2,
		Servers: map[string]ServerV2{
			"b": {Transport: "stdio", Stdio: &StdioV2{Command: "echo"}, Targets: &ServerTargetsV2{Gemini: boolPtr(false)}},
			"a": {Transport: "stdio", Stdio: &StdioV2{Command: "echo"}, Targets: &ServerTargetsV2{Gemini: boolPtr(false)}},
			"c": {Transport: "stdio", Stdio: &StdioV2{Command: "echo"}},
		},
	}
	ids := DisabledServerIDsForTarget(s, TargetGemini)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}
