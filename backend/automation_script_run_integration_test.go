package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"ant-chrome/backend/internal/automation"
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/launchcode"
)

func TestAutomationScriptRunWithOptionsExecutesPlaywrightScript(t *testing.T) {
	nodeExecPath := lookupAutomationTestNode(t)

	app := NewApp(t.TempDir())
	app.config = config.DefaultConfig()
	app.config.Automation.Enabled = true
	app.config.Automation.NodeSource = config.AutomationNodeSourceSystem
	app.config.Automation.SystemNodePath = nodeExecPath
	app.config.Automation.NodeVersion = "test-node"
	app.config.Automation.PlaywrightCoreVersion = "1.59.0"
	app.config.Automation.RuntimeVersion = "test-runtime"
	app.automationMgr = automation.NewManager(app.appRoot, app.config, nil, automation.Options{})

	prepareAutomationTestRuntime(t, app.automationMgr, app.config.Automation.PlaywrightCoreVersion)

	app.launchServer = launchcode.NewLaunchServer(
		launchcode.NewLaunchCodeService(launchcode.NewMemoryLaunchCodeDAO()),
		nil,
		nil,
		0,
	)
	if err := app.launchServer.Start(); err != nil {
		t.Fatalf("start launch server failed: %v", err)
	}
	defer func() {
		_ = app.launchServer.Stop()
	}()

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "playwright-success",
		Name:       "Playwright 成功脚本",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "scripts/index.cjs",
		ScriptText: "const fs = require('fs')\nmodule.exports.run = async ({ params, artifact }) => {\n  const outputPath = artifact('result.txt')\n  fs.writeFileSync(outputPath, String(params.message || 'default'), 'utf8')\n  return { ok: true, summary: 'artifact ready', outputPath }\n}\n",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	run, err := app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
		ScriptID:          saved.ID,
		SelectorText:      `{}`,
		ParamsText:        `{"message":"hello integration"}`,
		UseScriptSelector: false,
		UseScriptParams:   false,
	})
	if err != nil {
		t.Fatalf("AutomationScriptRunWithOptions returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRunWithOptions returned nil result")
	}
	if run.Status != "success" {
		t.Fatalf("expected success status, got %+v", run)
	}
	if run.Summary != "artifact ready" {
		t.Fatalf("unexpected run summary: %q", run.Summary)
	}

	var payload struct {
		OK        bool     `json:"ok"`
		Summary   string   `json:"summary"`
		Artifacts []string `json:"artifacts"`
		Result    struct {
			OutputPath string `json:"outputPath"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(run.ResultText), &payload); err != nil {
		t.Fatalf("unmarshal run result failed: %v; result=%s", err, run.ResultText)
	}
	if !payload.OK {
		t.Fatalf("expected payload ok=true, got %+v", payload)
	}
	if payload.Result.OutputPath == "" {
		t.Fatalf("expected outputPath in payload, got %+v result=%s", payload, run.ResultText)
	}
	if len(payload.Artifacts) != 1 || payload.Artifacts[0] != payload.Result.OutputPath {
		t.Fatalf("expected artifacts to contain output path, got %+v", payload)
	}

	data, err := os.ReadFile(payload.Result.OutputPath)
	if err != nil {
		t.Fatalf("read output artifact failed: %v", err)
	}
	if string(data) != "hello integration" {
		t.Fatalf("unexpected artifact content: %q", string(data))
	}
}

func TestAutomationScriptRunWithOptionsPrestartsStoredTargetForConnectOnlyScript(t *testing.T) {
	nodeExecPath := lookupAutomationTestNode(t)

	app := NewApp(t.TempDir())
	app.config = config.DefaultConfig()
	app.config.Automation.Enabled = true
	app.config.Automation.NodeSource = config.AutomationNodeSourceSystem
	app.config.Automation.SystemNodePath = nodeExecPath
	app.config.Automation.NodeVersion = "test-node"
	app.config.Automation.PlaywrightCoreVersion = "1.59.0"
	app.config.Automation.RuntimeVersion = "test-runtime"
	app.browserMgr = browser.NewManager(app.config, app.appRoot)
	app.launchCodeSvc = launchcode.NewLaunchCodeService(launchcode.NewMemoryLaunchCodeDAO())
	app.browserMgr.CodeProvider = app.launchCodeSvc
	app.automationMgr = automation.NewManager(app.appRoot, app.config, nil, automation.Options{})

	prepareAutomationTestRuntimeWithPlaywrightModule(
		t,
		app.automationMgr,
		app.config.Automation.PlaywrightCoreVersion,
		automationTestConnectProbePlaywrightModule,
	)

	var debugHits atomic.Int32
	debugServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		debugHits.Add(1)
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Browser": "Chrome/123.0.0.0",
		})
	}))
	defer debugServer.Close()

	debugURL, err := url.Parse(debugServer.URL)
	if err != nil {
		t.Fatalf("parse debug server url failed: %v", err)
	}
	debugPort, err := strconv.Atoi(debugURL.Port())
	if err != nil {
		t.Fatalf("parse debug server port failed: %v", err)
	}

	profile, err := app.browserMgr.Create(browser.ProfileInput{
		ProfileName: "buyer-connect-only",
	})
	if err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	if profile == nil {
		t.Fatal("create profile returned nil")
	}
	app.browserMgr.Profiles[profile.ProfileId].Running = true
	app.browserMgr.Profiles[profile.ProfileId].DebugReady = true
	app.browserMgr.Profiles[profile.ProfileId].DebugPort = debugPort
	app.browserMgr.Profiles[profile.ProfileId].Pid = 12345

	app.launchServer = launchcode.NewLaunchServer(
		app.launchCodeSvc,
		app,
		app.browserMgr,
		0,
	)
	if err := app.launchServer.Start(); err != nil {
		t.Fatalf("start launch server failed: %v", err)
	}
	defer func() {
		_ = app.launchServer.Stop()
	}()

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:        "playwright-connect-stored-target",
		Name:      "Playwright Connect Stored Target",
		Type:      "playwright-cdp",
		Status:    "ready",
		EntryFile: "scripts/index.cjs",
		ScriptText: "module.exports.run = async ({ connect }) => {\n" +
			"  const { browser } = await connect()\n" +
			"  return { ok: true, summary: 'connected through stored target', contextCount: browser.contexts().length }\n" +
			"}\n",
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

	run, err := app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
		ScriptID:          saved.ID,
		UseScriptSelector: true,
		UseScriptParams:   true,
	})
	if err != nil {
		t.Fatalf("AutomationScriptRunWithOptions returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRunWithOptions returned nil result")
	}
	if run.Status != "success" {
		t.Fatalf("expected success status, got %+v", run)
	}
	if !strings.Contains(run.Summary, "connected through stored target") {
		t.Fatalf("unexpected run summary: %q", run.Summary)
	}
	if !strings.Contains(run.ResultText, `"contextCount":1`) {
		t.Fatalf("expected connect result payload, got %s", run.ResultText)
	}
	if debugHits.Load() == 0 {
		t.Fatalf("expected connect() to hit active debug endpoint through launch server")
	}
}

func TestAutomationScriptRunWithOptionsPrestartsManualCodeTargetForConnectOnlyScript(t *testing.T) {
	app, cleanup := newAutomationPlaywrightRunTestApp(t, automationTestConnectProbePlaywrightModule)
	defer cleanup()

	var debugHits atomic.Int32
	debugServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		debugHits.Add(1)
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Browser": "Chrome/123.0.0.0",
		})
	}))
	defer debugServer.Close()

	profile := createAutomationRunningProfileWithCode(
		t,
		app,
		"buyer-manual-code",
		"BUYER_001",
		automationTestServerPort(t, debugServer.URL),
	)

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:        "playwright-connect-manual-code",
		Name:      "Playwright Connect Manual Code",
		Type:      "playwright-cdp",
		Status:    "ready",
		EntryFile: "scripts/index.cjs",
		ScriptText: "module.exports.run = async ({ connect }) => {\n" +
			"  const { browser } = await connect()\n" +
			"  return { ok: true, summary: 'connected through manual code', contextCount: browser.contexts().length }\n" +
			"}\n",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	run, err := app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
		ScriptID:          saved.ID,
		SelectorText:      `{"code":"BUYER_001"}`,
		UseScriptSelector: false,
		UseScriptParams:   true,
	})
	if err != nil {
		t.Fatalf("AutomationScriptRunWithOptions returned error: %v", err)
	}
	if run == nil {
		t.Fatalf("AutomationScriptRunWithOptions returned nil result")
	}
	if run.Status != "success" {
		t.Fatalf("expected success status, got %+v", run)
	}
	if !strings.Contains(run.Summary, "connected through manual code") {
		t.Fatalf("unexpected run summary: %q", run.Summary)
	}
	if !strings.Contains(run.ResultText, `"contextCount":1`) {
		t.Fatalf("expected connect result payload, got %s", run.ResultText)
	}
	if debugHits.Load() == 0 {
		t.Fatalf("expected connect() to hit active debug endpoint through launch server")
	}
	if app.launchServer == nil {
		t.Fatal("expected launch server to be initialized")
	}
	activeProfileID, _, _ := app.launchServer.ActiveProfile()
	if activeProfileID != profile.ProfileId {
		t.Fatalf("expected active profile %s, got %s", profile.ProfileId, activeProfileID)
	}
}

func TestAutomationScriptRunWithOptionsAllowsSameScriptOnDifferentProfiles(t *testing.T) {
	app, cleanup := newAutomationPlaywrightRunTestApp(t, "module.exports = { chromium: {} }\n")
	defer cleanup()

	debugServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer debugServer.Close()

	debugPort := automationTestServerPort(t, debugServer.URL)
	profileA := createAutomationRunningProfileWithCode(t, app, "buyer-a", "BUYER_A", debugPort)
	profileB := createAutomationRunningProfileWithCode(t, app, "buyer-b", "BUYER_B", debugPort)

	saved, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "slow-shared-script",
		Name:       "Slow Shared Script",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "scripts/index.cjs",
		ScriptText: "module.exports.run = async () => {\n  await new Promise((resolve) => setTimeout(resolve, 120))\n  return { ok: true, summary: 'slow ok' }\n}\n",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	results := runAutomationScriptsConcurrently(t, 2, func(index int) (*automation.ScriptRunRecord, error) {
		profileID := profileA.ProfileId
		if index == 1 {
			profileID = profileB.ProfileId
		}
		return app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
			ScriptID:          saved.ID,
			SelectorText:      fmt.Sprintf(`{"profileId":"%s"}`, profileID),
			UseScriptSelector: false,
			UseScriptParams:   true,
			TimeoutMs:         5000,
		})
	})

	for _, result := range results {
		if result.err != nil {
			t.Fatalf("AutomationScriptRunWithOptions returned error: %v", result.err)
		}
		if result.run == nil {
			t.Fatal("AutomationScriptRunWithOptions returned nil result")
		}
		if result.run.Status != "success" {
			t.Fatalf("expected both runs to succeed on different profiles, got %+v", result.run)
		}
	}
}

func TestAutomationScriptRunWithOptionsBlocksDifferentScriptsOnSameProfile(t *testing.T) {
	app, cleanup := newAutomationPlaywrightRunTestApp(t, "module.exports = { chromium: {} }\n")
	defer cleanup()

	debugServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer debugServer.Close()

	profile := createAutomationRunningProfileWithCode(
		t,
		app,
		"buyer-shared",
		"BUYER_SHARED",
		automationTestServerPort(t, debugServer.URL),
	)

	firstScript, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "slow-script-a",
		Name:       "Slow Script A",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "scripts/index.cjs",
		ScriptText: "module.exports.run = async () => {\n  await new Promise((resolve) => setTimeout(resolve, 120))\n  return { ok: true, summary: 'slow ok a' }\n}\n",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}
	secondScript, err := app.AutomationScriptSave(automation.ScriptRecord{
		ID:         "slow-script-b",
		Name:       "Slow Script B",
		Type:       "playwright-cdp",
		Status:     "ready",
		EntryFile:  "scripts/index.cjs",
		ScriptText: "module.exports.run = async () => {\n  await new Promise((resolve) => setTimeout(resolve, 120))\n  return { ok: true, summary: 'slow ok b' }\n}\n",
	})
	if err != nil {
		t.Fatalf("AutomationScriptSave returned error: %v", err)
	}

	results := runAutomationScriptsConcurrently(t, 2, func(index int) (*automation.ScriptRunRecord, error) {
		scriptID := firstScript.ID
		if index == 1 {
			scriptID = secondScript.ID
		}
		return app.AutomationScriptRunWithOptions(automation.ScriptRunRequest{
			ScriptID:          scriptID,
			SelectorText:      fmt.Sprintf(`{"profileId":"%s"}`, profile.ProfileId),
			UseScriptSelector: false,
			UseScriptParams:   true,
			TimeoutMs:         5000,
		})
	})

	successCount := 0
	failedCount := 0
	for _, result := range results {
		if result.err != nil {
			t.Fatalf("AutomationScriptRunWithOptions returned error: %v", result.err)
		}
		if result.run == nil {
			t.Fatal("AutomationScriptRunWithOptions returned nil result")
		}
		switch result.run.Status {
		case "success":
			successCount++
		case "failed":
			failedCount++
			if !strings.Contains(result.run.Error, "已有自动化任务在运行中") {
				t.Fatalf("expected target lock failure, got %+v", result.run)
			}
		default:
			t.Fatalf("unexpected run status: %+v", result.run)
		}
	}
	if successCount != 1 || failedCount != 1 {
		t.Fatalf("expected one success and one failure on same profile, got success=%d failed=%d results=%+v", successCount, failedCount, results)
	}
}
