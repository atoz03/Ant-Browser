export function openExternalURL(rawURL: string): void {
  const url = rawURL.trim()
  if (!url) return

  try {
    const parsed = new URL(url)
    if (!['http:', 'https:', 'mailto:'].includes(parsed.protocol)) {
      throw new Error('不支持的外部链接协议')
    }
  } catch (error) {
    console.error('无法打开外部链接', error)
    return
  }

  const runtime = (window as Window & {
    runtime?: { BrowserOpenURL?: (value: string) => void }
  }).runtime
  if (runtime?.BrowserOpenURL) {
    runtime.BrowserOpenURL(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}
