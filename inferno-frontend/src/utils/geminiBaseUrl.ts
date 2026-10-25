const OFFICIAL_GEMINI_API_HOST = 'generativelanguage.googleapis.com'

/** A blank URL uses the official Gemini API; custom/proxied URLs are third-party. */
export function isOfficialGeminiBaseUrl(raw: string | null | undefined): boolean {
  const value = (raw ?? '').trim()
  if (!value) return true
  if (!/^https:\/\/[^/?#\\]/i.test(value) || value.includes('\\')) return false
  try {
    const url = new URL(value)
    const authority = value.match(/^https:\/\/([^/?#\\]*)/i)?.[1]
    const path = url.pathname.replace(/\/+$/, '')
    return url.protocol === 'https:' &&
      url.hostname.toLowerCase() === OFFICIAL_GEMINI_API_HOST &&
      authority?.toLowerCase() === OFFICIAL_GEMINI_API_HOST &&
      url.username === '' &&
      url.password === '' &&
      url.port === '' &&
      path === '' &&
      url.search === '' &&
      url.hash === ''
  } catch {
    return false
  }
}
