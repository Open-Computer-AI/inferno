import type { Router } from 'vue-router'

type ViewTransitionDocument = Document & {
  startViewTransition?: (update: () => void | Promise<void>) => {
    finished: Promise<void>
  }
}

/** Navigate between the public auth modes with a shared-surface transition. */
export async function navigateAuthMode(router: Router, path: string): Promise<void> {
  const transitionDocument = document as ViewTransitionDocument

  if (!transitionDocument.startViewTransition) {
    await router.push(path)
    return
  }

  // Keep the method call attached to the document. Chromium throws
  // "Illegal invocation" when startViewTransition is called detached.
  const transition = transitionDocument.startViewTransition(() => router.push(path))
  await transition.finished
}
