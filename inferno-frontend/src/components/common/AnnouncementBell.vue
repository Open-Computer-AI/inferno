<template>
  <div>
    <Button
      variant="ghost"
      size="md"
      class="relative inline-flex h-9 w-9 items-center justify-center rounded-md text-[var(--muted-foreground)] transition-[background-color,color] hover:bg-[var(--sidebar-accent)] hover:text-[var(--foreground)]"
      :class="unreadCount > 0 ? 'text-[var(--brand)]' : ''"
      :aria-label="t('announcements.title')"
      @click="openModal"
    >
      <Icon name="bell" size="md" />
      <span
        v-if="unreadCount > 0"
        class="absolute right-1 top-1 h-2 w-2 rounded-full bg-[var(--destructive)]"
        aria-hidden="true"
      />
    </Button>

    <BaseDialog :show="isModalOpen" :title="t('announcements.title')" width="wide" @close="closeModal">
      <div class="announcement-list">
        <div v-if="loading" class="announcement-loading">
          <LoadingSpinner size="md" />
          <span>{{ t('common.loading') }}</span>
        </div>

        <div v-else-if="announcements.length" class="announcement-rows">
          <button
            v-for="item in announcements"
            :key="item.id"
            type="button"
            class="announcement-row"
            :class="{ 'announcement-row--unread': !item.read_at }"
            @click="openDetail(item)"
          >
            <span
              class="announcement-row__icon"
              :class="item.read_at ? 'announcement-row__icon--read' : 'announcement-row__icon--unread'"
              aria-hidden="true"
            >
              <Icon :name="item.read_at ? 'checkCircle' : 'infoCircle'" size="md" />
            </span>
            <span class="announcement-row__content">
              <span class="announcement-row__title">{{ item.title }}</span>
              <span class="announcement-row__meta">
                <time>{{ formatRelativeTime(item.created_at) }}</time>
                <span v-if="!item.read_at" class="announcement-row__badge">
                  {{ t('announcements.unread') }}
                </span>
              </span>
            </span>
            <Icon name="chevronRight" size="sm" class="announcement-row__arrow" aria-hidden="true" />
          </button>
        </div>

        <EmptyState
          v-else
          :title="t('announcements.empty')"
          :description="t('announcements.emptyDescription')"
          icon="hgi-inbox"
        />
      </div>

      <template #footer>
        <Button v-if="unreadCount > 0" variant="ghost" :disabled="loading" @click="markAllAsRead">
          {{ t('announcements.markAllRead') }}
        </Button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="detailModalOpen && Boolean(selectedAnnouncement)"
      :title="selectedAnnouncement?.title || t('announcements.title')"
      width="wide"
      @close="closeDetail"
    >
      <div v-if="selectedAnnouncement" class="announcement-detail">
        <div class="announcement-detail__meta">
          <span>
            <Icon name="clock" size="sm" aria-hidden="true" />
            {{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}
          </span>
          <span>
            <Icon :name="selectedAnnouncement.read_at ? 'checkCircle' : 'infoCircle'" size="sm" aria-hidden="true" />
            {{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.unread') }}
          </span>
        </div>
        <div
          class="markdown-body announcement-detail__content"
          v-html="renderMarkdown(selectedAnnouncement.content)"
        />
      </div>

      <template #footer>
        <Button variant="ghost" @click="closeDetail">{{ t('common.close') }}</Button>
        <Button
          v-if="selectedAnnouncement && !selectedAnnouncement.read_at"
          variant="solid"
          @click="markAsReadAndClose(selectedAnnouncement.id)"
        >
          {{ t('announcements.markRead') }}
        </Button>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import BaseDialog from './BaseDialog.vue'
import Button from './Button.vue'
import EmptyState from './EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from './LoadingSpinner.vue'
import '@/styles/announcement-markdown.css'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()
const { announcements, loading } = storeToRefs(announcementStore)

const unreadCount = computed(() => announcementStore.unreadCount)
const isModalOpen = ref(false)
const detailModalOpen = ref(false)
const selectedAnnouncement = ref<UserAnnouncement | null>(null)

marked.setOptions({ breaks: true, gfm: true })

function renderMarkdown(content: string): string {
  if (!content) return ''
  return DOMPurify.sanitize(marked.parse(content) as string)
}

function openModal() {
  isModalOpen.value = true
}

function closeModal() {
  isModalOpen.value = false
}

function openDetail(announcement: UserAnnouncement) {
  selectedAnnouncement.value = announcement
  detailModalOpen.value = true
  if (!announcement.read_at) void markAsRead(announcement.id)
}

function closeDetail() {
  detailModalOpen.value = false
  selectedAnnouncement.value = null
}

async function markAsRead(id: number) {
  try {
    await announcementStore.markAsRead(id)
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : t('common.unknownError'))
  }
}

async function markAsReadAndClose(id: number) {
  await markAsRead(id)
  appStore.showSuccess(t('announcements.markedAsRead'))
  closeDetail()
}

async function markAllAsRead() {
  try {
    await announcementStore.markAllAsRead()
    appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : t('common.unknownError'))
  }
}
</script>

<style scoped>
.announcement-list {
  min-height: 180px;
}

.announcement-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-3);
  min-height: 180px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.announcement-rows {
  display: flex;
  flex-direction: column;
}

.announcement-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  width: 100%;
  min-height: 64px;
  padding: var(--sp-3) 0;
  border: 0;
  border-bottom: 1px solid var(--border-subtle);
  background: transparent;
  color: var(--foreground);
  text-align: left;
  cursor: pointer;
  transition: background var(--motion-hover);
}

.announcement-row:last-child {
  border-bottom: 0;
}

.announcement-row:hover {
  background: var(--sidebar-accent);
}

.announcement-row:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.announcement-row--unread::before {
  position: absolute;
  left: calc(var(--sp-3) * -1);
  top: 0;
  bottom: 0;
  width: 2px;
  background: var(--brand);
  content: '';
}

.announcement-row__icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: var(--r-md);
}

.announcement-row__icon--unread {
  background: var(--brand-tint);
  color: var(--brand);
}

.announcement-row__icon--read {
  background: var(--muted);
  color: var(--muted-foreground);
}

.announcement-row__content {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  flex-direction: column;
  gap: var(--sp-1);
}

.announcement-row__title {
  overflow: hidden;
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.announcement-row__meta,
.announcement-detail__meta {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.announcement-row__badge {
  color: var(--brand);
  font-weight: var(--fw-medium);
}

.announcement-row__arrow {
  flex: 0 0 auto;
  color: var(--muted-foreground);
}

.announcement-detail {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}

.announcement-detail__meta span {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
}

.announcement-detail__content {
  min-width: 0;
  padding-top: var(--sp-2);
  border-top: 1px solid var(--border-subtle);
}
</style>
