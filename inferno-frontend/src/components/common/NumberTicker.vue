<script setup lang="ts">
/**
 * A number that springs to its target instead of snapping.
 *
 * "The hero number is not an odometer, it is one continuous spring that
 * retargets from wherever it sits and writes straight to the DOM text." Both
 * halves of that matter:
 *
 *   NOT AN ODOMETER  Per-digit rolling breaks as soon as the digit COUNT
 *                    changes -- 980 to 1,240 has to grow a column mid-flight,
 *                    and every odometer does that with a visible jolt. One
 *                    spring over the value, formatted per frame, has no
 *                    columns to grow.
 *
 *   STRAIGHT TO DOM  Writing textContent skips Vue's reactivity for a value
 *                    that changes 60 times a second and is never read by
 *                    anything else. A ref here would schedule a component
 *                    update per frame, per ticker, and this dashboard renders
 *                    six of them at once.
 *
 * Tabular figures are not cosmetic: proportional digits change width as the
 * value climbs, so the number visibly shimmies while it counts.
 */
import { onMounted, ref, watch } from 'vue'
import { useSpringValue } from '@/composables/useSpringValue'

const props = withDefaults(
  defineProps<{
    value: number
    /** Rendered ahead of the number, inside the same element, e.g. "$" or "+". */
    prefix?: string
    /** Decimal places. Counts are whole; money is 2. */
    decimals?: number
  }>(),
  { prefix: '', decimals: 0 }
)

const el = ref<HTMLElement | null>(null)
const spring = useSpringValue(props.value)

function format(n: number): string {
  return (
    props.prefix +
    n.toLocaleString(undefined, {
      minimumFractionDigits: props.decimals,
      maximumFractionDigits: props.decimals
    })
  )
}

watch(
  () => spring.value.value,
  (n) => {
    if (el.value) el.value.textContent = format(n)
  }
)

watch(
  () => props.value,
  (next) => spring.set(next)
)

onMounted(() => {
  /* Painted from the spring's own current value, not from the prop: on a
     remount mid-flight those differ, and seeding from the prop would make the
     first frame jump backwards before springing forward again. */
  if (el.value) el.value.textContent = format(spring.value.value)
})
</script>

<template>
  <span ref="el" class="ticker">{{ format(value) }}</span>
</template>

<style scoped>
.ticker {
  font-variant-numeric: tabular-nums;
  font-feature-settings: 'tnum' 1;
}
</style>
