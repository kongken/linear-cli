# Plane 支持实施计划

日期：2026-10-29。设计基于代码 `19bfbf6` 和 [Plane API 调研](../research/plane-api.md)。状态：实施中，已有 `workctl` 双入口及 Plane 预览代码。本文保留完整目标与交付门槛，当前实际能力、实现差异和待验收项以 [兼容矩阵](../compatibility.md) 为准；不能将下列所有目标视为均已完成。

本计划将现有 Linear 专用工具扩展为同时支持 Linear 与 Plane 的工作管理 CLI。采用中性名称 **`workctl`**，按服务显式分组；保留 **`linear`** 可执行文件及现有使用方式。先交付 Cloud v2 核心闭环，再实现自建 v1，最后完成资源工作流、发布与兼容验收。不承诺把全部 Linear 特性机械复制到 Plane。

## 1. 关键决策

| 决策 | 采用方案 | 理由与代价 |
| --- | --- | --- |
| 产品与可执行文件 | 新增 `workctl`，继续分发 `linear` | 名称不再绑定一家服务；需要构建、测试两个入口 |
| 服务选择 | `workctl linear ...` / `workctl plane ...` | 同名 issue 和 workspace 不会因默认服务变化而被误解析；命令多一层 |
| Linear 兼容 | 原命令树、别名、环境变量、配置、凭据和 JSON 合约保持 | 新 binary 不要求迁移现有脚本；早期维护两条入口 |
| Plane API 版本 | Cloud 默认 v2；自建显式选 v1/v2；版本写入 profile | 文档与 edition 能力不同；维护两个 adapter |
| 跨服务抽象 | 暂不抽象统一 Linear/Plane CRUD；先统一 Plane v1/v2 操作 | 两家语义不同；避免先重写全部 Linear 命令 |
| 认证 | PAT 先行，OAuth 后续独立里程碑 | 没有 app 注册/刷新流程时也可完成核心接入 |
| JSON | Linear 保持原合约；Plane `--json` 返回一个原生响应页 | 保留版本字段与真实续页信息；跨页导出另用 `--jsonl` |
| 产品范围 | 首个稳定版本覆盖工作项、评论、目录查询、周期、模块及链接/附件 | 页面、initiative 等独立排期，发布说明明确未覆盖项 |

比较过的命名方案：

| 方案 | 结论 |
| --- | --- |
| `linear plane issue ...` | 最少新增入口，但长期品牌和命令语义不清楚；替代上一份调研中的临时建议 |
| `linear issue ... --provider plane` | 同名 flag 的 team/project 语义易混淆，默认 provider 会影响旧脚本；不采用 |
| 改为 `plane`，删除 `linear` | 无法自然容纳两个服务且破坏旧安装；不采用 |
| `workctl <service> <resource> <action>` | 采用；无默认 provider，不提供隐式 `workctl issue` |

`workctl` 是本计划选定的技术名称，尚未核验分发平台包名及命令冲突。PL-01 在发布前核验；若名称冲突，只替换新 binary/安装包名称，不改变服务分组、配置版本和接口设计。本轮不重命名 GitHub 仓库或 Go module：保留 `github.com/kongken/linear-cli`，避免安装路径与功能改造同时变化。

## 2. 命令与产品范围

统一采用 `issue` 作为用户输入的资源名，与当前 CLI 一致；Plane 内部使用 work item。初期不再添加 `ticket` / `work-item` 等同义命令。Plane `--project` 始终表示 Plane project，不接收 `--team`，也不隐式映射到 module。

