import type { LocationQuery } from 'vue-router'

/**
 * Resolves the `?redirect=` target for an already-authenticated visit to
 * `/login`, or `''` when there is none to honour.
 *
 * Extracted into its own file (mirrors setupRedirect.ts's existing
 * pattern) so router/index.ts's beforeEach guard and its test can both
 * import the SAME function, rather than the test re-implementing the
 * branch by hand -- Task 4 fix round 2, review NEW-1: the first version of
 * this logic lived inline in router/index.ts, and guards.spec.ts asserted
 * against a hand-copied mirror of it. The mirror and the real branch
 * agreed on the day it was written, but nothing would have caught them
 * drifting apart, and reverting the real fix left the mirrored test green
 * -- exactly the "backend was tested, the integration seam was not"
 * mistake F1 itself was.
 *
 * GET/POST /oauth/authorize (backend/internal/handler/
 * oauth_authorize_handler.go) bounces EVERY bearer-less hit to
 * /login?redirect=<original oauth path+query> -- including a hit from a
 * browser that is already signed in, since Inferno has no session cookie
 * and the backend cannot otherwise tell "logged out" from "logged in but
 * this particular navigation carries no header" apart. Without honouring
 * `redirect` on an authenticated /login visit, that already-authenticated
 * case was bounced to the hardcoded dashboard path and the OAuth request
 * silently evaporated -- the common case, not an edge case, since a
 * desktop app's login link opens in whatever browser profile the person
 * already uses daily.
 *
 * Scoped to `/login` only (not `/register`) and to a genuine string query
 * value -- `to.query.redirect` is `string | string[] | null` per
 * vue-router's LocationQuery, and only the single-string shape is a
 * `next()`-able target.
 */
export function resolveAuthenticatedLoginRedirect(toPath: string, toQuery: LocationQuery): string {
  if (toPath !== '/login') {
    return ''
  }
  const redirect = toQuery.redirect
  return typeof redirect === 'string' ? redirect : ''
}
