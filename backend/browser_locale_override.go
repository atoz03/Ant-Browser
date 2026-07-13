package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"ant-chrome/backend/internal/logger"

	"github.com/gorilla/websocket"
)

const (
	localeOverrideReconnectDelay = 500 * time.Millisecond
	localeOverrideStartTimeout   = 2 * time.Second
)

type localeOverrideEvent struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params struct {
		SessionID  string `json:"sessionId"`
		TargetInfo struct {
			Type string `json:"type"`
		} `json:"targetInfo"`
	} `json:"params"`
}

type localeOverrideWriter struct {
	mu     sync.Mutex
	nextID int
	conn   *websocket.Conn
}

func fingerprintLocaleOverride(args []string) string {
	for _, arg := range normalizeFingerprintLaunchArgs(args) {
		if !strings.HasPrefix(arg, "--lang=") {
			continue
		}
		locale := strings.TrimSpace(strings.TrimPrefix(arg, "--lang="))
		if locale == "" {
			return ""
		}
		return strings.ReplaceAll(locale, "-", "_")
	}
	return ""
}

func requiresLocaleOverrideBeforeNavigation(args []string) bool {
	return goruntime.GOOS == "darwin" && fingerprintLocaleOverride(args) != ""
}

func (a *App) startProfileLocaleOverride(profileID string, debugPort int, args []string) <-chan struct{} {
	if a == nil || debugPort <= 0 || goruntime.GOOS != "darwin" {
		return nil
	}
	locale := fingerprintLocaleOverride(args)
	if locale == "" {
		a.stopProfileLocaleOverride(profileID)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.localeOverrideMu.Lock()
	if a.localeOverrideCancels == nil {
		a.localeOverrideCancels = make(map[string]context.CancelFunc)
	}
	if previous := a.localeOverrideCancels[profileID]; previous != nil {
		previous()
	}
	a.localeOverrideCancels[profileID] = cancel
	a.localeOverrideMu.Unlock()

	ready := make(chan struct{})
	go a.runProfileLocaleOverride(ctx, profileID, debugPort, locale, ready)
	return ready
}

func (a *App) stopProfileLocaleOverride(profileID string) {
	if a == nil {
		return
	}
	a.localeOverrideMu.Lock()
	cancel := a.localeOverrideCancels[profileID]
	delete(a.localeOverrideCancels, profileID)
	a.localeOverrideMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func waitForLocaleOverrideReady(ready <-chan struct{}) bool {
	if ready == nil {
		return true
	}
	select {
	case <-ready:
		return true
	case <-time.After(localeOverrideStartTimeout):
		return false
	}
}

func (a *App) runProfileLocaleOverride(ctx context.Context, profileID string, debugPort int, locale string, ready chan<- struct{}) {
	log := logger.New("Browser")
	connected := false
	var readyOnce sync.Once
	markReady := func() {
		readyOnce.Do(func() { close(ready) })
	}
	for ctx.Err() == nil {
		err := maintainLocaleOverrideConnection(ctx, debugPort, locale, markReady)
		if ctx.Err() != nil {
			return
		}
		if !connected {
			log.Warn("语言接口接管暂时失败，将继续重试",
				logger.F("profile_id", profileID),
				logger.F("debug_port", debugPort),
				logger.F("locale", locale),
				logger.F("error", err),
			)
		}
		connected = true
		select {
		case <-ctx.Done():
			return
		case <-time.After(localeOverrideReconnectDelay):
		}
	}
}

func maintainLocaleOverrideConnection(ctx context.Context, debugPort int, locale string, onReady func()) error {
	body, err := cdpGetEndpointBody(debugPort, "/json/version")
	if err != nil {
		return err
	}
	var version cdpBrowserVersion
	if err := json.Unmarshal(body, &version); err != nil {
		return fmt.Errorf("CDP browser target 解析失败: %w", err)
	}
	wsURL := strings.TrimSpace(version.WebSocketDebuggerUrl)
	if wsURL == "" {
		return fmt.Errorf("CDP browser websocket 为空")
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		return err
	}
	defer conn.Close()

	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-watchDone:
		}
	}()

	writer := &localeOverrideWriter{conn: conn}
	autoAttachID, err := writer.send("", "Target.setAutoAttach", map[string]any{
		"autoAttach":             true,
		"waitForDebuggerOnStart": true,
		"flatten":                true,
	})
	if err != nil {
		return err
	}
	pendingResume := make(map[int]string)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var event localeOverrideEvent
		if json.Unmarshal(data, &event) != nil {
			continue
		}
		if event.ID == autoAttachID {
			if onReady != nil {
				onReady()
			}
			continue
		}
		if sessionID, ok := pendingResume[event.ID]; ok {
			delete(pendingResume, event.ID)
			_, _ = writer.send(sessionID, "Runtime.runIfWaitingForDebugger", nil)
			continue
		}
		if event.Method != "Target.attachedToTarget" || event.Params.SessionID == "" {
			continue
		}

		sessionID := event.Params.SessionID
		_, _ = writer.send(sessionID, "Target.setAutoAttach", map[string]any{
			"autoAttach":             true,
			"waitForDebuggerOnStart": true,
			"flatten":                true,
		})
		if localeOverrideTargetType(event.Params.TargetInfo.Type) {
			localeRequestID, localeErr := writer.send(sessionID, "Emulation.setLocaleOverride", map[string]any{"locale": locale})
			if localeErr == nil {
				pendingResume[localeRequestID] = sessionID
				continue
			}
		}
		_, _ = writer.send(sessionID, "Runtime.runIfWaitingForDebugger", nil)
	}
}

func localeOverrideTargetType(targetType string) bool {
	switch strings.ToLower(strings.TrimSpace(targetType)) {
	case "page", "iframe", "worker", "shared_worker", "service_worker":
		return true
	default:
		return false
	}
}

func (w *localeOverrideWriter) send(sessionID string, method string, params map[string]any) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextID++
	message := map[string]any{
		"id":     w.nextID,
		"method": method,
	}
	if params != nil {
		message["params"] = params
	}
	if sessionID != "" {
		message["sessionId"] = sessionID
	}
	if err := w.conn.WriteJSON(message); err != nil {
		return 0, err
	}
	return w.nextID, nil
}
