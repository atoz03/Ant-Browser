package automation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ant-chrome/backend/internal/config"
)

func TestRunScriptTaskLaunchPassesTemporaryProxyParams(t *testing.T) {
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
		ProxyID              string `json:"proxyId"`
		ProxyConfig          string `json:"proxyConfig"`
		SkipDefaultStartURLs bool   `json:"skipDefaultStartUrls"`
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
	scriptPath := filepath.Join(scriptDir, "script-launch-proxy.cjs")
	scriptSource := `module.exports.run = async ({ launch }) => {
  await launch({
    proxyId: 'proxy-picked',
    proxyConfig: 'socks5://127.0.0.1:1080',
    skipDefaultStartUrls: true,
  })

  return {
    ok: true,
    summary: '脚本执行成功',
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:launch-proxy",
		ScriptPath:    scriptPath,
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}

	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if receivedBody.ProxyID != "proxy-picked" {
		t.Fatalf("expected proxyId to be forwarded, got %+v", receivedBody)
	}
	if receivedBody.ProxyConfig != "socks5://127.0.0.1:1080" {
		t.Fatalf("expected proxyConfig to be forwarded, got %+v", receivedBody)
	}
	if !receivedBody.SkipDefaultStartURLs {
		t.Fatalf("expected skipDefaultStartUrls to stay true, got %+v", receivedBody)
	}
}

func TestRunScriptTaskLaunchSkipsDefaultStartUrlsByDefault(t *testing.T) {
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
		SkipDefaultStartURLs *bool `json:"skipDefaultStartUrls"`
	}
	receivedBody := launchRequestPayload{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/launch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
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
	scriptPath := filepath.Join(scriptDir, "script-launch-default-skip.cjs")
	scriptSource := `module.exports.run = async ({ launch, selector }) => {
  await launch({ selector })
  return { ok: true, summary: '脚本执行成功' }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:launch-default-skip",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if receivedBody.SkipDefaultStartURLs == nil || *receivedBody.SkipDefaultStartURLs != true {
		t.Fatalf("expected skipDefaultStartUrls default true, got %+v", receivedBody)
	}
}

func TestRunScriptTaskUseBrowserWithoutURLDoesNotCreateBlankPage(t *testing.T) {
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

	markerPath := filepath.Join(t.TempDir(), "new-page-count.txt")
	if err := writeMockPlaywrightModuleCountingNewPages(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion, markerPath); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-script-no-blank",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-no-blank.cjs")
	scriptSource := `module.exports.run = async ({ useBrowser, selector }) => {
  const runtime = await useBrowser({ selector })
  return { ok: true, summary: runtime.page ? runtime.page.url() : 'no-page' }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:no-blank",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if result.Summary != "no-page" {
		t.Fatalf("summary = %q, want no-page", result.Summary)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat marker failed: %v", err)
		}
		data, _ := os.ReadFile(markerPath)
		t.Fatalf("context.newPage should not be called, marker=%q", string(data))
	}
}

func TestRunScriptTaskOpenPageWithoutURLDoesNotCreateBlankPage(t *testing.T) {
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

	markerPath := filepath.Join(t.TempDir(), "new-page-count.txt")
	if err := writeMockPlaywrightModuleCountingNewPages(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion, markerPath); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-script-open-no-url",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-open-page-no-url.cjs")
	scriptSource := `module.exports.run = async ({ launch, connect, openPage, selector }) => {
  const session = await launch({ selector })
  const connection = await connect(session)
  const opened = await openPage(connection, { reuseCurrentPage: true })
  return { ok: true, summary: opened.page ? opened.page.url() : 'no-page' }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:open-page-no-url",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if result.Summary != "no-page" {
		t.Fatalf("summary = %q, want no-page", result.Summary)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat marker failed: %v", err)
		}
		data, _ := os.ReadFile(markerPath)
		t.Fatalf("context.newPage should not be called, marker=%q", string(data))
	}
}

