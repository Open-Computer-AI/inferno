/**
 * Layout Components
 * Export all layout components for easy importing
 */

export { default as AppLayout } from './AppLayout.vue'
export { default as AppSidebar } from './AppSidebar.vue'
export { default as AuthLayout } from './AuthLayout.vue'

/* Page primitives, one per part-14 archetype. Every view in phase 5 assembles
   from these rather than inventing its own header, strip or group. */
export { default as PageHeader } from './PageHeader.vue'
export { default as TablePageLayout } from './TablePageLayout.vue'
export { default as StatStrip } from './StatStrip.vue'
export { default as SettingsGroup } from './SettingsGroup.vue'
export { default as SettingsRow } from './SettingsRow.vue'
export { default as MessagePage } from './MessagePage.vue'
export { default as InterstitialState } from './InterstitialState.vue'
