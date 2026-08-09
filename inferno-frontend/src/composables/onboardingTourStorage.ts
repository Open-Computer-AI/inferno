/**
 * The onboarding tour's "already seen" flag, split out from
 * `useOnboardingTour.ts` so it can be read without dragging in driver.js.
 *
 * AppSidebar needs this to decide its first-paint section, and it must not
 * import the composable to get it, for two separate reasons:
 *
 *   1. Calling `useOnboardingTour()` registers lifecycle hooks and an
 *      auto-start timer, so calling it merely to read a flag would start a
 *      second tour.
 *   2. Even a type-only path through that module pulls in `driver.js` and its
 *      stylesheet, which would then be a hard dependency of every sidebar unit
 *      test.
 *
 * Keeping the key derivation here rather than duplicating it at each call site
 * matters because of STORAGE_VERSION: two copies would drift on the next bump,
 * and the sidebar would believe the tour was pending forever.
 */

const STORAGE_VERSION = 'v4_interactive' // Bump version for new tour type

export function onboardingTourStorageKey(
  baseKey: string,
  userId: string | number | undefined,
  role: string | undefined
): string {
  return `${baseKey}_${userId ?? 'guest'}_${role ?? 'user'}_${STORAGE_VERSION}`
}

export function hasSeenOnboardingTour(
  baseKey: string,
  userId: string | number | undefined,
  role: string | undefined
): boolean {
  try {
    return localStorage.getItem(onboardingTourStorageKey(baseKey, userId, role)) === 'true'
  } catch {
    // localStorage unavailable (private browsing, quota). Report the tour as
    // already seen: a tour that cannot record its own completion would
    // otherwise restart on every navigation.
    return true
  }
}
