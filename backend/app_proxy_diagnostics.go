package backend

import "ant-chrome/backend/internal/proxy"

// BrowserProxyBuildDiagnostic 构建代理桥接诊断信息，不启动代理进程。
func (a *App) BrowserProxyBuildDiagnostic(proxyId string, proxyConfig string) ProxyBuildDiagnostic {
	proxies := a.getLatestProxies()
	return proxy.BuildProxyDiagnostic(proxyConfig, proxies, proxyId, proxy.BuildDiagnosticOptions{
		XrayMgr:    a.xrayMgr,
		SingBoxMgr: a.singboxMgr,
	})
}
