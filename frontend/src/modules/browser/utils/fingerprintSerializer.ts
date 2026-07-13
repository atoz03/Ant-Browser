// 指纹参数序列化/反序列化工具

/**
 * 获取系统当前时区
 * @returns IANA 时区标识符，如 "Asia/Shanghai"
 */
export function getSystemTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

export interface FingerprintConfig {
  // 指纹种子（核心）
  seed?: string            // --fingerprint=<seed>  控制所有随机噪声的根种子

  // 基础身份
  brand?: string           // --fingerprint-brand=
  brandVersion?: string    // --fingerprint-brand-version=
  platform?: string        // --fingerprint-platform=
  platformVersion?: string // --fingerprint-platform-version=
  lang?: string            // --lang=
  timezone?: string        // --timezone=

  // 屏幕与窗口
  resolution?: string      // --window-size=（预设值或 'custom'）
  customResolution?: string // 当 resolution === 'custom' 时使用

  // 硬件信息
  hardwareConcurrency?: string  // --fingerprint-hardware-concurrency=

  // 关闭指定的自动指纹伪装；未关闭的项目由 seed 自动生成
  disabledSpoofing?: string[]   // --disable-spoofing=font,audio,canvas,clientrects,gpu

  // 网络与隐私
  disableNonProxiedUDP?: boolean // --disable-non-proxied-udp

  unknownArgs?: string[]        // 无法识别的原始参数，原样保留
}

export const PRESET_RESOLUTIONS = ['1920,1080', '1440,900', '1366,768', '2560,1440', '1280,800', '1600,900']

// CLI 参数前缀 → FingerprintConfig 字段映射
export const KEY_MAP: Record<string, keyof FingerprintConfig> = {
  '--fingerprint': 'seed',
  '--fingerprint-brand': 'brand',
  '--fingerprint-brand-version': 'brandVersion',
  '--fingerprint-platform': 'platform',
  '--fingerprint-platform-version': 'platformVersion',
  '--lang': 'lang',
  '--timezone': 'timezone',
  '--window-size': 'resolution',
  '--fingerprint-hardware-concurrency': 'hardwareConcurrency',
}

// FingerprintConfig → string[]
export function serialize(config: FingerprintConfig): string[] {
  const args: string[] = []
  if (config.seed) args.push(`--fingerprint=${config.seed}`)
  if (config.brand) args.push(`--fingerprint-brand=${config.brand}`)
  if (config.brandVersion) args.push(`--fingerprint-brand-version=${config.brandVersion}`)
  if (config.platform) args.push(`--fingerprint-platform=${normalizePlatform(config.platform)}`)
  if (config.platformVersion) args.push(`--fingerprint-platform-version=${config.platformVersion}`)
  if (config.lang) {
    args.push(`--lang=${config.lang}`)
    args.push(`--accept-lang=${preferredAcceptLanguages(config.lang)}`)
  }
  if (config.timezone) {
    // 如果是 system，替换为实际系统时区
    const tz = config.timezone === 'system' ? getSystemTimezone() : config.timezone
    args.push(`--timezone=${tz}`)
  }

  const res = config.resolution === 'custom' ? config.customResolution : config.resolution
  if (res) args.push(`--window-size=${res}`)

  if (config.hardwareConcurrency) args.push(`--fingerprint-hardware-concurrency=${config.hardwareConcurrency}`)
  const disabledSpoofing = Array.from(new Set(config.disabledSpoofing || [])).filter(Boolean).sort()
  if (disabledSpoofing.length > 0) args.push(`--disable-spoofing=${disabledSpoofing.join(',')}`)
  if (config.disableNonProxiedUDP) args.push('--disable-non-proxied-udp')

  return [...args, ...(config.unknownArgs ?? [])]
}

// string[] → FingerprintConfig
export function deserialize(args: string[]): FingerprintConfig {
  const config: FingerprintConfig = { unknownArgs: [] }

  for (const arg of args) {
    if (arg === '--disable-non-proxied-udp') {
      config.disableNonProxiedUDP = true
      continue
    }
    const eqIdx = arg.indexOf('=')
    if (eqIdx === -1) {
      config.unknownArgs!.push(arg)
      continue
    }
    const key = arg.slice(0, eqIdx)
    const val = arg.slice(eqIdx + 1)
    const field = KEY_MAP[key]

    if (key === '--disable-spoofing') {
      config.disabledSpoofing = val.split(',').map(value => value.trim()).filter(Boolean)
      continue
    }
    if (key === '--accept-lang') {
      continue
    }
    if (key === '--fingerprint-platform' && ['mac', 'darwin', 'osx'].includes(val.toLowerCase())) {
      config.platform = 'macos'
      continue
    }
    if (key === '--webrtc-ip-handling-policy') {
      if (val === 'disable_non_proxied_udp') {
        config.disableNonProxiedUDP = true
      } else if (val) {
        config.unknownArgs!.push(`--force-webrtc-ip-handling-policy=${val}`)
      }
      continue
    }
    if (key === '--fingerprint-canvas-noise' || key === '--fingerprint-audio-noise') {
      if (val === 'false') {
        const category = key.includes('canvas') ? 'canvas' : 'audio'
        config.disabledSpoofing = [...(config.disabledSpoofing || []), category]
      }
      continue
    }
    if (isRetiredFingerprintKey(key)) {
      continue
    }
    if (!field) {
      config.unknownArgs!.push(arg)
      continue
    }

    if (field === 'resolution') {
      if (PRESET_RESOLUTIONS.includes(val)) {
        config.resolution = val
      } else {
        config.resolution = 'custom'
        config.customResolution = val
      }
    } else {
      (config as Record<string, unknown>)[field] = val
    }
  }

  return config
}

