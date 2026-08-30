<template>
  <div class="auth-shell">
    <div class="auth">
      <!-- The watercolor is the left panel only. The right column uses the same
           paper tone as the canvas so the split stays quiet and intentional. -->
      <div class="auth__panel">
        <WatercolorMosaic />
      </div>

      <div class="auth__col">
        <div class="auth__inner">
          <header class="auth__head">
            <!-- The wordmark IS the logo: the site name set in the serif, no tile
                 and no monogram. An operator-uploaded logo still wins, because
                 that is their brand and not ours to override. -->
            <img v-if="settingsLoaded && siteLogo" :src="siteLogo" alt="" class="auth__logo" />
            <h1 v-else class="auth__wordmark">{{ siteName }}</h1>
            <!-- Two different things, so two lines. The operator's site_subtitle
                 is their tagline and upstream showed it on every auth page; the
                 per-view slot is the instruction for THIS page ("Log into your
                 account"). Folding the tagline into the slot as a fallback hid it
                 on exactly the four views that set one -- login, register, reset,
                 forgot -- which is where an operator looks for it. -->
            <p v-if="siteSubtitle" class="auth__tagline">{{ siteSubtitle }}</p>
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
import WatercolorMosaic from '@/components/auth/WatercolorMosaic.vue'
import { PRODUCT_NAME } from '@/config/brand'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || PRODUCT_NAME)
// Operator-configured, and still edited in Admin -> General settings. Losing it
// here left it visible on the landing page but not on login/register/reset.
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
/* The browser is the quiet outer canvas; the auth surface is an inset card. */
.auth-shell {
  box-sizing: border-box;
  min-height: 100vh;
  min-height: 100dvh;
  padding: 10px;
  background: rgb(237 235 231);
}

/* Two equal halves, as the mock renders them (575 | 575). */
.auth {
  --auth-paper: rgb(247 244 237);
  --background: var(--auth-paper);
  --foreground: rgb(45 42 38);
  --body-copy: rgb(84 78 70);
  --muted-foreground: rgb(116 108 98);
  --card: rgb(255 253 248 / 0.78);
  --surface-subtle: rgb(255 253 248 / 0.62);
  --input: rgb(118 108 96 / 0.38);
  --border: rgb(118 108 96 / 0.32);
  --border-subtle: rgb(118 108 96 / 0.2);
  --sidebar-accent: rgb(118 108 96 / 0.1);
  --primary: rgb(45 42 38);
  --primary-foreground: rgb(255 253 248);
  --ring-focus: rgb(169 83 61);
  --focus-ring: rgb(169 83 61 / 0.18);
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 100vh;
  min-height: calc(100dvh - 20px);
  overflow: hidden;
  border: 1px solid rgb(48 44 38 / 0.08);
  border-radius: 14px;
  box-shadow: 0 1px 4px rgb(48 44 38 / 0.06);
  background: var(--auth-paper);
}

.auth__panel {
  position: relative;
  min-width: 0;
  overflow: hidden;
}

.auth__col {
  display: grid;
  grid-column: 2;
  align-items: center;
  min-width: 0;
  padding: 40px 30px;
  background: var(--auth-paper);
}

/* Fixed measure, centred. The column does not stretch with the viewport -- the
   panel absorbs the extra width instead. */
.auth__inner {
  width: 100%;
  max-width: 340px;
  margin: 0 auto;
}

.auth__head {
  text-align: center;
}

.auth__wordmark {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-auth-mark);
  font-weight: 400;
  line-height: 1.5;
}

.auth__logo {
  display: block;
  height: 30px;
  width: auto;
  max-width: 200px;
  margin: 0 auto;
  object-fit: contain;
}

/* Set in the serif, like the wordmark above it -- the two read as one lockup. */
.auth__tagline {
  margin: 8px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.5;
}

.auth__subtitle {
  margin: 16px 0 0;
  color: var(--body-copy);
  font-family: var(--font-serif);
  font-size: var(--fs-xl);
  line-height: 1.5;
}

.auth__body {
  margin-top: 28px;
}

.auth__foot:not(:empty) {
  margin-top: 34px;
  text-align: center;
}

/* Below this the panel is dropped rather than stacked. A decorative panel above
   a sign-in form on a phone is just something to scroll past. */
@media (max-width: 900px) {
  .auth-shell {
    padding: 6px;
  }

  .auth {
    grid-template-columns: minmax(0, 1fr);
    min-height: calc(100dvh - 12px);
  }

  .auth__panel {
    display: none;
  }

  .auth__col {
    grid-column: 1;
  }
}
</style>
