import type { BrowserCore, BrowserProfile, BrowserProxy, BrowserSettings } from '../types'

export async function getBindings() {
  try {
    return await import('../../../wailsjs/go/main/App')
  } catch {
    return null
  }
}

export function getGoApp(): any {
  return (globalThis as any).go?.main?.App ?? null
}

export function nowISOString(): string {
  return new Date().toISOString()
}

export function createDefaultBrowserSettings(): BrowserSettings {
  return {
    userDataRoot: 'data',
    defaultFingerprintArgs: defaultFingerprintArgsForHost(),
    defaultLaunchArgs: [],
    defaultStartUrls: [],
    lightStartEnabled: true,
    restoreLastSession: false,
    startReadyTimeoutMs: 3000,
    startStableWindowMs: 1200,
    defaultConnectorType: 'xray',
  }
}

let mockProfiles: BrowserProfile[] = [
  {
    profileId: 'mock-1',
    profileName: '默认指纹配置',
    userDataDir: 'data/default',
    coreId: 'default',
    fingerprintArgs: defaultFingerprintArgsForHost(),
    proxyId: '',
    proxyConfig: '',
    launchArgs: ['--disable-features=Translate'],
    tags: ['默认'],
    keywords: [],
    running: false,
    debugPort: 0,
    debugReady: false,
    pid: 0,
    runtimeWarning: '',
    lastError: '',
    createdAt: nowISOString(),
    updatedAt: nowISOString(),
  },
]

function defaultFingerprintArgsForHost(): string[] {
  const platform = navigator.platform.toLowerCase().includes('mac')
    ? 'macos'
    : navigator.platform.toLowerCase().includes('linux')
      ? 'linux'
      : 'windows'
  return ['--fingerprint-brand=Chrome', `--fingerprint-platform=${platform}`]
}

let mockCores: BrowserCore[] = []
let mockProxies: BrowserProxy[] = []

export function getMockProfiles(): BrowserProfile[] {
  return mockProfiles
}

export function setMockProfiles(next: BrowserProfile[]): void {
  mockProfiles = next
}

export function getMockCores(): BrowserCore[] {
  return mockCores
}

export function setMockCores(next: BrowserCore[]): void {
  mockCores = next
}

export function getMockProxies(): BrowserProxy[] {
  return mockProxies
}

export function setMockProxies(next: BrowserProxy[]): void {
  mockProxies = next
}
