<template>
  <div class="relative" ref="dropdownRef">
    <Button
      variant="secondary"
      size="sm"
      type="button"
      @click="showDropdown = !showDropdown"
      class="auto-refresh__trigger"
      :title="t('common.autoRefresh.title')"
      :aria-expanded="showDropdown"
      :aria-controls="menuId"
      aria-haspopup="menu"
      @keydown.esc.stop.prevent="closeDropdown"
    >
      <svg
        class="h-3.5 w-3.5"
        :class="enabled ? 'animate-spin' : ''"
        xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor"
      >
        <path fill-rule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.433a.75.75 0 000-1.5H4.598a.75.75 0 00-.75.75v3.634a.75.75 0 001.5 0v-2.033l.312.312a7 7 0 0011.712-3.138.75.75 0 00-1.449-.39zm-10.624-2.848a5.5 5.5 0 019.201-2.466l.312.311H11.768a.75.75 0 000 1.5h3.634a.75.75 0 00.75-.75V3.537a.75.75 0 00-1.5 0v2.034l-.312-.312A7 7 0 002.628 8.397a.75.75 0 001.449.39z" clip-rule="evenodd" />
      </svg>
      <span>
        {{ enabled
          ? t('common.autoRefresh.countdown', { seconds: countdown })
          : t('common.autoRefresh.title')
        }}
      </span>
    </Button>

    <div
      v-if="showDropdown"
      :id="menuId"
      class="auto-refresh__menu"
      role="menu"
      :aria-label="t('common.autoRefresh.title')"
      @keydown.esc.stop.prevent="closeDropdown"
    >
      <div class="p-1.5">
        <button
          type="button"
          role="menuitemcheckbox"
          :aria-checked="enabled"
          @click="toggleEnabled"
          class="auto-refresh__option"
        >
          <span>{{ t('common.autoRefresh.enable') }}</span>
          <svg v-if="enabled" class="h-4 w-4 text-[var(--brand)]" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M16.704 4.153a.75.75 0 01.143 1.052l-8 10.5a.75.75 0 01-1.127.075l-4.5-4.5a.75.75 0 011.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 011.05-.143z" clip-rule="evenodd" />
          </svg>
        </button>
        <div class="my-1 border-t border-[var(--brand-line)] border-[var(--brand-line)]"></div>
        <button
          v-for="sec in intervals"
          :key="sec"
          type="button"
          role="menuitemradio"
          :aria-checked="intervalSeconds === sec"
          @click="selectInterval(sec)"
          class="auto-refresh__option"
        >
          <span>{{ t('common.autoRefresh.seconds', { n: sec }) }}</span>
          <svg v-if="intervalSeconds === sec" class="h-4 w-4 text-[var(--brand)]" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M16.704 4.153a.75.75 0 01.143 1.052l-8 10.5a.75.75 0 01-1.127.075l-4.5-4.5a.75.75 0 011.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 011.05-.143z" clip-rule="evenodd" />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, toRefs, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from './Button.vue'

const props = defineProps<{
  enabled: boolean
  intervalSeconds: number
  countdown: number
  intervals: readonly number[]
}>()

const { enabled, intervalSeconds, countdown, intervals } = toRefs(props)

const emit = defineEmits<{
  (e: 'update:enabled', value: boolean): void
  (e: 'update:interval', value: number): void
}>()

const { t } = useI18n()
const showDropdown = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const menuId = `auto-refresh-menu-${Math.random().toString(36).slice(2, 9)}`

const closeDropdown = () => {
  showDropdown.value = false
}

const toggleEnabled = () => {
  emit('update:enabled', !enabled.value)
  closeDropdown()
}

const selectInterval = (seconds: number) => {
  emit('update:interval', seconds)
  closeDropdown()
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    showDropdown.value = false
  }
}

onMounted(() => document.addEventListener('click', handleClickOutside))
onBeforeUnmount(() => document.removeEventListener('click', handleClickOutside))
</script>

<style scoped>
.auto-refresh__trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: var(--s2a-h-sm);
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--muted-foreground);
  font-family: inherit;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.auto-refresh__trigger:hover,
.auto-refresh__option:hover {
  background: var(--sidebar-accent);
}

.auto-refresh__trigger:focus-visible,
.auto-refresh__option:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.auto-refresh__menu {
  position: absolute;
  right: 0;
  z-index: 20;
  width: 176px;
  margin-top: 4px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--popover);
  box-shadow: var(--shadow-sm);
}

.auto-refresh__option {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  min-height: var(--s2a-h-sm);
  padding: 0 12px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--body-copy);
  font: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
}
</style>
