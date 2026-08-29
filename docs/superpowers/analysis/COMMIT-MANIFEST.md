# Commit manifest — upstream frontend catch-up

Every upstream commit that touched `frontend/` and never reached
`inferno-frontend/`. One row each. A commit is DONE only when its row says so.

The point of this file: "have we looked at X?" must be a lookup, not another
investigation. Four separate rounds of file-diffing each found a category the
last one missed, because a diff cannot tell our redesign apart from a missing
feature. A commit can — it says what it did and why.

## Totals

| set | commits | period |
|---|---|---|
| **A. missed at vendor time** | 22 (21 with content, 1 merge) | 2026-08-07 → 08-15 |
| **B. since vendor time** | 78 non-merge | 2026-08-16 → 08-29 |
| | **100** | zero overlap |

## Set A — missed at vendor time

Status: `TODO` · `PORTED` · `SKIPPED` · `PRESENT` (already in our tree)
Kind: `LOGIC` (port as-is) · `UI` (rebuild in June) · `BOTH`

| # | sha | date | what they did | kind | status |
|---|-----|------|---------------|------|--------|
| 1 | `9b54b46b0` | 2026-08-07 | fix(admin): enable image generation permission for C | LOGIC | PRESENT |
| 2 | `4999231d6` | 2026-08-08 | 修复邮箱域名注册额度策略 | BOTH | PRESENT |
| 3 | `563a72ca7` | 2026-08-09 | feat: add default-off switch for email domain regist | BOTH | PRESENT |
| 4 | `bbc8b6e90` | 2026-08-09 | 完善大文件备份分卷上传与恢复 | BOTH | PORTED |
| 5 | `f2da30bcd` | 2026-08-09 | Merge pull request #5423 from lyen1688/feat/email-do | - | merge, no content |
| 6 | `33351c7bc` | 2026-08-10 | fix(billing): gofmt channel.go and drop the redundan | BOTH | PRESENT |
| 7 | `5350b3d98` | 2026-08-10 | fix(usage): restore request ID column visibility | BOTH | PORTED |
| 8 | `9096492b5` | 2026-08-10 | feat(billing): support safe upstream response model  | BOTH | PORTED |
| 9 | `b689e5b40` | 2026-08-10 | fix(billing): harden response-model billing and repa | BOTH | PRESENT |
| 10 | `0d7b6ae64` | 2026-08-11 | fix(frontend): show security audit menu in simple mo | UI | PORTED |
| 11 | `943f09d35` | 2026-08-11 | fix: 优化运营监控内存容量显示 | BOTH | PORTED |
| 12 | `670b03f7e` | 2026-08-12 | fix(i18n): move account scheduling threshold keys ou | LOGIC | PORTED |
| 13 | `c0ab3a00e` | 2026-08-12 | feat: Codex OAuth 设备指纹收敛，减少上游可见的设备数和会话数 | BOTH | PORTED |
| 14 | `0ae151a23` | 2026-08-13 | fix: 徽章改读 grok_usage_snapshot，增量刷新比较 Grok 快照 | LOGIC | PORTED |
| 15 | `363cc4994` | 2026-08-13 | fix: SuperGrokPro 用 4.5 窗口区分 Heavy，容量抖动只封单模型 | LOGIC | PORTED |
| 16 | `69648476d` | 2026-08-13 | fix: 账号徽章与用量格按实时档位展示，避免账单滞后误判 | BOTH | PORTED |
| 17 | `a04ce4901` | 2026-08-13 | feat: 新增 grok-4.6 目录、官方定价与请求路径支持 | LOGIC | PRESENT |
| 18 | `b830bc14d` | 2026-08-13 | fix: 长上下文默认保持开启，并与 OpenAI 账号开关取交集 | BOTH | PORTED |
| 19 | `e215c98c2` | 2026-08-13 | fix: 账号页自动刷新偏好改为模块初始化时恢复 | LOGIC | PRESENT |
| 20 | `f3d949107` | 2026-08-13 | feat: 分组支持逐模型定价，并可关闭长上下文阶梯 | BOTH | PORTED |
| 21 | `cb7b03795` | 2026-08-14 | feat: 优化分组用量统计 | BOTH | PORTED |
| 22 | `fce41e318` | 2026-08-15 | fix(openai): make Codex fingerprint convergence opt- | BOTH | PORTED |

