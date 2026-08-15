<template>
  <div class="profile-settings-row" :class="{ 'profile-settings-row--open': open }">
    <div class="profile-settings-row__main">
      <div class="profile-settings-row__copy">
        <strong>{{ label }}</strong>
        <span>{{ description }}</span>
      </div>
      <div class="profile-settings-row__actions">
        <span v-if="value" class="profile-settings-row__value" :class="`profile-settings-row__value--${tone}`">
          {{ value }}
        </span>
        <button
          v-if="action"
          type="button"
          class="profile-settings-row__button"
          :aria-expanded="open"
          @click="toggle"
        >
          {{ open ? closeAction : action }}
        </button>
      </div>
    </div>
    <div v-if="open" class="profile-settings-row__details">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{
  label: string
  description: string
  value?: string
  tone?: 'default' | 'success' | 'muted'
  action?: string
  closeAction?: string
  open?: boolean
}>(), {
  value: '',
  tone: 'default',
  action: '',
  closeAction: 'Close',
  open: false,
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

function toggle(): void {
  emit('update:open', !props.open)
}
</script>

<style scoped>
.profile-settings-row + .profile-settings-row {
  border-top: 1px solid var(--border-subtle);
}

.profile-settings-row__main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  padding: 9px 14px;
}

.profile-settings-row__copy {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.profile-settings-row__copy strong,
.profile-settings-row__copy span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-settings-row__copy strong {
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-regular);
}

.profile-settings-row__copy span {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.4;
}

.profile-settings-row__actions {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 7px;
  flex-shrink: 0;
}

.profile-settings-row__value {
  color: var(--body-copy);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.profile-settings-row__value--success {
  color: var(--s2a-ok, var(--success));
}

.profile-settings-row__value--muted {
  color: var(--muted-foreground);
}

.profile-settings-row__button {
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--body-copy);
  font: inherit;
  font-size: var(--fs-sm);
  white-space: nowrap;
  cursor: pointer;
  transition: background-color var(--t-fast) var(--ease-out), border-color var(--t-fast) var(--ease-out);
}

.profile-settings-row__button:hover,
.profile-settings-row__button:focus-visible {
  border-color: var(--brand-line);
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.profile-settings-row__button:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.profile-settings-row__details {
  padding: 0 14px 14px;
}

.profile-settings-row--open .profile-settings-row__main {
  padding-bottom: 7px;
}

@media (max-width: 600px) {
  .profile-settings-row__main {
    grid-template-columns: 1fr;
    align-items: start;
  }

  .profile-settings-row__actions {
    justify-content: flex-start;
  }
}
</style>
