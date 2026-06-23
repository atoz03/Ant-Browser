package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ant-chrome/backend/internal/automation"
	"ant-chrome/backend/internal/browser"
)

func TestAutomationScriptListSeedsDefaultScriptsOnFreshApp(t *testing.T) {
	app := NewApp(t.TempDir())

	items, err := app.AutomationScriptList()
	if err != nil {
		t.Fatalf("AutomationScriptList returned error: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected four default scripts, got %d", len(items))
	}

	byID := make(map[string]automation.ScriptRecord, len(items))
	for _, script := range items {
		byID[script.ID] = script
	}

	expectedNames := map[string]string{
		"dual-instance-runtime-switch": "双实例启动与 Runtime 切换",
		"news-query-txt":               "查询新闻并写 TXT",
		"proton-mail-first-message":    "Proton 邮件搜索并读取最新邮件",
		"web-image-generate-download":  "网页图片生成并下载",
	}

	for scriptID, expectedName := range expectedNames {
		script, ok := byID[scriptID]
		if !ok {
			t.Fatalf("missing default script %q", scriptID)
		}
		if script.Name != expectedName {
			t.Fatalf("unexpected default script name for %q: %q", scriptID, script.Name)
		}
		if script.EntryFile != "index.cjs" {
			t.Fatalf("unexpected default entry file for %q: %q", scriptID, script.EntryFile)
		}
		if script.Source.Type != "builtin" {
			t.Fatalf("expected builtin source for %q, got %+v", scriptID, script.Source)
		}

		scriptDir := filepath.Join(app.resolveAppPath(filepath.ToSlash(filepath.Join("data", "automation", "scripts"))), script.ID)
		if _, err := os.Stat(filepath.Join(scriptDir, "config")); err != nil {
			t.Fatalf("expected default config to exist for %q: %v", scriptID, err)
		}
		if _, err := os.Stat(filepath.Join(scriptDir, script.EntryFile)); err != nil {
			t.Fatalf("expected default entry file to exist for %q: %v", scriptID, err)
		}
	}

	dualScript := byID[automation.DualInstanceRuntimeScriptID]
	if !strings.Contains(dualScript.ParamsText, `"browsers"`) {
		t.Fatalf("expected dual-instance default params to use browsers array, got %s", dualScript.ParamsText)
	}
	if strings.Contains(dualScript.ParamsText, `"primaryCode"`) {
		t.Fatalf("expected dual-instance default params to drop legacy primaryCode fields, got %s", dualScript.ParamsText)
	}

	for scriptID := range expectedNames {
		if err := app.AutomationScriptDelete(scriptID); err != nil {
			t.Fatalf("AutomationScriptDelete returned error for %q: %v", scriptID, err)
		}
	}

	items, err = app.AutomationScriptList()
	if err != nil {
		t.Fatalf("AutomationScriptList returned error after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected deleted default script not to be re-seeded, got %d items", len(items))
	}
}

func TestAutomationScriptListAddsMissingBuiltinWhenLegacyMarkerExists(t *testing.T) {
	app := NewApp(t.TempDir())

	if _, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "custom-script",
		Name:       "自定义脚本",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ScriptText: "module.exports.run = async () => ({ ok: true })",
	}); err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	legacyMarkerPath := app.automationScriptDefaultsMarkerPath("defaults-seeded-v7")
	if err := os.MkdirAll(filepath.Dir(legacyMarkerPath), 0o755); err != nil {
		t.Fatalf("create legacy marker dir failed: %v", err)
	}
	if err := os.WriteFile(legacyMarkerPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write legacy marker failed: %v", err)
	}

	items, err := app.AutomationScriptList()
	if err != nil {
		t.Fatalf("AutomationScriptList returned error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected custom script plus four defaults, got %d items", len(items))
	}

	expectedDefaultIDs := []string{
		automation.DualInstanceRuntimeScriptID,
		automation.NewsQueryTXTScriptID,
		automation.ProtonMailFirstMessageID,
		automation.WebImageGenerateScriptID,
	}
	for _, scriptID := range expectedDefaultIDs {
		found := false
		for _, item := range items {
			if item.ID == scriptID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected migrated default script %q to exist", scriptID)
		}
	}

	if !app.automationScriptDefaultsInitialized() {
		t.Fatalf("expected new defaults marker to be written")
	}
}

func TestAutomationScriptSaveListAndDelete(t *testing.T) {
	app := NewApp(t.TempDir())

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "app-script",
		Name:       "App 脚本",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ScriptText: "module.exports.run = async () => ({ ok: true })",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}
	if saved == nil {
		t.Fatalf("AutomationScriptSave returned nil result")
	}
	if saved.ID != "app-script" {
		t.Fatalf("expected saved id app-script, got %q", saved.ID)
	}

	items, err := app.AutomationScriptList()
	if err != nil {
		t.Fatalf("AutomationScriptList returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one script, got %d", len(items))
	}

	if err := app.AutomationScriptDelete(saved.ID); err != nil {
		t.Fatalf("AutomationScriptDelete returned error: %v", err)
	}

	items, err = app.AutomationScriptList()
	if err != nil {
		t.Fatalf("AutomationScriptList returned error after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected zero scripts after delete, got %d", len(items))
	}
}

