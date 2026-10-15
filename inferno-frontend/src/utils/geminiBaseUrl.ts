const OFFICIAL_GEMINI_API_HOST = 'generativelanguage.googleapis.com'

/** A blank URL uses the official Gemini API; custom/proxied URLs are third-party. */
export function isOfficialGeminiBaseUrl(raw: string | null | undefined): boolean {
  const value = (raw ?? '').trim()
  if (!value) return true
  try {
    const url = new URL(value)
    const path = url.pathname.replace(/\/+$/, '')
    return url.protocol === 'https:' &&
      url.hostname.toLowerCase() === OFFICIAL_GEMINI_API_HOST &&
      url.port === '' && path === '' && url.search === ''
  } catch {
    return false
  }
}
