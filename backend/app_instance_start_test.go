package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"errors"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEnsureNewWindowLaunchArgAddsFlagOnce(t *testing.T) {
	t.Parallel()

	got := ensureNewWindowLaunchArg([]string{"--lang=en-US"})
	want := []string{"--lang=en-US", "--new-window"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureNewWindowLaunchArg 结果错误: got=%v want=%v", got, want)
	}

	got = ensureNewWindowLaunchArg([]string{"--new-window", "--lang=en-US"})
	want = []string{"--new-window", "--lang=en-US"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureNewWindowLaunchArg 不应重复追加: got=%v want=%v", got, want)
	}
}

func TestShouldPreferVisibleWindowForStartWithParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startURLs []string
		want      bool
	}{
		{
			name:      "nil start URLs",
			startURLs: nil,
			want:      false,
		},
		{
			name:      "empty start URLs",
			startURLs: []string{},
			want:      false,
		},
		{
			name:      "blank start URLs",
			startURLs: []string{"  ", "\t"},
			want:      false,
		},
		{
			name:      "valid start URL",
			startURLs: []string{"https://finance.sina.com.cn"},
			want:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldPreferVisibleWindowForStartWithParams(tt.startURLs); got != tt.want {
				t.Fatalf("shouldPreferVisibleWindowForStartWithParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBrowserProfileLive(t *testing.T) {
	t.Parallel()

	ln := mustListenLoopback(t)
	defer ln.Close()

	profile := &BrowserProfile{
		Running:   true,
		DebugPort: listenerPort(t, ln),
	}
	if !isBrowserProfileLive(profile, nil) {
		t.Fatal("期望存活中的调试端口被识别为运行中实例")
	}

	if isBrowserProfileLive(&BrowserProfile{Running: true, DebugPort: 0}, nil) {
		t.Fatal("debugPort=0 不应被识别为运行中实例")
	}
}

func TestIsBrowserProfileLiveKeepsPendingDebugProcessAlive(t *testing.T) {
	t.Parallel()

	cmd := longLivedCommand(2 * time.Second)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动长生命周期测试进程失败: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	profile := &BrowserProfile{
		Running:    true,
		Pid:        cmd.Process.Pid,
		DebugPort:  0,
		DebugReady: false,
	}
	if !isBrowserProfileLive(profile, cmd) {
		t.Fatal("期望调试接口未就绪但进程仍存活时识别为运行中实例")
	}
}

func TestWaitBrowserDebugPortStableKeepsListeningPort(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if _, err := waitBrowserDebugPortStable(server.port, "", time.Second, 250*time.Millisecond, nil); err != nil {
		t.Fatalf("waitBrowserDebugPortStable 返回错误: %v", err)
	}
}

func TestWaitBrowserDebugPortStableRejectsEphemeralPort(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	port := server.port
	time.AfterFunc(120*time.Millisecond, func() {
		_ = server.Close()
	})

	_, err := waitBrowserDebugPortStable(port, "", time.Second, 400*time.Millisecond, nil)
	if err == nil {
		t.Fatal("期望短暂就绪后关闭的端口被判定为失败")
	}
}

func TestWaitBrowserDebugPortStableRejectsPlainTCPPort(t *testing.T) {
	t.Parallel()

	ln := mustListenLoopback(t)
	defer ln.Close()

	_, err := waitBrowserDebugPortStable(listenerPort(t, ln), "", 700*time.Millisecond, 250*time.Millisecond, nil)
	if err == nil {
		t.Fatal("期望仅开放 TCP 端口但无 DevTools HTTP 时启动失败")
	}
}

func TestWaitBrowserDebugPortStableDiscoversPortFromStderr(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := stderrPortCommand(server.port, 2*time.Second)
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试命令失败: %v", err)
	}
	monitor.Start()

	debugPort, err := waitBrowserDebugPortStable(0, "", 2*time.Second, 250*time.Millisecond, monitor)
	if err != nil {
		t.Fatalf("期望从 stderr 自动发现调试端口，实际错误: %v", err)
	}
	if debugPort != server.port {
		t.Fatalf("期望发现调试端口 %d，实际=%d", server.port, debugPort)
	}
}

func TestWaitBrowserDebugPortStableDiscoversPortFromDevToolsFile(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	userDataDir := t.TempDir()
	writeDevToolsActivePortFile(t, userDataDir, server.port)

	debugPort, err := waitBrowserDebugPortStable(0, userDataDir, time.Second, 250*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("期望从 DevToolsActivePort 自动发现调试端口，实际错误: %v", err)
	}
	if debugPort != server.port {
		t.Fatalf("期望发现调试端口 %d，实际=%d", server.port, debugPort)
	}
}

func TestWaitBrowserDebugPortStableReturnsProcessExitDetail(t *testing.T) {
	t.Parallel()

	cmd := stderrFailingCommand("missing libEGL.dll")
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试命令失败: %v", err)
	}
	monitor.Start()

	startedAt := time.Now()
	_, err = waitBrowserDebugPortStable(0, "", 2*time.Second, 250*time.Millisecond, monitor)
	if err == nil {
		t.Fatal("期望启动前退出被判定为失败")
	}
	if time.Since(startedAt) >= 2*time.Second {
		t.Fatalf("期望在超时前返回进程退出错误，实际耗时=%s", time.Since(startedAt))
	}

	var exitErr *browserStartupExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("期望 browserStartupExitError，实际=%T %v", err, err)
	}
	if !strings.Contains(exitErr.Detail(), "missing libEGL.dll") {
		t.Fatalf("期望 stderr 细节被捕获，实际=%q", exitErr.Detail())
	}
}

