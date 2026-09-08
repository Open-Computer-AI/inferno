# MiniMax frontend rebuild manifest

Base: `9b1673a10aaa9e5f96c8a1233cdd5fb531a78c2a`
Candidate: `port/inferno-minimax-20260908` (exact final head recorded below)

This rebuild restores the June product tree first, then applies only the scoped
MiniMax semantic delta from `31f550738c481c00cd6de9dd70d21ca5ab19f9d5`. The
previous candidate's unrelated CreateAccountModal test churn is intentionally absent.
This is cumulative changed-path evidence, not dynamic acceptance evidence.

## Changed inferno-frontend paths

- `src/api/admin/channelMonitor.ts`
- `src/api/admin/settings.ts`
- `src/components/account/AccountUsageCell.vue`
- `src/components/account/CnBaseUrlPresets.vue`
- `src/components/account/CreateAccountModal.vue`
- `src/components/account/EditAccountModal.vue`
- `src/components/account/ModelWhitelistSelector.vue`
- `src/components/account/credentialsBuilder.ts`
- `src/components/admin/monitor/MonitorFiltersBar.vue`
- `src/components/admin/monitor/MonitorFormDialog.vue`
- `src/components/admin/monitor/MonitorTemplateManagerDialog.vue`
- `src/components/user/monitor/MonitorCard.vue`
- `src/components/user/monitor/ProviderIcon.vue`
- `src/constants/channelMonitor.ts`
- `src/constants/platforms.ts`
- `src/i18n/locales/en/admin/accounts.ts`
- `src/i18n/locales/en/admin/overview.ts`
- `src/i18n/locales/en/dashboard.ts`
- `src/i18n/locales/zh/admin/accounts.ts`
- `src/i18n/locales/zh/admin/overview.ts`
- `src/i18n/locales/zh/dashboard.ts`
- `src/types/index.ts`
- `src/utils/platformColors.ts`
- `src/views/admin/ChannelsView.vue`

The following immutable/product regression tests are restored from June and
then receive only the required MiniMax-aware assertions where applicable:

- `src/components/account/__tests__/AccountUsageCell.spec.ts`
- `src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- `src/components/account/__tests__/CreateAccountModal.spec.ts`
- `src/components/account/__tests__/credentialsBuilder.cnAdaptive.spec.ts`
- `src/components/account/__tests__/credentialsBuilder.spec.ts`
- `src/components/common/__tests__/PlatformTypeBadge.grok.spec.ts`
- `src/constants/__tests__/platforms.spec.ts`
- `src/views/admin/__tests__/GroupsView.compositePlatforms.spec.ts`
- `src/views/admin/__tests__/channelPlatformOptions.spec.ts`

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

Exact final head: `1c22ad2cfb480f8a8cf872212cab0281fa543dae`

Authoritative inventory: `git diff --name-only
9b1673a10aaa9e5f96c8a1233cdd5fb531a78c2a..1c22ad2cfb480f8a8cf872212cab0281fa543dae`.
The cumulative diff contains 140 paths; this count and the exact path set are
 evaluated from the command above, rather than inferred from semantic categories.
