import { computed, getCurrentInstance, inject } from 'vue'
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
  const bindings = injected ?? setupState(owner)
  const form = bindings.form ?? setupState(owner).form
  // The API stores platform user IDs as a comma-separated string; the selector exposes IDs.
  const riskControlAllowlistedUserIds = computed<number[]>({
    get: () => Array.from(new Set(
      String(form.cyber_policy_user_allowlist ?? '')
        .split(/[,\s]+/)
        .map(Number)
        .filter((id) => Number.isSafeInteger(id) && id > 0),
    )),
    set: (ids) => {
      form.cyber_policy_user_allowlist = ids.join(',')
    },
  })
  /* Return plain enumerable keys. Vue registers setup bindings by enumerating
   * the setup result, while the refs inside this object remain live. */
  return { ...bindings, riskControlAllowlistedUserIds, t }
}
