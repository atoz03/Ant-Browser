package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ant-chrome/backend/internal/config"
)

func TestFormatTaskRunnerLogs(t *testing.T) {
	logText := formatTaskRunnerLogs([]taskRunnerLogEntry{
		{Time: "2026-04-02T09:00:00Z", Values: []any{"打开页面", map[string]any{"url": "https://example.test"}}},
		{Time: "2026-04-02T09:00:01Z", Values: []any{"点击按钮"}},
	})

	if !strings.Contains(logText, "2026-04-02T09:00:00Z 打开页面") {
		t.Fatalf("expected first log line, got %q", logText)
	}
	if !strings.Contains(logText, `{"url":"https://example.test"}`) {
		t.Fatalf("expected structured log value, got %q", logText)
	}
	if !strings.Contains(logText, "2026-04-02T09:00:01Z 点击按钮") {
		t.Fatalf("expected second log line, got %q", logText)
	}
}

func TestRunScriptTaskExecutesCustomRunner(t *testing.T) {
	nodeExecPath := lookupNodeExecutable(t)

	cfg := config.DefaultConfig()
	cfg.Automation.Enabled = true
	cfg.Automation.NodeSource = config.AutomationNodeSourceSystem
	cfg.Automation.SystemNodePath = nodeExecPath
	cfg.Automation.NodeVersion = "test-node"
	cfg.Automation.PlaywrightCoreVersion = "1.59.0"
	cfg.Automation.RuntimeVersion = "test-runtime"

	manager := NewManager(t.TempDir(), cfg, nil, Options{})

	state := manager.CurrentState()
	if err := writeRunnerScript(state.RunnerPath); err != nil {
		t.Fatalf("write runner script failed: %v", err)
	}
	if err := writeMockPlaywrightModule(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	receivedBody := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/launch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-script",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script.cjs")
	scriptSource := `const fs = require('fs');

module.exports.run = async ({ launch, connect, selector, params, log, artifact }) => {
  const session = await launch({
    selector,
    startUrls: params.startUrls,
    skipDefaultStartUrls: true,
  })

  const { browser } = await connect(session)
  const context = browser.contexts()[0]
  const page = context.pages()[0] || await context.newPage()
  await page.goto(params.url, { waitUntil: 'domcontentloaded', timeout: params.timeoutMs || 30000 })

  const filePath = artifact('script-output.txt')
  fs.writeFileSync(filePath, 'artifact-ready')
  log('profile', session.profileId)

  return {
    ok: true,
    summary: '脚本执行成功',
    profileId: session.profileId,
    url: page.url(),
    artifactPath: filePath,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:test",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "BUYER_001"},
		Params:        map[string]any{"url": "https://example.com/script", "startUrls": []string{"https://example.com/script"}},
		LaunchBaseURL: server.URL,
		ArtifactDir:   artifactDir,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}

	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if result.Summary != "脚本执行成功" {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.ResultText, `"profileId":"profile-script"`) {
		t.Fatalf("expected result text to contain profileId, got %s", result.ResultText)
	}
	if !strings.Contains(result.ResultText, `"artifactPath":"`) {
		t.Fatalf("expected result text to contain artifact path, got %s", result.ResultText)
	}

	if selector, ok := receivedBody["selector"].(map[string]any); !ok || selector["code"] != "BUYER_001" {
		t.Fatalf("unexpected selector payload: %+v", receivedBody)
	}

	artifactData, err := os.ReadFile(filepath.Join(artifactDir, "script-output.txt"))
	if err != nil {
		t.Fatalf("read script artifact failed: %v", err)
	}
	if string(artifactData) != "artifact-ready" {
		t.Fatalf("unexpected script artifact payload: %s", string(artifactData))
	}
}

