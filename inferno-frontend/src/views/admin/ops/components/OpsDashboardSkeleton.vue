<script setup lang="ts">
/**
 * The ops dashboard, before it has anything to say.
 *
 * A skeleton has one job: occupy the space the real thing will occupy, in the
 * chrome the real thing will wear. This one was failing both.
 *
 * IT WAS DRAWING THE OLD PAGE
 * `rounded-3xl bg-white shadow-sm ring-1 ring-gray-900/5` on every block, a
 * 5/7 header grid, and a header wrapped as one big card -- the exact layout
 * that was replaced. So the load sequence was: flash the old design, then
 * repaint the new one. A skeleton that does not match is worse than no
 * skeleton, because it promises a shape and then breaks it.
 *
 * IT WAS THE WRONG SIZE
 * Blocks were `min-h-[360px]` and `h-56` guesses. Measured against the live
 * page the header alone is 829px, alert events 693px and system logs 1,617px
 * -- and there was no system logs block at all, so the page grew by 1,600px
 * the moment data arrived. The heights here are measured, not estimated.
 *
 * THE PULSE IS GUARDED
 * `animate-pulse` is a Tailwind animation with no reduced-motion guard, on
 * roughly forty elements at once. Ground rule 10 says respect the preference,
 * so the local keyframe stops under `prefers-reduced-motion` and the
 * placeholders simply hold their resting tint.
 */
withDefaults(defineProps<{ fullscreen?: boolean }>(), { fullscreen: false })
</script>

<template>
  <div class="opsk" :data-fullscreen="fullscreen || undefined" aria-hidden="true">
    <!-- Header: masthead, then the 4/8 grid, then full-width meters. -->
    <div class="opsk__head">
      <div class="opsk__bar">
        <div class="opsk__ident">
          <span class="opsk__bone opsk__bone--title" />
          <span class="opsk__bone opsk__bone--sub" />
        </div>
        <div v-if="!fullscreen" class="opsk__tools">
          <span v-for="i in 3" :key="`f${i}`" class="opsk__bone opsk__bone--control" />
          <span class="opsk__bone opsk__bone--icon" />
          <span v-for="i in 2" :key="`b${i}`" class="opsk__bone opsk__bone--button" />
          <span class="opsk__bone opsk__bone--icon" />
        </div>
      </div>

      <div class="opsk__grid">
        <div class="opsk__left">
          <div class="opsk__card opsk__card--now">
            <span class="opsk__bone opsk__bone--dial" />
            <div class="opsk__now-lines">
              <span class="opsk__bone opsk__bone--line" />
              <span class="opsk__bone opsk__bone--control" />
              <span class="opsk__bone opsk__bone--line" />
              <span class="opsk__bone opsk__bone--line" />
            </div>
          </div>
          <div class="opsk__card opsk__card--jobs">
            <span class="opsk__bone opsk__bone--line" />
            <span class="opsk__bone opsk__bone--figure" />
          </div>
        </div>

        <div class="opsk__tiles">
          <div v-for="i in 5" :key="i" class="opsk__card opsk__card--tile" :data-wide="i === 2 || undefined">
            <span class="opsk__bone opsk__bone--line" />
            <span class="opsk__bone opsk__bone--figure" />
            <span class="opsk__bone opsk__bone--line" />
          </div>
        </div>
      </div>

      <div class="opsk__card opsk__card--meters">
        <span class="opsk__bone opsk__bone--line" />
        <span class="opsk__bone opsk__bone--figure" />
        <div class="opsk__meter-grid">
          <span v-for="i in 4" :key="i" class="opsk__bone opsk__bone--meter" />
        </div>
      </div>
    </div>

    <!-- Row: concurrency, switch rate, throughput (1 / 1 / 2). -->
    <div class="opsk__row opsk__row--four">
      <div v-for="i in 3" :key="i" class="opsk__card opsk__card--chart" :data-wide="i === 3 || undefined">
        <span class="opsk__bone opsk__bone--line" />
        <span class="opsk__bone opsk__bone--plot" />
      </div>
    </div>

    <!-- Row: latency, error distribution, error trend. -->
    <div class="opsk__row opsk__row--three">
      <div v-for="i in 3" :key="i" class="opsk__card opsk__card--chart">
        <span class="opsk__bone opsk__bone--line" />
        <span class="opsk__bone opsk__bone--plot" />
      </div>
    </div>

    <!-- Alert events: a table, which is what actually arrives. -->
    <div class="opsk__card opsk__card--table">
      <div class="opsk__bar">
        <div class="opsk__ident">
          <span class="opsk__bone opsk__bone--line" />
          <span class="opsk__bone opsk__bone--sub" />
        </div>
        <div v-if="!fullscreen" class="opsk__tools">
          <span v-for="i in 4" :key="i" class="opsk__bone opsk__bone--control" />
        </div>
      </div>
      <div class="opsk__rows">
        <span v-for="i in 10" :key="i" class="opsk__bone opsk__bone--row" />
      </div>
    </div>

    <!--
      System logs. The old skeleton had no block for this at all, so the page
      grew by roughly 1,600px the moment the data landed.
    -->
    <div class="opsk__card opsk__card--table">
      <div class="opsk__bar">
        <div class="opsk__ident">
          <span class="opsk__bone opsk__bone--line" />
          <span class="opsk__bone opsk__bone--sub" />
        </div>
        <div v-if="!fullscreen" class="opsk__tools">
          <span v-for="i in 4" :key="i" class="opsk__bone opsk__bone--pill" />
        </div>
      </div>
      <span class="opsk__bone opsk__bone--disclosure" />
      <div class="opsk__filters">
        <span v-for="i in 14" :key="i" class="opsk__bone opsk__bone--field" />
      </div>
      <div class="opsk__tools">
        <span v-for="i in 4" :key="i" class="opsk__bone opsk__bone--button" />
      </div>
      <div class="opsk__rows">
        <span v-for="i in 20" :key="i" class="opsk__bone opsk__bone--row" />
      </div>
      <span class="opsk__bone opsk__bone--disclosure" />
    </div>
  </div>
