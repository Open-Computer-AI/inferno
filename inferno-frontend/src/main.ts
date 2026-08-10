import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import { updateFavicon } from '@/utils/branding'
import { isIOSDevice } from '@/utils/device'
import './style.css'
// June after style.css so its token layer and base rules win the cascade over
// the Tailwind preflight that style.css pulls in.
import './design-system/june.css'
import { PRODUCT_NAME, PRODUCT_TAGLINE } from '@/config/brand'

function initIOSViewportZoomFix() {
  // iOS Safari 在输入框字号小于 16px 时聚焦会自动放大页面，且失焦后不会恢复。
  // 限制 maximum-scale 可阻止该行为；iOS 10+ 用户仍可双指手动缩放，不影响可访问性。
  // 仅在 iOS 设备上注入，避免影响 Android Chrome 的手动缩放能力。
  if (!isIOSDevice()) return

  const viewport = document.querySelector('meta[name="viewport"]')
  if (!viewport) return

  const content = viewport.getAttribute('content') || ''
  if (/maximum-scale/i.test(content)) return
  viewport.setAttribute('content', `${content}, maximum-scale=1.0`)
}

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
  syncJuneAttributes()
}

/**
 * Mirror the existing .dark class onto June's [data-theme], and set the brand.
 *
 * The two systems signal dark differently: Tailwind toggles .dark, June swaps
 * token values under [data-theme="dark"]. During the rewrite both kinds of
 * component are on screen at once, so both signals have to be present or the
 * converted half renders its light palette on a dark page.
 *
 * A MutationObserver rather than a call site: .dark is toggled from several
 * places (this bootstrap, the settings screen, the OS preference listener), and
 * mirroring at the source keeps all of them working untouched. Delete this,
 * the .dark half of tailwind.config's darkMode, and the class itself once the
 * last Tailwind component is gone.
 *
 * data-brand selects one of June's five presets (clay, rose, sage, ocean,
 * plum). Fixed to clay until there is a setting for it.
 */
function syncJuneAttributes() {
  const root = document.documentElement
  root.setAttribute('data-theme', root.classList.contains('dark') ? 'dark' : 'light')
  if (!root.hasAttribute('data-brand')) root.setAttribute('data-brand', 'clay')
}

function watchThemeClass() {
  new MutationObserver(syncJuneAttributes).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class']
  })
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()
  watchThemeClass()
  initIOSViewportZoomFix()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings from injected config BEFORE mounting (prevents flash)
  // This must happen after pinia is installed but before router and i18n
  const appStore = useAppStore()
  appStore.initFromInjectedConfig()

  // Set document title immediately after config is loaded
  // Only override when the operator actually set a name; otherwise index.html's
  // own title already carries the product name and rewriting it just flickers.
  if (appStore.siteName && appStore.siteName !== PRODUCT_NAME) {
    document.title = `${appStore.siteName} - ${PRODUCT_TAGLINE}`
  }
  updateFavicon(appStore.siteLogo)

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

bootstrap()
