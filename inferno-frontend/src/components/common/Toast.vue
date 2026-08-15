<template>
  <Teleport to="body">
    <div
      class="june-toast-stack"
      aria-live="polite"
      aria-atomic="true"
    >
      <TransitionGroup
        enter-active-class="transition ease-out duration-300"
        enter-from-class="opacity-0 translate-x-full"
        enter-to-class="opacity-100 translate-x-0"
        leave-active-class="transition ease-in duration-200"
        leave-from-class="opacity-100 translate-x-0"
        leave-to-class="opacity-0 translate-x-full"
      >
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="['june-toast relative overflow-hidden', `june-toast--${toast.type}`]"
          :role="toast.type === 'error' || toast.type === 'warning' ? 'alert' : 'status'"
        >
          <!-- Icon -->
          <div class="june-toast-icon mt-0.5">
            <Icon
              :name="getToastIconName(toast.type)"
              size="md"
              aria-hidden="true"
            />
          </div>

          <!-- Content -->
          <div class="june-toast-content">
            <p v-if="toast.title" class="june-toast-title">
              {{ toast.title }}
            </p>
            <p class="june-toast-description">
              {{ toast.message }}
            </p>
          </div>

          <!-- Close button -->
          <IconButton
            class="june-toast-close"
            icon="hgi-cancel-01"
            label="Close notification"
            size="sm"
            @click="removeToast(toast.id)"
          />

          <!-- Progress bar -->
          <div v-if="toast.duration" class="june-toast-progress-track">
            <div
              :class="['h-full june-toast-progress', getProgressBarColor(toast.type)]"
              :style="{ animationDuration: `${toast.duration}ms` }"
            ></div>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import IconButton from './IconButton.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()

const toasts = computed(() => appStore.toasts)

const getToastIconName = (type: string): 'checkCircle' | 'xCircle' | 'exclamationTriangle' | 'infoCircle' => {
  switch (type) {
    case 'success':
      return 'checkCircle'
    case 'error':
      return 'xCircle'
    case 'warning':
      return 'exclamationTriangle'
    case 'info':
    default:
      return 'infoCircle'
  }
}

const getProgressBarColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'bg-[var(--success)]',
    error: 'bg-[var(--destructive)]',
    warning: 'bg-[var(--warm-strong)]',
    info: 'bg-[var(--brand)]'
  }
  return colors[type] || colors.info
}

const removeToast = (id: string) => {
  appStore.hideToast(id)
}
</script>

<style scoped>
.june-toast-stack {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 9999;
  display: grid;
  width: min(calc(100vw - 32px), 400px);
  gap: 12px;
  pointer-events: none;
}

.june-toast-stack :deep(.june-toast) {
  pointer-events: auto;
}

.june-toast-close {
  flex: 0 0 auto;
  margin: -2px -2px 0 0;
}

.june-toast-progress-track {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 2px;
  background: var(--muted);
}

.june-toast-progress {
  width: 100%;
  animation-name: toast-progress-shrink;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
}

@media (max-width: 520px) {
  .june-toast-stack {
    top: 12px;
    right: 12px;
    width: calc(100vw - 24px);
  }
}

@keyframes toast-progress-shrink {
  from {
    width: 100%;
  }
  to {
    width: 0%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .june-toast-progress {
    animation: none;
  }
}
</style>
