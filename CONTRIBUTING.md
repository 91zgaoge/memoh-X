# 贡献指南

感谢您对 Memoh-v2 项目的关注！我们欢迎任何形式的贡献，包括但不限于：

- 提交 Bug 报告
- 提交功能请求
- 提交代码修复或新功能
- 改进文档
- 分享使用经验

## 如何贡献

### 报告 Bug

如果您发现了 Bug，请通过 [GitHub Issues](https://github.com/91zgaoge/memoh-X/issues) 报告。报告时请包含以下信息：

1. **问题描述**：清晰简洁地描述 Bug
2. **复现步骤**：列出复现 Bug 的具体步骤
3. **预期行为**：描述您期望发生什么
4. **实际行为**：描述实际发生了什么
5. **环境信息**：操作系统、Docker 版本、浏览器等
6. **截图或日志**：如有相关截图或错误日志，请一并提供

### 请求新功能

如果您有新功能的想法，欢迎通过 [GitHub Issues](https://github.com/91zgaoge/memoh-X/issues) 提交。请包含：

1. **功能描述**：清晰描述您想要的功能
2. **使用场景**：描述这个功能在什么情况下有用
3. **可能的实现方案**：如果您有实现思路，欢迎分享

### 提交代码

1. **Fork 仓库**：点击右上角的 Fork 按钮创建您的 Fork
2. **克隆仓库**：
   ```bash
   git clone https://github.com/YOUR_USERNAME/memoh-X.git
   cd memoh-X
   ```
3. **创建分支**：
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. **进行开发**：编写您的代码
5. **提交更改**：
   ```bash
   git add .
   git commit -m "feat: 添加新功能描述"
   git push origin feature/your-feature-name
   ```
6. **创建 Pull Request**：在 GitHub 上创建 PR 到主仓库

## 代码规范

### Go 代码规范

- 使用 `gofmt` 格式化代码
- 遵循 [Effective Go](https://golang.org/doc/effective_go.html) 指南
- 函数和变量使用有意义的命名
- 导出函数和类型需要添加文档注释
- 错误处理要完整，不要忽略错误返回值

### TypeScript/Vue 代码规范

- 使用 ESLint 检查代码
- 组件名使用 PascalCase
- Props 和事件使用有意义的命名
- 复杂逻辑需要添加注释

### 提交信息规范

我们使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Type 类型：**

- `feat`: 新功能
- `fix`: 修复 Bug
- `docs`: 文档更新
- `style`: 代码格式修改（不影响功能的空格、分号等）
- `refactor`: 代码重构
- `perf`: 性能优化
- `test`: 测试相关
- `chore`: 构建过程或辅助工具的变动
- `ci`: CI/CD 相关修改

**示例：**

```
feat(memory): 添加记忆压缩功能

实现了基于 LLM 的记忆自动压缩机制，当记忆条目过多时自动合并冗余信息。

Closes #123
```

## 开发环境设置

### 前置要求

- Docker 和 Docker Compose
- Go 1.25+（如需本地开发后端）
- Node.js 20+（如需本地开发前端）
- Bun（如需开发 Agent Gateway）

### 启动开发环境

```bash
# 克隆仓库
git clone https://github.com/91zgaoge/memoh-X.git
cd memoh-X

# 启动服务
docker compose up -d

# 访问 Web UI
open http://localhost:8082
```

## 测试

在提交 PR 之前，请确保：

1. 代码可以正常编译
2. 相关的测试用例通过
3. 没有引入新的 lint 错误

运行测试：

```bash
# Go 测试
cd /data2/memoh-v2
go test ./...

# 前端测试
cd packages/web
pnpm test
```

## 代码审查

所有 Pull Request 都需要经过至少一名维护者的审查。审查过程中可能会要求您：

- 修改代码风格
- 添加或修改测试
- 更新文档
- 解释设计决策

请保持耐心，积极响应审查意见。

## 许可证

通过向本项目提交代码，您同意您的贡献将按照 [AGPL-3.0](LICENSE) 许可证发布。

## 获取帮助

如果您在贡献过程中遇到任何问题，可以通过以下方式获取帮助：

- 在 [GitHub Discussions](https://github.com/91zgaoge/memoh-X/discussions) 发起讨论
- 在 Issue 中 @ 相关维护者

---

再次感谢您的贡献！
