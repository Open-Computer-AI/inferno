<template>
  <div class="tpl" :data-mobile="isMobile || undefined">
    <!-- Title is a display moment (part 07 v2 #header): AppHeader.vue is deleted and
         nothing else in the shell renders a page heading, so this layout owns it now
         ("the page owns its title... recovering 44px everywhere"). No existing call
         site passes `title` yet, so this row stays absent until a view is updated to
         pass one -- exactly today's rendering, unchanged.
         Sits flush against AppLayout's own shell__card padding, no padding of its own:
         that card already supplies the page's outer inset (see AppLayout.vue), so
         adding another top/side offset here would double it. -->
    <div v-if="title || $slots.actions" class="tpl__header">
      <div v-if="title" class="tpl__heading">
        <h1 class="tpl__title">{{ title }}</h1>
        <p v-if="description" class="tpl__desc">{{ description }}</p>
      </div>
      <!-- margin-left: auto rather than the header's own justify-content, so actions
           still land at the right edge when there is no title (KeysView today: an
           actions slot with no heading, previously right-aligned by its own wrapper
           div -- this keeps that same result now that the wrapper is mine). -->
      <div v-if="$slots.actions" class="tpl__actions">
        <slot name="actions" />
      </div>
    </div>

    <!-- Toolbar: search + filters. Its bottom edge is the data card's own top border
         (spec.pagelayout.rhythm: "1px, filter bar to table, a real border") -- one
         hairline, not a border here plus another on the card. -->
    <div v-if="$slots.filters" class="tpl__toolbar">
      <slot name="filters" />
    </div>

    <!-- The one card this layout draws: the data unit, table plus pager. 1px
         --border-subtle, --r-lg, --card fill, no shadow -- ground rule 9, and the
         shell's inset card is the documented shadow exception, not this one. The
         scroll containment and sticky-column support live in DataTable.vue itself
         (unchanged, per the migration note); this card only has to give it a bounded
         flex-1/min-height:0 box to fill. -->
    <div class="tpl__card">
      <div class="tpl__body">
        <slot name="table" />
      </div>
      <!-- rhythm: "0px, table to pager, they share the bottom border" -- no gap,
           just the divider; Pagination.vue supplies its own internal padding. -->
      <div v-if="$slots.pagination" class="tpl__pagination">
        <slot name="pagination" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * TablePageLayout -- Library 04 Tables, section 06 (spec.pagelayout).
 *
 * Fifteen admin and user views compose themselves through this shell. Its job is
 * unchanged from before this rewrite: the table body scrolls, the filter bar and the
 * pager do not. What the migration note adds is the fixed vertical rhythm ("nineteen
 * views stop spacing themselves differently") and the page title, which has nowhere
 * else to live now that AppHeader.vue is deleted (part 07 v2 #header).
 *
 * `title` / `description` are the only additions to the public surface, and both are
 * optional with no default: no existing call site passes either today, since there was
 * nowhere for a title to go before this rewrite. `actions`, `filters`, `table` and
 * `pagination` are the same four named slots, with the same no-payload signature, all
 * fifteen call sites already use -- verified against every current consumer, not just
 * assumed.
 */
import { ref, onMounted, onUnmounted } from 'vue'

interface Props {
  /** The page's own heading, since no header bar renders one anymore. --font-serif
   *  --fs-2xl, the size 01-TOKENS' role column reserves for "page heading"
   *  (Foundations, part 01: `{ token:'--fs-2xl', ..., role:'Page heading, KPI value' }`).
   *  Left unset, the header row does not render at all unless the actions slot is in
   *  use -- a caller not yet updated to pass a title keeps its current, title-less look. */
  title?: string
  /** One line of support copy under the title, e.g. "348 pooled credentials, 12 in
   *  cooldown" (Library 04 Tables, spec.pagelayout demo). --font-sans --fs-sm
   *  --muted-foreground -- never the title's serif, and never rendered without a title. */
  description?: string
}

defineProps<Props>()

const isMobile = ref(false)

const checkMobile = () => {
  isMobile.value = window.innerWidth < 1024
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
/* Reading column, not full bleed: spec.pagelayout.tokens prints "--content-max
 * column" first in its token list. 760px, stepping to 880 at 1440 and 1000 at 1920
 * (controls.css) -- the same rule note-editor uses elsewhere in the app. A wide table
 * still scrolls horizontally inside this column through DataTable's own
 * .table-wrapper; only the page's outer width is capped, never hardcoded here. */
.tpl {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: var(--content-max);
  min-height: 0;
  margin: 0 auto;
  /* Bounded height so the data card scrolls internally while the title, the toolbar
   * and the pager stay in place. AppLayout's shell__card is a plain block (padding +
   * overflow:auto, not a flex/grid chain), so this element cannot inherit a height
   * through it; the sum below is shell__card's own vertical chrome (7px margin top
   * and bottom, plus its responsive padding -- see AppLayout.vue), now without the
   * 64px AppHeader that part 07 v2 deletes. Coupled to those numbers by necessity,
   * not by choice: there is no shared token for either side to read instead. */
  height: calc(100vh - 46px);
}
@media (min-width: 768px) {
  .tpl {
    height: calc(100vh - 62px);
  }
}
@media (min-width: 1024px) {
  .tpl {
    height: calc(100vh - 78px);
  }
}

/* Mobile: DataTable itself swaps to a stacked card list below 768px, and that list
 * needs to grow with its content and let the page scroll normally, not be clipped at
 * a fixed height. isMobile trips at 1024px, wider than DataTable's own breakpoint, so
 * the layout has already dropped its height cap and card chrome before DataTable
 * changes shape -- the same threshold and the same reasoning the prior implementation
 * used, carried forward rather than rewritten. */
.tpl[data-mobile] {
  height: auto;
}

.tpl__header {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 12px;
  /* rhythm: "12px, summary block to filter bar" */
  margin-bottom: 12px;
}

.tpl__heading {
  min-width: 0;
}

.tpl__title {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.15;
}

.tpl__desc {
  /* rhythm: "2px, title to its one line summary" */
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

.tpl__actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

.tpl__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  /* rhythm: "1px, filter bar to table, a real border" -- padding, not margin, so this
   * box ends exactly at the card's top border with nothing extra between them. */
  padding-bottom: 12px;
}

.tpl__card {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  overflow: hidden;
}

.tpl[data-mobile] .tpl__card {
  flex: none;
  min-height: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  overflow: visible;
}

.tpl__body {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.tpl[data-mobile] .tpl__body {
  flex: none;
  overflow: visible;
}

.tpl__pagination {
  flex-shrink: 0;
  border-top: 1px solid var(--border);
}
</style>
