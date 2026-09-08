# MiniMax frontend rebuild manifest

Base: `9b1673a10aaa9e5f96c8a1233cdd5fb531a78c2a`
Branch: `port/inferno-minimax-20260908`

This rebuild restores the June product tree first, then applies only the scoped
MiniMax semantic delta from `31f550738c481c00cd6de9dd70d21ca5ab19f9d5`. The
previous candidate's unrelated CreateAccountModal test churn is intentionally absent.
This is cumulative changed-path evidence, not dynamic acceptance evidence.

## Changed inferno-frontend paths (relative to `inferno-frontend/src`)

- `api/admin/channelMonitor.ts`
- `api/admin/settings.ts`
- `components/account/AccountUsageCell.vue`
- `components/account/CnBaseUrlPresets.vue`
- `components/account/CreateAccountModal.vue`
- `components/account/EditAccountModal.vue`
- `components/account/ModelWhitelistSelector.vue`
- `components/account/__tests__/AccountUsageCell.spec.ts`
- `components/account/credentialsBuilder.ts`
- `components/admin/monitor/MonitorFiltersBar.vue`
- `components/admin/monitor/MonitorFormDialog.vue`
- `components/admin/monitor/MonitorTemplateManagerDialog.vue`
- `components/common/__tests__/PlatformTypeBadge.grok.spec.ts`
- `components/user/monitor/MonitorCard.vue`
- `components/user/monitor/ProviderIcon.vue`
- `constants/__tests__/platforms.spec.ts`
- `constants/channelMonitor.ts`
- `constants/platforms.ts`
- `i18n/locales/en/admin/accounts.ts`
- `i18n/locales/en/admin/overview.ts`
- `i18n/locales/en/dashboard.ts`
- `i18n/locales/zh/admin/accounts.ts`
- `i18n/locales/zh/admin/overview.ts`
- `i18n/locales/zh/dashboard.ts`
- `types/index.ts`
- `utils/platformColors.ts`
- `views/admin/ChannelsView.vue`
- `views/admin/GroupsView.vue`
- `views/admin/__tests__/GroupsView.compositePlatforms.spec.ts`
- `views/admin/groupsCompositeRoutes.ts`

## Required proof

- `PlatformTypeBadge.grok.spec.ts` retains X Basic free-tier/no-expiry coverage
  and canonical SuperGrok Plus labeling.
- `constants/__tests__/platforms.spec.ts` retains the non-empty-label invariant.
- `GroupsView.compositePlatforms.spec.ts` directly checks the view derivation
  from `CONCRETE_PLATFORM_OPTIONS`, excludes `GROUP_PLATFORM_OPTIONS`, and
  includes MiniMax through the concrete catalog.
- No changed path is intentionally replaced by an upstream-identical blob;
  any identical result must be justified from the MiniMax-only delta. The
  cumulative path/hash proof is emitted in the build handoff from `git diff`
  against this base.

## Cumulative changed-files inventory

The exact product inventory is the 30 paths emitted by:
`git diff --name-only 9b1673a10aaa9e5f96c8a1233cdd5fb531a78c2a HEAD -- inferno-frontend/src`.
The evaluator MUST record and validate the current shared HEAD at evaluation time,
confirm it is on `port/inferno-minimax-20260908`, and verify that this command emits
exactly 30 paths after stripping the `inferno-frontend/src/` prefix. This manifest
intentionally does not assert a stale or self-referential final SHA.
