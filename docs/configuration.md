# 配置与凭据

Linear 的 `LINEAR_*`、当前目录 `linear.toml` / `.linear.toml`、原 credentials 路径和 keyring service 不变。Plane 读取另一套配置；`workctl plane` 不初始化 Linear 凭据。

Plane 用户配置：Unix 为 `$XDG_CONFIG_HOME/workctl/config.toml`，默认 `~/.config/workctl/config.toml`；Windows 为 `%APPDATA%/workctl/config.toml`。当前目录可有 `workctl.toml`，不存在时读取 `.workctl.toml`，不向父目录搜索。

用户配置示例（login 会创建，不需手写 token）：

```toml
schema_version = 1

[plane]
default_profile = "cloud"

[plane.profiles.cloud]
base_url = "https://api.plane.so"
workspace = "my-workspace"
api_version = "v2"
project_id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
```

项目配置仅允许 profile、workspace、project_id；可省略 schema_version（提供时必须为 1）：

```toml
schema_version = 1

[plane]
profile = "cloud"
project_id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
```

## 解析规则

- profile：`--profile` > `PLANE_PROFILE` > 项目 profile > 用户 default_profile。
- `--no-profile` 禁用所有 profile 来源，与 `--profile` 互斥；仅使用显式上下文和环境 token。
- 已选 profile 的实例、workspace、version 固定；同名 flag、环境变量或项目 workspace 不同则失败。换实例/version/workspace 应新建 profile。
- project：`--project` > `PLANE_PROJECT_ID` > 项目配置 > profile 默认值。
- 无 profile：对应 flag > 环境变量；base URL 默认 Cloud，Cloud version 默认 v2；workspace 必填，自建 version 必须显式提供。
- token：login 的 `--token-stdin` 与 `PLANE_API_KEY` 互斥；普通调用优先 `PLANE_API_KEY`，否则使用所选 profile 的存储凭据。无 profile 不借用 keyring token。

支持 `PLANE_PROFILE/BASE_URL/WORKSPACE/API_VERSION/PROJECT_ID/API_KEY`。项目文件不允许 token、base_url、api_version；坏 TOML、未知字段、未知 schema_version 都报错，不静默回落到默认实例。`config view` 显示最终上下文、各字段来源和 token 来源，不显示 token 内容。

`auth login --profile <new-name>` 可创建新 profile，先验证当前用户和 workspace 项目访问，再保存。已有 profile 可以更新 token，不能改变身份上下文。首个 profile 自动成为默认；用 `profile use` 修改默认。

## 存储与隔离

系统 keyring service 为 `workctl-plane`，identity 由规范化实例 base URL（含路径前缀）、workspace、profile 名组成。不会复用 Linear keyring entry，也不会因 profile 同名而跨实例取 token。

credentials.toml 与用户 config 在同目录，使用私有临时文件原子替换，Unix mode 为 0600。默认只记录存储位置和登出状态；只有明确选择 `auth login --plaintext` 才存 PAT。明文不会进入项目配置。

切换存储方式后，登出状态阻止旧副本自动恢复认证；已知 keyring 副本也会清理。若 keyring 暂不可用，登出报告残留副本的清理失败，可在 keyring 恢复后重试。登出不会撤销服务器 PAT，环境变量仍优先。

HTTP 开发实例必须显式 `--allow-http`（保存到 profile）；base URL 不允许 userinfo/query/fragment，保留部署前缀。API 请求不跟随重定向，原始 REST path 不能跨版本/实例；存储上传使用独立的无 PAT 请求。
