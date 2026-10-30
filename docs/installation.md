# 安装与构建

仓库暂保留 `github.com/kongken/linear-cli`。两个入口来自同一版本源码：

```bash
go build -o linear ./cmd/linear
go build -o workctl ./cmd/workctl
./workctl --help
./workctl linear --help
./workctl plane --help
```

现有 `go install github.com/kongken/linear-cli/cmd/linear@latest` 不变。当前 Plane 预览需从包含本实现的 checkout 构建；尚未发布携带新 binary 的发行版。发布后可安装同一明确版本的两个 `cmd/` 路径，不要求重命名 Go module。

不要让安装步骤静默覆盖另一工具的同名 binary。先检查 PATH 中已有 `workctl` / `linear` 的来源；试用时可直接运行构建产物。Windows 输出名可用 `workctl.exe` / `linear.exe`。

CI 对现有代码执行 test/vet，同时构建两个入口，并交叉编译 Linux/macOS/Windows amd64；这不等于已验证这些平台的系统 keyring 运行行为。实际能力及验证状态见 [兼容矩阵](compatibility.md)。
