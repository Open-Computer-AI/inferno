<template>
  <div class="shell" :class="{ 'shell--collapsed': sidebarCollapsed, 'shell--settings': variant === 'settings' }">
    <AppSidebar v-if="variant === 'default'" />
    <slot v-else name="sidebar" />

    <div class="shell__content">
      <!-- The header's hamburger needed a new home once the header was
           deleted (part 07 v2 #header). Below the rail's own breakpoint the
           sidebar is an off-canvas overlay again, so this corner button is
           the only way back in on a narrow viewport. Desktop-first, so the
           prototype (a 1280px preview) never shows it. -->
      <button
        type="button"
        class="shell__mobile-toggle"
        :aria-label="t('shell.openMenu')"
        @click="appStore.toggleMobileSidebar()"
      >
        <i class="hgi-stroke hgi-menu-01" aria-hidden="true" />
      </button>

      <!-- The content card: June's one shell exception to "cards have no
           shadow" (ground rule 9). 10px --r-window radius, --shadow-sm plus
           --shadow-inset, inset 7px on three sides. -->
      <main class="shell__card">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * AppLayout -- part 07 v2, sections 01 and 02.
 *
 * 37 views wrap themselves in this component; its default slot and its
 * `defineExpose({ replayTour })` are the entire public contract, and both are
 * unchanged from the previous shell. What moved is everything inside: no
 * header (AppHeader.vue is deleted, see the report for where its four jobs
 * went), and the sidebar now sits on the tinted canvas instead of inside the
 * same white surface as the content.
 */
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const { variant = 'default' } = defineProps<{
  variant?: 'default' | 'settings'
}>()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
/* Geometry (part 07 v2 #geometry): one tinted canvas, full bleed -- it is the
   window, not a panel -- with the rail and the content card as its two
   children. --sidebar-w / --sidebar-w-collapsed drive both this grid and the
   rail's own width, so they can never drift apart. */
.shell {
  display: grid;
  grid-template-columns: var(--sidebar-w) minmax(0, 1fr);
  /* Exactly the viewport, not a floor. The shell is the window frame, so it
     never grows -- scrolling happens inside the card below. dvh, not vh, so
     mobile Safari's collapsing URL bar does not leave a strip of canvas under
     the card. */
  height: 100dvh;
  overflow: hidden;
  background: var(--sidebar);
  transition: grid-template-columns var(--motion-layout);
}
.shell--collapsed {
  grid-template-columns: var(--sidebar-w-collapsed) minmax(0, 1fr);
}

.shell__content {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  position: relative;
}

.shell__mobile-toggle {
  display: none;
  position: absolute;
  top: 14px;
  left: 14px;
  z-index: 10;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--foreground);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.shell__mobile-toggle:hover {
  background: var(--sidebar-accent);
}
.shell__mobile-toggle:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.shell__mobile-toggle i {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph */
}

/* The content card: --r-window matches the native window curve, and this is
   the one surface in the whole shell allowed a shadow -- ground rule 9 says
   cards carry none, and this composite of --shadow-sm plus --shadow-inset is
   the shell's documented exception to it, not an oversight. */
/*
 * THE CARD IS THE SCROLL CONTAINER, NOT THE DOCUMENT.
 *
 * This was `min-height: calc(100vh - 14px)`, and a minimum let the card grow
 * to whatever its content needed -- 2,485px on the admin dashboard. Its
 * `overflow: auto` therefore never engaged (scrollHeight === clientHeight) and
 * the DOCUMENT scrolled instead, dragging the card with it: the 7px inset, the
 * rounded corners and the shadow all slid off the top, so the window frame only
 * existed at scroll position 0.
 *
 * `flex: 1` against a shell fixed to 100dvh pins the card to the viewport, and
 * `min-height: 0` is what actually lets it shrink below its content so the
 * overflow can do its job -- a flex item defaults to min-height:auto and would
 * refuse. Now the frame never moves and only the content inside it scrolls.
 */
.shell__card {
  flex: 1;
  min-width: 0;
  min-height: 0;
  margin: 7px 7px 7px 0;
  border-radius: var(--r-window);
  background: var(--card);
  box-shadow: var(--shadow-sm), var(--shadow-inset);
  overflow-x: hidden;
  overflow-y: auto;
  padding: 16px;
  /* Scrolls, never shows a bar -- and that is a padding fix as much as a
     cosmetic one. A classic scrollbar is laid out INSIDE the content box, so
     it took 8px off the right: content sat 40px from the right edge against
     32px on the other three sides. Removing the bar restores the symmetry the
     padding always declared. Wheel, trackpad, keyboard and scrollIntoView are
     untouched. */
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.shell--settings .shell__card {
  padding: 0;
}

.shell__card::-webkit-scrollbar {
  width: 0;
  height: 0;
  display: none;
}

@media (min-width: 768px) {
  .shell__card {
    padding: 24px;
  }
}
@media (min-width: 1024px) {
  .shell__card {
    padding: 32px;
  }
}

@media (max-width: 1023px) {
  .shell {
    grid-template-columns: minmax(0, 1fr);
  }
  .shell__mobile-toggle {
    display: grid;
  }
  .shell__card {
    margin: 7px;
  }

  .shell--settings {
    grid-template-rows: auto minmax(0, 1fr);
  }

  .shell--settings .shell__card {
    margin: 0 7px 7px;
  }

  /* Settings keeps its own visible horizontal navigation rail on mobile, so
     the global off-canvas menu button would only cover the page heading. */
  .shell--settings .shell__mobile-toggle {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .shell {
    transition: none;
  }
}
</style>