| 命令（均以 `workctl plane` 开头） | 核心行为 | 阶段 |
| --- | --- | --- |
| `auth login/status/whoami/logout` | PAT 登录、身份/访问校验、删除指定凭据 | P1–P2 |
| `profile list/use`、`config view` | 选择默认 Plane profile，查看脱敏后的配置及来源 | P1 |
| `project list/view`、`state list`、`label list`、`user list`、`estimate list` | 查询项目及其工作流/成员/估算目录；项目未启用估算时明确报告 | P2；P3 增加 v1 |
| `issue list/view/create/update` | 读写闭环，名称/UUID/identifier 解析 | P2；P3 增加 v1 |
| `issue comment list/add/update/delete` | 评论内容及一层回复；修改/删除仍由服务校验权限 | P2 list/add；P4 完整操作 |
| `issue query/mine` | 范围、状态、负责人、日期、搜索；只暴露 adapter 能保证的过滤器 | P3 |
| `api METHOD path --input <file>` | 配置版本下的原始 REST 请求；JSON 输出用于排查未封装端点 | P2 |
| `issue delete/archive/unarchive` | 单项操作、明确确认；按版本能力开放 | P4 |
| `issue id/title/url/describe` | 工作项摘要及本地文本辅助操作 | P4 |
| `issue start` | 解析 started 状态并切分支；报告本地 Git 与远端状态的分别结果 | P4 |
| `project create/update/delete`、`state create/update/delete`、`label create/update/delete` | 管理资源；尊重治理模式及权限 | P4 |
| `cycle/module list/view/create/update/delete` 与 `add-issue/remove-issue` | 目录及成员关系，不冒充 issue PATCH 字段 | P4 |
| `issue link list/add/update/delete`、`issue attach`、`issue attachment list/delete` | 链接和文件上传/确认生命周期 | P4 |
| 列表 `--jsonl` | 跨页逐行导出原生资源对象 | P4 |
| OAuth、page/document、milestone、initiative、template 等 | 每项先验证 API/edition 合约，再排期 | P6，不作为首版稳定发布条件 |

bulk 写入、跨服务同步/迁移、watch/webhook daemon、Linear agent session、Linear PR/commit 集成不属于本次 Plane 稳定版范围。不能支持的选项直接报错并说明版本/能力，不忽略参数或伪装成功。P4 的可选能力若某 edition 不具备，必须有能力矩阵及拒绝测试；核心 CRUD/评论/目录功能缺失则该目标不能标记为受支持。

目标用法（实施后才有效）：

```bash
# 已有入口继续有效
linear issue view ENG-42 --json

# 新入口的 Linear 命令保持同样参数和输出
workctl linear issue view ENG-42 --json

# 首次 Plane 登录：token 从交互隐藏输入、环境变量或 stdin 取得
workctl plane auth login --profile cloud --workspace acme --api-version v2
workctl plane auth login --profile onprem --base-url https://plane.example.com --workspace eng --api-version v1

workctl plane --profile cloud project list --json
workctl plane --profile cloud issue list --project <project-uuid> --limit 100
workctl plane --profile cloud issue view ENG-42 --json
workctl plane --profile cloud issue create --project <project-uuid> --title 'Verify Plane' --priority high
workctl plane --profile cloud issue update ENG-42 --assignee user@example.com --label bug
workctl plane --profile cloud issue comment add ENG-42 --body-file ./comment.md
```

Plane 读写必须有确定的实例和 workspace；创建还必须有 project。详情可由完整 identifier 定位；裸数字需要 project context；UUID 详情在版本要求时仍需要 project。提供 project 与 identifier 不一致时失败。禁止因解析不到而猜测另一个项目。名称出现重复时列候选 UUID 并要求明确输入。

`issue list` 默认限定 project；P3 `query` 固定提供 `--project`、`--state`、`--state-group`、`--priority`、`--assignee`、`--mine`、`--unassigned`、`--created-after`、`--updated-after`、`--search` 和 `--sort`。三种负责人过滤互斥，`mine` 是 project 内 `--mine` 的快捷入口。`--all-projects` 显式选择 workspace 范围，与 `--project` 互斥；只有 adapter 能准确执行对应组合时才接受。PL-01/08 按端点冻结各版本支持矩阵，v1 不具备的过滤返回能力错误，不下载前 N 条再客户端过滤冒充完整查询，也不隐式遍历全部项目。`--search` 在帮助中写明各版本匹配范围不同。

Plane 非交互调用缺必填项直接失败；TTY 仅在用户选择交互时提示。删除等操作使用 `--yes`；非 TTY 未传确认则失败。所有正常数据只写 stdout，进度/警告/错误只写 stderr；`--json` 不混入表格或登录提示。退出码：Plane 成功 0、请求/网络失败 1、参数/配置错误 2、取消 130；Linear 两条入口沿用现有行为。

## 3. Linear 兼容与双入口

