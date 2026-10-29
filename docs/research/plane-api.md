# linear-cli 支持 Plane API 调研

调研日期：2026-10-29。代码基线：`f72d39e85ca6bb09da1962f2b92dd9094f9c4dc8`。

## 结论与交付范围

Plane 的公开 API 可以覆盖工作项、项目、状态、标签、成员、评论及周期等核心操作，适合接入 linear-cli。本次 issue 的要求是「调研」，因此交付 API 映射、代码改造建议及验证清单；**没有实现 Plane 命令，也没有声称完成真实实例联调**。

不能仅把 `LINEAR_GRAPHQL_ENDPOINT` 换成 Plane 地址。当前命令直接构造 Linear GraphQL 查询，Plane 使用 REST；两者的资源边界、字段类型和分页格式也不同。建议先以独立 `linear plane` 命令组落地核心功能，再根据使用情况决定是否抽取跨服务接口。以下命令名、环境变量和目录是设计建议，当前版本尚不支持。

## 当前代码的接入点

下列判断来自本仓库代码，路径均相对于仓库根目录：

| 位置 | 当前行为 | 接入 Plane 的影响 |
| --- | --- | --- |
| `internal/cli/root.go` | Cobra 注册 Linear 命令，初始化 workspace、配置和凭据 | 增加 `cmdplane.New()`；先避免让已有命令隐式改变服务 |
| `internal/graphql/client.go` | POST `{query, variables}`，发送 `Authorization: <key>`，解析 `data/errors` | 新建 REST client，不能复用请求协议或错误解析 |
| `internal/cmdauth/auth.go` | login 调用 `ViewerWhoami`，从 organization 自动获得 workspace slug | Plane 登录单独验证当前用户及显式 workspace 的可访问项目 |
| `internal/config/config.go` | 读取 `LINEAR_*` 和当前目录 `linear.toml` / `.linear.toml` | 单独定义 Plane 配置，避免同名 workspace 混用凭据 |
| `internal/credentials/credentials.go` | keyring service 为 `linear-cli`，以 workspace 保存 key；可显式选择明文文件 | Plane 的 keyring identity 应包含服务类型、实例 base URL、workspace |
| `internal/gql/queries/`、`internal/gql/generated.go` | genqlient 绑定 Linear schema；其他命令还内嵌大量 GraphQL | 新建 Plane DTO 和操作函数；无需修改 Linear schema 或生成文件 |
| `internal/issues/create.go`、`internal/cmdissue/issue.go` | 创建、更新处理 team、project、单 assignee、数字 priority | 逐项转换模型，不能直接把现有 input 当 Plane payload |
| `internal/issues/query.go` | 合并 `nodes`，保留 `pageInfo` | Plane 分页独立实现；保持已有 Linear `--json` 合约 |
| `internal/linear/linear.go`、`internal/issueid/issueid.go` | 详情渲染依赖 Linear 字段；`ENG-123` 的前缀视为 team key | Plane identifier 的前缀应按 project identifier 解析，不复用 team 解析语义 |
| `internal/upload/upload.go`、`internal/const/const.go` | Linear 上传协议及固定 Web/CDN host | 上传、网页链接、私有图片鉴权另做适配 |

仓库 `AGENTS.md` 明确要求 Linear JSON 保持 GraphQL 字段、嵌套和 connection shape。因此建议 `linear plane ... --json` 保留 Plane 的原生字段和分页 envelope；终端展示可以转换成统一摘要，不伪造 Linear GraphQL 返回值。

## API 版本、部署与认证

