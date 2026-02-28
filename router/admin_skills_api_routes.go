package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/skills"
	"realms/internal/store"
)

func setAdminSkillsAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/skills", adminSkillsGetHandler(opts))
	r.GET("/skills/scan", adminSkillsScanHandler(opts))
	r.POST("/skills/auto_adopt", adminSkillsAutoAdoptHandler(opts))
	r.POST("/skills/import", adminSkillsImportHandler(opts))
	r.POST("/skills/delete", adminSkillsDeleteHandler(opts))
	r.PUT("/skills", adminSkillsPutHandler(opts))
	r.POST("/skills/apply", adminSkillsApplyHandler(opts))
}

type adminSkillsPutReq struct {
	StoreJSON string          `json:"store_json"`
	Store     json.RawMessage `json:"store"`

	TargetEnabled map[string]bool `json:"target_enabled"`

	ApplyOnSave *bool `json:"apply_on_save"`
	Force       *bool `json:"force"`
}

type adminSkillsApplyReq struct {
	Targets     []string                    `json:"targets"`
	RemoveIDs   []string                    `json:"remove_ids"`
	Resolutions []skills.ConflictResolution `json:"resolutions"`
	Force       *bool                       `json:"force"`
}

type adminSkillsImportReq struct {
	Source     string `json:"source"`
	Mode       string `json:"mode"`
	ApplyAfter bool   `json:"apply_after"`
	Force      *bool  `json:"force"`
}

type adminSkillsDeleteReq struct {
	ID      string   `json:"id"`
	Targets []string `json:"targets"`
	Force   *bool    `json:"force"`
}

type adminSkillsAutoAdoptReq struct {
	Targets []string `json:"targets"`
}

type skillsTargetState struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
}

func adminSkillsGetHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		ctx := c.Request.Context()

		storeV1, parseErr, err := loadSkillsStoreV1(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询配置失败"})
			return
		}

		desiredHashes := map[string]map[string]string{}
		targetRoots := map[skills.Target]string{}
		for _, t := range []skills.Target{skills.TargetCodex, skills.TargetClaude, skills.TargetGemini} {
			dir, _ := skills.ResolveTargetDir(t)
			targetRoots[t] = dir
		}
		for _, id := range storeV1.IDs() {
			sk := storeV1.Skills[id]
			sk.ID = id
			per := map[string]string{}
			for _, t := range []skills.Target{skills.TargetCodex, skills.TargetClaude, skills.TargetGemini} {
				b, err := skills.RenderForTargetInDir(sk, t, targetRoots[t])
				if err != nil {
					continue
				}
				sum := sha256.Sum256([]byte(strings.TrimSpace(string(b))))
				per[string(t)] = hex.EncodeToString(sum[:])
			}
			desiredHashes[id] = per
		}

		te, _ := loadSkillsTargetEnabled(ctx, opts.Store)
		targets := make(map[string]skillsTargetState, 3)
		for _, t := range []skills.Target{skills.TargetCodex, skills.TargetClaude, skills.TargetGemini} {
			dir, _ := skills.ResolveTargetDir(t)
			exists := false
			if strings.TrimSpace(dir) != "" {
				if st, err := os.Stat(dir); err == nil && st != nil && st.IsDir() {
					exists = true
				}
			}
			targets[string(t)] = skillsTargetState{Enabled: te.Effective(t), Path: dir, Exists: exists}
		}

		storeJSON := "{}"
		if pretty, err := skills.PrettyStoreV1JSON(storeV1); err == nil && strings.TrimSpace(pretty) != "" {
			storeJSON = pretty
		}
		teJSON, _ := skills.PrettyTargetEnabledV1JSON(te)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":          storeJSON,
				"skill_count":         len(storeV1.Skills),
				"parse_error":         parseErr,
				"targets":             targets,
				"store":               storeV1,
				"target_enabled_json": teJSON,
				"target_enabled":      te,
				"desired_hashes":      desiredHashes,
			},
		})
	}
}

func adminSkillsScanHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		targets := map[string]gin.H{}
		for _, t := range []skills.Target{skills.TargetCodex, skills.TargetClaude, skills.TargetGemini} {
			dir, _ := skills.ResolveTargetDir(t)
			scan := skills.ScanTarget(t, dir)
			targets[string(t)] = gin.H{
				"target":      scan.Target,
				"path":        scan.Path,
				"exists":      scan.Exists,
				"parse_error": scan.ParseError,
				"skill_count": scan.SkillCount,
				"skills":      scan.Skills,
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"targets": targets}})
	}
}

func adminSkillsAutoAdoptHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req adminSkillsAutoAdoptReq
		_ = c.ShouldBindJSON(&req)

		want := map[skills.Target]bool{skills.TargetCodex: true, skills.TargetClaude: true, skills.TargetGemini: true}
		if len(req.Targets) > 0 {
			for k := range want {
				want[k] = false
			}
			for _, t := range req.Targets {
				switch strings.TrimSpace(strings.ToLower(t)) {
				case string(skills.TargetCodex):
					want[skills.TargetCodex] = true
				case string(skills.TargetClaude):
					want[skills.TargetClaude] = true
				case string(skills.TargetGemini):
					want[skills.TargetGemini] = true
				}
			}
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
		cur, _, err := loadSkillsStoreV1(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}

		scans := map[skills.Target]skills.ScanTargetResult{}
		for _, t := range []skills.Target{skills.TargetCodex, skills.TargetClaude, skills.TargetGemini} {
			if !want[t] {
				scans[t] = skills.ScanTargetResult{Target: t, Path: "", Exists: false, Skills: map[string]skills.ScannedSkill{}}
				continue
			}
			dir, _ := skills.ResolveTargetDir(t)
			scans[t] = skills.ScanTarget(t, dir)
		}

		adopted, err := skills.AutoAdoptMissing(cur, scans)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "纳管失败"})
			return
		}
		if adopted.StoreChanged {
			if err := saveSkillsStoreV1(ctx, opts.Store, adopted.Store); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
				return
			}
		}

		if mut != nil {
			if err := mut.Commit(ctx, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
				return
			}
		}
		finalized = true

		storeJSON := "{}"
		if pretty, err := skills.PrettyStoreV1JSON(adopted.Store); err == nil && strings.TrimSpace(pretty) != "" {
			storeJSON = pretty
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"adopted_count": len(adopted.AdoptedIDs),
				"adopted_ids":   adopted.AdoptedIDs,
				"conflicts":     adopted.Conflicts,
				"store":         adopted.Store,
				"store_json":    storeJSON,
			},
		})
	}
}

func adminSkillsImportHandler(opts Options) gin.HandlerFunc {
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

		var req adminSkillsImportReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		source := strings.TrimSpace(strings.ToLower(req.Source))
		mode := strings.TrimSpace(strings.ToLower(req.Mode))
		if mode == "" {
			mode = "merge"
		}
		if source != string(skills.TargetCodex) && source != string(skills.TargetClaude) && source != string(skills.TargetGemini) {
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
		cur, _, err := loadSkillsStoreV1(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}

		t := skills.Target(source)
		dir, _ := skills.ResolveTargetDir(t)
		imported, _, err := skills.ImportFromTarget(t, dir)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导入失败"})
			return
		}

		var next skills.StoreV1
		switch mode {
		case "replace":
			next = imported
		default:
			next = cur
			if next.Skills == nil {
				next.Skills = map[string]skills.SkillV1{}
			}
			for id, sk := range imported.Skills {
				if _, ok := next.Skills[id]; ok {
					continue
				}
				next.Skills[id] = sk
			}
		}

		if err := saveSkillsStoreV1(ctx, opts.Store, next); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
			return
		}

		var applyOut skills.ApplyOutput
		if req.ApplyAfter {
			te, _ := loadSkillsTargetEnabled(ctx, opts.Store)
			applyOut, _ = skills.ApplyStore(next, skills.ApplyOptions{Force: force, TargetEnabled: te})
			if applyOut.StoreChanged {
				_ = saveSkillsStoreV1(ctx, opts.Store, applyOut.Store)
			}
		}

		if mut != nil {
			if err := mut.Commit(ctx, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
				return
			}
		}
		finalized = true

		storeJSON := "{}"
		if pretty, err := skills.PrettyStoreV1JSON(next); err == nil && strings.TrimSpace(pretty) != "" {
			storeJSON = pretty
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":    storeJSON,
				"skill_count":   len(next.Skills),
				"store":         next,
				"imported_from": gin.H{"source": source, "path": dir, "count": len(imported.Skills)},
				"apply_results": applyOut.Results,
				"conflicts":     applyOut.Conflicts,
			},
		})
	}
}

