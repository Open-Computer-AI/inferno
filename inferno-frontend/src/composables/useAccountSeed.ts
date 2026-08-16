import { computed, toValue, type ComputedRef, type MaybeRefOrGetter } from 'vue'

/**
 * WHY THIS EXISTS.
 *
 * `avatar_seed` is the persisted "regenerate my identicon" seed. Four places
 * render the same person's generated avatar -- the shell rail (AppSidebar), the
 * profile card that writes the seed (ProfileAvatarCard), the profile settings
 * sidebar, and the admin settings sidebar -- and each one had hand-copied the
 * same `avatar_seed || String(id || email || displayName)` expression.
 *
 * Four copies is four chances to drift, and it did drift: AppSidebar was missing
 * the `avatar_seed` branch entirely, so regenerating the avatar on the profile
 * page left the rail painting the old pattern and the same user wore two
 * different avatars on screen at once. The fix was a one-line patch to one copy;
 * nothing stopped the next copy from going stale the same way.
 *
 * So the derivation lives here once. A new reader calls this instead of
 * re-deriving, and a change to the precedence order reaches every avatar at
 * the same time.
 *
 * PRECEDENCE. `avatar_seed`, then id, then email, then the caller's display
 * name, then an optional caller-supplied last resort. The chain is `||`, not
 * `??`: an id of `0` and an empty email are placeholders, not identities, and
 * must fall through rather than produce the seed `'0'` or `''`.
 */

/** The subset of a user this derivation actually reads. */
export interface AccountSeedUser {
  id?: number | string | null
  email?: string | null
  avatar_seed?: string | null
}

/**
 * @param source      The user to seed from. A ref, a getter, or a plain value,
 *                    so a component can pass either a prop (`() => props.user`)
 *                    or a store (`() => authStore.user`).
 * @param displayName The caller's already-computed display name. Each call site
 *                    builds this differently (some fall back to a translated
 *                    string, some to `''`), so it stays the caller's business.
 * @param finalFallback Last resort when nothing else identifies the user.
 *                    Defaults to `''`, which reproduces the plain
 *                    `String(id || email || displayName)` behaviour exactly.
 *                    The shell rail passes `'inferno-account'` because it is
 *                    always mounted and must never render an empty seed.
 */
export function useAccountSeed(
  source: MaybeRefOrGetter<AccountSeedUser | null | undefined>,
  displayName: MaybeRefOrGetter<string>,
  finalFallback = ''
): ComputedRef<string> {
  return computed(() => {
    const user = toValue(source)
    return user?.avatar_seed || String(user?.id || user?.email || toValue(displayName) || finalFallback)
  })
}
