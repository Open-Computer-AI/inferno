export type SettingsSectionKey =
  | 'general'
  | 'appearance'
  | 'agreement'
  | 'features'
  | 'security'
  | 'users'
  | 'gateway'
  | 'payment'
  | 'email'
  | 'backup'

export type SettingsSectionIcon =
  | 'home'
  | 'sun'
  | 'document'
  | 'bolt'
  | 'shield'
  | 'user'
  | 'server'
  | 'creditCard'
  | 'mail'
  | 'database'

export type SettingsGroupKey =
  | 'workspace'
  | 'access'
  | 'platform'
  | 'revenue'
  | 'communication'
  | 'data'

export interface SettingsSection {
  key: SettingsSectionKey
  icon: SettingsSectionIcon
  group: SettingsGroupKey
}

export interface SettingsSectionGroup {
  key: SettingsGroupKey
  sections: SettingsSection[]
}

export const SETTINGS_SECTIONS: readonly SettingsSection[] = [
  { key: 'general', icon: 'home', group: 'workspace' },
  { key: 'appearance', icon: 'sun', group: 'workspace' },
  { key: 'security', icon: 'shield', group: 'access' },
  { key: 'users', icon: 'user', group: 'access' },
  { key: 'agreement', icon: 'document', group: 'platform' },
  { key: 'features', icon: 'bolt', group: 'platform' },
  { key: 'gateway', icon: 'server', group: 'platform' },
  { key: 'payment', icon: 'creditCard', group: 'revenue' },
  { key: 'email', icon: 'mail', group: 'communication' },
  { key: 'backup', icon: 'database', group: 'data' },
]

const SETTINGS_GROUP_KEYS = [
  'workspace',
  'access',
  'platform',
  'revenue',
  'communication',
  'data',
] as const satisfies readonly SettingsGroupKey[]

export const SETTINGS_GROUPS: readonly SettingsSectionGroup[] = [
  ...SETTINGS_GROUP_KEYS,
].map((key): SettingsSectionGroup => ({
  key,
  sections: SETTINGS_SECTIONS.filter((section) => section.group === key),
}))

export function isSettingsSectionKey(value: unknown): value is SettingsSectionKey {
  return typeof value === 'string' && SETTINGS_SECTIONS.some((section) => section.key === value)
}