function normalizePlatform(value: string): string {
  return ['mac', 'darwin', 'osx'].includes(value.toLowerCase()) ? 'macos' : value.toLowerCase()
}

function preferredAcceptLanguages(lang: string): string {
  const normalized = lang.trim()
  const base = normalized.split('-', 1)[0]?.toLowerCase()
  return base && base !== normalized.toLowerCase() ? `${normalized},${base}` : normalized
}

function isRetiredFingerprintKey(key: string): boolean {
  return [
    '--fingerprint-color-depth',
    '--fingerprint-device-memory',
    '--fingerprint-webgl-vendor',
    '--fingerprint-webgl-renderer',
    '--fingerprint-gpu-vendor',
    '--fingerprint-gpu-renderer',
    '--fingerprint-fonts',
    '--fingerprint-do-not-track',
    '--fingerprint-media-devices',
    '--fingerprint-touch-points',
  ].includes(key)
}

// 生成随机指纹种子（32位正整数）
export function randomFingerprintSeed(): string {
  return String(Math.floor(Math.random() * 2147483647) + 1)
}

// ─── 预设指纹配置 ────────────────────────────────────────────────────────────

export interface FingerprintPreset {
  id: string
  name: string
  description: string
  config: Partial<FingerprintConfig>
}

export const FINGERPRINT_PRESETS: FingerprintPreset[] = [
  {
    id: 'win-chrome-office',
    name: 'Windows / Chrome / 办公',
    description: 'Windows、中文环境、1920x1080',
    config: {
      brand: 'Chrome',
      platform: 'windows',
      lang: 'zh-CN',
      timezone: 'Asia/Shanghai',
      resolution: '1920,1080',
      hardwareConcurrency: '8',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'win-chrome-gaming',
    name: 'Windows / Chrome / 游戏主机',
    description: 'Windows、高核心数、2560x1440',
    config: {
      brand: 'Chrome',
      platform: 'windows',
      lang: 'en-US',
      timezone: 'America/New_York',
      resolution: '2560,1440',
      hardwareConcurrency: '16',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'mac-chrome-designer',
    name: 'macOS / Chrome / 设计师',
    description: 'macOS、中文环境、Retina 窗口尺寸',
    config: {
      brand: 'Chrome',
      platform: 'macos',
      lang: 'zh-CN',
      timezone: 'Asia/Shanghai',
      resolution: '2560,1440',
      hardwareConcurrency: '10',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'win-edge-enterprise',
    name: 'Windows / Edge / 企业',
    description: 'Windows、Edge、中文企业环境',
    config: {
      brand: 'Edge',
      platform: 'windows',
      lang: 'zh-CN',
      timezone: 'Asia/Shanghai',
      resolution: '1366,768',
      hardwareConcurrency: '4',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'win-chrome-us-user',
    name: 'Windows / Chrome / 美国用户',
    description: 'Windows、美国英语、洛杉矶时区',
    config: {
      brand: 'Chrome',
      platform: 'windows',
      lang: 'en-US',
      timezone: 'America/Los_Angeles',
      resolution: '1920,1080',
      hardwareConcurrency: '8',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'mac-safari-jp',
    name: 'macOS / Safari / 日本用户',
    description: 'macOS、Safari 品牌、日语环境',
    config: {
      brand: 'Safari',
      platform: 'macos',
      lang: 'ja-JP',
      timezone: 'Asia/Tokyo',
      resolution: '1440,900',
      hardwareConcurrency: '8',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'win-chrome-uk-office',
    name: 'Windows / Chrome / 英国-办公',
    description: 'Windows、英国英语、伦敦时区',
    config: {
      brand: 'Chrome',
      platform: 'windows',
      lang: 'en-GB',
      timezone: 'Europe/London',
      resolution: '1920,1080',
      hardwareConcurrency: '8',
      disableNonProxiedUDP: true,
    },
  },
  {
    id: 'mac-chrome-us-edu',
    name: 'macOS / Chrome / 美国-教育',
    description: 'macOS、美国英语、纽约时区',
    config: {
      brand: 'Chrome',
      platform: 'macos',
      lang: 'en-US',
      timezone: 'America/New_York',
      resolution: '1440,900',
      hardwareConcurrency: '8',
      disableNonProxiedUDP: true,
    },
  },
]

export function applyLocaleToFingerprintArgs(args: string[], lang: string, timezone: string): string[] {
  const nextConfig = deserialize(args || [])
  if (lang) nextConfig.lang = lang
  if (timezone) nextConfig.timezone = timezone
  return serialize(nextConfig)
}
