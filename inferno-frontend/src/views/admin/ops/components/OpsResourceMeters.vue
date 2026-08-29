<script setup lang="ts">
/**
 * Resource utilisation as meter rows.
 *
 * WHAT THIS REPLACES, AND WHY IT IS NOT A CHART
 * The page rendered CPU, memory, DB, Redis and goroutines as five flat text
 * boxes -- "CPU 0.5%", "DB ok", "REDIS 1%" -- each of which already knew its
 * own ceiling and its warning/critical thresholds. That is a meter with the
 * bar missing. Adding the track turns five numbers you have to interpret into
 * five you can read at a glance, and it needs no new data at all.
 *
 * COLOUR HERE IS EARNED
 * Ground rule 5 reserves colour for state, and utilisation against a threshold
 * IS state -- the same number is fine at 40% and an incident at 96%. So the
 * fill and the pill take ink, and they are the only things on the panel that
 * do. The icons stay muted: which resource a row describes is a category, and
 * the label already says it.
 *
 * THE HEADLINE IS THE PEAK, NOT THE AVERAGE
 * The reference deck averages its meters. That works when every meter measures
 * the same kind of thing; ours do not -- CPU percent, connection-pool
 * occupancy and a goroutine count against a fixed ceiling do not share units,
 * so a mean across them is a number with no meaning. The tightest resource is
 * what actually breaks first, so the headline names it and shows its level.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsSystemMetricsSnapshot } from '@/api/admin/ops'
import { formatMemorySizeMB } from '../utils/opsFormatters'

const props = defineProps<{
  metrics: OpsSystemMetricsSnapshot | null
  loading?: boolean
}>()

const { t } = useI18n()

type MeterStatus = 'ok' | 'warning' | 'critical'

interface Meter {
  key: string
  icon: string
  label: string
  /** 0-100. */
  pct: number
  /** What the row shows on the right, in its own units. */
  display: string
  status: MeterStatus
}

const WARN = 80
const CRITICAL = 95
/* Goroutines have their own absolute ceilings rather than a percentage, and
   they are the thresholds the old cards already displayed. */
const GOROUTINE_WARN = 8_000
const GOROUTINE_CRITICAL = 15_000

function statusFor(pct: number): MeterStatus {
  if (pct >= CRITICAL) return 'critical'
  if (pct >= WARN) return 'warning'
  return 'ok'
}

function finite(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null
}

const meters = computed<Meter[]>(() => {
  const m = props.metrics
  if (!m) return []
  const rows: Meter[] = []

  const cpu = finite(m.cpu_usage_percent)
  if (cpu != null) {
    rows.push({
      key: 'cpu',
      icon: 'cpu',
      label: t('admin.ops.resources.cpu'),
      pct: Math.min(100, cpu),
      display: `${cpu.toFixed(1)}%`,
      status: statusFor(cpu)
    })
  }

  const memPct = finite(m.memory_usage_percent)
  const memUsed = finite(m.memory_used_mb)
  const memTotal = finite(m.memory_total_mb)
  if (memPct != null || (memUsed != null && memTotal)) {
    const pct = memPct ?? (memUsed! / memTotal!) * 100
    rows.push({
      key: 'memory',
      icon: 'hard-drive',
      label: t('admin.ops.resources.memory'),
      pct: Math.min(100, pct),
      /* The used/total pair, not just the percent: "54%" of an unknown total
         is not actionable. formatMemorySizeMB carries its own unit, so a 64GB
         host reads "32 GB / 64 GB" rather than "32768 / 65536 MB". */
      display:
        memUsed != null && memTotal
          ? `${formatMemorySizeMB(memUsed)} / ${formatMemorySizeMB(memTotal)}`
          : `${pct.toFixed(1)}%`,
      status: statusFor(pct)
    })
  }

  const dbActive = finite(m.db_conn_active)
  const dbMax = finite(m.db_max_open_conns)
  if (dbActive != null && dbMax) {
    const pct = (dbActive / dbMax) * 100
    rows.push({
      key: 'db',
      icon: 'database-01',
      label: t('admin.ops.resources.database'),
      pct: Math.min(100, pct),
      display: `${dbActive} / ${dbMax}`,
      status: statusFor(pct)
    })
  }

  const redisTotal = finite(m.redis_conn_total)
  const redisPool = finite(m.redis_pool_size)
  if (redisTotal != null && redisPool) {
    const pct = (redisTotal / redisPool) * 100
    rows.push({
      key: 'redis',
      icon: 'layers-01',
      label: t('admin.ops.resources.redis'),
      pct: Math.min(100, pct),
      display: `${redisTotal} / ${redisPool}`,
      status: statusFor(pct)
    })
  }

  const goroutines = finite(m.goroutine_count)
  if (goroutines != null) {
    const pct = Math.min(100, (goroutines / GOROUTINE_CRITICAL) * 100)
    rows.push({
      key: 'goroutines',
      icon: 'flash',
      label: t('admin.ops.resources.goroutines'),
      pct,
      display: goroutines.toLocaleString(),
      status:
        goroutines >= GOROUTINE_CRITICAL
          ? 'critical'
          : goroutines >= GOROUTINE_WARN
            ? 'warning'
            : 'ok'
    })
  }

  return rows
})

