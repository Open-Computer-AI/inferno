<script setup lang="ts">
/**
 * 导航进度条组件
 * 在页面顶部显示加载进度，提供导航反馈
 */
import { computed } from 'vue'
import { useNavigationLoadingState } from '@/composables/useNavigationLoading'

const { isLoading } = useNavigationLoadingState()

// 进度条可见性
const isVisible = computed(() => isLoading.value)
</script>

<template>
  <Transition name="progress-fade">
    <div
      v-show="isVisible"
      class="navigation-progress"
      role="progressbar"
      aria-label="Loading"
      aria-valuenow="0"
      aria-valuemin="0"
      aria-valuemax="100"
    >
      <div class="navigation-progress-bar" />
    </div>
  </Transition>
</template>

<style scoped>
.navigation-progress {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  z-index: 9999;
  overflow: hidden;
  background: transparent;
}

/* This is global chrome -- it renders above every route, so it was the one
 * remaining teal surface a customer saw on every single navigation, converted
 * views included.
 *
 * Was a four-stop teal gradient sweeping a full-width bar, which ground rule 7
 * bans. A solid short bar crossing the track reads as the same indeterminate
 * progress without one. */
.navigation-progress-bar {
  height: 100%;
  width: 32%;
  background: var(--brand);
  animation: progress-slide 1.5s ease-in-out infinite;
}

/* The dark-mode override this replaced hard-coded a second pair of teal stops.
 * `--brand` is already redefined per theme, so one declaration now covers both
 * and the two can no longer drift apart. */

/* Travels from fully off the left edge to fully off the right. The end value is
 * expressed against the bar's OWN width (the percentage basis for translateX),
 * so at 32% wide it needs 100/32 ~= 313% to clear a full track, not 100%. */
@keyframes progress-slide {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(313%);
  }
}

/* 淡入淡出过渡 */
.progress-fade-enter-active {
  transition: opacity 0.15s ease-out;
}

.progress-fade-leave-active {
  transition: opacity 0.3s ease-out;
}

.progress-fade-enter-from,
.progress-fade-leave-to {
  opacity: 0;
}

/* 减少动画模式 */
@media (prefers-reduced-motion: reduce) {
  .navigation-progress-bar {
    animation: progress-pulse 2s ease-in-out infinite;
  }

  @keyframes progress-pulse {
    0%,
    100% {
      opacity: 0.4;
    }
    50% {
      opacity: 1;
    }
  }
}
</style>
