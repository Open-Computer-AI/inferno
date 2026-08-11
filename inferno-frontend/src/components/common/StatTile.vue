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
 */
type Tone = 'brand' | 'info' | 'success' | 'accent'

withDefaults(
  defineProps<{
    /** The metric's name. Sentence case, ground rule 1. */
    label: string
    /** Pre-formatted. The tile never formats -- the view owns units. */
    value: string
    /** Tray-floor line: the second value that gives the headline a baseline. */
    context?: string
    /** Paints the context line. Only 'attention' takes ink (ground rule 5). */
    contextTone?: 'muted' | 'attention'
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
  }>(),
  { tone: 'brand', contextTone: 'muted' }
)
</script>

<template>
  <div class="tile">
    <div class="tile__card">
      <div class="tile__head">
        <span v-if="icon" class="tile__icon" :data-tone="tone">
          <i class="hgi-stroke" :class="`hgi-${icon}`" aria-hidden="true" />
        </span>
        <span class="tile__label">{{ label }}</span>
      </div>
      <p class="tile__value" :title="exact || value">{{ value }}</p>
    </div>
    <p class="tile__foot" :data-tone="contextTone">{{ context }}</p>
  </div>
</template>

<style scoped>
/*
 * INSET is the single source of truth for the nesting. --r-2xl (18) is
 * --r-lg (10) + 8, so the concentric rule holds by construction; if this
 * changes, the tray radius has to change with it.
 */
.tile {
  --tile-inset: 8px;

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
  min-width: 0;
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
  color: var(--on-solid);
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.tile__icon[data-tone='brand'] {
  background: var(--brand);
}
.tile__icon[data-tone='info'] {
  background: oklch(55% 0.15 255);
}
.tile__icon[data-tone='success'] {
  background: oklch(52% 0.12 155);
}
/* Deliberately not the attention state colour: see the tone note at the top. */
.tile__icon[data-tone='accent'] {
  background: oklch(52% 0.13 300);
}

.tile__label {
  color: var(--foreground);
  font-size: var(--fs-lg);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* The hairline is a border on the value, not a separate element: it inherits
   the card's padding, so it insets exactly like the reference's divider. */
.tile__value {
  margin: 12px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* min-height, not a fixed height: tiles without a context line still reserve
   the floor so a row of them keeps one baseline. */
.tile__foot {
  display: flex;
  align-items: center;
  margin: 0;
  padding: 7px 6px;
  min-height: 30px;
  font-size: var(--fs-sm);
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tile__foot[data-tone='muted'] {
  color: var(--muted-foreground);
}
.tile__foot[data-tone='attention'] {
  color: var(--s2a-attn);
}
</style>
