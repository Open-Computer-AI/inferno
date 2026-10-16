import { describe, expect, it } from 'vitest'
import { isOfficialGeminiBaseUrl } from '../geminiBaseUrl'

describe('isOfficialGeminiBaseUrl', () => {
  it.each([
    undefined,
    '',
    '  ',
    'https://generativelanguage.googleapis.com',
    'https://generativelanguage.googleapis.com/',
    'https://generativelanguage.googleapis.com//',
    'HTTPS://GENERATIVELANGUAGE.GOOGLEAPIS.COM',
    'hTtPs://GeNeRaTiVeLaNgUaGe.GoOgLeApIs.CoM/'
  ])('recognizes official Gemini base URLs (%s)', (url) => {
    expect(isOfficialGeminiBaseUrl(url)).toBe(true)
  })

  it.each([
    'http://generativelanguage.googleapis.com',
    'https://generativelanguage.googleapis.com/v1beta',
    'https://generativelanguage.googleapis.com:443',
    'https://generativelanguage.googleapis.com:8443',
    'https://api.example.test',
    'https://evil-googleapis.com',
    'https://generativelanguage.googleapis.com.evil.test',
    'https://evil.generativelanguage.googleapis.com',
    'https://generativelanguage.googleapis.com.evil-googleapis.com',
    'https://user@generativelanguage.googleapis.com',
    'https://:password@generativelanguage.googleapis.com',
    'https://generativelanguage.googleapis.com@evil.test',
    'https://generativelanguage.googleapis.com.',
    'https://generativelanguage.googleapis.com?key=value',
    'https://generativelanguage.googleapis.com#fragment',
    'https:generativelanguage.googleapis.com',
    'https:/generativelanguage.googleapis.com',
    'https:///generativelanguage.googleapis.com',
    'https:\\\\generativelanguage.googleapis.com',
    'not a URL',
    'https://'
  ])('treats non-official URLs as third-party (%s)', (url) => {
    expect(isOfficialGeminiBaseUrl(url)).toBe(false)
  })
})
