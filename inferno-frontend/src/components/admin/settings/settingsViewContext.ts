import type { InjectionKey } from 'vue'

/**
 * Settings pages are presentation modules around one existing controller.
 * Keeping this bridge private to the settings workspace lets page extraction
 * preserve the controller's API calls and validation until the state itself is
 * split into domain composables.
 */
export type SettingsViewContext = Record<string, any>

export const SETTINGS_VIEW_CONTEXT: InjectionKey<SettingsViewContext> =
  Symbol('inferno-settings-view-context')
