# Plane 使用说明

Plane 目前为预览实现。以下命令已经实现，本地接口测试不代表已验证真实 Cloud/自建部署；当前能力见 [兼容矩阵](compatibility.md)。旧 `linear` 命令继续访问 Linear。

## 登录与实例

Cloud 默认使用 `https://api.plane.so` 的 v2 API。首次登录会隐藏输入 PAT，验证当前用户及 workspace 的项目访问，再保存 profile。PAT 从 Plane Profile Settings 的 Personal Access Tokens 创建。

```bash
workctl plane auth login --profile cloud --workspace my-workspace
workctl plane auth status
workctl plane profile list
workctl plane profile use cloud
workctl plane config view
```

自动化可从 `PLANE_API_KEY` 读取，或给 login 指定 `--token-stdin`；两种输入不能同时使用。系统 keyring 不可用时，可继续使用环境变量，或明确指定 login 的 `--plaintext`。不会自动选择明文保存。

自建实例必须显式给出 API version，base URL 可含部署路径前缀：

```bash
workctl plane auth login --profile selfhost \
  --base-url https://plane.example.com/prefix \
  --workspace engineering --api-version v1
workctl plane --profile selfhost project list --json
```

仅开发用 HTTP 实例可显式加 `--allow-http`。已有 profile 不允许通过环境变量/参数换成另一实例、workspace 或 API version；应创建新 profile。详见 [配置](configuration.md)。

`auth logout` 删除当前 profile 的存储凭据，不撤销服务器上的 PAT，也不清除 shell 中的 `PLANE_API_KEY`。需要停止使用环境 token 时，另行 `unset PLANE_API_KEY`。

## 工作项与目录

Plane 的 project 是工作项所属项目；`--project` 不等于 Linear project，也没有 `--team` 翻译。项目可以使用 UUID、identifier 或唯一名称。裸数字工作项引用需 project context；UUID 详情也需要 project；完整 identifier 可在 workspace 内查找。

```bash
workctl plane project list
workctl plane --project PROJECT_UUID state list --json
workctl plane --project PROJECT_UUID user list --json
workctl plane --project PROJECT_UUID label list

workctl plane issue view ENG-42 --json
workctl plane --project PROJECT_UUID issue view 42
workctl plane --project PROJECT_UUID issue create \
  --title 'Fix login' --description-file ./issue.md --priority high \
  --assignee alice@example.com --label bug --json
workctl plane issue update ENG-42 --state 'In Progress' --due-date 2026-11-01
workctl plane issue update ENG-42 --clear-assignees --clear-labels --description ''
```

名称歧义时使用 UUID。负责人接受 UUID、唯一邮箱、`me` / `@me` / `self`，不猜显示名称；v2 membership ID 不会误当作用户 ID。一次命令内目录缓存复用，并要求目录遍历完整，超过 10000 条时要求显式 UUID。

- priority 接受 `none/urgent/high/medium/low`；Linear 仍使用原来的 0–4。
- 描述默认 Markdown → HTML；已是 HTML 时加 `--description-format html`。终端展示为可读文本；`--json` 保留原始 HTML/其他字段。
- 重复 `--assignee` / `--label` 设置完整集合，替换原值。清空必须用 `--clear-assignees` / `--clear-labels`，不能与设置值同时使用。
- `--clear-parent`、`--clear-due-date`、`--clear-estimate` 明确发送 null；未提供的字段不修改。
- `--estimate` 在本预览中要求 estimate point UUID，不支持按数值查找。`estimate list` 仅 v2；v1 可显式调用配置的估算接口。
- `--parent` 使用同项目的 UUID/identifier；不支持移动项目或跨服务复制。

## 查询与分页

v2 支持服务端过滤：

```bash
workctl plane --project PROJECT_UUID issue mine
workctl plane --project PROJECT_UUID issue query \
  --priority high --state-group started --assignee alice@example.com \
  --created-after 2026-10-01 --sort=-updated_at --limit 100
workctl plane issue query --all-projects --search login --json
```

`--mine`、`--assignee`、`--unassigned` 互斥。日期可用 `YYYY-MM-DD` 或 RFC3339，边界为包含该日期/时间。v2 search 匹配名称。当前 v1 adapter 的 query 只提供无过滤的项目列表与保守排序；未验证的过滤器明确拒绝，不在前 N 条结果上做客户端过滤。

终端列表默认最多 50 条，可设 `--limit 1..10000`。`--page-size` 控制请求大小（v1 最大 100，v2 最大 200）。v1 按 cursor，v2 默认 offset，可用 `--pagination cursor`；priority/state_group/sort_order 的语义排序不接受 cursor。

```bash
# --json 只返回一个原生响应页，续页信息不被改写。
workctl plane --project PROJECT_UUID issue list --json --page-size 50
workctl plane --project PROJECT_UUID issue list --json --cursor 'RETURNED_CURSOR'
workctl plane --project PROJECT_UUID issue list --json --offset 50

# 跨页导出，每行一个原生资源对象。
workctl plane --project PROJECT_UUID issue list --jsonl --limit 500
```

