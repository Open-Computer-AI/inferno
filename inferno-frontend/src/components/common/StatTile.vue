<script setup lang="ts">
/**
 * StatTile — a headline number in a card, inside a tray, with one line of
 * context on the tray floor.
 *
 * WHY THE TRAY EXISTS
 * The strip treatment (StatStrip + StatCard) puts four numbers in one bordered
 * instrument, which is right when the numbers are all you have. It is wrong
 * here, because every number on this dashboard has a second value that matters
 * -- requests have a rate, cost has a list price, accounts have a failing count
 * -- and in the strip those landed as a third line nobody reads.
 *
 * The tray solves that structurally. The inner card holds the headline; the
 * strip of tray below it is a slot that exists specifically for the second
 * value. It reads because it has somewhere to be, not because it is bolder.
 *
 * THE TWO RADII ARE NOT INDEPENDENT
 * Concentric rounded rectangles only look nested when their curves stay
 * parallel, which requires:
 *
 *     outer_radius = inner_radius + gap
 *
 * Break it and the corners pinch: the visible gap narrows at each corner even
 * though the padding is uniform, and the whole thing reads as slightly cheap
 * without an obvious cause. Here that is 10 + 8 = 18, which is exactly
 * --r-lg + INSET = --r-2xl. All three are existing tokens; none is a magic
 * number, and changing one means changing the others.
 *
 * COLOUR ON THE ICON TILES
 * Ground rule 5 says colour encodes state, not category, and four differently
 * coloured tiles is category. This is a deliberate, owner-approved exception
 * for the dashboard fold and nowhere else: the tiles are the only chrome that
 * distinguishes four otherwise identical cards at a glance. Set every tile to
 * `tone="brand"` to fold it back into the rule -- nothing else has to change.
 *
 * The exception is bounded in one specific way: NO tone here may reuse a state
 * colour. An --s2a-attn tile would sit amber on a perfectly healthy dashboard,
 * which is worse than category colour -- it is a false alarm that never
 * clears, and it devalues the amber that the verdict line uses to mean
 * something. Hence `accent` (a hue with no assigned meaning) rather than
 * `attention`.
 *
 * The tone resolves to ONE custom property, --tile-accent, which both the icon
 * tile and the sparkline read. Two things tinted from one declaration cannot
 * drift apart.
 */
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

type Tone = 'brand' | 'info' | 'success' | 'accent'

const props = withDefaults(
  defineProps<{
    /** The metric's name. Sentence case, ground rule 1. */
    label: string
    /** Pre-formatted. The tile never formats -- the view owns units. */
    value: string
    /** Tray-floor line: the second value that gives the headline a baseline. */
    context?: string
    /**
     * Paints the context line and gives it a state glyph.
     *
     * 'muted' is the default and the common case, because most context lines
     * are FACTS, not states -- "1,204 per minute", "8 active", "across 4.24M
     * requests" have no bad version to contrast against. Only pass 'good' or
     * 'attention' where the line could genuinely have said the opposite.
     *
     * Marking every line would make the marks meaningless: a check that is
     * always present stops reading as "this is fine" and starts reading as
     * "this is a line". It would also re-break ground rule 5, since colour
     * would then be sitting on things that encode no state at all.
     */
    contextTone?: 'muted' | 'good' | 'attention'
    /**
     * Hugeicons glyph name, without the `hgi-` prefix.
     *
     * Optional, and omitting it is the hierarchy. A coloured icon tile is the
     * loudest thing on the card, so the fold's health numbers carry one and
     * the reference figures below the fold do not. Same structure, same
     * context slot, lower volume -- which is how two rows of four avoid
     * becoming eight equally loud things, the exact failure of the original
     * eight-card dashboard.
     */
    icon?: string
    tone?: Tone
    /** Exact figure for the title attribute when `value` is abbreviated. */
    exact?: string
    /**
     * Like-for-like change against the same window yesterday, as a whole
     * percent plus its direction.
     *
     * DELIBERATELY TONELESS. The arrow says which way; nothing says whether
     * that is good. Requests up 21% is growth, cost up 21% might be growth or
     * might be a leak -- it depends entirely on whether requests moved with
     * it, which this component cannot know. Painting it green would be a claim
     * the number does not support, and on the cost tile it would be actively
     * misleading.
     */
    delta?: { pct: number; direction: 'up' | 'down' } | null
    /**
     * Today's shape, one entry per hour so far.
     *
     * Says the thing the delta cannot: WHEN it happened. "Up 21%" is the same
     * chip whether traffic rose steadily all morning or arrived in one spike
     * at 11am, and those are different situations.
     *
     * Pass raw values; the component normalises. Omit for anything that is not
     * a time series -- a sparkline on a standing count would be drawing a
     * trend that does not exist.
     */
    series?: number[]
    /**
     * Route this tile drills into. Omit when there is no page that explains
     * the number: a link that lands somewhere unrelated is worse than none,
     * because the reader now has to work out why they are there.
     */
    to?: string
  }>(),
  { tone: 'brand', contextTone: 'muted' }
)