func TestAutomationScriptSaveHydratesExactTargetSelectorWithCode(t *testing.T) {
	app := newAutomationTargetTestApp(t)
	profile := createAutomationTargetProfile(t, app, browser.ProfileInput{
		ProfileName: "buyer-001",
	})
	code, err := app.launchCodeSvc.SetCode(profile.ProfileId, "BUYER_001")
	if err != nil {
		t.Fatalf("set code failed: %v", err)
	}

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "app-script",
		Name:       "App 脚本",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ScriptText: "module.exports.run = async () => ({ ok: true })",
		TargetConfig: automation.ScriptTargetConfig{
			Mode: "existing",
			Selector: automation.ScriptTargetSelector{
				ProfileID: profile.ProfileId,
			},
		},
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}
	if saved == nil {
		t.Fatalf("AutomationScriptSave returned nil result")
	}
	if saved.TargetConfig.Selector.ProfileID != profile.ProfileId {
		t.Fatalf("expected profileId to be preserved, got %+v", saved.TargetConfig.Selector)
	}
	if saved.TargetConfig.Selector.Code != code {
		t.Fatalf("expected code snapshot %q, got %+v", code, saved.TargetConfig.Selector)
	}
}

func TestAutomationScriptRunRecordsUnsupportedType(t *testing.T) {
	app := NewApp(t.TempDir())

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "playwright-script",
		Name:       "Playwright 脚本",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ScriptText: "module.exports.run = async () => ({ ok: true })",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	run, err := app.AutomationScriptRun(saved.ID)
	if err != nil {
		t.Fatalf("AutomationScriptRun returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRun returned nil result")
	}
	if run.Status != "failed" {
		t.Fatalf("expected unsupported script to fail, got %q", run.Status)
	}
	if run.Error == "" {
		t.Fatalf("expected unsupported script run to contain error")
	}

	runs, err := app.AutomationScriptRunList(10)
	if err != nil {
		t.Fatalf("AutomationScriptRunList returned error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run record, got %d", len(runs))
	}
}

func TestAutomationScriptRunWithOptionsInvalidSelector(t *testing.T) {
	app := NewApp(t.TempDir())

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:           "launch-script",
		Name:         "Launch 脚本",
		Type:         "launch-api",
		Status:       "ready",
		EntryFile:    "index.cjs",
		SelectorText: `{"code":"BUYER_001"}`,
		ParamsText:   `{"startUrls":["https://example.com"]}`,
		ScriptText:   "export async function run() {}",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	run, err := app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
		ScriptID:          saved.ID,
		SelectorText:      "{invalid",
		UseScriptSelector: false,
		UseScriptParams:   true,
	})
	if err != nil {
		t.Fatalf("AutomationScriptRunWithOptions returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRunWithOptions returned nil result")
	}
	if run.Status != "failed" {
		t.Fatalf("expected invalid selector run to fail, got %q", run.Status)
	}
	if run.Error == "" {
		t.Fatalf("expected invalid selector run to contain error")
	}
	if run.Summary != "脚本执行失败" {
		t.Fatalf("expected invalid selector summary, got %q", run.Summary)
	}
}

func TestAutomationScriptRunWithOptionsAllowsEmptySelectorForDualInstanceRuntimeScript(t *testing.T) {
	app := NewApp(t.TempDir())

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         automation.DualInstanceRuntimeScriptID,
		Name:       "双实例启动与 Runtime 切换",
		Type:       "launch-api",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ParamsText: `{"browsers":[{"code":"BUYER_001"},{"code":"BUYER_002"}],"timeoutMs":45000}`,
		ScriptText: "export async function run() {}",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	run, err := app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
		ScriptID:          saved.ID,
		SelectorText:      "",
		UseScriptSelector: false,
		UseScriptParams:   true,
	})
	if err != nil {
		t.Fatalf("AutomationScriptRunWithOptions returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRunWithOptions returned nil result")
	}
	if run.Summary != "双实例流程执行失败" {
		t.Fatalf("expected dual-instance flow to bypass selector validation, got %+v", run)
	}
	if strings.Contains(run.Error, "selector is required") {
		t.Fatalf("expected dual-instance script to allow empty selector, got %+v", run)
	}
}

func TestAutomationScriptRunWithOptionsSeedsDefaultScriptsOnFreshApp(t *testing.T) {
	app := NewApp(t.TempDir())

	run, err := app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
		ScriptID:          automation.DualInstanceRuntimeScriptID,
		UseScriptSelector: true,
		UseScriptParams:   true,
	})
	if err != nil {
		t.Fatalf("AutomationScriptRunWithOptions returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRunWithOptions returned nil result")
	}
	if run.ScriptID != automation.DualInstanceRuntimeScriptID {
		t.Fatalf("unexpected script id: %+v", run)
	}
	if run.ScriptName != "双实例启动与 Runtime 切换" {
		t.Fatalf("expected default script metadata to be hydrated, got %+v", run)
	}
	if run.ScriptType != "launch-api" {
		t.Fatalf("expected default script type launch-api, got %+v", run)
	}
	if run.Summary == "脚本读取失败" {
		t.Fatalf("expected direct run to seed defaults before execution, got %+v", run)
	}
	if strings.Contains(strings.ToLower(run.Error), "script not found") {
		t.Fatalf("expected seeded default script, got %+v", run)
	}
}

