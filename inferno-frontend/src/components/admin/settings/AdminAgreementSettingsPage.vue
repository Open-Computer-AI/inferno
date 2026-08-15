<template>
	        <!-- Tab: Login Agreement -->
        <div class="space-y-6">
	          <div class="settings-surface">
	            <div class="border-b border-[var(--border-subtle)] px-6 py-4 	             ">
<div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
	                <div>
	                  <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
	                    {{ localText("登录条款确认", "Login agreement") }}
	                  </h2>
	                  <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
	                    {{
	                      localText(
	                        "控制登录页是否要求用户先阅读并同意服务条款、隐私政策或其他 Markdown 文档。",
	                        "Control whether the login page requires users to accept Markdown policy documents first.",
	                      )
	                    }}
	                  </p>
	                </div>
	                <div class="flex items-center gap-3">
	                  <span class="text-sm text-[var(--body-copy)] text-[var(--body-copy)]">
	                    {{ form.login_agreement_enabled ? localText("已启用", "Enabled") : localText("未启用", "Disabled") }}
	                  </span>
	                  <Toggle v-model="form.login_agreement_enabled" />
	                </div>
	              </div>
	            </div>

	            <div class="space-y-6 p-6">
	              <div class="grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_220px]">
	                <div>
	                  <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
	                    {{ localText("展示形式", "Display mode") }}
	                  </label>
	                  <div class="grid grid-cols-2 gap-2 rounded-[var(--r-md)] bg-[var(--surface-subtle)] p-1                    ">
<Button
                      type="button"
                      :variant="form.login_agreement_mode === 'modal' ? 'solid' : 'ghost'"
                      size="sm"
                      class="w-full"
                      :class="
                        form.login_agreement_mode === 'modal'
                          ? 'bg-[var(--card)] text-[var(--brand)]   '
                          : 'text-[var(--body-copy)] hover:text-[var(--foreground)]  '
                      "
                      @click="form.login_agreement_mode = 'modal'"
                    >
                      <Icon name="shield" size="sm" />
                      {{ localText("弹窗", "Modal") }}
                    </Button>
                    <Button
                      type="button"
                      :variant="form.login_agreement_mode === 'checkbox' ? 'solid' : 'ghost'"
                      size="sm"
                      class="w-full"
                      :class="
                        form.login_agreement_mode === 'checkbox'
                          ? 'bg-[var(--card)] text-[var(--brand)]   '
                          : 'text-[var(--body-copy)] hover:text-[var(--foreground)]  '
                      "
                      @click="form.login_agreement_mode = 'checkbox'"
                    >
                      <Icon name="checkCircle" size="sm" />
                      {{ localText("复选框", "Checkbox") }}
                    </Button>
                  </div>
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      form.login_agreement_mode === "checkbox"
                        ? localText("复选框会显示在登录按钮下方，未勾选前所有登录入口禁用。", "The checkbox appears below the login button and gates all login actions.")
                        : localText("弹窗会在登录页打开，用户拒绝后所有登录入口保持禁用。", "The modal opens on the login page and gates all login actions until accepted.")
                    }}
                  </p>
                </div>

                <div>
                  <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                    {{ localText("条款更新日期", "Updated date") }}
                  </label>
                  <input
                    v-model="form.login_agreement_updated_at"
                    type="date"
                    class="field-control"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ localText("日期或文档内容变化后，用户需要重新同意。", "Changing the date or content requires fresh consent.") }}
                  </p>
                </div>
              </div>

              <div>
                <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h3 class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] ">
                      {{ localText("协议文档", "Agreement documents") }}
                    </h3>
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        localText(
                          "文档名称可自定义，内容按 Markdown 保存。可参考：服务条款、使用政策、支持的国家和地区、服务特定条款。",
                          "Document titles are customizable and content is saved as Markdown.",
                        )
                      }}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="solid"
                    size="sm"
                    @click="addLoginAgreementDocument"
                  >
                    <Icon name="plus" size="sm" />
                    {{ localText("添加文档", "Add document") }}
                  </Button>
                </div>

                <div class="mt-4 space-y-3">
                  <div
                    v-for="(doc, index) in form.login_agreement_documents"
                    :key="doc.id || index"
                    class="rounded-[var(--r-md)] border border-[var(--border-subtle)] bg-[var(--card)] p-4                   ">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <div class="flex min-w-0 items-center gap-3">
                        <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-[var(--r-md)] bg-[var(--surface-subtle)] text-[var(--body-copy)]  ">
                          <Icon
                            :name="
                              index === 1
                                ? 'shield'
                                : index === 2
                                  ? 'globe'
                                  : index === 3
                                    ? 'cog'
                                    : 'document'
                            "
                            size="sm"
                          />
                        </span>
                        <div class="min-w-0">
                          <p class="truncate text-sm font-[var(--fw-medium)] text-[var(--foreground)] ">
                            {{ doc.title || localText("未命名文档", "Untitled document") }}
                          </p>
                          <p class="truncate text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                            {{ loginAgreementRoutePath(doc, index) }}
                          </p>
                        </div>
                      </div>
                      <Button
                        type="button"
                        variant="danger"
                        size="xs"
                        :aria-label="localText('删除文档', 'Remove document')"
                        :disabled="
                          form.login_agreement_enabled &&
                          form.login_agreement_documents.length <= 1
                        "
                        @click="removeLoginAgreementDocument(index)"
                      >
                        <Icon name="trash" size="sm" />
                      </Button>
                    </div>

                    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
                      <div>
                        <label class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]">
                          {{ localText("文档名称", "Document title") }}
                        </label>
                        <input
                          v-model="doc.title"
                          type="text"
                          class="field-control text-sm"
                          :placeholder="localText('例如：服务条款', 'Example: Terms of Service')"
                        />
                      </div>
                      <div>
                        <label class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]">
                          {{ localText("路由标识", "Route slug") }}
                        </label>
                        <div class="flex overflow-hidden rounded-[var(--r-md)] border border-[var(--border-subtle)] bg-[var(--card)] focus-within:border-[var(--brand-line)] focus-within:ring-1 focus-within:ring-[var(--ring-focus)]                           ">