v2 的 `--cursor` 自动选择 cursor 风格；`--offset` 仅 v2，不能与 cursor 同用。`--json` 不能同时使用显式 `--limit` 或 `--jsonl`。中途失败时 JSONL 已输出行仍保留，退出码非零，消费者必须检查退出码。发生重复 cursor、空页但仍有续页、offset 不前进或超出上限时失败，不假装遍历完成。

## 评论、链接与附件

```bash
workctl plane issue comment list ENG-42 --json
workctl plane issue comment add ENG-42 --body-file ./comment.md --json
workctl plane issue comment add ENG-42 --body 'Reply' --reply-to COMMENT_UUID
workctl plane issue comment update ENG-42 COMMENT_UUID --body-file ./replacement.md
workctl plane issue comment delete ENG-42 COMMENT_UUID --yes

workctl plane issue link list ENG-42 --json
workctl plane issue link add ENG-42 https://example.com/design --title 'Design'
workctl plane issue link update ENG-42 LINK_UUID --input ./link.json
workctl plane issue link delete ENG-42 LINK_UUID --yes

workctl plane issue attach ENG-42 ./design.pdf
workctl plane issue attachment list ENG-42 --json
workctl plane issue attachment delete ENG-42 ASSET_UUID --yes
```

评论同样默认 Markdown，可用 `--body-format html`；回复层数由部署校验。文件必须是 1 byte–100 MiB 的常规文件，部署可能有更低限制；可用 `--content-type` 明确 MIME type。上传依次分配 attachment、发送签名 multipart 表单、确认 `is_uploaded=true`；不把 Plane PAT 发送给存储服务。

存储/确认失败时错误包含已分配 asset ID，便于回读或显式清理；不自动重复分配、重传或删除。不要在结果未知时盲目重复评论、创建或上传。

## 管理资源与关联

project、state、label、cycle、module 都提供 list/view/create/update/delete。管理写入要求显式版本原生 JSON 文件：

```bash
# label.json 示例：{"name":"bug","color":"#FF0000"}
workctl plane --project PROJECT_UUID label create --input ./label.json
workctl plane --project PROJECT_UUID label update LABEL_UUID --input ./label.json
workctl plane --project PROJECT_UUID label delete LABEL_UUID --yes

workctl plane --project PROJECT_UUID cycle add-issue CYCLE_UUID ENG-42
workctl plane --project PROJECT_UUID cycle remove-issue CYCLE_UUID ENG-42
workctl plane --project PROJECT_UUID module add-issue MODULE_UUID ENG-42
```

管理的 `--input -` 从 stdin 读 JSON；内容必须为非空 object，字段由所选版本的 API 校验。这类命令返回原生 JSON，不再要求 `--json`。project 之外的资源需要 project context。治理模式、权限或 edition 不允许某操作时，保留服务器错误，不静默跳过。

v1 关联使用 cycle-issues/module-issues 及 issues 数组；v2 使用 add/remove 数组。CLI 只变更指定的一个工作项，不模拟批量原子操作。

```bash
workctl plane issue delete ENG-42 --yes
workctl plane issue archive ENG-42 --yes
workctl plane --project PROJECT_UUID issue unarchive ENG-42 --yes
```

archive/unarchive 当前只支持 v2；unarchive 需要显式 project 及 UUID/完整 identifier，以避开已归档工作项的普通详情查询。404 仍报错，不直接认定「已经归档」。删除和归档操作必须指定 `--yes`。

## 本地工作流与诊断

```bash
workctl plane issue id ENG-42
workctl plane issue title ENG-42
workctl plane issue describe ENG-42
workctl plane issue start ENG-42 --branch fix/eng-42
workctl plane api GET /api/v2/users/me/
workctl plane api PATCH /api/v2/workspaces/acme/projects/PROJECT_UUID/work-items/ISSUE_UUID/ --input ./patch.json
```

`issue start` 要求干净 Git 工作区，选择已有分支或创建新分支，不强制重置；started group 有多个状态时需 `--state`。Git 成功而远端更新失败时明确报告已选分支，不自动回滚本地状态。`issue url` 仅当 API 原生响应提供 URL 时输出；不会根据 API 域名猜测 Web URL。

原始 `api` 只接受所选 `/api/v1/` 或 `/api/v2/` 下相对路径，不接受另一 origin 或跳转；请求方法限 GET/POST/PATCH/DELETE。它是显式原生接口，不代表某个高级功能已验证可用。

默认整体命令 deadline 为 2 分钟，`--timeout` 可调整；单次请求 timeout 为 30 秒。GET 最多尝试 3 次，遵守 Retry-After；写请求不自动重试或降级版本。stderr 错误不包含 PAT 或签名存储凭据。退出码：成功 0，请求/网络失败 1，参数/配置错误 2，取消 130；Linear 沿用旧行为。
