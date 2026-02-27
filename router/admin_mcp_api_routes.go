package router

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/mcp"
	"realms/internal/store"
)

func setAdminMCPAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/mcp", adminMCPGetHandler(opts))
	r.GET("/mcp/scan", adminMCPScanHandler(opts))
	r.POST("/mcp/import", adminMCPImportHandler(opts))
	r.POST("/mcp/parse", adminMCPParseHandler(opts))
	r.POST("/mcp/delete", adminMCPDeleteHandler(opts))
	r.PUT("/mcp", adminMCPPutHandler(opts))
	r.POST("/mcp/apply", adminMCPApplyHandler(opts))
	r.GET("/mcp/export/claude", adminMCPExportClaudeHandler(opts))
	r.GET("/mcp/export/gemini", adminMCPExportGeminiHandler(opts))
	r.GET("/mcp/export/codex", adminMCPExportCodexHandler(opts))
}

type adminMCPPutReq struct {
	// StoreJSON is Realms canonical MCP store (v2) JSON (object).
	// Prefer `store` when provided; this string field exists for simple clients.
	StoreJSON string          `json:"store_json"`
	Store     json.RawMessage `json:"store"`

	TargetEnabled map[string]bool `json:"target_enabled"`

	ApplyOnSave *bool `json:"apply_on_save"`
	Force       *bool `json:"force"`
}

type adminMCPApplyReq struct {
	Targets   []string `json:"targets"`
	RemoveIDs []string `json:"remove_ids"`
	Force     *bool    `json:"force"`
}

type adminMCPImportReq struct {
	Source     string `json:"source"`
	Mode       string `json:"mode"`
	ApplyAfter bool   `json:"apply_after"`
	Force      *bool  `json:"force"`
}

type adminMCPParseReq struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

type adminMCPDeleteReq struct {
	ID      string   `json:"id"`
	Targets []string `json:"targets"`
	Force   *bool    `json:"force"`
}

type mcpTargetState struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
}

func adminMCPGetHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		ctx := c.Request.Context()

		storeV2, parseErr, migrated, err := loadMCPStoreV2(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}

		targets := make(map[string]mcpTargetState, 3)
		for _, t := range []mcp.Target{mcp.TargetCodex, mcp.TargetClaude, mcp.TargetGemini} {
			path, _ := mcp.ResolveTargetPath(t)
			exists := false
			if strings.TrimSpace(path) != "" {
				if st, err := os.Stat(path); err == nil && st != nil && !st.IsDir() {
					exists = true
				}
			}
			enabled := true
			switch t {
			case mcp.TargetCodex:
				if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingMCPApplyCodexEnabled); err == nil && ok {
					enabled = v
				}
			case mcp.TargetClaude:
				if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingMCPApplyClaudeEnabled); err == nil && ok {
					enabled = v
				}
			case mcp.TargetGemini:
				if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingMCPApplyGeminiEnabled); err == nil && ok {
					enabled = v
				}
			}
			targets[string(t)] = mcpTargetState{Enabled: enabled, Path: path, Exists: exists}
		}

		storeJSON := "{}"
		if pretty, err := mcp.PrettyStoreV2JSON(storeV2); err == nil && strings.TrimSpace(pretty) != "" {
			storeJSON = pretty
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":           storeJSON,
				"server_count":         len(storeV2.Servers),
				"parse_error":          parseErr,
				"migrated_from_legacy": migrated,
				"targets":              targets,
				"store":                storeV2,
			},
		})
	}
}

func adminMCPScanHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		targets := map[string]gin.H{}
		for _, t := range []mcp.Target{mcp.TargetCodex, mcp.TargetClaude, mcp.TargetGemini} {
			path, _ := mcp.ResolveTargetPath(t)
			scan := mcp.ScanTarget(t, path)
			sv2 := mcp.StoreV2FromRegistry(scan.Servers)
			targets[string(t)] = gin.H{
				"target":       scan.Target,
				"path":         scan.Path,
				"exists":       scan.Exists,
				"server_count": len(sv2.Servers),
				"parse_error":  scan.ParseError,
				"servers":      sv2.Servers,
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"targets": targets,
			},
		})
	}
}

func adminMCPImportHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		mut, ok := beginPersonalConfigMutation(c, opts)
		if !ok {
			return
		}
		finalized := false
		defer func() {
			if mut != nil && !finalized {
				abortPersonalConfigMutation(c, mut)
			}
		}()

		var req adminMCPImportReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		source := strings.TrimSpace(strings.ToLower(req.Source))
		mode := strings.TrimSpace(strings.ToLower(req.Mode))
		if mode == "" {
			mode = "merge"
		}
		if source != string(mcp.TargetCodex) && source != string(mcp.TargetClaude) && source != string(mcp.TargetGemini) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的 source"})
			return
		}
		if mode != "merge" && mode != "replace" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的 mode"})
			return
		}
		force := false
		if req.Force != nil {
			force = *req.Force
		}

		ctx := c.Request.Context()
		cur, _, _, err := loadMCPStoreV2(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}

		// Scan source.
		t := mcp.Target(source)
		path, _ := mcp.ResolveTargetPath(t)
		scan := mcp.ScanTarget(t, path)
		if scan.ParseError != "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "扫描失败"})
			return
		}
		imported := mcp.StoreV2FromRegistry(scan.Servers)

		var next mcp.StoreV2
		switch mode {
		case "replace":
			next = imported
		default:
			next = cur
			if next.Servers == nil {
				next.Servers = map[string]mcp.ServerV2{}
			}
			for id, sv := range imported.Servers {
				next.Servers[id] = sv
			}
		}

		if err := saveMCPStoreV2(ctx, opts.Store, next); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
			return
		}

		applyResults := []mcp.ApplyResult{}
		if req.ApplyAfter {
			applyResults = applyMCPConfigs(ctx, opts, next, nil, nil, force)
		}

		if mut != nil {
			if err := mut.Commit(ctx, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
				return
			}
		}
		finalized = true

		outJSON, _ := mcp.PrettyStoreV2JSON(next)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":   outJSON,
				"server_count": len(next.Servers),
				"store":        next,
				"imported_from": gin.H{
					"source": source,
					"path":   scan.Path,
					"count":  scan.ServerCount,
				},
				"apply_results": applyResults,
			},
		})
	}
}

func adminMCPParseHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminMCPParseReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		source := strings.TrimSpace(strings.ToLower(req.Source))
		if source == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "source 不能为空"})
			return
		}
		if source != "codex" && source != "claude" && source != "gemini" && source != "realms" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的 source"})
			return
		}
		content := strings.TrimSpace(req.Content)
		if content == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "content 不能为空"})
			return
		}

		storeV2, err := mcp.ParseTargetContentToStoreV2(source, content)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "解析失败: " + err.Error()})
			return
		}
		outJSON, err := mcp.PrettyStoreV2JSON(storeV2)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "解析失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":   outJSON,
				"server_count": len(storeV2.Servers),
				"store":        storeV2,
			},
		})
	}
}

func adminMCPPutHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		mut, ok := beginPersonalConfigMutation(c, opts)
		if !ok {
			return
		}
		finalized := false
		defer func() {
			if mut != nil && !finalized {
				abortPersonalConfigMutation(c, mut)
			}
		}()

		var req adminMCPPutReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		var storeV2 mcp.StoreV2
		raw := strings.TrimSpace(req.StoreJSON)
		if raw != "" {
			parsed, err := mcp.ParseStoreV2JSON(raw)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "mcp store JSON 不合法"})
				return
			}
			storeV2 = parsed
		} else if len(req.Store) > 0 {
			parsed, err := mcp.ParseStoreV2JSON(string(req.Store))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "mcp store JSON 不合法"})
				return
			}
			storeV2 = parsed
		} else {
			storeV2 = mcp.StoreV2{Version: 2, Servers: map[string]mcp.ServerV2{}}
		}
		storeV2 = storeV2.Normalize()

		ctx := c.Request.Context()

		// Update target toggles (optional).
		if len(req.TargetEnabled) > 0 {
			for k, v := range req.TargetEnabled {
				switch strings.TrimSpace(strings.ToLower(k)) {
				case string(mcp.TargetCodex):
					if err := opts.Store.UpsertBoolAppSetting(ctx, store.SettingMCPApplyCodexEnabled, v); err != nil {
						c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
						return
					}
				case string(mcp.TargetClaude):
					if err := opts.Store.UpsertBoolAppSetting(ctx, store.SettingMCPApplyClaudeEnabled, v); err != nil {
						c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
						return
					}
				case string(mcp.TargetGemini):
					if err := opts.Store.UpsertBoolAppSetting(ctx, store.SettingMCPApplyGeminiEnabled, v); err != nil {
						c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
						return
					}
				}
			}
		}

		if err := saveMCPStoreV2(ctx, opts.Store, storeV2); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
			return
		}

		applyOnSave := true
		if req.ApplyOnSave != nil {
			applyOnSave = *req.ApplyOnSave
		}
		force := false
		if req.Force != nil {
			force = *req.Force
		}

		applyResults := []mcp.ApplyResult{}
		if applyOnSave {
			applyResults = applyMCPConfigs(ctx, opts, storeV2, nil, nil, force)
		}

		if mut != nil {
			if err := mut.Commit(ctx, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
				return
			}
		}
		finalized = true

		storeJSON := "{}"
		if pretty, err := mcp.PrettyStoreV2JSON(storeV2); err == nil && strings.TrimSpace(pretty) != "" {
			storeJSON = pretty
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":    storeJSON,
				"server_count":  len(storeV2.Servers),
				"store":         storeV2,
				"apply_results": applyResults,
			},
		})
	}
}

func adminMCPApplyHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req adminMCPApplyReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		force := false
		if req.Force != nil {
			force = *req.Force
		}

		sv2, _, _, err := loadMCPStoreV2(c.Request.Context(), opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		results := applyMCPConfigs(c.Request.Context(), opts, sv2, req.Targets, req.RemoveIDs, force)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"apply_results": results,
			},
		})
	}
}

func applyMCPConfigs(ctx context.Context, opts Options, storeV2 mcp.StoreV2, targets []string, removeIDs []string, force bool) []mcp.ApplyResult {
	want := map[mcp.Target]bool{
		mcp.TargetCodex:  true,
		mcp.TargetClaude: true,
		mcp.TargetGemini: true,
	}
	if len(targets) > 0 {
		for k := range want {
			want[k] = false
		}
		for _, t := range targets {
			switch strings.TrimSpace(strings.ToLower(t)) {
			case string(mcp.TargetCodex):
				want[mcp.TargetCodex] = true
			case string(mcp.TargetClaude):
				want[mcp.TargetClaude] = true
			case string(mcp.TargetGemini):
				want[mcp.TargetGemini] = true
			}
		}
	}

	enabled := map[mcp.Target]bool{
		mcp.TargetCodex:  true,
		mcp.TargetClaude: true,
		mcp.TargetGemini: true,
	}
	if opts.Store != nil {
		if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingMCPApplyCodexEnabled); err == nil && ok {
			enabled[mcp.TargetCodex] = v
		}
		if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingMCPApplyClaudeEnabled); err == nil && ok {
			enabled[mcp.TargetClaude] = v
		}
		if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingMCPApplyGeminiEnabled); err == nil && ok {
			enabled[mcp.TargetGemini] = v
		}
	}

	out := make([]mcp.ApplyResult, 0, 3)
	for _, t := range []mcp.Target{mcp.TargetCodex, mcp.TargetClaude, mcp.TargetGemini} {
		if !want[t] {
			continue
		}
		regT := mcp.StoreV2ToRegistryForTarget(storeV2, t)
		disabledIDs := mcp.DisabledServerIDsForTarget(storeV2, t)
		mergedRemoveIDs := mergeRemoveIDs(removeIDs, disabledIDs)

		path, err := mcp.ResolveTargetPath(t)
		res := mcp.ApplyResult{Target: t, Path: path, Enabled: enabled[t]}
		if strings.TrimSpace(path) != "" {
			if st, err2 := os.Stat(path); err2 == nil && st != nil && !st.IsDir() {
				res.Exists = true
			}
		}
		if !enabled[t] {
			out = append(out, res)
			continue
		}
		if err != nil {
			res.Error = err.Error()
			out = append(out, res)
			continue
		}
		changed, err := mcp.ApplyTargetWithRemovals(t, path, regT, mergedRemoveIDs, force)
		res.Changed = changed
		if err != nil {
			res.Error = err.Error()
		}
		out = append(out, res)
	}
	return out
}

func mergeRemoveIDs(a []string, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))

	for _, slice := range [][]string{a, b} {
		for _, x := range slice {
			x = strings.TrimSpace(x)
			if x == "" {
				continue
			}
			if _, ok := seen[x]; ok {
				continue
			}
			seen[x] = struct{}{}
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func adminMCPDeleteHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req adminMCPDeleteReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		id := strings.TrimSpace(req.ID)
		if id == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 不能为空"})
			return
		}
		force := false
		if req.Force != nil {
			force = *req.Force
		}

		mut, ok := beginPersonalConfigMutation(c, opts)
		if !ok {
			return
		}
		finalized := false
		defer func() {
			if mut != nil && !finalized {
				abortPersonalConfigMutation(c, mut)
			}
		}()

		ctx := c.Request.Context()
		sv2, _, _, err := loadMCPStoreV2(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		if sv2.Servers != nil {
			delete(sv2.Servers, id)
		}
		if err := saveMCPStoreV2(ctx, opts.Store, sv2); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
			return
		}

		applyResults := applyMCPConfigs(ctx, opts, sv2, req.Targets, []string{id}, force)
		if mut != nil {
			if err := mut.Commit(ctx, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
				return
			}
		}
		finalized = true

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"server_count":  len(sv2.Servers),
				"apply_results": applyResults,
			},
		})
	}
}