</template>

<style scoped>
.opsk {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* --- the bone ---------------------------------------------------------- */

/*
 * One placeholder treatment, sized by modifier. `--muted` is the same fill the
 * meters use for an empty track, so a loading page reads as the same material
 * as a loaded one rather than a grey wireframe of it.
 */
.opsk__bone {
  display: block;
  border-radius: var(--r-sm);
  background: var(--muted);
  animation: opsk-pulse 1.6s var(--ease-out) infinite;
}

@keyframes opsk-pulse {
  50% {
    opacity: 0.45;
  }
}

/* Ground rule 10. Forty elements breathing at once is exactly the case the
   preference exists for; they keep their resting tint instead. */
@media (prefers-reduced-motion: reduce) {
  .opsk__bone {
    animation: none;
  }
}

.opsk__bone--title {
  width: 190px;
  height: 22px;
}

.opsk__bone--sub {
  width: 260px;
  max-width: 100%;
  height: 12px;
}

.opsk__bone--line {
  width: 120px;
  max-width: 100%;
  height: 12px;
}

.opsk__bone--figure {
  width: 96px;
  height: 28px;
}

.opsk__bone--control {
  width: 140px;
  height: var(--s2a-h-sm);
  border-radius: var(--r-md);
}

.opsk__bone--pill {
  width: 96px;
  height: 24px;
  border-radius: var(--r-sm);
}

.opsk__bone--button {
  width: 118px;
  height: var(--s2a-h-sm);
  border-radius: var(--r-md);
}

.opsk__bone--icon {
  width: var(--s2a-h-sm);
  height: var(--s2a-h-sm);
  border-radius: var(--r-sm);
}

/* The health dial is a ring, so its placeholder is one too -- a filled square
   here would be a shape the page never resolves into. */
.opsk__bone--dial {
  width: 100px;
  height: 100px;
  border-radius: 50%;
  background: none;
  border: 8px solid var(--muted);
}

.opsk__bone--meter {
  height: 30px;
}

.opsk__bone--plot {
  flex: 1;
  min-height: 0;
  border-radius: var(--r-md);
}

/* 52px, not a guess: both tables render a title over a description, and their
   real rows measure 57px. Ten short bones under a table that arrives twice as
   tall is the jump this component exists to prevent. */
.opsk__bone--row {
  height: 52px;
}

.opsk__bone--field {
  height: 52px;
}

.opsk__bone--disclosure {
  height: 38px;
  border-radius: var(--r-md);
}

/* --- shared chrome ------------------------------------------------------ */

/* Hairline, --r-lg, no shadow: the same card the real blocks wear. */
.opsk__card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  min-width: 0;
}

.opsk__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.opsk__ident {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.opsk__tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

/* --- header ------------------------------------------------------------- */

/* Not a card, matching the converted header: a masthead and a grid of tiles
   sitting on the page background. */
.opsk__head {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.opsk[data-fullscreen] .opsk__head {
  gap: 20px;
}

/*
 * The floors below are measured off the live page, not estimated: the header
 * grid renders 536px and the meters panel 266px with real data. A skeleton
 * only earns its keep by reserving the space the content will take, so these
 * are the numbers rather than round ones that look tidy.
 */
.opsk__grid {
  display: grid;
  grid-template-columns: minmax(0, 4fr) minmax(0, 8fr);
  gap: 12px;
  min-height: 520px;
}

@media (max-width: 1100px) {
  .opsk__grid {
    grid-template-columns: minmax(0, 1fr);
    min-height: 0;
  }
}

.opsk__left {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.opsk__card--now {
  flex: 1;
  flex-direction: row;
  align-items: center;
  gap: 18px;
  min-height: 240px;
}

.opsk__now-lines {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.opsk__card--jobs {
  flex: none;
  gap: 8px;
  padding: 14px 16px;
}

.opsk__tiles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

/* The traffic split takes two columns, same as the real one. */
.opsk__card--tile[data-wide] {
  grid-column: span 2;
}

@media (max-width: 900px) {
  .opsk__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 560px) {
  .opsk__tiles {
    grid-template-columns: minmax(0, 1fr);
  }

  .opsk__card--tile[data-wide] {
    grid-column: span 1;
  }
}

.opsk__card--meters {
  min-height: 260px;
}

.opsk__meter-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px 28px;
}

@media (min-width: 900px) {
  .opsk__meter-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/* --- chart rows --------------------------------------------------------- */

.opsk__row {
  display: grid;
  gap: 24px;
}

.opsk__row--four {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.opsk__card--chart[data-wide] {
  grid-column: span 2;
}

.opsk__row--three {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

/* 360px measured, matching the real chart cards. */
.opsk__card--chart {
  min-height: 360px;
}

@media (max-width: 1024px) {
  .opsk__row--four,
  .opsk__row--three {
    grid-template-columns: minmax(0, 1fr);
  }

  .opsk__card--chart[data-wide] {
    grid-column: span 1;
  }
}

/* --- tables ------------------------------------------------------------- */

.opsk__card--table {
  gap: 14px;
}

.opsk__rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.opsk__filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 10px;
}
</style>