const SPARK_W = 56
const SPARK_H = 18
/* One pixel of headroom top and bottom, or a peak sitting exactly on the
   viewBox edge gets its stroke clipped in half. */
const SPARK_PAD = 1.5

const sparkPoints = computed(() => {
  const s = props.series
  if (!s || s.length < 2) return ''
  const max = Math.max(...s)
  const min = Math.min(...s)
  /* An all-zero day would otherwise draw a flat line across the middle, which
     reads as steady traffic rather than as no traffic. */
  if (max <= 0) return ''
  const range = max - min || 1
  const usable = SPARK_H - SPARK_PAD * 2
  return s
    .map((v, i) => {
      const x = (i / (s.length - 1)) * SPARK_W
      const y = SPARK_H - SPARK_PAD - ((v - min) / range) * usable
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
})
</script>

<template>
  <component :is="to ? RouterLink : 'div'" :to="to" class="tile" :data-tone="tone">
    <div class="tile__card">
      <div class="tile__head">
        <span v-if="icon" class="tile__icon">
          <i class="hgi-stroke" :class="`hgi-${icon}`" aria-hidden="true" />
        </span>
        <span class="tile__label">{{ label }}</span>
      </div>
      <div class="tile__measure">
        <p class="tile__value" :title="exact || value">{{ value }}</p>
        <span v-if="delta" class="tile__delta">
          <i
            class="hgi-stroke tile__delta-icon"
            :class="delta.direction === 'up' ? 'hgi-arrow-up-right-01' : 'hgi-arrow-down-right-01'"
            aria-hidden="true"
          />{{ delta.pct }}%
        </span>
      </div>
    </div>
    <p class="tile__foot" :data-tone="contextTone">
      <i
        v-if="contextTone !== 'muted'"
        class="hgi-stroke tile__foot-icon"
        :class="contextTone === 'attention' ? 'hgi-alert-circle' : 'hgi-checkmark-circle-02'"
        aria-hidden="true"
      />
      <span class="tile__foot-text">{{ context }}</span>
      <!-- Decorative: the number and the delta already state the finding, so
           a screen reader gains nothing from the shape and loses time. -->
      <svg
        v-if="sparkPoints"
        class="tile__spark"
        :viewBox="`0 0 ${SPARK_W} ${SPARK_H}`"
        :width="SPARK_W"
        :height="SPARK_H"
        fill="none"
        aria-hidden="true"
        focusable="false"
      >
        <polyline
          :points="sparkPoints"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </p>
  </component>
</template>

<style scoped>
/*
 * INSET is the single source of truth for the nesting. --r-2xl (18) is
 * --r-lg (10) + 8, so the concentric rule holds by construction; if this
 * changes, the tray radius has to change with it.
 */
.tile {
  --tile-inset: 8px;

  display: block;
  padding: var(--tile-inset);
  padding-bottom: 0; /* the foot supplies its own, so its text is optically centred */
  border-radius: var(--r-2xl);
  /*
   * SELF-INVERTING, which --sidebar is not.
   *
   * The tray has to step AWAY from the page surface, and the page surface here
   * is --card. In light that means darker; in dark it means lighter. --sidebar
   * only satisfies the first: its dark value is L 0.14 against --card's 0.195,
   * so a tray painted with it went darker than the page and read as a hole
   * punched in the card rather than a tray sitting on it.
   *
   * Mixing toward --foreground inverts automatically, because --foreground is
   * near-black in light and near-white in dark. One declaration, both themes,
   * and it stays correct if the surface ramp is ever retuned.
   */
  background: color-mix(in oklch, var(--card) 95%, var(--foreground));
  color: inherit;
  text-decoration: none;
  min-width: 0;
}

/* One accent per tone, read by both the icon tile and the sparkline. */
.tile[data-tone='brand'] {
  --tile-accent: var(--brand);
}
.tile[data-tone='info'] {
  --tile-accent: oklch(55% 0.15 255);
}
.tile[data-tone='success'] {
  --tile-accent: oklch(52% 0.12 155);
}
/* Deliberately not the attention state colour: see the tone note at the top. */
.tile[data-tone='accent'] {
  --tile-accent: oklch(52% 0.13 300);
}

/*
 * Only the linked variant reacts. A hover affordance on a tile that does not
 * navigate promises something it cannot deliver, and the reader learns to
 * distrust the affordance everywhere else.
 *
 * Background only, never border-color (ground rule 6).
 */
a.tile {
  transition: background var(--motion-hover);
}

a.tile:hover {
  background: color-mix(in oklch, var(--card) 90%, var(--foreground));
}

a.tile:focus-visible {
  outline: 2px solid var(--ring);
  outline-offset: 2px;
}

.tile__card {
  padding: 12px 14px 14px;
  border-radius: var(--r-lg);
  background: var(--card);
  min-width: 0;
}

/* min-height matches the icon tile so a tile without one keeps the same
   vertical rhythm: the two rows differ in loudness, not in metrics. */
.tile__head {
  display: flex;
  align-items: center;
  gap: 9px;
  min-height: 28px;
  min-width: 0;
}

/* Fixed 28px chrome. Sized in px, not --fs-*, because a glyph tile is not
   text and must not grow with --font-scale (ground rule 4's carve-out). */
.tile__icon {
  display: grid;
  place-items: center;
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: var(--r-md);
  background: var(--tile-accent);
  color: var(--on-solid);
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.tile__label {
  color: var(--foreground);
  font-size: var(--fs-lg);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* The hairline lives on the measure row, not on the value, so it still spans
   the full card width once a delta chip and a sparkline sit beside the
   number. */
.tile__measure {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
  min-width: 0;
}

.tile__value {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/*
 * The chip never shrinks and never truncates: it is four characters, and a
 * clipped "21" reading as "2" would be worse than showing no delta at all.
 * The number truncates first because it carries a title attribute.
 */
.tile__delta {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 2px;
  padding: 2px 7px;
  border-radius: var(--r-pill);
  background: var(--muted);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  line-height: 1.4;
  white-space: nowrap;
}

.tile__delta-icon {
  font-size: 12px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

/*
 * ON THE TRAY FLOOR, not in the measure row.
 *
 * It started beside the number and pushed the headline into an ellipsis: the
 * measure row is 245px at four-across, and a 30px serif value plus a delta
 * chip plus 56px of shape came to exactly 245 with the value needing 246. One
 * pixel of overflow put the ellipsis on the single most important element on
 * the card.
 *
 * Shrinking things does not fix that; the row was already full. The shape is
 * supporting evidence, and the floor is the slot this component defines for
 * supporting evidence -- the same argument that put the context line there.
 * The floor carries ~140px of text in 245px, so the shape fits with room.
 *
 * Tinted from --tile-accent at partial opacity so it reads as belonging to
 * this tile without competing with the icon that already carries the colour.
 */
.tile__spark {
  flex: none;
  margin-left: auto;
  padding-left: 8px;
  color: var(--tile-accent);
  opacity: 0.55;
}

/* min-height, not a fixed height: tiles without a context line still reserve
   the floor so a row of them keeps one baseline. */
.tile__foot {
  display: flex;
  align-items: center;
  gap: 5px;
  margin: 0;
  padding: 7px 6px;
  min-height: 30px;
  font-size: var(--fs-sm);
  line-height: 1.35;
  min-width: 0;
}

/* The text truncates, never the glyph: a clipped state icon is worse than no
   icon, because a half-drawn mark still reads as a mark. */
.tile__foot-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tile__foot-icon {
  flex: none;
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.tile__foot[data-tone='muted'] {
  color: var(--muted-foreground);
}

/*
 * Asymmetric by design: wrong is loud, fine is quiet.
 *
 * 'attention' takes the full state colour on both glyph and text, so a broken
 * account is the only thing on the fold that catches the eye unread. 'good'
 * tints ONLY the glyph and leaves its text muted -- a healthy dashboard should
 * not be eight lines of green competing with the four icon tiles above it. The
 * check confirms when you look; it does not summon you.
 */
.tile__foot[data-tone='attention'] {
  color: var(--s2a-attn);
}

.tile__foot[data-tone='good'] {
  color: var(--muted-foreground);
}
.tile__foot[data-tone='good'] .tile__foot-icon {
  color: var(--success);
}
</style>