官方将 v2 标为当前版本，v1 继续支持。Cloud origin 为 `https://api.plane.so`，自建使用自己的部署地址；v2 文档提供 `GET /api/v2/schema/`，但本次未调用。[v2 介绍](https://developers.plane.so/api-reference/v2/introduction)、[迁移说明](https://developers.plane.so/api-reference/v2/migrating-from-v1)

**不能假定所有自建实例都有 v2。** 官方 Community Edition 源码固定到 `37a15d99231042b3ff5a4401fed479fdbec9effe`（2026-10-28），顶层注册只有 `/api/v1/` 等路由，没有 `/api/v2/`。这证明该源码版本的边界，不代表所有商业自建版本。建议 Cloud 以 v2 为目标；CE 自建确认部署后采用明确的 v1 adapter；使用显式版本配置，禁止失败写请求自动降级重发。[固定版本 URL 注册](https://github.com/makeplane/plane/blob/37a15d99231042b3ff5a4401fed479fdbec9effe/apps/api/plane/urls.py)

v1 同时保留已标 deprecated 的 `issues/` 和替代的 `work-items/` 路由。新接入使用 `work-items/`；资源改名与 API 升到 v2 是两件事，该源码没有给出旧别名移除日期。[v1 路由](https://github.com/makeplane/plane/blob/37a15d99231042b3ff5a4401fed479fdbec9effe/apps/api/plane/api/urls/work_item.py)

| 认证方式 | 请求头与要求 | 建议 |
| --- | --- | --- |
| Personal Access Token | `X-API-Key: <token>`；Profile Settings → Personal Access Tokens 创建；按创建者权限访问 | 首版使用；不能沿用 Linear 裸 `Authorization` key |
| Plane OAuth | `Authorization: Bearer <access_token>`；需注册 app 并授权 scope | 后续单独实现；scope 和用户权限都要满足 |
| OAuth Bridge | 管理员配置的 IdP access token，使用 Bearer | 部署相关能力，不作为首版前提 |

认证表依据 [v2 authentication](https://developers.plane.so/api-reference/v2/authentication)。HTTP header 名大小写不影响语义。PAT 先验证 `GET /api/v2/users/me/`，再读取指定 workspace 的项目确认权限；身份有效不代表任意项目可访问。[当前用户](https://developers.plane.so/api-reference/v2/users/get-current-user)

## 核心 API 映射

以下为官方 v2 合约。缩写：`W=/api/v2/workspaces/{workspace_slug}`，`P=W/projects/{project_id}`，`I=P/work-items/{work_item_id}`；`pk` 为对应资源 ID。首版建议使用 UUID，并保留尾部 `/`。

| linear-cli 能力 | Plane REST 路由 | 官方来源 |
| --- | --- | --- |
| issue list/create | `GET / POST P/work-items/` | [Work items](https://developers.plane.so/api-reference/v2/work-items/overview) |
| issue view/update/delete | `GET / PATCH / DELETE P/work-items/{pk}/` | [Work items](https://developers.plane.so/api-reference/v2/work-items/overview) |
| 根据 `ENG-42` 查找 | `GET W/work-items/ENG-42/` | [Identifier lookup](https://developers.plane.so/api-reference/v2/work-items/get-work-item-by-identifier) |
| 项目内搜索 | `GET P/work-items/?search=...` | [项目工作项列表](https://developers.plane.so/api-reference/v2/work-items/list-work-items) |
| workspace 列表/搜索 | `GET W/work-items/?search=...` | [Workspace 列表](https://developers.plane.so/api-reference/v2/work-items/list-workspace-work-items) |
| project list/create；view/update/delete | `GET / POST W/projects/`；`GET / PATCH / DELETE W/projects/{pk}/` | [Projects](https://developers.plane.so/api-reference/v2/projects/overview) |
| state list/create；view/update/delete | `GET / POST P/states/`；`GET / PATCH / DELETE P/states/{pk}/` | [States](https://developers.plane.so/api-reference/v2/states/overview) |
| label list/create；view/update/delete | `GET / POST P/labels/`；`GET / PATCH / DELETE P/labels/{pk}/` | [Labels](https://developers.plane.so/api-reference/v2/labels/overview) |
| user list | `GET W/members/` 或 `GET P/members/` | [Members](https://developers.plane.so/api-reference/v2/members/overview) |
| auth whoami/status | `GET /api/v2/users/me/` | [当前用户](https://developers.plane.so/api-reference/v2/users/get-current-user) |
| issue comment list/add | `GET / POST I/comments/` | [Comments](https://developers.plane.so/api-reference/v2/work-item-comments/overview) |
| comment view/update/delete | `GET / PATCH / DELETE I/comments/{pk}/` | [Comments](https://developers.plane.so/api-reference/v2/work-item-comments/overview) |
| cycle list/create；view/update/delete | `GET / POST P/cycles/`；`GET / PATCH / DELETE P/cycles/{pk}/` | [Cycles](https://developers.plane.so/api-reference/v2/cycles/overview) |
| cycle 增删工作项 | `POST P/cycles/{pk}/work-items/`，body 为 `add` / `remove` UUID 数组 | [Cycle membership](https://developers.plane.so/api-reference/v2/cycles/manage-cycle-work-items) |
| module list/create；view/update/delete | `GET / POST P/modules/`；`GET / PATCH / DELETE P/modules/{pk}/` | [Modules](https://developers.plane.so/api-reference/v2/modules/overview) |
| module 增删工作项 | `POST P/modules/{pk}/work-items/`，body 为 `add` / `remove` UUID 数组 | [Module membership](https://developers.plane.so/api-reference/v2/modules/manage-module-work-items) |

v2 `search` 匹配工作项名称，不能声称等同于 Linear 全文搜索。两种列表的过滤条件按各自文档翻译，不能直接发送 GraphQL filter。[项目列表](https://developers.plane.so/api-reference/v2/work-items/list-work-items)、[workspace 列表](https://developers.plane.so/api-reference/v2/work-items/list-workspace-work-items)

### 自建 v1 的关键差异

- 工作项 CRUD 使用 `/api/v1/workspaces/{slug}/projects/{project_uuid}/work-items/` 及 UUID 详情后缀；评论也在该工作项下。其他资源需对照目标路由，不机械替换版本号。[固定版本路由](https://github.com/makeplane/plane/blob/37a15d99231042b3ff5a4401fed479fdbec9effe/apps/api/plane/api/urls/work_item.py)
- **v1 已有** `GET /api/v1/workspaces/{slug}/work-items/{project_identifier}-{issue_identifier}/`；不必仅为 identifier 查询升级 v2。[v1 identifier](https://developers.plane.so/api-reference/issue/get-issue-sequence-id)
- 简单搜索为 `GET /api/v1/workspaces/{slug}/work-items/search/?search=...`，支持项目/工作区范围等参数。[v1 search](https://developers.plane.so/api-reference/issue/search-issues)
- v1 文档有 POST `.../work-items/advanced-search/`，但所检查 CE 路由没有它；该 CE 列表 view 也明确拒绝非空 `pql` / `filters`，返回 400。高级查询按 edition 判断能力。[高级搜索文档](https://developers.plane.so/api-reference/issue/advanced-search-work-items)、[CE view](https://github.com/makeplane/plane/blob/37a15d99231042b3ff5a4401fed479fdbec9effe/apps/api/plane/api/views/issue.py#L298)
- v1 周期关联为 `POST P/cycles/{cycle_id}/cycle-issues/`，移除为 `DELETE .../cycle-issues/{issue_id}/`；module 使用 `module-issues/`。此处 `P` 改用 v1 前缀，不能发送 v2 的 `add/remove` body。[Cycle 路由](https://github.com/makeplane/plane/blob/37a15d99231042b3ff5a4401fed479fdbec9effe/apps/api/plane/api/urls/cycle.py)、[Module 路由](https://github.com/makeplane/plane/blob/37a15d99231042b3ff5a4401fed479fdbec9effe/apps/api/plane/api/urls/module.py)

## 数据模型映射与语义差异

| Linear 当前模型 | Plane v2 | 处理方式 |
| --- | --- | --- |
| `title` | `name`（创建必填） | 显式重命名 |
| Markdown `description` | `description_html` | 写入前 Markdown → HTML，读取时转为终端可读文本 |
| 数字 priority 0–4 | `none / urgent / high / medium / low` | 建议 `0→none, 1→urgent, 2→high, 3→medium, 4→low` |
| 单一 `assigneeId` | `assignee_ids: []` | 支持多人；不能只保留第一个 |
| `stateId`、`labelIds`、`parentId` | `state_id`、`label_ids`、`parent_id` | 名称/邮箱解析到对应 UUID |
| `dueDate` | `target_date` | 日历日期，不混同周期 timestamp |
| `estimate` 数值 | `estimate_point_id` 或文档支持的值解析 | 按项目估算配置解析，不把数字当 UUID |
| team 是 issue 所属边界 | project 是 work item 所属边界 | 业务上近似，仍须显式 Plane project context |
| project 用于组织交付 | module 是项目内工作项集合 | 候选业务映射，两个资源并不等价 |

字段表依据 [v2 创建](https://developers.plane.so/api-reference/v2/work-items/create-work-item)、[工作项结构](https://developers.plane.so/api-reference/v2/work-items/overview)、[Modules](https://developers.plane.so/api-reference/v2/modules/overview)；Linear 侧依据仓库代码。内容转换、数字优先级转换和资源对应关系是本报告的实现建议。

v2 PATCH 省略字段保持不变，支持的 nullable 字段用 null 清空；`assignee_ids` / `label_ids` 整体替换，`[]` 清空。响应中的 `cycle_id` / `module_ids` 不代表可以直接 PATCH，使用关联接口。[更新工作项](https://developers.plane.so/api-reference/v2/work-items/update-work-item)

v1 创建/更新使用 `assignees`、`labels`、`state`、`parent`、`estimate_point` 等字段，描述为 `description_html`，与 v2 payload 不兼容。v1 示例还混用了 `description`，应以字段定义和目标 schema/实测确认。[v1 create](https://developers.plane.so/api-reference/issue/add-issue)、[v1 update](https://developers.plane.so/api-reference/issue/update-issue-detail)

另外需要处理：

- v2 成员行 `id` 为 membership ID，`member_id` 才是用户 ID；`assignee_ids` 使用后者，`expand=member` 获得姓名/邮箱。[Members](https://developers.plane.so/api-reference/v2/members/overview)
- 状态名称可自定义；v2 group 有 `backlog/unstarted/started/completed/cancelled/triage`，按 group 寻找状态，不硬编码显示名称。[States](https://developers.plane.so/api-reference/v2/states/overview)
- 评论写 `comment_html`，纯文本字段为派生内容；`parent_id` 支持一层回复。重复正文或 external ID 不保证去重。[Create comment](https://developers.plane.so/api-reference/v2/work-item-comments/create-comment)
- 列表精简字段，描述需显式选择或请求详情；缺省不等于空内容。[工作项列表](https://developers.plane.so/api-reference/v2/work-items/list-work-items)

## 分页、错误与限流

| 版本 | 合约 | 实现影响 |
| --- | --- | --- |
| v1 | `results/next_cursor/next_page_results`；介绍页给出每页默认/最大 100，按 key 60 次/分钟及 reset header | 独立分页；限额仅作该文档基准，服从实际响应 |
| v2 | `data` envelope；默认 offset，`per_page` 默认 50、最大 200，`next` 为下个 offset 或 null；`paginate=cursor` 返回 `next_cursor/has_more` | 分别处理两种分页，不是 GraphQL connection |
| v2 错误 | `application/problem+json`，含 `type/code/detail` 及可选校验 `errors`；429 返回 `Retry-After` | 按状态及结构解析，不沿用 GraphQL errors |

来源：[v1 介绍](https://developers.plane.so/api-reference/introduction)、[v2 pagination](https://developers.plane.so/api-reference/v2/pagination)、[v2 errors](https://developers.plane.so/api-reference/v2/errors)。v2 错误页按凭据类别说明限流桶，没有在该节承诺统一每分钟配额，不能直接套用 v1 的 60/min。

官方文档亦有不一致：v2 当前用户页将 header 大小写列为 401 原因，但认证页明确大小写不敏感；`/users/me/` 的 OAuth scope 要求也不完全一致。首版 PAT 不依赖 OAuth scope 推断，后续 OAuth 针对目标版本验证。[当前用户](https://developers.plane.so/api-reference/v2/users/get-current-user)、[认证](https://developers.plane.so/api-reference/v2/authentication)

## 建议的实现边界

### 第一阶段：可验证的核心命令

建议增加 `internal/plane/`（REST client、DTO、分页、资源解析）和 `internal/cmdplane/`（Cobra 命令），以 `linear plane` 为入口。优先支持：

1. `auth login/status/logout`：PAT、实例地址、workspace；验证用户和项目权限。
2. `project list/view`、`state list`、`label list`、`user list`：获得创建工作项需要的 UUID。
3. `issue list/view/create/update`，随后支持 `issue comment list/add`。名称沿用 CLI 的 issue，内部使用 Plane work item。

第一阶段要求显式 project context，不把现有 `--team` 自动翻译成 Plane project，也不把现有 `--project` 自动翻译成 module。跨项目搜索及 `mine` 可随后增加。删除、归档、文件上传、关系、页面、initiative、模板和 Linear agent session 等分别评估能力；文档有某个资源不等于所有 Linear 子命令都具备对应语义。

### 配置与凭据设计

建议字段为 `PLANE_API_KEY`、`PLANE_BASE_URL`、`PLANE_API_VERSION`、`PLANE_WORKSPACE`、`PLANE_PROJECT_ID`，持久配置采用独立 Plane 段。保留所有现有 `LINEAR_*` 的行为。base URL 表示实例部署前缀，version 显式指定 `v1` / `v2`；自建部署包含路径前缀时不可在 URL 拼接时丢失。

凭据先使用环境变量或系统 keyring。保存时以 `(plane, normalized instance base URL, workspace slug)` 隔离；仅允许显式选择明文保存。不要沿用 Linear login 对 token 做正则裁剪的方式改变 Plane token。发送凭据前检查目标 origin；预签名存储上传只发送存储服务要求的字段，不转发 Plane PAT。

### Client 必须处理的行为

- 使用 `net/http`、请求 context 和明确 timeout；按资源路径选择 GET/POST/PATCH/DELETE。
- 识别 HTTP 状态及 REST 错误体，将认证、权限、找不到资源、校验失败分别转为 `internal/errors` 中的错误类型。204 不解析 JSON；非 JSON 错误要保留可理解的状态信息。
- 429 等限流按服务器返回信息有界等待，尊重 context；读操作可有界重试，创建和评论等写操作不能在结果未知时盲目重试。
- 以服务返回的 cursor 前进；保留 `--limit`，检测重复 cursor，避免无限循环。不同资源的响应结构分别验证。
- 对 PATCH 区分「未提供」「清空」「设置」；多成员、多标签必须明确替换还是追加，不静默丢弃已有值。

## 验证结果与后续验收

本次已核对上述本地代码、公开官方 API 文档，并在代码基线上运行：

| 验证 | 结果 | 能证明什么 |
| --- | --- | --- |
| `go test ./...` | 通过 | 现有测试基线正常；不证明 Plane 接入已实现 |
| `go vet ./...` | 通过 | 现有代码静态检查通过 |
| `go build -o ../linear-validation ./cmd/linear` | 通过 | 现有 CLI 可以构建 |
| Plane 带凭据 API 调用 | 未执行 | 本次未提供 Plane token、workspace 或实例信息 |
| Cloud / 自建写入、回读、清理 | 未执行 | 真实行为、部署版本及权限仍需后续实测 |

本次仅增加研究文档，不新增镜像实现细节的测试。后续实现应使用 `httptest.Server` 验证方法、路径、认证头、字段类型、204、401/403/404/429、错误体、超时、分页和重复 cursor，以及 PATCH 清空和重定向凭据隔离；再回归 `go test ./...`、`go vet ./...`。

真实实例验收应在专用测试项目完成：先读取当前用户、项目、状态和成员，再创建带唯一标题的工作项，按 identifier 回读，修改状态/优先级/描述/负责人，添加并回读评论，最后清理。Cloud 与目标自建版本分别记录请求路径、状态码、分页和内容格式；记录中删去 token。出现 404 时区分版本不支持、无权限和资源不存在，不自动将失败的写操作切到另一版本重发。

实施前必须确定目标实例及版本、workspace/project 和可用 PAT。它们是集成验收的输入，不影响本次调研结论的交付。