<span class="inline-flex flex-shrink-0 items-center border-r border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3 text-sm text-[var(--muted-foreground)]   ">
                            /legal/
                          </span>
                          <input
                            v-model="doc.id"
                            type="text"
                            class="min-w-0 flex-1 border-0 bg-transparent px-3 py-2 text-sm text-[var(--foreground)] outline-none placeholder:text-[var(--muted-foreground)] focus:ring-0  "
                            placeholder="usage-policy"
                          />
                        </div>
                      </div>
                    </div>
                    <div class="mt-3">
                      <label class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]">
                        {{ localText("Markdown 内容", "Markdown content") }}
                      </label>
                        <textarea
                          v-model="doc.content_md"
                          rows="8"
                          class="field-control font-mono text-sm"
                          :placeholder="localText('在这里填写正式 Markdown 内容。', 'Write the final Markdown content here.')"
                        ></textarea>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Login Agreement -->

</template>

<script lang="ts">
import { defineComponent } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import Button from '@/components/common/Button.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminAgreementSettingsPage',
  components: { Button, Icon, Toggle },
  setup() {
    return useSettingsPageBindings()
  },
})
</script>

<style scoped>
:deep(.settings-surface) {
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  box-shadow: none;
  overflow: hidden;
}

:deep(.settings-surface > .border-b) {
  border-bottom-color: var(--border-subtle) !important;
  background: transparent;
}

:deep(.settings-surface [class~="border-[var(--border-subtle)]"]) {
  border-color: var(--border-subtle) !important;
}

:deep(.settings-surface [class~="bg-[var(--surface-subtle)]"]) {
  background: var(--surface-subtle) !important;
}

:deep(.field-control) {
  border-color: var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
}

:deep(.field-control:focus) {
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

@media (max-width: 700px) {
  :deep(.settings-surface) {
    border-radius: var(--r-md);
  }
}
</style>