func adminSkillsPutHandler(opts Options) gin.HandlerFunc {
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

		var req adminSkillsPutReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		ctx := c.Request.Context()

		raw := strings.TrimSpace(req.StoreJSON)
		if len(req.Store) > 0 {
			raw = strings.TrimSpace(string(req.Store))
		}
		sv1, err := skills.ParseStoreV1JSON(raw)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 store"})
			return
		}
		if err := saveSkillsStoreV1(ctx, opts.Store, sv1); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
			return
		}

		if len(req.TargetEnabled) > 0 {
			te := skills.TargetEnabledV1{}
			if v, ok := req.TargetEnabled[string(skills.TargetCodex)]; ok {
				te.Codex = &v
			}
			if v, ok := req.TargetEnabled[string(skills.TargetClaude)]; ok {
				te.Claude = &v
			}
			if v, ok := req.TargetEnabled[string(skills.TargetGemini)]; ok {
				te.Gemini = &v
			}
			_ = saveSkillsTargetEnabled(ctx, opts.Store, te)
		}

		applyOnSave := false
		if req.ApplyOnSave != nil {
			applyOnSave = *req.ApplyOnSave
		}
		force := false
		if req.Force != nil {
			force = *req.Force
		}

		var applyOut skills.ApplyOutput
		if applyOnSave {
			te, _ := loadSkillsTargetEnabled(ctx, opts.Store)
			applyOut, _ = skills.ApplyStore(sv1, skills.ApplyOptions{Force: force, TargetEnabled: te})
			if applyOut.StoreChanged {
				_ = saveSkillsStoreV1(ctx, opts.Store, applyOut.Store)
				sv1 = applyOut.Store
			}
		}

		if mut != nil {
			if err := mut.Commit(ctx, true); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
				return
			}
		}
		finalized = true

		storeJSON := "{}"
		if pretty, err := skills.PrettyStoreV1JSON(sv1); err == nil && strings.TrimSpace(pretty) != "" {
			storeJSON = pretty
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"store_json":    storeJSON,
				"skill_count":   len(sv1.Skills),
				"store":         sv1,
				"apply_results": applyOut.Results,
				"conflicts":     applyOut.Conflicts,
			},
		})
	}
}

func adminSkillsApplyHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req adminSkillsApplyReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		force := false
		if req.Force != nil {
			force = *req.Force
		}

		ctx := c.Request.Context()
		sv1, _, err := loadSkillsStoreV1(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		te, _ := loadSkillsTargetEnabled(ctx, opts.Store)

		targets := []skills.Target{}
		for _, t := range req.Targets {
			switch strings.TrimSpace(strings.ToLower(t)) {
			case string(skills.TargetCodex):
				targets = append(targets, skills.TargetCodex)
			case string(skills.TargetClaude):
				targets = append(targets, skills.TargetClaude)
			case string(skills.TargetGemini):
				targets = append(targets, skills.TargetGemini)
			}
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

		applyOut, _ := skills.ApplyStore(sv1, skills.ApplyOptions{
			Targets:       targets,
			Force:         force,
			RemoveIDs:     req.RemoveIDs,
			Resolutions:   req.Resolutions,
			TargetEnabled: te,
		})
		if applyOut.StoreChanged {
			_ = saveSkillsStoreV1(ctx, opts.Store, applyOut.Store)
		}

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
				"apply_results": applyOut.Results,
				"conflicts":     applyOut.Conflicts,
				"store":         applyOut.Store,
			},
		})
	}
}

func adminSkillsDeleteHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		var req adminSkillsDeleteReq
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
		sv1, _, err := loadSkillsStoreV1(ctx, opts.Store)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取配置失败"})
			return
		}
		if sv1.Skills != nil {
			delete(sv1.Skills, id)
		}
		if err := saveSkillsStoreV1(ctx, opts.Store, sv1); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "写入配置失败"})
			return
		}

		te, _ := loadSkillsTargetEnabled(ctx, opts.Store)
		targets := []skills.Target{}
		for _, t := range req.Targets {
			switch strings.TrimSpace(strings.ToLower(t)) {
			case string(skills.TargetCodex):
				targets = append(targets, skills.TargetCodex)
			case string(skills.TargetClaude):
				targets = append(targets, skills.TargetClaude)
			case string(skills.TargetGemini):
				targets = append(targets, skills.TargetGemini)
			}
		}

		applyOut, _ := skills.ApplyStore(sv1, skills.ApplyOptions{
			Targets:       targets,
			Force:         force,
			RemoveIDs:     []string{id},
			TargetEnabled: te,
		})

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
				"skill_count":   len(sv1.Skills),
				"apply_results": applyOut.Results,
				"conflicts":     applyOut.Conflicts,
			},
		})
	}
}

func loadSkillsStoreV1(ctx context.Context, st *store.Store) (s skills.StoreV1, parseErr string, err error) {
	if st == nil {
		return skills.StoreV1{Version: skills.StoreVersionV1, Skills: map[string]skills.SkillV1{}}, "", nil
	}
	raw, ok, err := st.GetStringAppSetting(ctx, store.SettingSkillsStoreV1)
	if err != nil {
		return skills.StoreV1{}, "", err
	}
	if ok && strings.TrimSpace(raw) != "" {
		parsed, err := skills.ParseStoreV1JSON(raw)
		if err == nil {
			return parsed, "", nil
		}
		parseErr = err.Error()
	}
	return skills.StoreV1{Version: skills.StoreVersionV1, Skills: map[string]skills.SkillV1{}}, parseErr, nil
}

func saveSkillsStoreV1(ctx context.Context, st *store.Store, s skills.StoreV1) error {
	if st == nil {
		return nil
	}
	s = s.Normalize()
	if err := s.Validate(); err != nil {
		return err
	}
	pretty, err := skills.PrettyStoreV1JSON(s)
	if err != nil {
		return err
	}
	if len(s.Skills) == 0 {
		return st.DeleteAppSetting(ctx, store.SettingSkillsStoreV1)
	}
	return st.UpsertStringAppSetting(ctx, store.SettingSkillsStoreV1, pretty)
}

func loadSkillsTargetEnabled(ctx context.Context, st *store.Store) (skills.TargetEnabledV1, error) {
	if st == nil {
		return skills.TargetEnabledV1{}, nil
	}
	raw, ok, err := st.GetStringAppSetting(ctx, store.SettingSkillsTargetEnabledV1)
	if err != nil || !ok {
		return skills.TargetEnabledV1{}, err
	}
	return skills.ParseTargetEnabledV1JSON(raw)
}

func saveSkillsTargetEnabled(ctx context.Context, st *store.Store, te skills.TargetEnabledV1) error {
	if st == nil {
		return nil
	}
	raw, err := skills.PrettyTargetEnabledV1JSON(te)
	if err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return st.DeleteAppSetting(ctx, store.SettingSkillsTargetEnabledV1)
	}
	return st.UpsertStringAppSetting(ctx, store.SettingSkillsTargetEnabledV1, raw)
}