func TestAutomationScriptRefreshFromLocalFile(t *testing.T) {
	app := NewApp(t.TempDir())

	sourcePath := filepath.Join(t.TempDir(), "demo-script.cjs")
	if err := os.WriteFile(sourcePath, []byte("module.exports.run = async () => ({ ok: true, source: 'local-file' })"), 0o644); err != nil {
		t.Fatalf("write source file failed: %v", err)
	}

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "refresh-local-file",
		Name:       "本地文件脚本",
		Type:       "launch-api",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ScriptText: "module.exports.run = async () => ({ ok: false })",
		Source: automation.ScriptSource{
			Type: "local-file",
			URI:  sourcePath,
		},
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	refreshed, err := app.AutomationScriptRefresh(saved.ID)
	if err != nil {
		t.Fatalf("AutomationScriptRefresh returned error: %v", err)
	}
	if refreshed == nil {
		t.Fatalf("AutomationScriptRefresh returned nil result")
	}
	if refreshed.ID != saved.ID {
		t.Fatalf("expected same script id, got %q want %q", refreshed.ID, saved.ID)
	}
	if refreshed.Status != "ready" {
		t.Fatalf("expected status to be preserved, got %q", refreshed.Status)
	}
	if refreshed.Type != "playwright-cdp" {
		t.Fatalf("expected type to follow imported source, got %q", refreshed.Type)
	}
	if refreshed.EntryFile != "demo-script.cjs" {
		t.Fatalf("expected entry file from source bundle, got %q", refreshed.EntryFile)
	}
	if !strings.Contains(refreshed.ScriptText, "source: 'local-file'") {
		t.Fatalf("expected refreshed script text from local file, got %q", refreshed.ScriptText)
	}
	if refreshed.Source.Type != "local-file" || refreshed.Source.URI != sourcePath {
		t.Fatalf("unexpected refreshed source: %+v", refreshed.Source)
	}
	if refreshed.Source.ImportedAt == "" {
		t.Fatalf("expected refreshed source importedAt to be populated")
	}
}

func TestAutomationScriptRefreshFromBuiltin(t *testing.T) {
	app := NewApp(t.TempDir())

	savedImportedAt := "2026-01-01T00:00:00Z"
	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         automation.NewsQueryTXTScriptID,
		Name:       "旧新闻脚本",
		Type:       "launch-api",
		Status:     "ready",
		EntryFile:  "index.cjs",
		ScriptText: "module.exports.run = async () => ({ ok: false })",
		Source: automation.ScriptSource{
			Type:       "builtin",
			URI:        "repo://backend/internal/automation/demo-library/news-query-txt",
			Ref:        "HEAD",
			Path:       automation.NewsQueryTXTScriptID,
			ImportedAt: savedImportedAt,
		},
		PublicAPI: automation.ScriptPublicAPIConfig{
			Enabled:     true,
			Path:        "demo/news-refresh",
			RequestMode: "params-only",
		},
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	refreshed, err := app.AutomationScriptRefresh(saved.ID)
	if err != nil {
		t.Fatalf("AutomationScriptRefresh returned error: %v", err)
	}
	if refreshed == nil {
		t.Fatalf("AutomationScriptRefresh returned nil result")
	}
	if refreshed.ID != saved.ID {
		t.Fatalf("expected same script id, got %q want %q", refreshed.ID, saved.ID)
	}
	if refreshed.Status != "ready" {
		t.Fatalf("expected status to be preserved, got %q", refreshed.Status)
	}
	if refreshed.Name != "查询新闻并写 TXT" {
		t.Fatalf("expected builtin script name to be restored, got %q", refreshed.Name)
	}
	if !strings.Contains(refreshed.ScriptText, "acceptedItems") {
		t.Fatalf("expected refreshed builtin script text to contain news filtering logic, got %q", refreshed.ScriptText)
	}
	if refreshed.Source.Type != "builtin" || refreshed.Source.Path != automation.NewsQueryTXTScriptID {
		t.Fatalf("unexpected refreshed source: %+v", refreshed.Source)
	}
	if refreshed.Source.ImportedAt == "" || refreshed.Source.ImportedAt == savedImportedAt {
		t.Fatalf("expected builtin refresh to update importedAt, got %+v", refreshed.Source)
	}
	if refreshed.PublicAPI.Path != "demo/news-refresh" || !refreshed.PublicAPI.Enabled {
		t.Fatalf("expected public api config to be preserved on refresh, got %+v", refreshed.PublicAPI)
	}
}
