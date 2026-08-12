<template>
  <div class="tpl" :data-width="width" :data-mobile="isMobile || undefined">
    <!-- Title is a display moment (part 07 v2 #header): AppHeader.vue is deleted and
         nothing else in the shell renders a page heading, so this layout owns it now
         ("the page owns its title... recovering 44px everywhere"). No existing call
         site passes `title` yet, so this row stays absent until a view is updated to
         pass one -- exactly today's rendering, unchanged.
         Sits flush against AppLayout's own shell__card padding, no padding of its own:
         that card already supplies the page's outer inset (see AppLayout.vue), so
         adding another top/side offset here would double it. -->
    <div v-if="resolvedTitle || $slots.actions" class="tpl__header">
      <div v-if="resolvedTitle" class="tpl__heading">
        <h1 class="tpl__title">{{ resolvedTitle }}</h1>
        <p v-if="resolvedDescription" class="tpl__desc">{{ resolvedDescription }}</p>
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
 * `title` / `description` / `width` are the only additions to the public surface.
 * `title` and `description` are optional with no default: no existing call site
 * passes either today, since there was nowhere for a title to go before this rewrite.
 * `width` defaults to `'reading'`, so it is also a no-op for every existing call site
 * until an orchestrator-approved view opts a wide table into `'wide'` (see the prop's
 * own doc for the ruling). `actions`, `filters`, `table` and `pagination` are the same
 * four named slots, with the same no-payload signature, all fifteen call sites already
 * use -- verified against every current consumer, not just assumed.
 */
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

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
  /** Opt out of the reading column for pages whose table is wide, not prose. Ruling:
   *  spec.pagelayout.tokens lists "--content-max column" first, but 01-TOKENS
   *  (controls.css) defines --content-max under "Reading columns" -- prose and forms,
   *  where a narrow measure aids reading; it is the same token note-editor uses
   *  elsewhere in the app. A dense admin table with 10+ columns is not a reading
   *  column, and the prototype's own table demo backs that: it ships
   *  `max-width:1080px` on the table page, not the --content-max token, contradicting
   *  spec.pagelayout.tokens' own line. 'reading' keeps today's centred, capped column
   *  -- the default, so all fifteen existing call sites are unchanged since none pass
   *  this prop yet. 'wide' drops the cap so the table fills the shell's inset card. */
  width?: 'reading' | 'wide'
}

const props = withDefaults(defineProps<Props>(), {
  /**
   * Wide by default.
   *
   * --content-max is 760px (880 above 1440), which 01-TOKENS defines under
   * "Reading columns" -- prose and forms, where a narrow measure aids reading.
   * Part 14 says the same thing from the other side: a dense admin table is not
   * a reading column. Every consumer of THIS layout is a table, so capping them
   * all at a prose measure squeezed a 7-column grid into 880px and left a third
   * of the screen empty beside it.
   *
   * 'reading' is still available for the rare table that is genuinely prose.
   */
  width: 'wide'
})

/**
 * Title and summary fall back to the route's own meta.
 *
 * Every route already carries `title` + `titleKey`, and 30 of them carry a
 * `descriptionKey` -- the one-line summary part 14 asks for already exists as
 * data. Reading it here gives all 21 table pages a heading in one edit instead
 * of 21 near-identical prop lines, and it cannot drift from the tab title,
 * which is resolved from exactly the same meta (router/title.ts).
 *
 * A view that wants something different still passes the prop; this is only the
 * floor. `te()` is checked before `t()` so a missing key renders nothing rather
 * than its own path.
 */
const route = useRoute()
const { t, te } = useI18n()

const resolvedTitle = computed(() => {
  if (props.title) return props.title
  const key = route.meta?.titleKey
  if (typeof key === 'string' && te(key)) return t(key)
  const fallback = route.meta?.title
  return typeof fallback === 'string' ? fallback : ''
})

const resolvedDescription = computed(() => {
  if (props.description) return props.description
  const key = route.meta?.descriptionKey
  return typeof key === 'string' && te(key) ? t(key) : ''
})

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
   * and the pager stay in place.
   *
   * This used to be `calc(100vh - 46px / 62px / 78px)` at three breakpoints, with a
   * comment explaining that shell__card was "a plain block (padding + overflow:auto,
   * not a flex/grid chain), so this element cannot inherit a height through it" and
   * that the sums duplicated shell__card's own margin and padding out of necessity.
   *
   * That stopped being true when the shell became the scroll container: .shell is
   * pinned to 100dvh and the card is `flex: 1; min-height: 0`, which is a definite
   * height, so a percentage here resolves against it. The three magic numbers are
   * gone and the coupling with them -- change the card's padding now and this
   * follows, instead of drifting by exactly that many pixels.
   *
   * Measured identical across five table routes before and after: 1022px, against a
   * card content box of 1086 - 64 padding. */
  height: 100%;
}

/* Wide opt-out (see the `width` prop doc for the ruling): a data table is not prose,
 * so it does not owe the reading column anything. Dropping the cap here, rather than
 * skipping it above, keeps the default rule the single source of truth for the
 * reading case and this the single override for the wide one. */
.tpl[data-width='wide'] {
  max-width: none;
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

/*
 * The toolbar deliberately does NOT size its children.
 *
 * Views already lay their own filter bars out (GroupsView: an outer
 * `lg:flex-row` with a `flex-wrap` group of search + selects inside). An
 * earlier attempt here capped each direct child at 240px, which hit that outer
 * wrapper and forced everything inside it onto separate lines -- it made the
 * exact problem it was trying to fix worse.
 *
 * The filters were only stacking because --content-max squeezed the page to
 * 880px, leaving the left group too little room to keep search and three
 * selects on one line. Widening the column fixed it; nothing here needed to.
 */

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
