import { describe, expect, it } from 'vitest'
import { isOfficialGeminiBaseUrl } from '../geminiBaseUrl'

describe('isOfficialGeminiBaseUrl', () => {
  it.each([undefined, '', '  ', 'https://generativelanguage.googleapis.com', 'https://generativelanguage.googleapis.com/'])('recognizes official Gemini base URLs (%s)', (url) => {
    expect(isOfficialGeminiBaseUrl(url)).toBe(true)
  })

  it.each([
    'http://generativelanguage.googleapis.com',
    'https://generativelanguage.googleapis.com/v1beta',
    'https://generativelanguage.googleapis.com:8443',
    'https://api.example.test'
  ])('treats custom URLs as third-party (%s)', (url) => {
    expect(isOfficialGeminiBaseUrl(url)).toBe(false)
  })
})