func TestRunScriptTaskFallsBackToLaunchBaseURLWhenSessionEndpointIsInvalid(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/launch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"profileId":  "profile-script",
			"debugPort":  0,
			"debugReady": false,
			"cdpUrl":     "http://127.0.0.1:0",
		})
	}))
	defer server.Close()

	if err := writeMockPlaywrightModuleWithExpectedEndpoint(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion, server.URL); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-fallback.cjs")
	scriptSource := `module.exports.run = async ({ launch, connect, selector }) => {
  const session = await launch({ selector })
  const connection = await connect(session)

  return {
    ok: true,
    summary: '脚本已通过 Launch 地址回退连接',
    connectedEndpoint: connection.session.cdpUrl,
    profileId: session.profileId,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:fallback",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}

	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
	if result.Summary != "脚本已通过 Launch 地址回退连接" {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
	if !strings.Contains(result.ResultText, `"connectedEndpoint":"`+server.URL+`"`) {
		t.Fatalf("expected result text to contain fallback endpoint, got %s", result.ResultText)
	}
}

func TestRunScriptTaskClosesBrowserConnections(t *testing.T) {
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
	if err := writeMockPlaywrightModuleWithPersistentConnection(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion, ""); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-script-close",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-close.cjs")
	scriptSource := `module.exports.run = async ({ launch, connect, selector }) => {
  const session = await launch({ selector })
  const connection = await connect(session)

  return {
    ok: true,
    summary: '脚本执行成功',
    connectedEndpoint: connection.session.cdpUrl,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := manager.RunScriptTask(ctx, ScriptTaskRequest{
		TaskKey:       "script:close",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
}

func TestRunScriptTaskConnectHonorsPerCallTimeout(t *testing.T) {
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
	if err := writeMockPlaywrightModuleWithExpectedConnectTimeout(state.RuntimeDir, cfg.Automation.PlaywrightCoreVersion, 47000); err != nil {
		t.Fatalf("write mock playwright module failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"profileId": "profile-timeout",
			"debugPort": 9333,
			"cdpUrl":    "http://127.0.0.1:9333",
		})
	}))
	defer server.Close()

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-connect-timeout.cjs")
	scriptSource := `module.exports.run = async ({ launch, connect, selector }) => {
  const session = await launch({ selector })
  const connection = await connect(session, { timeoutMs: 47000 })

  return {
    ok: true,
    summary: '脚本执行成功',
    connectedEndpoint: connection.session.cdpUrl,
  }
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	result, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:connect-timeout",
		ScriptPath:    scriptPath,
		Selector:      map[string]any{"code": "DEMO_READY"},
		LaunchBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("RunScriptTask returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected script task to succeed, got %+v", result)
	}
}

func TestRunScriptTaskTerminatesHungScriptOnTimeout(t *testing.T) {
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

	scriptDir := filepath.Join(state.RuntimeDir, "tmp", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create script dir failed: %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "script-timeout.cjs")
	scriptSource := `module.exports.run = async () => {
  await new Promise(() => setInterval(() => {}, 1000))
}`
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	startedAt := time.Now()
	_, err := manager.RunScriptTask(context.Background(), ScriptTaskRequest{
		TaskKey:       "script:timeout",
		ScriptPath:    scriptPath,
		LaunchBaseURL: "http://127.0.0.1",
		Timeout:       150 * time.Millisecond,
	})
	elapsed := time.Since(startedAt)
	if err == nil {
		t.Fatalf("expected RunScriptTask to fail on timeout")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected timeout to terminate quickly, took %s", elapsed)
	}

	manager.mu.Lock()
	activeTaskCount := len(manager.activeTasks)
	profileTaskCount := len(manager.profileTask)
	manager.mu.Unlock()
	if activeTaskCount != 0 || profileTaskCount != 0 {
		t.Fatalf("expected timed out task to be unregistered, active=%d profile=%d", activeTaskCount, profileTaskCount)
	}
}
