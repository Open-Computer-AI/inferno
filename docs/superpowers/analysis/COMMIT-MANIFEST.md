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
| 1 | `9b54b46b0` | 2026-08-07 | fix(admin): enable image generation permission for C | ? | TODO |
| 2 | `4999231d6` | 2026-08-08 | 修复邮箱域名注册额度策略 | ? | TODO |
| 3 | `563a72ca7` | 2026-08-09 | feat: add default-off switch for email domain regist | ? | TODO |
| 4 | `bbc8b6e90` | 2026-08-09 | 完善大文件备份分卷上传与恢复 | ? | TODO |
| 5 | `f2da30bcd` | 2026-08-09 | Merge pull request #5423 from lyen1688/feat/email-do | ? | TODO |
| 6 | `33351c7bc` | 2026-08-10 | fix(billing): gofmt channel.go and drop the redundan | ? | TODO |
| 7 | `5350b3d98` | 2026-08-10 | fix(usage): restore request ID column visibility | ? | TODO |
| 8 | `9096492b5` | 2026-08-10 | feat(billing): support safe upstream response model  | ? | TODO |
| 9 | `b689e5b40` | 2026-08-10 | fix(billing): harden response-model billing and repa | ? | TODO |
| 10 | `0d7b6ae64` | 2026-08-11 | fix(frontend): show security audit menu in simple mo | ? | TODO |
| 11 | `943f09d35` | 2026-08-11 | fix: 优化运营监控内存容量显示 | ? | TODO |
| 12 | `670b03f7e` | 2026-08-12 | fix(i18n): move account scheduling threshold keys ou | ? | TODO |
| 13 | `c0ab3a00e` | 2026-08-12 | feat: Codex OAuth 设备指纹收敛，减少上游可见的设备数和会话数 | ? | TODO |
| 14 | `0ae151a23` | 2026-08-13 | fix: 徽章改读 grok_usage_snapshot，增量刷新比较 Grok 快照 | ? | TODO |
| 15 | `363cc4994` | 2026-08-13 | fix: SuperGrokPro 用 4.5 窗口区分 Heavy，容量抖动只封单模型 | ? | TODO |
| 16 | `69648476d` | 2026-08-13 | fix: 账号徽章与用量格按实时档位展示，避免账单滞后误判 | ? | TODO |
| 17 | `a04ce4901` | 2026-08-13 | feat: 新增 grok-4.6 目录、官方定价与请求路径支持 | ? | TODO |
| 18 | `b830bc14d` | 2026-08-13 | fix: 长上下文默认保持开启，并与 OpenAI 账号开关取交集 | ? | TODO |
| 19 | `e215c98c2` | 2026-08-13 | fix: 账号页自动刷新偏好改为模块初始化时恢复 | ? | TODO |
| 20 | `f3d949107` | 2026-08-13 | feat: 分组支持逐模型定价，并可关闭长上下文阶梯 | ? | TODO |
| 21 | `cb7b03795` | 2026-08-14 | feat: 优化分组用量统计 | ? | TODO |
| 22 | `fce41e318` | 2026-08-15 | fix(openai): make Codex fingerprint convergence opt- | ? | TODO |