## Set B — since the vendor point

81 non-merge commits touching `frontend/`, 2026-08-16 → 08-29. Regenerate before
each batch; upstream adds ~5/day.

```sh
VB=$(git log --format=%H -1 --grep='vendor upstream frontend as the redesign target')
git rev-list --no-merges --reverse "${VB}..upstream/main" -- frontend
```

Tier 1 (27) touch api/types/stores/utils/composables — contract and logic, highest
risk of silently wrong values. Tier 2 (54) touch only components/views/i18n. Set A
proved tier 2 is NOT safe to skip: 15 of its 40 component changes were bug fixes.

| # | sha | date | tier | what they did | kind | status |
|---|-----|------|------|---------------|------|--------|
| 1 | `5e72deb7d` | 05-01 | T2 | feat: ops 错误详情弹窗支持自定义时间区间 | ? | TODO |
| 2 | `3bff4b64b` | 07-10 | T2 | fix(ui): localize user role label in app header | ? | TODO |
| 3 | `7d796f111` | 07-11 | T2 | fix(ui): adapt native form controls to dark mode via col | ? | TODO |
| 4 | `a6d868f27` | 07-11 | T2 | fix(dashboard): include cache tokens in token card break | ? | TODO |
| 5 | `35e8ba2a3` | 07-11 | T2 | fix(announcements): use proper empty-state copy instead  | ? | TODO |
| 6 | `0d5e3ca9b` | 07-11 | T2 | fix(ops): show neutral SLA card when window has no reque | ? | TODO |
| 7 | `901a0439f` | 08-15 | T1 | feat: 国产供应商一等支持（Kimi/Zhipu/DeepSeek 多协议 + 配额/余额监控） | ? | TODO |
| 8 | `e8ff2017c` | 08-16 | T2 | fix(admin): show category labels in ops error distributi | ? | TODO |
| 9 | `cb7841d85` | 08-16 | T2 | fix(i18n): add missing expired key to account status blo | ? | TODO |
| 10 | `22fc0cdbf` | 08-16 | T2 | fix(frontend): clarify OpenAI Fast/Flex policy rules | ? | TODO |
| 11 | `1977810cf` | 08-17 | T2 | fix(frontend): isolate account helper data loading | ? | TODO |
| 12 | `9aac3b73f` | 08-16 | T2 | add Ollama usage query action | ? | TODO |
| 13 | `8d82bb069` | 08-17 | T1 | fix(openai): expose bulk account settings | ? | TODO |
| 14 | `7cdca9e49` | 08-17 | T2 | feat(groups): 放行 kimi/zhipu/deepseek 平台分组创建入口 | ? | TODO |
| 15 | `c38c5beef` | 08-17 | T2 | fix(i18n): CN 余额单元格误引 grokBalance 键致渲染原始 key | ? | TODO |
| 16 | `9f24a5530` | 08-17 | T1 | 功能：支持渠道模型分时倍率定价 | ? | TODO |
| 17 | `7e45634df` | 08-18 | T2 | chore: remove leftover Sora references after platform re | ? | TODO |
| 18 | `a20e1c00c` | 08-18 | T1 | feat(monitor-ui): 配额模式表单、用量快照视图与 8 平台支持 | ? | TODO |
| 19 | `302a10b88` | 08-18 | T2 | test(monitor-ui): 配额视图渲染与开关门控用例 | ? | TODO |
| 20 | `269fbcac0` | 08-18 | T2 | feat: Grok 用量条补齐本站 24h/7d/30d 聚合 | ? | TODO |
| 21 | `fd42d3722` | 08-18 | T2 | fix: hide Grok prepaid and used/limit when they are empt | ? | TODO |
| 22 | `8f6f45983` | 08-18 | T2 | fix(channels): support kimi/zhipu/deepseek platforms in  | ? | TODO |
| 23 | `03c3f3b6f` | 08-18 | T2 | feat(ui): Select 组件支持可选远程搜索（remote/loading props + searc | ? | TODO |
| 24 | `5cbd0c96a` | 08-18 | T2 | fix(monitor-ui): 关联账号选择器改服务端搜索+回填，OpenAI 配额模式加消耗提示 | ? | TODO |
| 25 | `c9effc456` | 08-18 | T1 | fix(frontend): monitor form check-mode restore, account  | ? | TODO |
| 26 | `2c250bfd7` | 08-18 | T1 | fix(monitor-ui): localize the "quota" placeholder model  | ? | TODO |
| 27 | `58e147fba` | 08-19 | T2 | feat(composite): support Codex endpoints | ? | TODO |
| 28 | `b171bb0e4` | 08-19 | T2 | fix(composite): support CN providers | ? | TODO |
| 29 | `f917d19d3` | 08-19 | T2 | test(frontend): align Grok API key placeholder assertion | ? | TODO |
| 30 | `994fbfedd` | 08-19 | T2 | fix(frontend): prevent CN quota labels overlapping bars | ? | TODO |
| 31 | `63839f193` | 08-19 | T2 | fix(frontend): align admin role selector styling | ? | TODO |
| 32 | `1b30a2d74` | 08-19 | T2 | feat(accounts): support header overrides for CN provider | ? | TODO |
| 33 | `26be82cc8` | 08-19 | T1 | 前端：配置渠道倍率并精简长上下文开关 | ? | TODO |
| 34 | `d4d2c746c` | 08-19 | T2 | 前端：修正账号长上下文开关门控 | ? | TODO |
| 35 | `1f2a87adb` | 08-20 | T1 | fix(admin): 补全平台筛选选项 | LOGIC | PORTED |
| 36 | `85051616f` | 08-20 | T2 | feat(accounts): add adaptive API protocol routing | ? | TODO |
| 37 | `b3092145d` | 08-20 | T2 | fix(accounts): harden adaptive protocol compatibility | ? | TODO |
| 38 | `e4f869e0c` | 08-19 | T2 | 完善运维错误详情兼容展示 | ? | TODO |
| 39 | `39485f2e2` | 08-20 | T1 | 更新 Grok 默认模型与官方计费目录 | LOGIC | PORTED |
| 40 | `6c3edc095` | 08-20 | T1 | feat(429): add configurable cooldown and retry strategie | - | SKIPPED — reverted by e62ec2c42, net no-op |
| 41 | `e62ec2c42` | 08-20 | T1 | Revert "feat(429): add configurable cooldown and retry s | - | SKIPPED — the revert; verified total |
| 42 | `2e279c81d` | 08-21 | T2 | fix(frontend): make CN provider quota/balance refresh af | ? | TODO |
| 43 | `68653fb2c` | 08-21 | T2 | fix: allow messages dispatch for composite groups | ? | TODO |
| 44 | `3445485eb` | 08-21 | T1 | fix(frontend): prevent token refresh lock loop | LOGIC | PORTED — fix was present, test added |
| 45 | `d9d2854d2` | 08-21 | T2 | Make enabled model plaza discoverable from /home | ? | TODO |
| 46 | `22e1b8144` | 08-22 | T1 | feat(gateway): expose routed Codex model catalogs | ? | TODO |
| 47 | `e471be730` | 08-22 | T2 | feat(codex): complete routed model catalogs | ? | TODO |
| 48 | `b16ed03ca` | 08-22 | T2 | fix(codex): align routed catalogs with actual routes | ? | TODO |
| 49 | `ee62dfbaf` | 08-22 | T2 | fix(proxy): support bracketed IPv6 hosts in batch proxy  | ? | TODO |
| 50 | `5dfad32b8` | 08-22 | T2 | fix(frontend): accept unlimited (0) user concurrency in  | ? | TODO |
| 51 | `e39fce270` | 08-22 | T1 | fix(codex): sync routed capabilities from upstream | BOTH | PORTED |
| 52 | `77e0409f7` | 08-23 | T1 | 新增渠道时间段定价工作日规则 | ? | TODO |
| 53 | `616df479e` | 08-23 | T2 | fix(admin): show account priority by default | ? | TODO |
| 54 | `4a1da2950` | 08-23 | T2 | fix(deps): bump dompurify to patch multiple sanitizer-by | ? | TODO |
| 55 | `40ea3aeba` | 08-24 | T1 | feat: add OAuth outbound transport plugin system | ? | TODO |
| 56 | `684d9efb1` | 08-24 | T2 | fix: harden plugin runtime and UI bridge | ? | TODO |
| 57 | `391d69e08` | 08-24 | T2 | fix: preserve initial plugin bridge requests | ? | TODO |
| 58 | `377d1230f` | 08-24 | T1 | 模型广场：按计费阶梯单价表展示长上下文档位 | ? | TODO |
| 59 | `ecce0769c` | 08-24 | T2 | 模型广场：上下文档位统一标签形态并保证升序 | ? | TODO |
| 60 | `83d4eb6a4` | 08-24 | T1 | 模型广场：增加渠道分时段计价展示 | ? | TODO |
| 61 | `b07d85c49` | 08-24 | T1 | 模型广场：分时计价同步渠道仅工作日规则 | ? | TODO |
| 62 | `f19095f96` | 08-24 | T2 | 模型广场：分时时段行明确不含高峰倍率口径并披露叠加 | ? | TODO |
| 63 | `cfecc8d11` | 08-24 | T2 | feat: 运维监控错误详情支持返回列表并保留筛选状态 | ? | TODO |
| 64 | `6f972145b` | 08-24 | T1 | feat: 支持 OpenAI 重置卡按用量阈值自动使用 | types + EditAccountModal (verbatim, not June) + OpenAIQuotaResetCell chip (rebuilt in June) + en/zh keys, en dash removed | **PORTED** |
| 65 | `eb594eefc` | 08-24 | T2 | fix(payment): refresh balance after fulfillment | ? | TODO |
| 66 | `3f1581b2d` | 08-25 | T1 | 修复上游倍率探测导致账号列表整页刷新的问题 | backend endpoint was already merged; ported the frontend half — types, `getUpstreamBillingRatesWithEtag`, AccountsView in-place snapshot patching. Script-only, so no June question. +2 tests upstream lacks | **PORTED** |
| 67 | `11ada80d5` | 08-25 | T1 | feat(usage): 使用记录展示映射前的推理强度 | ? | TODO |
| 68 | `5705f4a4a` | 08-25 | T1 | fix(usage): hide mapped reasoning effort from users | ? | TODO |
| 69 | `a8cfe746b` | 08-25 | T2 | test(usage): cover user and admin reasoning effort page  | ? | TODO |
| 70 | `d522aed65` | 08-26 | T2 | fix: preserve promo codes for OAuth signup | ? | TODO |
| 71 | `195b21970` | 08-26 | T1 | fix(codex): isolate API-key catalog cache and DeepSeek C | ? | TODO |
| 72 | `2abce6503` | 08-26 | T1 | fix(codex): harden routed catalog capability sync | BOTH | PARTIAL — preview hunk ported, UseKeyModal blocked |
| 73 | `5f09442fc` | 08-27 | T2 | fix(openai): refresh usage after quota reset | ? | TODO |
| 74 | `b56c61ecc` | 08-28 | T1 | feat(admin): let admins restrict which public groups a u | ? | TODO |
| 75 | `0756c9810` | 08-28 | T2 | fix(frontend): 批量编辑显式提交 codex_fingerprint_mode=off，修复无法关 | ? | TODO |
| 76 | `c4e46c3be` | 08-28 | T2 | feat(zhipu): support team GLM Coding Plan usage query | ? | TODO |
| 77 | `02eee39dd` | 08-28 | T2 | fix(payment): show selected currency in recharge rate | ? | TODO |
| 78 | `c03776604` | 08-28 | T2 | fix(keys): preserve Claude attribution headers | ? | TODO |
| 79 | `706b5676a` | 08-29 | T2 | fix(groups): show API error messages on create and updat | ? | TODO |
| 80 | `ed12ea716` | 08-29 | T2 | fix(frontend): authenticate Codex API key mode inline | ? | TODO |
| 81 | `b5827cfd5` | 08-29 | T1 | fix(pricing): align DeepSeek billing with official peak/ | BOTH | PORTED — merged, DeepSeek under-billing |
