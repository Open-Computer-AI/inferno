<template>
  <div
    :class="[
      'skel bg-[var(--brand-tint)]',
      variant === 'circle' ? 'rounded-full' : 'rounded-lg',
      customClass
    ]"
    :data-shimmer="shimmer || undefined"
    :style="style"
  ></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'rect' | 'circle' | 'text'
  shimmer?: boolean
  width?: string | number
  height?: string | number
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'rect',
  shimmer: false,
  width: '100%'
})

const customClass = computed(() => props.class || '')

const style = computed(() => {
  const s: Record<string, string> = {}
  
  if (props.width) {
    s.width = typeof props.width === 'number' ? `${props.width}px` : props.width
  }
  
  if (props.height) {
    s.height = typeof props.height === 'number' ? `${props.height}px` : props.height
  } else if (props.variant === 'text') {
    s.height = '1em'
    s.marginTop = '0.25em'
    s.marginBottom = '0.25em'
  }
  
  return s
})
</script>

<style scoped>
.skel {
  display: block;
}

.skel[data-shimmer] {
  background: var(--surface-subtle);
  opacity: 0.72;
}

@media (prefers-reduced-motion: no-preference) {
  .skel[data-shimmer] {
    animation: skel-pulse 1.8s ease-in-out infinite;
  }

  @keyframes skel-pulse {
    0%, 100% { opacity: 0.58; }
    50% { opacity: 0.92; }
  }
}
</style>