func loadMCPStoreV2(ctx context.Context, st *store.Store) (s mcp.StoreV2, parseErr string, migrated bool, err error) {
	if st == nil {
		return mcp.StoreV2{Version: 2, Servers: map[string]mcp.ServerV2{}}, "", false, nil
	}
	raw, ok, err := st.GetStringAppSetting(ctx, store.SettingMCPServersStoreV2)
	if err != nil {
		return mcp.StoreV2{}, "", false, err
	}
	if ok && strings.TrimSpace(raw) != "" {
		parsed, err := mcp.ParseStoreV2JSON(raw)
		if err == nil {
			return parsed, "", false, nil
		}
		parseErr = err.Error()
	}

	// Fallback to legacy registry (read-only migration).
	rawLegacy, okLegacy, err := st.GetStringAppSetting(ctx, store.SettingMCPServersRegistry)
	if err != nil || !okLegacy || strings.TrimSpace(rawLegacy) == "" {
		return mcp.StoreV2{Version: 2, Servers: map[string]mcp.ServerV2{}}, parseErr, false, err
	}
	reg, err := mcp.ParseRegistryJSON(rawLegacy)
	if err != nil {
		if parseErr == "" {
			parseErr = err.Error()
		} else {
			parseErr = parseErr + "; legacy: " + err.Error()
		}
		return mcp.StoreV2{Version: 2, Servers: map[string]mcp.ServerV2{}}, parseErr, false, nil
	}
	return mcp.StoreV2FromRegistry(reg), parseErr, true, nil
}

func saveMCPStoreV2(ctx context.Context, st *store.Store, s mcp.StoreV2) error {
	if st == nil {
		return nil
	}
	s = s.Normalize()
	if err := s.Validate(); err != nil {
		return err
	}
	pretty, err := mcp.PrettyStoreV2JSON(s)
	if err != nil {
		return err
	}

	// Canonical v2.
	if len(s.Servers) == 0 {
		if err := st.DeleteAppSetting(ctx, store.SettingMCPServersStoreV2); err != nil {
			return err
		}
	} else {
		if err := st.UpsertStringAppSetting(ctx, store.SettingMCPServersStoreV2, pretty); err != nil {
			return err
		}
	}

	// Legacy view (backward compatibility for other subsystems).
	reg := mcp.StoreV2ToRegistry(s)
	if len(reg) == 0 {
		return st.DeleteAppSetting(ctx, store.SettingMCPServersRegistry)
	}
	legacyPretty, err := mcp.PrettyJSON(reg)
	if err != nil {
		return err
	}
	return st.UpsertStringAppSetting(ctx, store.SettingMCPServersRegistry, legacyPretty)
}

func adminMCPExportClaudeHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		sv2, _, _, err := loadMCPStoreV2(c.Request.Context(), opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		reg := mcp.StoreV2ToRegistryForTarget(sv2, mcp.TargetClaude)
		platform := strings.TrimSpace(c.Query("platform"))
		cfg, err := mcp.ExportClaudeConfig(reg, platform, strings.EqualFold(platform, "windows"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出失败"})
			return
		}
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"config_json": strings.TrimSpace(string(b)),
			},
		})
	}
}

func adminMCPExportGeminiHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		sv2, _, _, err := loadMCPStoreV2(c.Request.Context(), opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		reg := mcp.StoreV2ToRegistryForTarget(sv2, mcp.TargetGemini)
		cfg, err := mcp.ExportGeminiConfig(reg)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出失败"})
			return
		}
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"config_json": strings.TrimSpace(string(b)),
			},
		})
	}
}

func adminMCPExportCodexHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		sv2, _, _, err := loadMCPStoreV2(c.Request.Context(), opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		reg := mcp.StoreV2ToRegistryForTarget(sv2, mcp.TargetCodex)
		platform := strings.TrimSpace(c.Query("platform"))
		out, err := mcp.ExportCodexConfigTOML(reg, platform)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"config_toml": out,
			},
		})
	}
}