- 保留 `cmd/linear/main.go`，原 `cli.NewRoot()` 和命令别名继续工作；新增 `cmd/workctl/main.go` 及 `internal/workcli/`。每次构建 fresh Cobra tree，将 Linear root 挂为 `workctl linear`，不复制命令实现。
- 现有 Linear 全局状态在其命令执行时初始化；`workctl plane` 不初始化/读取 Linear 凭据。特别测试 Cobra `PersistentPreRun` 的继承，避免新 root 覆盖原初始化。
- 两条 Linear 入口继续读取当前目录 `linear.toml` / `.linear.toml`、`LINEAR_*` 和原 credentials/keyring。保留 `LINEAR_API_KEY` 与 `--workspace` 冲突规则；不让新 Plane profile 改变这些优先级。
- 保留 Linear 原始 GraphQL JSON 字段/嵌套及 `nodes/pageInfo`，不经过 Plane DTO。`api` 仍接受 GraphQL；`schema` 仍输出 Linear schema。
- 初始发布不删除 `linear`、不打印弃用警告、不移动凭据。以后若停发旧 binary，必须另立 major 迁移计划；本计划不设自动删除期限。
- `linear plane` 只是旧调研建议，没有发布过，因此不增加兼容别名。README 已加入 source build 的预览入口，保留旧 Linear 安装路径；发行版发布仍需 P5 验收。

## 4. 认证、profile 与配置

Plane 使用独立的 `internal/planeconfig/` 和 `internal/planeauth/` module；不复用 Linear 的全局配置单例。一个 profile 绑定一个实例 base URL、workspace、API version 和可选默认 project，凭据独立保存。

用户配置位置：Unix 为 `$XDG_CONFIG_HOME/workctl/config.toml`，未设置时为 `~/.config/workctl/config.toml`；Windows 为 `%APPDATA%/workctl/config.toml`。项目配置只读取当前目录 `workctl.toml`（若不存在则 `.workctl.toml`），不向父目录自动搜索。

拟定 TOML 合约（不含 token）：

```toml
schema_version = 1

[plane]
default_profile = "cloud"

[plane.profiles.cloud]
base_url = "https://api.plane.so"
workspace = "acme"
api_version = "v2"
project_id = "<project-uuid>"
```

项目文件允许 `[plane]` 下的 `profile`、`workspace`、`project_id`；不允许存 token 或覆盖实例/version。profile 的 `base_url/workspace/api_version` 是身份上下文，变更实例或 workspace 应新建 profile。环境变量包括 `PLANE_PROFILE`、`PLANE_BASE_URL`、`PLANE_WORKSPACE`、`PLANE_API_VERSION`、`PLANE_PROJECT_ID`、`PLANE_API_KEY`。

解析顺序与冲突规则：

1. profile 选择：`--profile` > `PLANE_PROFILE` > 项目 `plane.profile` > 用户 `plane.default_profile`。显式 `--no-profile` 跳过全部 profile 来源，与 `--profile` 互斥，用于临时环境变量调用；通常命令引用不存在的 profile 直接失败。
2. 使用 profile 时，实例/version/workspace 取 profile；显式环境变量、flag 或项目 workspace 与之不一致则报错，不能拿该 key 请求另一个上下文。项目 UUID：`--project` > `PLANE_PROJECT_ID` > 项目配置 > profile 默认值。
3. 不使用 profile 的临时调用：对应 flag > 环境变量；base URL 默认 Cloud，Cloud API version 默认 v2，workspace 必填。自建必须显式给 version。此模式只用环境变量/登录 stdin，不自动借用默认 keyring token。
4. token：显式 `auth login --token-stdin`（仅 login）> `PLANE_API_KEY` > 当前 profile keyring；每次只取一个。`--token-stdin` 与环境 token 同时存在时报告冲突。profile 模式的环境 token 用于该 profile 的既定上下文，`config view` 显示来源但不显示内容。
5. 配置解析失败、未知 schema_version、未知键均明确报错；`config view` 输出最终来源便于诊断，不回显秘密。

`auth login --profile <new-name>` 是创建 profile 的入口：此时从 login flags > 环境变量 > Cloud 默认值组装新上下文，先验证再保存。对已存在的 profile，login 可以替换 token，但不能静默更改其身份上下文；实例/workspace 变化需使用新 profile 名。临时参数仅用于无 profile 模式，不持久写入默认 profile。

