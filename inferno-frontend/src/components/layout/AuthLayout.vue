<template>
  <div class="auth">
    <!-- Left: the field. Order matters for keyboard and screen readers -- the
         panel is decoration, so the form column comes first in the DOM and is
         placed right by the grid, not by source order. -->
    <div class="auth__col auth__col--form">
      <div class="auth__inner">
        <header class="auth__head">
          <!-- The wordmark IS the logo: the site name set in the serif, no tile
               and no monogram. A letter in a coloured square is worse than the
               name itself set properly. An operator-uploaded logo still wins,
               because that is their brand and not ours to override. -->
          <img v-if="settingsLoaded && siteLogo" :src="siteLogo" alt="" class="auth__logo" />
          <h1 v-else class="auth__wordmark">{{ siteName }}</h1>
        </header>

        <div class="auth__body">
          <slot />
        </div>

        <div class="auth__foot">
          <slot name="footer" />
        </div>

        <p class="auth__legal">
          <span>&copy; {{ currentYear }} {{ siteName }}</span>
        </p>
      </div>
    </div>

    <!-- Right: one surface, and things taken out of it. Hidden below the
         breakpoint rather than stacked -- a decorative panel above a sign-in
         form on a phone is just something to scroll past. -->
    <div class="auth__col auth__col--panel">
      <AuthFieldPanel />
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * The split screen behind every auth route: panel on one side, a single column
 * on the other. Nine views render through this slot (login, register, forgot,
 * reset, verify, and the four OAuth callbacks), so the shape is defined once
 * here and none of them position anything themselves.
 *
 * The column is deliberately plainer than anything else in the library, because
 * a front door has exactly one job. No card, no glass, no shadow -- the form
 * sits directly on the page.
 */
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import AuthFieldPanel from '@/components/auth/AuthFieldPanel.vue'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth {
  display: grid;
  /* The form column is fixed and the panel takes the rest, so the reading
     measure never stretches on a wide monitor -- the panel absorbs the width
     instead. */
  grid-template-columns: minmax(0, 560px) 1fr;
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--background);
}

.auth__col {
  min-width: 0;
}

.auth__col--form {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 40px;
}

.auth__col--panel {
  position: relative;
  overflow: hidden;
}

.auth__inner {
  display: flex;
  width: 100%;
  max-width: 380px;
  flex-direction: column;
}

.auth__head {
  margin-bottom: 32px;
}

.auth__wordmark {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: 30px;
  font-weight: 400;
  /* Slightly tight, because the serif is drawn generous and the wordmark is the
     only place it appears at this size. */
  letter-spacing: -0.01em;
  line-height: 1.1;
}

.auth__logo {
  display: block;
  height: 34px;
  width: auto;
  max-width: 200px;
  object-fit: contain;
}

.auth__foot:not(:empty) {
  margin-top: 24px;
}

.auth__legal {
  margin: 40px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* Below this the panel is dropped rather than stacked. The form keeps the exact
   same column it had, so nothing about it reflows -- it just centres. */
@media (max-width: 900px) {
  .auth {
    grid-template-columns: minmax(0, 1fr);
  }

  .auth__col--panel {
    display: none;
  }

  .auth__col--form {
    padding: 40px 24px;
  }
}
</style>
