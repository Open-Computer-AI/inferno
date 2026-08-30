<template>
  <div class="relative">
    <!-- Admin: Full version badge with dropdown -->
    <template v-if="isAdmin">
      <Button
        @click="toggleDropdown"
        size="xs"
        variant="ghost"
        class="version-badge-trigger"
        :class="[
          hasUpdate
            ? 'bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] text-[var(--warning)] hover:bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_30%,transparent)] text-[var(--warning)] dark:hover:bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_50%,transparent)]'
            : 'bg-[var(--brand-tint)] text-[var(--muted-foreground)] hover:bg-[var(--brand-tint)] bg-[var(--brand-tint)] text-[var(--muted-foreground)] dark:hover:bg-[var(--card)]'
        ]"
        :title="hasUpdate ? t('version.updateAvailable') : t('version.upToDate')"
      >
        <span v-if="currentVersion" class="font-[var(--fw-medium)]">v{{ currentVersion }}</span>
        <span
          v-else
          class="h-3 w-12 animate-pulse rounded bg-[var(--brand-tint)] font-[var(--fw-medium)] bg-[var(--brand-tint)]"
        ></span>
        <!-- Update indicator -->
        <span v-if="hasUpdate" class="relative flex h-2 w-2">
          <span
            class="absolute inline-flex h-full w-full animate-ping rounded-full bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] opacity-75"
          ></span>
          <span class="relative inline-flex h-2 w-2 rounded-full bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))]"></span>
        </span>
      </Button>

      <!-- Dropdown -->
      <transition name="dropdown">
        <div
          v-if="dropdownOpen"
          ref="dropdownRef"
          class="absolute left-0 z-50 mt-2 overflow-hidden whitespace-normal rounded-xl border border-[var(--brand-line)] bg-white shadow-lg transition-[background-color,color,opacity] duration-200 border-[var(--brand-line)] bg-[var(--brand-tint)]"
          :class="rollbackPanelOpen && isReleaseBuild ? 'w-80' : 'w-64'"
        >
          <!-- Header with refresh button -->
          <div
            class="flex items-center justify-between border-b border-[var(--brand-line)] px-4 py-3 border-[var(--brand-line)]"
          >
            <span class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]">{{
              t('version.currentVersion')
            }}</span>
            <IconButton
              @click="refreshVersion(true)"
              icon="hgi-arrow-reload-horizontal"
              :label="t('version.refresh')"
              size="sm"
              :disabled="loading"
            />
          </div>

          <div class="p-4">
            <!-- Loading state -->
            <div v-if="loading" class="flex items-center justify-center py-6">
              <svg class="h-6 w-6 animate-spin text-[var(--brand)]" fill="none" viewBox="0 0 24 24">
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
            </div>

            <!-- Content -->
            <template v-else>
              <!-- Version display - centered and prominent -->
              <div class="mb-4 text-center">
                <div class="inline-flex items-center gap-2">
                  <span
                    v-if="currentVersion"
                    class="text-2xl font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white"
                    >v{{ currentVersion }}</span
                  >
                  <span v-else class="text-2xl font-[var(--fw-medium)] text-[var(--muted-foreground)] text-[var(--muted-foreground)]">--</span>
                  <!-- Show check mark when up to date -->
                  <span
                    v-if="!hasUpdate"
                    class="flex h-5 w-5 items-center justify-center rounded-full bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--success)_14%,var(--card))_30%,transparent)]"
                  >
                    <svg
                      class="h-3 w-3 text-[var(--success)] text-[var(--success)]"
                      fill="currentColor"
                      viewBox="0 0 20 20"
                    >
                      <path
                        fill-rule="evenodd"
                        d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                        clip-rule="evenodd"
                      />
                    </svg>
                  </span>
                </div>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    hasUpdate
                      ? t('version.latestVersion') + ': v' + latestVersion
                      : t('version.upToDate')
                  }}
                </p>
              </div>

              <!-- Priority 1: Update error (must check before hasUpdate) -->
              <div v-if="updateError" class="space-y-2">
                <div
                  class="flex items-center gap-3 rounded-lg border border-[var(--destructive)] bg-[var(--destructive-soft)] p-3 border-[color-mix(in_srgb,var(--destructive)_50%,transparent)] bg-[color-mix(in_srgb,var(--destructive-soft)_20%,transparent)]"
                >
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[var(--destructive-soft)] bg-[color-mix(in_srgb,var(--destructive-soft)_50%,transparent)]"
                  >
                    <Icon
                      name="x"
                      size="sm"
                      :stroke-width="2"
                      class="text-[var(--destructive)] text-[var(--destructive)]"
                    />
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-[var(--fw-medium)] text-[var(--destructive)] text-[var(--destructive)]">
                      {{ t('version.updateFailed') }}
                    </p>
                    <p class="truncate text-xs text-[color-mix(in_srgb,var(--destructive)_70%,transparent)] text-[color-mix(in_srgb,var(--destructive)_70%,transparent)]">
                      {{ updateError }}
                    </p>
                  </div>
                </div>

                <!-- Retry button -->
                <Button
                  @click="handleUpdate"
                  :disabled="updating"
                  :loading="updating"
                  variant="danger"
                  size="md"
                  class="w-full"
                >
                  {{ t('version.retry') }}
                </Button>
              </div>

              <!-- Priority 2: Update success - need restart -->
              <div v-else-if="updateSuccess && needRestart" class="space-y-2">
                <div
                  class="flex items-center gap-3 rounded-lg border border-[var(--success)] bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] p-3 border-[color-mix(in_srgb,var(--success)_50%,transparent)] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--success)_14%,var(--card))_20%,transparent)]"
                >
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--success)_14%,var(--card))_50%,transparent)]"
                  >
                    <svg
                      class="h-4 w-4 text-[var(--success)] text-[var(--success)]"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="2"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-[var(--fw-medium)] text-[var(--success)] text-[var(--success)]">
                      {{
                        successKind === 'rollback'
                          ? t('version.rollbackComplete')
                          : t('version.updateComplete')
                      }}
                    </p>
                    <p class="text-xs text-[color-mix(in_srgb,var(--success)_70%,transparent)] text-[color-mix(in_srgb,var(--success)_70%,transparent)]">
                      {{ t('version.restartRequired') }}
                    </p>
                  </div>
                </div>

                <!-- Restart button with countdown -->
                <Button
                  @click="handleRestart"
                  :disabled="restarting"
                  :loading="restarting"
                  variant="success"
                  size="md"
                  class="w-full"
                >
                  <template v-if="restarting">
                    <span>{{ t('version.restarting') }}</span>
                    <span v-if="restartCountdown > 0" class="tabular-nums"
                      >({{ restartCountdown }}s)</span
                    >
                  </template>
                  <span v-else>{{ t('version.restartNow') }}</span>
                </Button>
              </div>

              <!-- Priority 3: Update available for source build - show git pull hint -->
              <div v-else-if="hasUpdate && !isReleaseBuild" class="space-y-2">
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="group flex items-center gap-3 rounded-lg border border-[var(--warning)] bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] p-3 transition-colors hover:bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] border-[color-mix(in_srgb,var(--warning)_50%,transparent)] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_20%,transparent)] dark:hover:bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_30%,transparent)]"
                >
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_50%,transparent)]"
                  >
                    <Icon
                      name="download"
                      size="sm"
                      :stroke-width="2"
                      class="text-[var(--warning)] text-[var(--warning)]"
                    />
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-[var(--fw-medium)] text-[var(--warning)] text-[var(--warning)]">
                      {{ t('version.updateAvailable') }}
                    </p>
                    <p class="text-xs text-[color-mix(in_srgb,var(--warning)_70%,transparent)] text-[color-mix(in_srgb,var(--warning)_70%,transparent)]">
                      v{{ latestVersion }}
                    </p>
                  </div>
                  <svg
                    class="h-4 w-4 text-[var(--warning)] transition-transform group-hover:translate-x-0.5 text-[var(--warning)]"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                </a>
                <!-- Source build hint -->
                <div
                  class="flex items-center gap-2 rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-2 border-[color-mix(in_srgb,var(--brand-line)_50%,transparent)] bg-[color-mix(in_srgb,var(--brand-tint)_20%,transparent)]"
                >
                  <svg
                    class="h-3.5 w-3.5 flex-shrink-0 text-[var(--brand)] text-[var(--brand)]"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <p class="text-xs text-[var(--brand)] text-[var(--brand)]">
                    {{ t('version.sourceModeHint') }}
                  </p>
                </div>
              </div>

              <!-- Priority 4: Update available for release build - show update button -->
              <div v-else-if="hasUpdate && isReleaseBuild" class="space-y-2">
                <!-- Update info card -->
                <div
                  class="flex items-center gap-3 rounded-lg border border-[var(--warning)] bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] p-3 border-[color-mix(in_srgb,var(--warning)_50%,transparent)] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_20%,transparent)]"
                >
                <div
                  class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_50%,transparent)]"
                >
                  <Icon
                    name="download"
                    size="sm"
                    :stroke-width="2"
                    class="text-[var(--warning)] text-[var(--warning)]"
                  />
                </div>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-[var(--fw-medium)] text-[var(--warning)] text-[var(--warning)]">
                      {{ t('version.updateAvailable') }}
                    </p>
                    <p class="text-xs text-[color-mix(in_srgb,var(--warning)_70%,transparent)] text-[color-mix(in_srgb,var(--warning)_70%,transparent)]">
                      v{{ latestVersion }}
                    </p>
                  </div>
                </div>

                <!-- Update button -->
                <Button
                  @click="handleUpdate"
                  :disabled="updating"
                  :loading="updating"
                  icon="hgi-download-01"
                  variant="solid"
                  size="md"
                  class="w-full"
                >
                  {{ updating ? t('version.updating') : t('version.updateNow') }}
                </Button>

                <!-- View release link -->
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="flex items-center justify-center gap-1 text-xs text-[var(--muted-foreground)] transition-colors hover:text-[var(--body-copy)] text-[var(--muted-foreground)] dark:hover:text-[var(--muted-foreground)]"
                >
                  {{ t('version.viewChangelog') }}
                  <Icon name="externalLink" size="xs" :stroke-width="2" />
                </a>
              </div>

              <!-- Priority 5: Up to date - GitHub link + version rollback -->
              <div v-else class="space-y-2">
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="flex items-center justify-center gap-2 py-2 text-sm text-[var(--muted-foreground)] transition-colors hover:text-[var(--body-copy)] text-[var(--muted-foreground)] dark:hover:text-[var(--muted-foreground)]"
                >
                  <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                    <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.464-1.11-1.464-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.418 22 12c0-5.523-4.477-10-10-10z"
                    />
                  </svg>
                  {{ t('version.viewRelease') }}
                </a>

                <!-- Version rollback entry -->
                <div class="border-t border-[var(--brand-line)] pt-2 border-[var(--brand-line)]">
                  <button
                    @click="toggleRollbackPanel"
                    class="group flex w-full items-center justify-between rounded-lg px-2 py-1.5 text-xs text-[var(--muted-foreground)] transition-colors hover:bg-[var(--brand-tint)] hover:text-[var(--muted-foreground)] text-[var(--muted-foreground)] dark:hover:bg-[color-mix(in_srgb,var(--card)_50%,transparent)] dark:hover:text-[var(--muted-foreground)]"
                  >
                    <span class="flex items-center gap-1.5">
                      <Icon name="clock" size="xs" :stroke-width="2" />
                      {{ t('version.rollback') }}
                    </span>
                    <Icon
                      name="chevronDown"
                      size="xs"
                      :stroke-width="2"
                      class="transition-transform duration-200"
                      :class="{ 'rotate-180': rollbackPanelOpen }"
                    />
                  </button>

                  <transition name="rollback">
                    <div v-if="rollbackPanelOpen" class="mt-2 space-y-2">
                      <!-- Source build: online rollback unavailable, use git instead -->
                      <div
                        v-if="!isReleaseBuild"
                        class="flex items-center gap-2 rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-2 border-[color-mix(in_srgb,var(--brand-line)_50%,transparent)] bg-[color-mix(in_srgb,var(--brand-tint)_20%,transparent)]"
                      >
                        <svg
                          class="h-3.5 w-3.5 flex-shrink-0 text-[var(--brand)] text-[var(--brand)]"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                          />
                        </svg>
                        <p class="min-w-0 flex-1 text-xs leading-4 text-[var(--brand)] text-[var(--brand)]">
                          {{ t('version.rollbackSourceHint') }}
                        </p>
                      </div>

                      <!-- Loading versions -->
                      <div
                        v-else-if="rollbackVersionsLoading"
                        class="flex items-center justify-center py-4"
                      >
                        <svg
                          class="h-5 w-5 animate-spin text-[var(--brand)]"
                          fill="none"
                          viewBox="0 0 24 24"
                        >
                          <circle
                            class="opacity-25"
                            cx="12"
                            cy="12"
                            r="10"
                            stroke="currentColor"
                            stroke-width="4"
                          ></circle>
                          <path
                            class="opacity-75"
                            fill="currentColor"
                            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                          ></path>
                        </svg>
                      </div>

                      <!-- Load error + retry -->
                      <div v-else-if="rollbackVersionsError" class="space-y-2">
                        <p
                          class="rounded-lg border border-[var(--destructive)] bg-[var(--destructive-soft)] p-2.5 text-xs text-[var(--destructive)] border-[color-mix(in_srgb,var(--destructive)_50%,transparent)] bg-[color-mix(in_srgb,var(--destructive-soft)_20%,transparent)] text-[var(--destructive)]"
                        >
                          {{ rollbackVersionsError }}
                        </p>
                        <Button
                          @click="loadRollbackVersions"
                          variant="secondary"
                          size="xs"
                          class="w-full"
                        >
                          {{ t('version.retry') }}
                        </Button>
                      </div>

                      <!-- No versions available -->
                      <p
                        v-else-if="rollbackVersions.length === 0"
                        class="py-3 text-center text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]"
                      >
                        {{ t('version.noRollbackVersions') }}
                      </p>

                      <!-- Version list -->
                      <template v-else>
                        <p class="px-0.5 text-[11px] text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t('version.rollbackSelectVersion') }}
                        </p>

                        <button
                          v-for="item in rollbackVersions"
                          :key="item.version"
                          @click="selectRollbackVersion(item.version)"
                          :disabled="rollingBack"
                          class="flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left transition-[background-color,color,opacity] disabled:cursor-not-allowed disabled:opacity-60"
                          :class="
                            selectedRollbackVersion === item.version
                              ? 'border-[var(--warning)] bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))] shadow-sm border-[var(--warning)] bg-[color-mix(in_srgb,color-mix(in_oklch,var(--warning)_14%,var(--card))_20%,transparent)]'
                              : 'border-[var(--brand-line)] hover:border-[var(--brand-line)] hover:bg-[var(--brand-tint)] border-[var(--brand-line)] dark:hover:border-[var(--brand-line)] dark:hover:bg-[color-mix(in_srgb,var(--card)_40%,transparent)]'
                          "
                        >
                          <span class="flex items-center gap-2">
                            <span
                              class="flex h-3.5 w-3.5 items-center justify-center rounded-full border transition-colors"
                              :class="
                                selectedRollbackVersion === item.version
                                  ? 'border-[var(--warning)]'
                                  : 'border-[var(--brand-line)] border-[var(--brand-line)]'
                              "
                            >
                              <span
                                v-if="selectedRollbackVersion === item.version"
                                class="h-1.5 w-1.5 rounded-full bg-[color-mix(in_oklch,var(--warning)_14%,var(--card))]"
                              ></span>
                            </span>
                            <span
                              class="text-sm font-[var(--fw-medium)]"
                              :class="
                                selectedRollbackVersion === item.version
                                  ? 'text-[var(--warning)] text-[var(--warning)]'
                                  : 'text-[var(--body-copy)] text-[var(--muted-foreground)]'
                              "
                              >v{{ item.version }}</span
                            >
                          </span>
                          <span class="text-[11px] tabular-nums text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                            {{ formatPublishedAt(item.published_at) }}
                          </span>
                        </button>

                        <!-- Selected version: manual command (per deploy method) + confirm -->
                        <transition name="rollback">
                          <div v-if="selectedRollbackVersion" class="space-y-2">
                            <p class="px-0.5 text-[11px] text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                              {{ t('version.manualRollbackCommand') }}
                            </p>

                            <!-- Terminal-style block with deploy-method tabs -->
                            <div
                              class="overflow-hidden rounded-lg border border-[var(--brand-line)] border-[var(--brand-line)]"
                            >
                              <div
                                class="flex items-center justify-between border-b border-[var(--brand-line)] bg-[var(--brand-tint)] px-2 py-1.5 border-[var(--brand-line)] bg-[var(--brand-tint)]"
                              >
                                <Segmented
                                  :model-value="manualTab"
                                  :items="manualTabItems"
                                  aria-label="Deployment method"
                                  @update:model-value="setManualTab"
                                />
                                <Button
                                  @click="copyToClipboard(activeManualCommand)"
                                  variant="ghost"
                                  size="xs"
                                  :icon="copied ? 'hgi-checkmark-circle-02' : 'hgi-copy-01'"
                                >
                                  {{ copied ? t('version.copied') : t('version.copyCommand') }}
                                </Button>
                              </div>
                              <code
                                class="block select-all whitespace-pre-wrap break-all bg-[var(--brand-tint)] p-2.5 font-mono text-[10px] leading-relaxed text-[var(--muted-foreground)] bg-[var(--brand-tint)] text-[var(--muted-foreground)]"
                                >{{ activeManualCommand }}</code
                              >
                            </div>

                            <p
                              class="flex items-start gap-1.5 px-0.5 text-[11px] leading-4 text-[var(--warning)] text-[var(--warning)]"
                            >
                              <Icon
                                name="exclamationTriangle"
                                size="xs"
                                :stroke-width="2"
                                class="mt-px flex-shrink-0"
                              />
                              {{ t('version.rollbackWarning') }}
                            </p>

                            <p
                              v-if="rollbackError"
                              class="rounded-lg border border-[var(--destructive)] bg-[var(--destructive-soft)] p-2 text-xs text-[var(--destructive)] border-[color-mix(in_srgb,var(--destructive)_50%,transparent)] bg-[color-mix(in_srgb,var(--destructive-soft)_20%,transparent)] text-[var(--destructive)]"
                            >
                              {{ rollbackError }}
                            </p>

                            <Button
                              @click="handleRollback"
                              :disabled="rollingBack"
                              :loading="rollingBack"
                              variant="warning"
                              size="md"
                              class="w-full"
                            >
                              <span>{{
                                rollingBack
                                  ? t('version.rollingBack')
                                  : t('version.rollbackConfirm', {
                                      version: 'v' + selectedRollbackVersion
                                    })
                              }}</span>
                            </Button>
                          </div>
                        </transition>
                      </template>
                    </div>
                  </transition>
                </div>
              </div>
            </template>
          </div>
        </div>
      </transition>
    </template>

    <!-- Non-admin: Simple static version text -->
    <span v-else-if="version" class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
      v{{ version }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import {
  performUpdate,
  restartService,
  getRollbackVersions,
  rollback as rollbackAPI,
  type RollbackVersionInfo
} from '@/api/admin/system'
import { useClipboard } from '@/composables/useClipboard'
import Icon from '@/components/icons/Icon.vue'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import Segmented from '@/components/common/Segmented.vue'

const GITHUB_REPO = 'Wei-Shaw/sub2api'
// Docker Hub image published by CI (tags carry no "v" prefix, e.g. weishaw/sub2api:0.1.146)
const DOCKER_IMAGE = 'weishaw/sub2api'

const { t } = useI18n()

const props = defineProps<{
  version?: string
}>()

const authStore = useAuthStore()
const appStore = useAppStore()

const isAdmin = computed(() => authStore.isAdmin)

const dropdownOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

// Use store's cached version state
const loading = computed(() => appStore.versionLoading)
const currentVersion = computed(() => appStore.currentVersion || props.version || '')
const latestVersion = computed(() => appStore.latestVersion)
const hasUpdate = computed(() => appStore.hasUpdate)
const releaseInfo = computed(() => appStore.releaseInfo)
const buildType = computed(() => appStore.buildType)

// Update process states (local to this component)
const updating = ref(false)
const restarting = ref(false)
const needRestart = ref(false)
const updateError = ref('')
const updateSuccess = ref(false)
const restartCountdown = ref(0)
// Distinguishes the success + restart panel between update and rollback flows
const successKind = ref<'update' | 'rollback'>('update')

// Rollback states
const rollbackPanelOpen = ref(false)
const rollbackVersions = ref<RollbackVersionInfo[]>([])
const rollbackVersionsLoading = ref(false)
const rollbackVersionsError = ref('')
const selectedRollbackVersion = ref('')
const rollingBack = ref(false)
const rollbackError = ref('')

const { copied, copyToClipboard } = useClipboard()

// Manual rollback methods differ by deployment: script installs use install.sh,
// docker deployments pin the image tag instead
const manualTab = ref<'script' | 'docker'>('script')

const manualTabs = computed(() => [
  { key: 'script' as const, label: t('version.deployScript') },
  { key: 'docker' as const, label: t('version.deployDocker') }
])

const manualTabItems = computed(() =>
  manualTabs.value.map((tab) => ({ value: tab.key, label: tab.label }))
)

function setManualTab(value: string) {
  if (value === 'script' || value === 'docker') manualTab.value = value
}

const scriptRollbackCommand = computed(() => {
  if (!selectedRollbackVersion.value) return ''
  const tag = `v${selectedRollbackVersion.value}`
  return `curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/${tag}/deploy/install.sh | sudo bash -s -- rollback ${tag}`
})

const dockerRollbackCommand = computed(() => {
  if (!selectedRollbackVersion.value) return ''
  return [
    `# ${t('version.dockerEditCompose')}`,
    `image: ${DOCKER_IMAGE}:${selectedRollbackVersion.value}`,
    '',
    `# ${t('version.dockerRecreate')}`,
    'docker compose up -d'
  ].join('\n')
})

const activeManualCommand = computed(() =>
  manualTab.value === 'docker' ? dockerRollbackCommand.value : scriptRollbackCommand.value
)

// Only show update check for release builds (binary/docker deployment)
const isReleaseBuild = computed(() => buildType.value === 'release')

function toggleDropdown() {
  dropdownOpen.value = !dropdownOpen.value
}

function closeDropdown() {
  dropdownOpen.value = false
}

async function refreshVersion(force = true) {
  if (!isAdmin.value) return

  // Reset update states when refreshing
  updateError.value = ''
  updateSuccess.value = false
  needRestart.value = false
  resetRollbackState()

  await appStore.fetchVersion(force)
}

async function handleUpdate() {
  if (updating.value) return

  updating.value = true
  updateError.value = ''
  updateSuccess.value = false

  try {
    const result = await performUpdate()
    successKind.value = 'update'
    updateSuccess.value = true
    needRestart.value = result.need_restart
    // Clear version cache to reflect update completed
    appStore.clearVersionCache()
  } catch (error: unknown) {
    const err = error as { response?: { data?: { message?: string } }; message?: string }
    updateError.value = err.response?.data?.message || err.message || t('version.updateFailed')
  } finally {
    updating.value = false
  }
}

function resetRollbackState() {
  rollbackPanelOpen.value = false
  rollbackVersions.value = []
  rollbackVersionsError.value = ''
  selectedRollbackVersion.value = ''
  rollbackError.value = ''
  manualTab.value = 'script'
}

async function toggleRollbackPanel() {
  if (!isAdmin.value) return
  rollbackPanelOpen.value = !rollbackPanelOpen.value
  // Source builds only show a hint, no version list to fetch
  if (
    rollbackPanelOpen.value &&
    isReleaseBuild.value &&
    rollbackVersions.value.length === 0 &&
    !rollbackVersionsLoading.value
  ) {
    await loadRollbackVersions()
  }
}

async function loadRollbackVersions() {
  if (!isAdmin.value) return
  rollbackVersionsLoading.value = true
  rollbackVersionsError.value = ''
  try {
    const data = await getRollbackVersions()
    rollbackVersions.value = data.versions || []
  } catch (error: unknown) {
    const err = error as { response?: { data?: { message?: string } }; message?: string }
    rollbackVersionsError.value =
      err.response?.data?.message || err.message || t('version.loadVersionsFailed')
  } finally {
    rollbackVersionsLoading.value = false
  }
}

function selectRollbackVersion(version: string) {
  if (rollingBack.value) return
  rollbackError.value = ''
  selectedRollbackVersion.value = selectedRollbackVersion.value === version ? '' : version
}

function formatPublishedAt(publishedAt: string): string {
  if (!publishedAt) return ''
  const date = new Date(publishedAt)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString()
}

async function handleRollback() {
  if (!isAdmin.value) return
  if (rollingBack.value || !selectedRollbackVersion.value) return

  rollingBack.value = true
  rollbackError.value = ''

  try {
    const result = await rollbackAPI(selectedRollbackVersion.value)
    successKind.value = 'rollback'
    updateSuccess.value = true
    needRestart.value = result.need_restart
    rollbackPanelOpen.value = false
    // Clear version cache so the next check reflects the rolled-back version
    appStore.clearVersionCache()
  } catch (error: unknown) {
    const err = error as { response?: { data?: { message?: string } }; message?: string }
    rollbackError.value = err.response?.data?.message || err.message || t('version.rollbackFailed')
  } finally {
    rollingBack.value = false
  }
}

async function handleRestart() {
  if (restarting.value) return

  restarting.value = true
  restartCountdown.value = 8

  try {
    await restartService()
    // Service will restart, page will reload automatically or show disconnected
  } catch (error) {
    // Expected - connection will be lost during restart
    console.log('Service restarting...')
  }

  // Start countdown
  const countdownInterval = setInterval(() => {
    restartCountdown.value--
    if (restartCountdown.value <= 0) {
      clearInterval(countdownInterval)
      // Try to check if service is back before reload
      checkServiceAndReload()
    }
  }, 1000)
}

async function checkServiceAndReload() {
  const maxRetries = 5
  const retryDelay = 1000

  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch('/health', {
        method: 'GET',
        cache: 'no-cache'
      })
      if (response.ok) {
        // Service is back, reload page
        window.location.reload()
        return
      }
    } catch {
      // Service not ready yet
    }

    if (i < maxRetries - 1) {
      await new Promise((resolve) => setTimeout(resolve, retryDelay))
    }
  }

  // After retries, reload anyway
  window.location.reload()
}

function handleClickOutside(event: MouseEvent) {
  const target = event.target as Node
  const button = (event.target as Element).closest('button')
  if (dropdownRef.value && !dropdownRef.value.contains(target) && !button?.contains(target)) {
    closeDropdown()
  }
}

onMounted(() => {
  if (isAdmin.value) {
    // Use cached version if available, otherwise fetch
    appStore.fetchVersion(false)
  }
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}

.rollback-enter-active,
.rollback-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.rollback-enter-from,
.rollback-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.line-clamp-3 {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
