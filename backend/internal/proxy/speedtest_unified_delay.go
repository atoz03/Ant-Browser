package proxy

import (
	"context"
	"fmt"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

// unifiedDelayTest 使用 mihomo 原生 URLTest 实现延迟检测。
// 这与 mihomo-party 调用 mihomo core 的 /proxies/{name}/delay 是同一套连接逻辑，
// 避免手写 HTTP 复用连接在部分节点上触发重复 HEAD 或 TLS 处理差异。
func unifiedDelayTest(proxyId string, px C.Proxy, testURL string, timeout time.Duration) TestResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	delay, err := px.URLTest(ctx, testURL, nil)
	if ctx.Err() != nil {
		return TestResult{ProxyId: proxyId, Ok: false, Error: fmt.Sprintf("测速超时: %v", ctx.Err())}
	}
	if err != nil {
		return TestResult{ProxyId: proxyId, Ok: false, Error: err.Error()}
	}
	if delay == 0 {
		return TestResult{ProxyId: proxyId, Ok: false, Error: "delay test returned 0"}
	}

	return TestResult{ProxyId: proxyId, Ok: true, LatencyMs: int64(delay)}
}
