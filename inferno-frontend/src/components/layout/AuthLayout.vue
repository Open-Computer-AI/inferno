<template>
  <div class="auth">
    <!-- Panel LEFT, column right. Part 17: "A split screen. The panel on the
         left ... The column on the right is deliberately plainer than anything
         else in the library, because a front door has exactly one job." -->
    <div class="auth__panel">
      <AuthFieldPanel />
    </div>

    <div class="auth__col">
      <div class="auth__inner">
        <header class="auth__head">
          <!-- The wordmark IS the logo: the site name set in the serif, no tile
               and no monogram. An operator-uploaded logo still wins, because
               that is their brand and not ours to override. -->
          <img v-if="settingsLoaded && siteLogo" :src="siteLogo" alt="" class="auth__logo" />
          <h1 v-else class="auth__wordmark">{{ siteName }}</h1>
          <p v-if="$slots.subtitle" class="auth__subtitle"><slot name="subtitle" /></p>
        </header>

        <div class="auth__body">
          <slot />
        </div>

        <div v-if="$slots.footer" class="auth__foot">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * The split screen behind every auth route. Nine views render through this slot
 * (login, register, forgot, reset, verify, and the four OAuth callbacks), so the
 * shape is defined once here and none of them position anything themselves.
 *
 * Measurements are taken from part 17's own mock, not interpreted from its prose:
 * two equal columns, a 296px content column centred in the right half with
 * 40px/30px padding, the wordmark at 20px in the serif and the subtitle at 14px
 * in the same face 14px below it.
 */
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import AuthFieldPanel from '@/components/auth/AuthFieldPanel.vue'
import { PRODUCT_NAME } from '@/config/brand'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || PRODUCT_NAME)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
/* Two equal halves, as the mock renders them (575 | 575). */
.auth {
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--background);
}

.auth__panel {
  position: relative;
  min-width: 0;
  overflow: hidden;
}

.auth__col {
  display: grid;
  align-items: center;
  min-width: 0;
  padding: 40px 30px;
}

/* Fixed measure, centred. The column does not stretch with the viewport -- the
   panel absorbs the extra width instead. */
.auth__inner {
  width: 100%;
  max-width: 296px;
  margin: 0 auto;
}

.auth__head {
  text-align: center;
}

.auth__wordmark {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.5;
}

.auth__logo {
  display: block;
  height: 26px;
  width: auto;
  max-width: 200px;
  margin: 0 auto;
  object-fit: contain;
}

/* Set in the serif, like the wordmark above it -- the two read as one lockup. */
.auth__subtitle {
  margin: 14px 0 0;
  color: var(--body-copy);
  font-family: var(--font-serif);
  font-size: var(--fs-lg);
  line-height: 1.5;
}

.auth__body {
  margin-top: 24px;
}

.auth__foot:not(:empty) {
  margin-top: 34px;
  text-align: center;
}

/* Below this the panel is dropped rather than stacked. A decorative panel above
   a sign-in form on a phone is just something to scroll past. */
@media (max-width: 900px) {
  .auth {
    grid-template-columns: minmax(0, 1fr);
  }

  .auth__panel {
    display: none;
  }
}
</style>
