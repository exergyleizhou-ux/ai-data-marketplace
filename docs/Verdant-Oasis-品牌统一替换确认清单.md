# Verdant Oasis（绿洲）品牌统一替换确认清单

本次将产品对外品牌统一为 **Verdant Oasis（绿洲）**,并落地 Slogan 与「纯净绿洲」理念。本清单记录替换范围、刻意保留项与验证结果。

## 1. 品牌信息(单一事实源)

| 项 | 值 |
|----|----|
| 名称 | **Verdant Oasis（绿洲）** |
| 英文名 | Verdant Oasis |
| 中文名 | 绿洲 |
| 描述性副标题 | AI 训练数据交易市场 |
| Slogan(EN) | *In the data desert, we build a pure oasis.* |
| Slogan(ZH) | 在数据荒漠中，我们筑起一片纯净绿洲。 |
| 理念 | 在杂乱无序的「数据荒漠」中,以质量可信、来源合规、资金安全为根基,为 AI 训练数据流通筑起一片干净、可追溯、可信赖的绿洲。 |

> 前端单一事实源:[`frontend/lib/brand.ts`](../frontend/lib/brand.ts)(`BRAND`)。所有前端展示从此读取,后续改名只改这一处。

## 2. 已替换的面向用户位置(✅ 已完成)

| 位置 | 文件 | 改动 |
|------|------|------|
| 站点标题/描述(metadata) | `frontend/app/layout.tsx` | title→`Verdant Oasis（绿洲） — AI 训练数据交易市场`;description→Slogan+描述 |
| 顶部导航品牌 | `frontend/components/Nav.tsx` | `AI 数据市场` → `Verdant Oasis 绿洲` |
| 落地页主标题 + 副标 | `frontend/app/page.tsx` | h1→品牌名;新增 Slogan(中英)+ 副标题 |
| 全站页脚 | `frontend/app/layout.tsx` | 新增品牌名 + Slogan(中英) |
| 用户服务协议页 | `frontend/app/terms/page.tsx` | 标题更名;**顶部新增 Slogan + 理念 block** |
| 隐私政策页 | `frontend/app/privacy/page.tsx` | 标题更名;**顶部新增 Slogan + 理念 block** |
| README 标题 + 简介 | `README.md` | 标题→品牌名;新增 Slogan;简介更名 |
| 设计文档(v2.0) | `docs/设计与实施文档-v2.0.md` | 标题更名 + Slogan 行 |
| 设计文档(架构) | `docs/设计文档-整体架构与核心模块.md` | 标题更名 + Slogan 行 |

## 3. Slogan / 理念 新增位置(需求 #2)

- `/terms`、`/privacy` 顶部:绿色 header block 含品牌名 + 中英 Slogan + 「纯净绿洲」理念。
- 落地页 hero 与全站页脚:中英 Slogan。

## 4. ⚠️ 刻意保留的代码标识符(未改名,原因)

以下是**内部技术标识符**,非对外品牌;改名会触及大量 import、配置与基础设施,属高风险且超出"品牌统一"范畴,故**保留原 slug**:

| 标识符 | 当前值 | 为何不改 |
|--------|--------|---------|
| Go 模块路径 | `github.com/lei/ai-data-marketplace` | 改名需重写每个 `.go` 文件的 import,且与远程仓库地址耦合 |
| 仓库名 | `ai-data-marketplace` | 改名影响 remote、CI、克隆路径、本地 worktree |
| npm 包名 | `ai-data-marketplace-frontend` | 内部包名,不对用户展示 |
| S3 / bucket | `S3_BUCKET=ai-data-marketplace` | 改名需迁移已存对象 |
| env / DB 名 | `ai_data_marketplace` 等 | 改名需数据迁移与配置同步 |

> 如确需彻底改名(含代码标识符),建议另起专项:① `go mod edit -module` + 全局 import 重写;② GitHub 仓库 rename;③ bucket/DB 迁移;④ 更新 CI、compose、k8s manifests、`.env.*`。本次**不含**该项。

## 5. 验证结果

- 前端:`npm run typecheck` / `lint` / `build` 全绿(14/14 静态页生成,含更新后的 `/`、`/terms`、`/privacy`)。
- 后端:本次无 `.go` 改动,不受影响;CI 全量(vet/build/`go test -race`/gofmt 门 + 真库集成)绿。
- 残留扫描:`grep 'AI 训练数据交易市场'` 剩余项均为**描述性副标题**用法(「是一个 AI 训练数据交易市场」/ `tagline` 常量),非品牌名独立出现,符合预期。

## 6. 后续可选(本次未做,需设计/运营决定)

- Logo / favicon / OG 图(`frontend/app/` 下的图标与 `opengraph-image`)。
- 邮件 / 短信模板署名。
- 自定义域名与对应文案。
- 若决定彻底改技术标识符,见第 4 节专项步骤。