复用现有 `internal/keyring.Store` 接口注入存储，Plane 使用独立 keyring service `workctl-plane`，key 包含规范化 base URL（保留部署路径前缀）、workspace 和 profile。相同 profile 名不能覆盖另一实例的 token；login 验证通过后原子保存，logout 只删除当前 profile 凭据。

PAT 通过 `X-API-Key` 发送，保留 token 原值，不套用 Linear token 字符过滤。login 先检查当前用户，再检查 workspace projects；没有项目时身份和 workspace 校验结果分开报告。没有 keyring 时提示环境变量或显式 `--plaintext`，不能静默落盘；明文凭据独立保存到同目录 credentials 文件，Unix 权限 0600，使用临时文件与原子替换。token 不进入项目 TOML、日志、错误或 fixture。

base URL 规范化保留 path prefix，拒绝 userinfo/query/fragment；默认 HTTPS，本地 HTTP 需要显式配置允许。请求不跟随跨 origin 重定向，不把 PAT 发给存储下载/上传域名。OAuth P6 需要独立 Plane app、目标部署的 scope、authorization-code/刷新及撤销验证，再决定是否支持浏览器登录；此计划不假设 PKCE 或各 edition OAuth 等价。

认证协议、API 版本与部署差异的事实依据见 [调研认证章节](../research/plane-api.md#api-版本部署与认证)。上述解析顺序和文件结构是拟定的 CLI 合约。

## 5. Module 与 API adapter 设计

调用链固定为：**Cobra command → Plane 操作 module → 版本 adapter → HTTP transport**。版本差异和业务解析集中在操作 module 内，命令不拼 URL、不处理 v1/v2 JSON，也不判断 edition。Linear 继续走原 GraphQL 实现。

| 目录（拟新增） | 职责与接口 | 测试入口 |
| --- | --- | --- |
| `internal/workcli/`、`cmd/workctl/` | 双服务命令树、入口与退出码 | Cobra Execute + subprocess |
| `internal/cmdplane/` | flags、输入校验、显示和错误转换 | 注入 Plane 操作接口、stdin/out/err |
| `internal/plane/` | typed command input、资源解析、工作项/评论操作、能力查询 | 对调用者公开的操作接口 |
| `internal/plane/v1/`、`v2/` | 路径、wire DTO、字段/PATCH、页结构、关联协议 | 通过操作接口调用到 `httptest.Server` |
| `internal/plane/httptransport/` | 认证、URL、timeout、重试、脱敏错误 | 注入 `http.Client`、clock/wait |
| `internal/planeconfig/`、`planeauth/` | profile 解析、credential identity、存储 | 临时文件、MemoryStore |

操作接口按调用者实际需要拆小，例如 `Issues` 的 `Get/List/Create/Update` 和 `Comments` 的 `List/Add`；v1/v2 两个 adapter 都实现它们。输入使用具体类型，不传播 `map[string]any`；JSON 原文仅用于 `json.RawMessage` 输出和显式原始 REST 命令。`IssueRef` 包含 project UUID、work-item UUID/identifier 等解析结果；URI escaping 在 adapter 中完成。

结果包含终端展示用 summary 和独立的原生 JSON；summary 允许 `Assignees []User`，不能先压成单负责人。PATCH 通过带 Set 标记的具体字段表达「未提供/清空/设置」，不能只靠字符串零值判断。关系更新保留版本协议。上层不需要知道 v1 的字段名是 `assignees` 还是 v2 的 `assignee_ids`。

能力结果至少包含版本、已验证 edition/部署信息，以及 search/archive/attachments/cycle/module 等 supported/unsupported/unknown 状态。只依据明确版本合约、只读检查及实测记录启用能力；不能仅凭一次 404 判定版本或永久缓存权限失败。未知能力在调用前说明，未验证部署不能标为正式支持。缓存只在单次命令内，按实例/workspace/project/凭据隔离。

`api` 仅接受配置 base URL 下 `/api/{selected-version}/` 的相对路径，不接受完整 URL、路径穿越或通过重定向改变目标。此命令返回原始服务器能力错误，不作为某个高级 CLI 功能已受支持的证明。

现有 `internal/errors.NewAuthError` 默认提示 `linear auth login`。Plane 使用明确的 suggestion，并通过适配或新增可选错误字段保留现有 Linear 文案/行为。错误打印仍遵循仓库的 `errors.HandleError(err, "Failed to ...")` 规则；Plane command 的 RunE 返回 typed error，process exit 只在入口处理。

## 6. 数据映射与用户输入合约

完整端点证据保留在 [调研 API 映射](../research/plane-api.md#核心-api-映射)，这里固定实施语义：

| 输入/模型 | v1 | v2 | CLI 决策 |
| --- | --- | --- | --- |
| `--title` | `name` | `name` | 必填创建名称，不发送 `title` |
| `--description[-file]` | `description_html` | `description_html` | 默认 Markdown 转 HTML；`--description-format html` 可显式提交 HTML；两种来源互斥 |
| `--priority` | 字符串 | 字符串 | Plane 只接受 `none/urgent/high/medium/low`；Linear 继续接受 0–4，不在 Plane 偷换数字语义 |
| 重复 `--assignee` | `assignees` | `assignee_ids` | UUID 或唯一邮箱；`me` 从当前用户解析；不猜显示名称 |
| `--state`、重复 `--label` | `state`、`labels` | `state_id`、`label_ids` | 项目目录中 UUID/唯一名称解析；v2 成员的 `member_id` 才是用户 ID |
| `--parent` | `parent` | `parent_id` | identifier/UUID 解析；验证项目约束，不猜跨项目父项 |
| `--due-date` | `target_date` | `target_date` | `YYYY-MM-DD`，不替用户变成 timestamp |
| `--estimate` | `estimate_point` | `estimate_point_id` | 先查询该项目估算配置；无法解析则失败，不传裸数值充当 UUID |
| 评论 `--body[-file]` | 版本评论 DTO | `comment_html` | Markdown/HTML 策略与描述一致；`--reply-to` 只允许部署支持的回复层数 |
| cycle/module | 专用关联资源 | `add/remove` 关联操作 | 使用专用命令；禁止误 PATCH 只读的关系字段 |

更新时重复 `--assignee` / `--label` 表示替换完整集合；`--clear-assignees` / `--clear-labels` 清空，两种输入互斥。`--clear-parent/--clear-due-date/--clear-estimate` 区分 null 和未提供。`--description ''` 是显式清空，不等于忽略 flag。追加集合使用 P4 独立操作或读取后有冲突检查的命令，不伪装为原子操作。

v1 文档示例中的 `description` 与字段定义不一致，按定义及目标部署回读确认 `description_html`。状态按 group 解释，但显示原名称；`issue start` 选 project 的 started group，若有多个则需配置/指定 state。日期、HTML、空集合和成员 ID 均进入版本合约测试。

Plane project 负责工作项归属，module 组织 project 内交付；不把 Linear team/project ID 直接导入。更换 workspace、project 或服务都不是本计划承诺的数据迁移功能。内容转换保留原生 HTML 的 JSON 获取方式，文档说明 Markdown 往返可能丢失富文本格式。

## 7. 分页、JSON、错误与速率限制

### 分页与输出

- 终端列表默认 `--limit 50`，允许 1–10000；内部自动续页至限制或结束。`--page-size` 默认 50，最大值按版本/端点校验。搜索端点不假设与普通列表相同 envelope。
- Plane `--json` 输出一个原生响应页，不重命名或伪造字段；可用 `--page-size` 和返回的续页参数。显式 `--limit` 与 `--json` 冲突，避免截断页后返回错误 cursor。详情/创建/更新返回原生资源响应。
- v1 跟随 `next_cursor/next_page_results`；v2 默认 offset，亦可 `--pagination cursor`。`--cursor` 与 `--offset` 互斥，版本/排序不支持时在请求前失败；保留原过滤/排序条件。
- 检测 cursor 重复、offset 不前进、异常空页和 v2 offset 上限；命中遍历保护时明确失败，不返回「已遍历全部」。终端到达 `--limit` 时提示可能还有结果。
- P4 `--jsonl --limit N` 自动续页，每行输出一个版本原生资源对象；不携带伪造 envelope。中途失败保留已输出行，退出非零并在 stderr 说明，文档要求消费者检查退出码。`--json` / `--jsonl` 互斥。

这是对旧调研「保留 Plane 原生分页 envelope」的具体化；不把跨页拼接结果伪装为服务器单页。Linear JSON 行为完全不变。

### 错误与重试

transport 返回包含 status、分类、可用 request ID、Retry-After 和脱敏 detail 的 typed error；命令转换为仓库错误类型。v1 常规错误 JSON、v2 problem+json、空 body 和非 JSON body 都要处理；204 不解码 JSON。

| 情况 | 行为 |
| --- | --- |
| 400 / 422 | 字段校验失败；列出可用字段提示，不自动修改 payload |
| 401 | 凭据无效/过期，提示对应 Plane 登录或 token 来源 |
| 403 | 权限/scope 不足；不假装 token 必定无效 |
| 404 | 资源不存在或不可访问；提示检查上下文，禁止自动切 API version |
| 402 / 409 | 功能/plan 限制或冲突；保留服务 code/detail，说明可行操作 |
| 429 | 显示/遵守 Retry-After；不能固定认为所有版本都是 60/min |
| 5xx、网络中断、超时 | 读请求有界重试；写请求报告失败或结果未知并给出回读建议 |

拟定默认：单次请求 timeout 30 秒；命令 deadline 2 分钟，可用 `--timeout` 调整。GET 最多 3 次总尝试，对网络临时错误、429、502/503/504 重试；指数退避加 jitter，优先服务器 Retry-After/reset 信息。等待超过剩余 deadline 时直接失败，不偷偷缩短服务器要求的等待。clock/wait 可注入，测试不真实 sleep。

写入请求默认不自动重试，包括评论、创建和多步骤关联；不假设 external ID 可用于幂等。收到写入后的超时/断流时提示「结果未知，请回读」，不跨版本重放。每个进程默认顺序请求，避免为目录解析制造突发请求；限流状态按 origin 和 credential 记录，尊重实际响应。跨进程限流协调不在首版范围，文档提示并行脚本的限制。

P4 上传流程依版本获取凭据、上传、确认；仅把存储服务需要的字段发到预签名目标。多步骤命令先校验所有参数，返回已完成步骤及失败位置；不自动删除用户资源做补偿。Git start 不使用强制重置已有分支的实现，先检查分支/工作区；远端状态更新失败时说明本地分支结果。

## 8. 可执行工作包与交付顺序

以下是实施切分建议，本轮不创建/分派子 issue。每个工作包一个可独立审阅的 PR，表中依赖完成后再合并后续工作；P2 Cloud 预览不等于已支持自建。

| 阶段/工作包 | 依赖 | 具体变更位置与交付物 | 完成标准 |
| --- | --- | --- | --- |
| P0 / PL-01 合约与目标矩阵 | 本计划 | 明确新 binary 命名可用性；确定 Cloud 测试 workspace、一个 CE 版本及可清理项目；冻结 flags/config schema/输出契约；建立 `docs/compatibility.md` | 每个目标记录 edition/version/base path；核心端点用只读检查确认；提供测试账号/密钥注入方案。缺真实目标时可继续本地开发，但不能声称目标受支持 |
| P1 / PL-02 双入口 | PL-01 命令合约 | `cmd/workctl`、`internal/workcli`；挂载原 Linear 命令；更新双 binary 构建 | 两条 Linear 入口对同一 fixture 的请求、数据 stdout/stderr、退出行为一致；帮助仅允许入口前缀差异；Plane 不读取 Linear secret |
| P1 / PL-03 配置与凭据 | PL-01 配置合约 | `planeconfig/planeauth`、profile/config/auth 命令与 keyring；login 验证逻辑接入后在 P2 完成 | 优先级、上下文冲突、原子保存、明文显式选择、错误 schema 均有测试；旧文件不变 |
| P1 / PL-04 transport 与 v2 adapter | PL-01，PL-03 context 类型 | `plane/httptransport`、`plane/v2`、typed DTO/错误/页模型 | 认证头、路径前缀、204、取消、重试、脱敏和原始 REST 请求通过 HTTP fixture 合约测试 |
| P2 / PL-05 Cloud 读取与解析 | PL-02–04 | `cmdplane` 身份、目录、identifier、详情/列表、`api`；原生 JSON | 完整 identifier、UUID/数字 context、重复名称、两页遍历、受限字段行为通过；登录可验证 workspace |
| P2 / PL-06 Cloud 写入闭环 | PL-05 | create/update、comment list/add、内容转换与三态 PATCH | 测试多负责人/清空/HTML；Cloud 专用项目 create→view→update→comment→cleanup 实测，记录版本/状态码；发布仅标 Cloud v2 preview |
| P3 / PL-07 自建 v1 | PL-06、PL-01 CE 目标 | `plane/v1`，相同操作接口，独立 routes/DTO/分页；版本能力矩阵 | 同一核心验收套件在 v1/v2 均通过；固定 CE 版本真实 CRUD 成功；确认 `/issues/` 非必需，失败写入不降级 |
| P3 / PL-08 查询 | PL-07 | mine/query、目录解析缓存、版本过滤器 | 支持过滤能在目标服务准确执行；不支持的过滤器明确失败；分页结果不因客户端误过滤而漏项 |
| P4 / PL-09 管理与关联 | PL-07–08 | 项目/状态/标签写入、评论完整操作、delete/archive、cycle/module 及关联 | 各能力两版本 fixtures 与可用部署验收；治理/权限/edition 缺失明确报告；多步失败可恢复 |
| P4 / PL-10 文件与本地工作流 | PL-09 | links/attachments 上传确认、摘要/Git start、JSONL | 上传凭据隔离、失败确认和文件回读；已有 Git 分支不被覆盖；部分导出失败非零退出 |
| P5 / PL-11 文档、发布、迁移 | PL-06–10 | README、安装与双 binary release、补全、认证/config/脚本示例、支持矩阵、变更日志 | Linux/macOS/Windows 构建；升级/降级与脚本回归；Cloud + 指定 CE 完整稳定版验收；密钥与产物校验完成 |
| P6 / PL-12 扩展 | P5 | OAuth 与每个后续资源分别立实现合约 | app/edition/API 事实和验收账户齐全后，独立交付，不阻塞首版稳定版 |

执行关键路径：PL-01 → PL-02/03/04 → PL-05 → PL-06 → PL-07 → PL-08/09/10 → PL-11。PL-02 与 PL-03 可独立开发；PL-04 可先用注入 context 完成 fixtures。整个计划的验收者检查「支持矩阵 + 命令契约 + 实测记录」，不以 PR 数量或仅仅构建成功作为功能完成。

## 9. 测试、文档与发布门槛

### 测试矩阵

| 范围 | 必须覆盖 | 验证方式 |
| --- | --- | --- |
| Linear 回归 | 原 flags/aliases、env/TOML/keyring、JSON connection、错误输出，两条入口初始化 | 原测试 + 关键命令 fixture/subprocess 对照；`NO_COLOR=1` |
| 配置/凭据 | 有/无 profile、所有优先级、跨实例冲突、坏 TOML/schema、token 不泄漏 | 临时 HOME/XDG/APPDATA、MemoryStore，无真实用户凭据 |
| v1/v2 HTTP | 方法/path/query/header/body、原生响应、特殊 URL 字符与 base prefix | `httptest.Server` fixture；新增 fixture 标注来源/version，去除秘密 |
| 业务操作 | 目录解析歧义、member ID、全体负责人、三态 PATCH、内容/日期、关系操作 | 从公开操作接口测试两个 adapter，不只测试内部字段赋值 |
| 分页与限流 | offset/cursor、多页/空页/重复/上限、limit、JSON 页与 JSONL 部分失败 | 确定性 clock/wait + 固定响应；不使用真实延时 |
| 错误/未知结果 | 204、非 JSON、401/403/404/402/409/429/5xx、context、写入已接收但断流 | 服务 fixture + 命令退出码/stdout/stderr；断流写入不得重复 POST |
| 真实目标 | Cloud v2 与指定 CE v1 的身份/目录/CRUD/评论/关联/文件；实际支持矩阵 | 显式 opt-in 集成测试，独立项目、唯一前缀、记录所建资源 ID 并清理 |
| 安装升级 | 两 binary、旧安装路径、旧配置只读兼容、命令冲突、补全 | CI 平台构建 + 隔离安装 smoke test |

真实目标测试默认不在普通 CI 写入外部账号；使用专门 secrets 的手动 job。失败时输出可清理资源 ID，避免在共享项目执行无范围删除。Cloud 和 CE 的状态/字段差异留在 fixture/兼容文档中，不能用跳过全部断言让测试通过。

本地常规检查沿用 `go test ./...`、`go vet ./...`，并在新入口实施后构建 `./cmd/linear` 与 `./cmd/workctl`。若更改 GraphQL 文档，按仓库规则生成并检查差异；本计划不需要更改 Linear schema。发布前运行关键凭据/配置测试的 race 检查，并验证每个平台都能编译两个 binary。

文档与对应代码同 PR 维护：`docs/installation.md`（新旧安装）、`docs/plane.md`（PAT/自建/CRUD/JSON）、`docs/configuration.md`（优先级与实例隔离）、`docs/compatibility.md`（版本/edition/能力矩阵）、`docs/migration.md`（脚本与回退）、变更日志；初次出现的命令都要有 `--help` 和非交互示例。

### 迁移与回退

1. P1–P2 同时构建两个 binary，保留 `go install .../cmd/linear@<version>`；新增 `.../cmd/workctl@<version>`，Go module 路径不变。安装器遇已有同名 binary 时说明来源并避免静默覆盖无关工具。
2. 原脚本可继续用 `linear ...`。选择迁移的脚本只改为 `workctl linear ...`，无需改参数/JSON/配置；对照 fixtures 验证，而非用字符串替换推断等价。
3. Plane 配置和 keyring 独立，无自动将 `.linear.toml` 转为 Plane profile 的迁移。旧版 `linear` 可回退使用，Plane 文件不影响它；不删除旧配置或 keyring entry。
4. 新 schema_version 更改需向后兼容读取或显式迁移（先 dry-run、备份、原子写）；旧版本遇不支持的 schema 明确报错。profile v1→v2 是用户显式切换：另建 v2 profile，先只读验证再跑测试项目，不复用未验证字段或重放历史写入。
5. 预览版逐步开放，达到 P5 才宣布稳定支持 Cloud v2 与经过验证的 CE 版本。未经验证的自建实例标为未认证兼容，不能宣称「支持全部自建」。发现问题可退回前一 binary，保持 profile/凭据不变；远端已写入数据不自动回滚。

稳定发布的退出条件：核心命令在两个已指定目标通过真实验收；P4 能力矩阵有实测或明确不支持依据；旧 Linear 脚本回归通过；双 binary/文档/安装和回退路径可用。任一核心项未满足，只能继续标预览。

## 10. 计划制定时的验证与实施前输入

以下历史记录对应计划提交 `2974e61`：当时只制定计划，没有实现命令、迁移凭据或向 Plane 写入；检查了 root/入口、config、credentials/keyring、GraphQL client、错误提示、issue 工作流和 CI。当时尚无 `cmd/workctl`，因此下面的基线检查不作为新功能验收证据。后续实现状态见 [兼容矩阵](../compatibility.md) 和对应 PR 的验证记录。

本轮验证记录（2026-10-29）：

| 检查 | 结果与含义 |
| --- | --- |
| `go test ./...` | 通过；现有 Linear 测试回归 |
| `go vet ./...` | 通过；现有代码静态检查 |
| `go build -o ../linear-plan-validation ./cmd/linear`、产物 `--help` | 通过；现有入口仍可构建、运行 |
| 文档相对链接/锚点、示例 TOML、PL-01–12 编号检查 | 通过；示例可解析，交付项齐全；不是功能验收 |
| 新 binary / Plane commands / Cloud / CE 联调 | 未实现或未执行，不计为通过 |

既有调研的检查结果见 [调研验证记录](../research/plane-api.md#验证结果与后续验收)。实施验收需要 Cloud/自建测试目标、准确 edition/version、workspace/project 和 PAT；本轮未提供这些信息，尚未做任何带凭据的 Plane API 调用。PL-01 负责补齐；缺少实测条件不妨碍本计划交付或本地 fixture 开发。