func TestRunScriptTaskOpenPageReusesInitialBlankPageAndGrantsPermissions(t *testing.T) {
	nodeExecPath := lookupNodeExecutable(t)

	cfg := config.DefaultConfig()
	cfg.Automation.Enabled = true
	cfg.Automation.NodeSource = config.AutomationNodeSourceSystem
	cfg.Automation.SystemNodePath = nodeExecPath
	cfg.Automation.NodeVersion = "test-node"
	cfg.Automation.PlaywrightCoreVersion = "1.59.0"
	cfg.Automation.RuntimeVersion = "test-runtime"

	manager := NewManager(t.TempDir(), cfg, nil, Options{})

	state := manager.CurrentState()
	if err := writeRunnerScript(state.RunnerPath); err != nil {
		t.Fatalf("write runner script failed: %v", err)
	}
	if err := writeMockPlaywrightModule(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-script",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-open-page.cjs")
	scriptSource := `module.exports.run = async ({ launch, connect, openPage, selector, params }) => {
  const session = await launch({
    selector,
    startUrls: [params.url],
    skipDefaultStartUrls: true,
  })

  const connection = await connect(session)
  const opened = await openPage(connection, {
    url: params.url,
    timeoutMs: params.timeoutMs || 30000,
    permissions: ['notifications'],
  })

  return {
    ok: true,
    summary: 'openPage helper ok',
    url: opened.page.url(),
    permissionApplied: opened.permissionResult.applied,
    permissionOrigin: opened.permissionResult.origin,
    permissionStrategy: opened.permissionResult.strategy || '',
    reusedPage: opened.reusedPage,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:open-page",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "BUYER_001"},
		Params:        map[string]any{"url": "https://example.com/inbox", "timeoutMs": 30000},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}

	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}

	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(result.ResultText), &parsed); err != nil {
		t.Fatalf("parse result text failed: %v result=%s", err, result.ResultText)
	}
	if nested, ok := parsed["result"].(map[string]any); ok && len(nested) > 0 {
		parsed = nested
	}
	if parsed["permissionApplied"] != true {
		t.Fatalf("expected permissionApplied to be true, got %+v", parsed)
	}
	if parsed["permissionOrigin"] != "https://example.com" {
		t.Fatalf("unexpected permissionOrigin: %+v", parsed)
	}
	if parsed["reusedPage"] != true {
		t.Fatalf("expected reusedPage to be true, got %+v", parsed)
	}
	if parsed["url"] != "https://example.com/inbox" {
		t.Fatalf("unexpected url: %+v", parsed)
	}
}

func TestRunScriptTaskCallPageAPIUsesBrowserContext(t *testing.T) {
	nodeExecPath := lookupNodeExecutable(t)

	cfg := config.DefaultConfig()
	cfg.Automation.Enabled = true
	cfg.Automation.NodeSource = config.AutomationNodeSourceSystem
	cfg.Automation.SystemNodePath = nodeExecPath
	cfg.Automation.NodeVersion = "test-node"
	cfg.Automation.PlaywrightCoreVersion = "1.59.0"
	cfg.Automation.RuntimeVersion = "test-runtime"

	manager := NewManager(t.TempDir(), cfg, nil, Options{})

	state := manager.CurrentState()
	if err := writeRunnerScript(state.RunnerPath); err != nil {
		t.Fatalf("write runner script failed: %v", err)
	}
	if err := writeMockPlaywrightModule(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-page-api",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-page-api.cjs")
	scriptSource := `module.exports.run = async ({ useBrowser, callPageAPI, browserFetch, selector, params }) => {
  const runtime = await useBrowser({
    selector,
    startUrls: [params.url],
    skipDefaultStartUrls: true,
    url: params.url,
    reuseCurrentPage: true,
    timeoutMs: 30000,
  })

  const created = await callPageAPI(runtime, {
    url: '/api/order/create',
    method: 'POST',
    query: {
      source: 'automation',
      tag: ['a', 'b'],
    },
    headers: {
      'X-Test': 'page-api',
    },
    json: {
      skuId: params.skuId,
      count: 2,
    },
  })
  const ping = await browserFetch(runtime.page, '/api/ping', { method: 'GET' })

  return {
    ok: true,
    summary: 'page api helper ok',
    status: created.status,
    requestUrl: created.json.url,
    method: created.json.method,
    credentials: created.json.credentials,
    contentType: created.json.headers['Content-Type'],
    testHeader: created.json.headers['X-Test'],
    requestBody: created.json.body,
    pingMethod: ping.json.method,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:page-api",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "BUYER_001"},
		Params:        map[string]any{"url": "https://example.com/app", "skuId": "sku-123"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}

	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}

	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(result.ResultText), &parsed); err != nil {
		t.Fatalf("parse result text failed: %v result=%s", err, result.ResultText)
	}
	if nested, ok := parsed["result"].(map[string]any); ok && len(nested) > 0 {
		parsed = nested
	}
	if parsed["status"] != float64(201) {
		t.Fatalf("unexpected status: %+v", parsed)
	}
	if parsed["method"] != "POST" || parsed["pingMethod"] != "GET" {
		t.Fatalf("unexpected methods: %+v", parsed)
	}
	if parsed["credentials"] != "include" {
		t.Fatalf("expected credentials=include, got %+v", parsed)
	}
	if parsed["contentType"] != "application/json" || parsed["testHeader"] != "page-api" {
		t.Fatalf("unexpected headers: %+v", parsed)
	}
	if !strings.Contains(fmt.Sprint(parsed["requestUrl"]), "/api/order/create?source=automation&tag=a&tag=b") {
		t.Fatalf("unexpected requestUrl: %+v", parsed)
	}
	if !strings.Contains(fmt.Sprint(parsed["requestBody"]), `"skuId":"sku-123"`) {
		t.Fatalf("unexpected requestBody: %+v", parsed)
	}
}

