<template>
  <section :id="id" class="profile-settings-group">
    <header class="profile-settings-group__header">
      <h2>{{ title }}</h2>
      <p v-if="description">{{ description }}</p>
    </header>
    <div class="profile-settings-group__rows">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  id: string
  title: string
  description?: string
}>()
</script>

<style scoped>
.profile-settings-group {
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.profile-settings-group__header {
  padding: 11px 14px 9px;
  border-bottom: 1px solid var(--border-subtle);
}

.profile-settings-group__header h2 {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.profile-settings-group__header p {
  max-width: 70ch;
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.45;
}

.profile-settings-group__rows {
  display: grid;
}
</style>
