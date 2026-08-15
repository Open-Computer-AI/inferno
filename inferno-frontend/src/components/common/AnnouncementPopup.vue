<template>
  <BaseDialog
    :show="Boolean(displayedAnnouncement)"
    :title="displayedAnnouncement?.title || t('announcements.title')"
    width="wide"
    @close="handleDismiss"
  >
    <div v-if="displayedAnnouncement" class="announcement-popup">
      <div class="announcement-popup__meta">
        <span>
          <Icon name="clock" size="sm" aria-hidden="true" />
          {{ formatRelativeWithDateTime(displayedAnnouncement.created_at) }}
        </span>
        <span class="announcement-popup__status">
          <Icon name="infoCircle" size="sm" aria-hidden="true" />
          {{ t('announcements.unread') }}
        </span>
      </div>
      <div
        class="markdown-body announcement-popup__content"
        v-html="renderedContent"
      />
    </div>

    <template #footer>
      <Button variant="solid" data-testid="announcement-popup-dismiss" @click="handleDismiss">
        {{ preview ? t('common.close') : t('announcements.markRead') }}
      </Button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeWithDateTime } from '@/utils/format'
import type { Announcement, UserAnnouncement } from '@/types'
import BaseDialog from './BaseDialog.vue'
import Button from './Button.vue'
import Icon from '@/components/icons/Icon.vue'
import '@/styles/announcement-markdown.css'

type PreviewAnnouncement = Pick<Announcement | UserAnnouncement, 'title' | 'content' | 'created_at'>

const props = withDefaults(defineProps<{
  announcement?: PreviewAnnouncement | null
  preview?: boolean
}>(), {
  announcement: null,
  preview: false,
})

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const announcementStore = useAnnouncementStore()
const displayedAnnouncement = computed(() => (
  props.preview ? props.announcement : announcementStore.currentPopup
))

marked.setOptions({ breaks: true, gfm: true })

const renderedContent = computed(() => {
  const content = displayedAnnouncement.value?.content
  if (!content) return ''
  return DOMPurify.sanitize(marked.parse(content) as string)
})

function handleDismiss() {
  if (props.preview) emit('close')
  else announcementStore.dismissPopup()
}
</script>

<style scoped>
.announcement-popup {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}

.announcement-popup__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--sp-3);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.announcement-popup__meta span {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
}

.announcement-popup__status {
  color: var(--brand);
  font-weight: var(--fw-medium);
}

.announcement-popup__content {
  min-width: 0;
  padding-top: var(--sp-4);
  border-top: 1px solid var(--border-subtle);
}
</style>