func TestWaitBrowserDebugPortStableAllowsDebugPortAfterLauncherExit(t *testing.T) {
	t.Parallel()

	port := freeLoopbackPort(t)
	cmd := shortLivedCommand()
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动短命测试命令失败: %v", err)
	}
	monitor.Start()

	serverReady := make(chan *devToolsTestServer, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		serverReady <- startDevToolsServerOnPort(t, port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/json/version":
				_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
			case "/json/list":
				_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
			default:
				http.NotFound(w, r)
			}
		}))
	}()

	debugPort, err := waitBrowserDebugPortStable(port, "", 100*time.Millisecond, 250*time.Millisecond, monitor)
	server := <-serverReady
	defer server.Close()

	if err != nil {
		t.Fatalf("期望启动器退出后仍能等待到调试端口就绪，实际错误: %v", err)
	}
	if debugPort != port {
		t.Fatalf("期望发现调试端口 %d，实际=%d", port, debugPort)
	}
}

func TestWaitBrowserProcessKeepsRunningWhileDebugPortAlive(t *testing.T) {
	ln := mustListenLoopback(t)
	port := listenerPort(t, ln)

	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-detached": {
			ProfileId:   "profile-detached",
			ProfileName: "Detached Browser",
			Running:     true,
			DebugPort:   port,
			DebugReady:  true,
			Pid:         12345,
		},
	}
	app.browserMgr.BrowserProcesses = make(map[string]*exec.Cmd)

	cmd := shortLivedCommand()
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化测试进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动短命测试进程失败: %v", err)
	}
	monitor.Start()
	app.browserMgr.BrowserProcesses["profile-detached"] = cmd

	done := make(chan struct{})
	go func() {
		app.waitBrowserProcess("profile-detached", monitor)
		close(done)
	}()

	waitForCondition(t, 3*time.Second, func() bool {
		app.browserMgr.Mutex.Lock()
		defer app.browserMgr.Mutex.Unlock()

		profile := app.browserMgr.Profiles["profile-detached"]
		_, tracked := app.browserMgr.BrowserProcesses["profile-detached"]
		return profile != nil && profile.Running && !tracked
	})

	_ = ln.Close()

	waitForCondition(t, 4*time.Second, func() bool {
		app.browserMgr.Mutex.Lock()
		defer app.browserMgr.Mutex.Unlock()

		profile := app.browserMgr.Profiles["profile-detached"]
		return profile != nil && !profile.Running && profile.DebugPort == 0 && profile.Pid == 0
	})

	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("waitBrowserProcess 未在调试端口关闭后结束")
	}
}
