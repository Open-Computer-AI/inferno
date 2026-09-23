import type { GroupPlatform } from '@/types'

export type KeyGroupProvider = 'anthropic' | 'openai' | 'domestic' | 'other'

export const KEY_GROUP_PROVIDERS = ['anthropic', 'openai', 'domestic', 'other'] as const

// Group names are user-provided and can be ambiguous. Classification must use
// the configured platform, which is the stable part of the group contract.
const PROVIDER_BY_PLATFORM: Record<GroupPlatform, KeyGroupProvider> = {
  anthropic: 'anthropic',
  openai: 'openai',
  gemini: 'other',
  antigravity: 'other',
  grok: 'other',
  kimi: 'domestic',
  zhipu: 'domestic',
  deepseek: 'domestic',
  minimax: 'domestic',
  opencode_go: 'other',
  composite: 'other'
}

export function getKeyGroupProvider(platform: GroupPlatform): KeyGroupProvider {
  return PROVIDER_BY_PLATFORM[platform] ?? 'other'
}

// Collections use representative marks rather than inventing a brand logo
// for an aggregate provider bucket.
export const KEY_GROUP_PROVIDER_ICONS: Record<KeyGroupProvider, GroupPlatform[]> = {
  anthropic: ['anthropic'],
  openai: ['openai'],
  domestic: ['deepseek', 'kimi'],
  other: ['gemini', 'grok']
}
