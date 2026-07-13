package browser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const extensionMaintenanceTimeout = 15 * time.Second

type extensionMaintenanceBrowser struct {
	cmd     *exec.Cmd
	input   *os.File
	output  *os.File
	reader  *bufio.Reader
	stderr  lockedBuffer
	request int
	closeMu sync.Mutex
	closed  bool
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type extensionProtocolError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type extensionProtocolResponse struct {
	ID     int                     `json:"id"`
	Result json.RawMessage         `json:"result"`
	Error  *extensionProtocolError `json:"error"`
}

func newExtensionMaintenanceBrowser(cmd *exec.Cmd, input *os.File, output *os.File) *extensionMaintenanceBrowser {
	client := &extensionMaintenanceBrowser{cmd: cmd, input: input, output: output}
	client.reader = bufio.NewReader(output)
	cmd.Stderr = &client.stderr
	return client
}

func extensionMaintenanceArgs(userDataDir string, allowedDirs []string) []string {
	args := []string{
		"--user-data-dir=" + userDataDir,
		"--remote-debugging-pipe",
		"--enable-unsafe-extension-debugging",
		"--headless=new",
		"--no-first-run",
		"--no-default-browser-check",
	}
	if extensionArg := strings.Join(appendUniqueExtensionDirs(nil, allowedDirs...), ","); extensionArg != "" || allowedDirs != nil {
		args = append(args, "--disable-extensions-except="+extensionArg)
	}
	return args
}

func (c *extensionMaintenanceBrowser) ensureReady() error {
	if _, err := c.List(); err != nil {
		c.Close()
		return err
	}
	return nil
}

func (c *extensionMaintenanceBrowser) List() ([]unpackedExtensionInfo, error) {
	var result struct {
		Extensions []unpackedExtensionInfo `json:"extensions"`
	}
	if err := c.call("Extensions.getExtensions", nil, &result); err != nil {
		return nil, err
	}
	return result.Extensions, nil
}

func (c *extensionMaintenanceBrowser) Load(path string) (string, error) {
	var result struct {
		ID string `json:"id"`
	}
	if err := c.call("Extensions.loadUnpacked", map[string]any{"path": path}, &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("Chromium 未返回插件 ID")
	}
	return result.ID, nil
}

func (c *extensionMaintenanceBrowser) Uninstall(id string) error {
	return c.call("Extensions.uninstall", map[string]any{"id": id}, nil)
}

func (c *extensionMaintenanceBrowser) call(method string, params map[string]any, target any) error {
	c.request++
	requestID := c.request
	if params == nil {
		params = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{"id": requestID, "method": method, "params": params})
	if err != nil {
		return err
	}
	payload = append(payload, 0)
	if err := c.input.SetWriteDeadline(time.Now().Add(extensionMaintenanceTimeout)); err == nil {
		defer c.input.SetWriteDeadline(time.Time{})
	}
	if _, err := c.input.Write(payload); err != nil {
		return c.withProcessError("写入 Chromium 调试管道失败", err)
	}
	if err := c.output.SetReadDeadline(time.Now().Add(extensionMaintenanceTimeout)); err == nil {
		defer c.output.SetReadDeadline(time.Time{})
	}
	for {
		message, err := c.reader.ReadBytes(0)
		if err != nil {
			return c.withProcessError("读取 Chromium 调试管道失败", err)
		}
		message = bytes.TrimSuffix(message, []byte{0})
		if len(message) == 0 {
			continue
		}
		var response extensionProtocolResponse
		if err := json.Unmarshal(message, &response); err != nil || response.ID != requestID {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("%s: %s", method, response.Error.Message)
		}
		if target != nil && len(response.Result) > 0 {
			if err := json.Unmarshal(response.Result, target); err != nil {
				return err
			}
		}
		return nil
	}
}

func (c *extensionMaintenanceBrowser) withProcessError(message string, err error) error {
	detail := strings.TrimSpace(c.stderr.String())
	if detail == "" {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s: %w；%s", message, err, detail)
}

func (c *extensionMaintenanceBrowser) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	_ = c.call("Browser.close", nil, nil)
	_ = c.input.Close()
	_ = c.output.Close()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-done
	}
}