/** The tightest resource: what breaks first, and therefore what to show. */
const peak = computed(() => {
  if (!meters.value.length) return null
  return meters.value.reduce((hi, m) => (m.pct > hi.pct ? m : hi))
})

const statusLabel = (status: MeterStatus) =>
  status === 'critical'
    ? t('common.critical')
    : status === 'warning'
      ? t('common.warning')
      : t('admin.ops.resources.normal')
</script>

<template>
  <div class="meters">
    <div class="meters__head">
      <h3 class="meters__title">
        <i class="hgi-stroke hgi-cpu meters__title-icon" aria-hidden="true" />
        {{ t('admin.ops.resources.title') }}
      </h3>
    </div>

    <template v-if="peak">
      <p class="meters__lead">{{ t('admin.ops.resources.peakLabel') }}</p>
      <p class="meters__peak">
        {{ Math.round(peak.pct) }}%
        <span class="meters__peak-name">{{ peak.label }}</span>
      </p>

      <ul class="meters__list">
        <li v-for="meter in meters" :key="meter.key" class="meters__row">
          <span class="meters__icon">
            <i class="hgi-stroke" :class="`hgi-${meter.icon}`" aria-hidden="true" />
          </span>
          <span class="meters__label">{{ meter.label }}</span>
          <span class="meters__display">{{ meter.display }}</span>
          <span class="meters__track">
            <span
              class="meters__fill"
              :data-status="meter.status"
              :style="{ width: `${Math.max(2, meter.pct)}%` }"
            />
          </span>
          <span class="meters__pill" :data-status="meter.status">{{ statusLabel(meter.status) }}</span>
        </li>
      </ul>
    </template>

    <p v-else class="meters__empty">{{ t('admin.ops.noData') }}</p>
  </div>
</template>

<style scoped>
.meters {
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.meters__head {
  margin-bottom: 10px;
}

.meters__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.meters__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.meters__lead {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* Serif at display size, matching every other headline number in the product
   -- this is the panel's finding, not a label. */
.meters__peak {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 2px 0 14px;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.1;
}

.meters__peak-name {
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

/*
 * Two columns once there is room for them.
 *
 * This panel used to sit in a 2/3 column with the jobs card beside it. Jobs
 * moved up next to the health ring, which left the meters running the full
 * page width -- and a full-width meter puts its value roughly 1,200px from
 * its own label, with a bar that is longer without saying more. A proportion
 * does not get more legible by being wider. Two columns halve that travel and
 * take the block from five rows to three.
 */
.meters__list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px 28px;
  margin: 0;
  padding: 0;
  list-style: none;
}

@media (min-width: 900px) {
  .meters__list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/*
 * Two rows per meter: identity and number on top, the track spanning the full
 * width beneath. A track squeezed between a label and a value ends up ~90px
 * wide, which is too short to read a proportion from.
 */
.meters__row {
  display: grid;
  grid-template-columns: 22px 1fr auto;
  grid-template-areas:
    'icon label display'
    'track track pill';
  align-items: center;
  gap: 4px 8px;
}

.meters__icon {
  grid-area: icon;
  display: grid;
  place-items: center;
  color: var(--muted-foreground);
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.meters__label {
  grid-area: label;
  color: var(--foreground);
  font-size: var(--fs-md);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meters__display {
  grid-area: display;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.meters__track {
  grid-area: track;
  display: block;
  height: 6px;
  border-radius: var(--r-pill);
  background: var(--muted);
  overflow: hidden;
}

.meters__fill {
  display: block;
  height: 100%;
  border-radius: var(--r-pill);
  /* Width only, never border-color (ground rule 6). */
  transition: width var(--t-slow) var(--ease-out);
}

/*
 * Colour by STATE, which is what ground rule 5 reserves it for: the same
 * number is unremarkable at 40% and an incident at 96%. Brand carries the
 * healthy case so a normal system reads as the product's own colour rather
 * than as a green light nobody looks at.
 */
.meters__fill[data-status='ok'] {
  background: var(--brand);
}
.meters__fill[data-status='warning'] {
  background: var(--s2a-attn);
}
.meters__fill[data-status='critical'] {
  background: var(--destructive);
}

.meters__pill {
  grid-area: pill;
  justify-self: end;
  padding: 1px 7px;
  border-radius: var(--r-pill);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
}

.meters__pill[data-status='ok'] {
  background: var(--muted);
  color: var(--muted-foreground);
}
.meters__pill[data-status='warning'] {
  background: color-mix(in oklch, var(--s2a-attn) 14%, var(--card));
  color: var(--s2a-attn);
}
.meters__pill[data-status='critical'] {
  background: color-mix(in oklch, var(--destructive) 14%, var(--card));
  color: var(--destructive);
}

.meters__empty {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}
</style>
