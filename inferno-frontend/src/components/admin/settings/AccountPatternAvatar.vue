<template>
  <span
    class="account-avatar account-pattern-avatar"
    :style="avatarStyle"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  seed: string
  size?: number
}>()

function hashSeed(value: string): number {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return hash >>> 0
}

const avatarStyle = computed<Record<string, string>>(() => {
  const hash = hashSeed(props.seed || 'inferno-account')
  const pick = (offset: number, max: number) => ((hash >>> offset) % max)

  return {
    '--avatar-cloud-x': `${18 + pick(0, 28)}%`,
    '--avatar-cloud-y': `${16 + pick(5, 32)}%`,
    '--avatar-cloud-angle': `${105 + pick(10, 150)}deg`,
    '--avatar-cloud-strength': `${44 + pick(15, 24)}%`,
    '--avatar-cloud-highlight-angle': `${-18 + pick(20, 36)}deg`,
    width: `${props.size ?? 32}px`,
    height: `${props.size ?? 32}px`,
  }
})
</script>

<style scoped>
.account-pattern-avatar {
  display: block;
}
</style>
