# 当前能力与验证状态

本实现为 Plane 预览，保留原 Linear 产品行为。没有收到真实 Plane 实例/PAT，以下「通过」指本地 `httptest` / 命令测试，不代表已认证任何部署。Cloud v2 文档和固定版本 CE v1 源码依据见 [调研](research/plane-api.md)。

| 能力 | v1 | v2 | 当前证据/限制 |
| --- | --- | --- | --- |
| PAT、profile、配置、keyring/显式明文、登出 | 已实现 | 已实现 | 配置冲突、存储隔离、无 keyring 明文登出、完整登录后操作测试 |
| 项目/状态/标签/成员列表，project/cycle/module/state/label 详情 | 已实现 | 已实现 | array/v1 cursor/v2 offset/cursor 解码；真实权限/edition 未验证 |
| 工作项创建/详情/更新，UUID/identifier/数字解析 | 已实现 | 已实现 | 两版本字段合约；三态更新、多负责人、Markdown、原生 JSON；CLI 端到端 fixture |
| 评论 list/add/update/delete | 已实现 | 已实现 | 保存凭据后发表评论及字段合约；真实回复/编辑权限未验证 |
| query/mine | 仅无过滤列表与保守排序；其他组合拒绝 | 服务端过滤已实现 | v2 filter 参数测试；v1 不静默忽略过滤 |
| project/state/label/cycle/module 管理 | 原生 JSON create/update/delete | 原生 JSON create/update/delete | 共用资源方法/路径；治理模式与 edition 交由服务报告 |
| cycle/module 增删工作项 | cycle-issues/module-issues 协议 | add/remove 协议 | 两版本关联方法与 payload 测试 |
| 工作项删除 | 已实现 | 已实现 | `--yes` 前置校验；真实删除权限未验证 |
| 工作项归档/恢复 | 显式拒绝，保留原生 api 入口 | 已实现 | 恢复直接请求 action，要求 project；404 不伪装成功 |
| 链接 list/add/update/delete | 已实现 | 已实现 | 原生子资源协议，HTTP/HTTPS 链接输入校验 |
| 附件 list/delete 与上传/确认 | 已实现 | 已实现 | 两版本三阶段 fixture；存储端未收到 Plane PAT；v1 204 后回读 |
| 摘要与 Git start | 已实现 | 已实现 | 脏工作区阻止；不强制重置；远端失败报告本地分支状态 |
| 工作项 Web URL | 仅输出 API 提供的原生 URL | 同左 | 当前响应无 URL 时明确报错，不猜 UI 路径 |
| 估算 | 只接受 estimate point UUID；无通用 collection list | UUID 输入及 estimate list | 尚未实现按数值查估算点 |
| 终端续页、单页 JSON、JSONL | 已实现 | 已实现 | 过滤条件保留、重复 cursor 防护、限制和分页风格校验 |
| OAuth、page/milestone/initiative/template、跨服务同步 | 未实现 | 未实现 | 后续计划范围 |

HTTP 行为已验证：v1/v2 路径前缀与 PAT header、POST 不自动重试、取消后结果未知、GET 遵守 Retry-After、跨 origin 重定向拒绝、原始路径拒绝跨版本/遍历。请求中只使用具体 DTO 或显式原生 JSON；Linear GraphQL client/schema 未改。

当前代码结构：`internal/plane` 为公开操作 module，私有 `v1Adapter/v2Adapter` 集中处理 payload 与分页格式；`cmdplane` 负责 CLI；`planeconfig/planeauth` 独立于 Linear；`workcli` 复用原 Linear 命令树。没有为两个版本制造不同的外部 CRUD 接口。

阶段状态：P1 已实现并通过本地测试；P2/P3/P4 已有上述功能代码，但真实目标验收尚未完成。P5 文档和双 binary CI 构建已加入，未发布发行版。PL-01 的分发命名核验、指定测试目标、Cloud/CE 真实 CRUD/关联/文件验收，以及后续估算查值、OAuth/扩展资源仍需完成。不能把本预览称为已稳定支持所有自建版本。

真实验收应使用专用可清理项目，记录实例 edition/version/base path、身份/项目访问、创建→回读→更新→评论→关联→上传→清理的状态码和资源 ID。不要把 PAT、签名 URL 或存储凭据写入记录。任何普通 CI 测试都不自动向外部账户写入。

本次本地验证（2026-10-29）：

| 检查 | 结果 |
| --- | --- |
| `go test ./... -count=1 -timeout 90s` | 全仓通过，包含原 Linear 测试 |
| `go vet ./...` | 通过 |
| 新增 Plane/config/auth/CLI module 的 `go test -race` | 通过 |
| `cmd/linear` 与 `cmd/workctl` 原生构建及帮助 | 通过 |
| Linux、macOS、Windows amd64（`CGO_ENABLED=0`）两个入口交叉编译 | 六个构建通过；不代表目标系统运行验收 |
| 17 条命令帮助、文档链接/锚点与 TOML 示例 | 通过 |
| 带凭据 Cloud/CE API、真实存储、目标平台 keyring | 未执行 |
