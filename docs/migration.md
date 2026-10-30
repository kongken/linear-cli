# 升级与回退

旧脚本继续使用 `linear ...`，无需更改参数、JSON、环境变量或凭据文件。若希望统一使用新入口，仅增加服务名称：

```bash
# 原脚本
linear --workspace acme issue view ENG-42 --json

# 同一实现的新入口
workctl linear --workspace acme issue view ENG-42 --json
```

`LINEAR_API_KEY` 与 Linear `--workspace` 的原冲突规则仍保留。不会因安装新 binary 而移动、重写或复制旧 credentials/keyring。Plane 的 profile 只影响 `workctl plane`；没有默认 provider 或 `workctl issue` 自动路由。

Plane 使用 [独立配置](configuration.md)。不要把 Linear team/project UUID 直接复制为 Plane project/module，也不要将 `.linear.toml` 自动改成 Plane 配置。本版本不进行跨服务数据迁移。

Plane v1→v2：新建一个明确使用 v2 的 profile，先验证身份/目录，再在测试项目确认工作项与评论。旧 profile 可保留；不会自动转换原始 JSON 文件、重放失败写请求或降级到 v1。原生 JSON 格式随 API version 不同，跨版本脚本应显式处理。

回退时继续运行先前 `linear` binary，旧配置仍在原位置。Plane 用户配置保留 schema_version=1；不认识该 schema 的版本应明确失败。远端已创建/修改的数据不会因 binary 回退而自动回滚；多步骤失败按错误中的资源 ID 显式恢复/清理。

当前没有宣布旧 `linear` 弃用或移除日期，没有发布稳定 Plane 支持。后续发布必须完成目标 Cloud/CE 实测；详见 [兼容与验证](compatibility.md)。
