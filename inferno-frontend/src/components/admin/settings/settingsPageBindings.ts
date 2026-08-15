import { getCurrentInstance, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { SETTINGS_VIEW_CONTEXT } from './settingsViewContext'

/**
 * Makes the controller bindings available to a page template while keeping
 * translations local to the extracted component. The local `t` fallback also
 * keeps isolated component tests independent from the parent instance proxy.
 */
export function useSettingsPageBindings(): Record<string, any> {
  const injected = inject(SETTINGS_VIEW_CONTEXT, undefined)
  const instance = getCurrentInstance()
  let owner = instance?.parent
  const setupState = (candidate: typeof owner) =>
    (candidate as (typeof candidate & { setupState?: Record<string, any> }) | null)
      ?.setupState ?? {}
  while (owner && !('form' in setupState(owner))) {
    owner = owner.parent
  }
  const { t } = useI18n()
  /* Return plain enumerable keys. Vue registers setup bindings by enumerating
   * the setup result, while the refs inside this object remain live. */
  return { ...(injected ?? setupState(owner)), t }
}