func TestRunScriptTaskLaunchFiltersNonLaunchParams(t *testing.T) {
	nodeExecPath := lookupNodeExecutable(t)

	cfg := config.DefaultConfig()
	cfg.Automation.Enabled = true
	cfg.Automation.NodeSource = config.AutomationNodeSourceSystem
	cfg.Automation.SystemNodePath = nodeExecPath
	cfg.Automation.NodeVersion = "test-node"
	cfg.Automation.PlaywrightCoreVersion = "1.59.0"
	cfg.Automation.RuntimeVersion = "test-runtime"

	manager := NewManager(t.TempDir(), cfg, nil, Options{})

	state := manager.CurrentState()
	if err := writeRunnerScript(state.RunnerPath); err != nil {
		t.Fatalf("write runner script failed: %v", err)
	}
	if err := writeMockPlaywrightModule(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	type launchRequestPayload struct {
		Code                 string         `json:"code"`
		Key                  string         `json:"key"`
		ProfileID            string         `json:"profileId"`
		ProfileName          string         `json:"profileName"`
		Keyword              string         `json:"keyword"`
		Keywords             []string       `json:"keywords"`
		Tag                  string         `json:"tag"`
		Tags                 []string       `json:"tags"`
		GroupID              string         `json:"groupId"`
		MatchMode            string         `json:"matchMode"`
		ProxyID              string         `json:"proxyId"`
		ProxyConfig          string         `json:"proxyConfig"`
		Selector             map[string]any `json:"selector"`
		LaunchArgs           []string       `json:"launchArgs"`
		StartURLs            []string       `json:"startUrls"`
		SkipDefaultStartURLs bool           `json:"skipDefaultStartUrls"`
	}

	receivedBody := launchRequestPayload{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/launch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&receivedBody); err != nil {
			t.Fatalf("decode launch request body failed: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-script",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-launch-filter.cjs")
	scriptSource := `module.exports.run = async ({ launch, selector, params }) => {
  const session = await launch({
    selector,
    startUrls: params.startUrls,
    skipDefaultStartUrls: true,
  })

  return {
    ok: true,
    summary: '脚本执行成功',
    profileId: session.profileId,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:launch-filter",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		Params:        map[string]any{"url": "https://www.baidu.com", "keyword": "OpenAI", "captureScreenshot": true, "waitAfterSearchMs": 1500, "startUrls": []string{"https://www.baidu.com"}},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}

	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if receivedBody.Selector["code"] != "DEMO_READY" {
		t.Fatalf("unexpected selector payload: %+v", receivedBody)
	}
	if len(receivedBody.StartURLs) != 1 || receivedBody.StartURLs[0] != "https://www.baidu.com" {
		t.Fatalf("unexpected startUrls payload: %+v", receivedBody.StartURLs)
	}
	if !receivedBody.SkipDefaultStartURLs {
		t.Fatalf("expected skipDefaultStartUrls to be true")
	}
	if receivedBody.Keyword != "" {
		t.Fatalf("expected non-launch params to be filtered, got keyword=%q", receivedBody.Keyword)
	}
	if receivedBody.ProxyID != "" || receivedBody.ProxyConfig != "" {
		t.Fatalf("expected proxy launch params to be empty, got %+v", receivedBody)
	}
}
