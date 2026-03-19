# Memoh 消息格式模板模块设计方案

## 背景与目标

### 需求分析
用户对 Bot 输出内容的格式需求是刚需，当前简单 Markdown 格式无法满足多样化需求：
- **办公场景**：精美日程表、Word 文档、Excel 表格、PPT 演示文稿
- **数据场景**：数据可视化图表、仪表盘、统计报告
- **展示场景**：HTML 页面、图片海报、PDF 文档
- **消息场景**：富文本卡片、结构化消息

### 目标
创建一个独立的**消息格式模板模块**（Message Format Template Module），实现：
1. **多样化格式**：支持 Markdown、HTML、Office、图表、图片等多种输出格式
2. **模板化管理**：预定义模板 + 用户自定义模板
3. **场景化匹配**：根据内容类型自动匹配最佳格式
4. **扩展性**：易于添加新的格式类型和模板

---

## 架构设计

### 整体架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        消息格式模板模块                                   │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │  模板管理器   │  │  格式渲染器   │  │  场景匹配器   │  │  输出转换器   │ │
│  │  Template    │  │  Renderer    │  │  Matcher     │  │  Converter   │ │
│  │  Manager     │  │              │  │              │  │              │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘ │
│         │                 │                 │                 │         │
│  ┌──────▼─────────────────▼─────────────────▼─────────────────▼───────┐ │
│  │                      输出格式引擎                                   │ │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐     │ │
│  │  │Markdown │ │  HTML   │ │ Office  │ │  Chart  │ │  Image  │     │ │
│  │  │Renderer │ │Renderer │ │Renderer │ │Renderer │ │Renderer │     │ │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘     │ │
│  └──────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
              ┌─────────┐     ┌─────────┐     ┌─────────┐
              │  企业微信 │     │ Telegram│     │  其他渠道 │
              └─────────┘     └─────────┘     └─────────┘
```

### 核心组件

#### 1. 模板管理器 (Template Manager)
- 模板 CRUD 操作
- 模板分类和标签
- 模板版本管理
- 权限控制（系统模板 vs 用户模板）

#### 2. 格式渲染器 (Format Renderer)
- 模板变量替换（Go template / Handlebars 语法）
- 条件渲染
- 循环渲染
- 嵌套模板支持

#### 3. 场景匹配器 (Scene Matcher)
- 根据内容类型自动选择模板
- 支持规则配置（关键词、数据类型、用户意图等）
- 手动指定模板

#### 4. 输出转换器 (Output Converter)
- Markdown → HTML
- Markdown → Word/Excel/PPT
- 数据 → 图表（ECharts、Chart.js）
- HTML → 图片（Puppeteer、Playwright）

---

## 数据模型设计

### 模板表 (message_templates)

```sql
CREATE TABLE message_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- 基础信息
    name VARCHAR(100) NOT NULL,                    -- 模板名称（如：精美日报）
    code VARCHAR(50) UNIQUE NOT NULL,              -- 模板编码（如：daily-report-premium）
    description TEXT,                              -- 模板描述

    -- 分类和标签
    category VARCHAR(50) NOT NULL,                 -- 分类：daily, report, chart, document
    format_type VARCHAR(50) NOT NULL,              -- 格式类型：markdown, html, office, chart, image
    tags TEXT[],                                   -- 标签：['日报', '办公', '精美']

    -- 模板内容
    content TEXT NOT NULL,                         -- 模板主体内容（使用 Go template 语法）
    style_config JSONB,                            -- 样式配置（颜色、字体、布局等）
    variables JSONB,                               -- 变量定义：[{"name": "date", "type": "string", "required": true}]

    -- 场景匹配规则
    match_rules JSONB,                             -- 自动匹配规则

    -- 模板类型
    is_system BOOLEAN DEFAULT false,               -- 是否系统模板
    owner_user_id UUID REFERENCES users(id),       -- 创建者（系统模板为NULL）

    -- 状态
    is_active BOOLEAN DEFAULT true,
    version INTEGER DEFAULT 1,

    -- 时间戳
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引
CREATE INDEX idx_message_templates_category ON message_templates(category);
CREATE INDEX idx_message_templates_format ON message_templates(format_type);
CREATE INDEX idx_message_templates_owner ON message_templates(owner_user_id);
CREATE INDEX idx_message_templates_active ON message_templates(is_active) WHERE is_active = true;
```

### Bot 模板关联表 (bot_message_templates)

```sql
CREATE TABLE bot_message_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES message_templates(id) ON DELETE CASCADE,

    -- 使用配置
    is_default BOOLEAN DEFAULT false,              -- 是否默认模板
    auto_match BOOLEAN DEFAULT true,               -- 是否自动匹配
    scene_rules JSONB,                             -- 场景规则（覆盖模板默认规则）

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(bot_id, template_id)
);
```

### 模板渲染日志表 (message_template_renders)

```sql
CREATE TABLE message_template_renders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID REFERENCES message_templates(id),
    bot_id UUID REFERENCES bots(id),

    -- 输入数据
    input_data JSONB,                              -- 输入的原始数据
    variables JSONB,                               -- 使用的变量值

    -- 输出结果
    output_format VARCHAR(50),                     -- 实际输出格式
    output_content TEXT,                           -- 渲染后的内容
    output_file_path TEXT,                         -- 输出文件路径（如有）

    -- 状态
    status VARCHAR(20) DEFAULT 'success',          -- success, failed
    error_message TEXT,

    -- 性能
    render_duration_ms INTEGER,                    -- 渲染耗时

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## 模板内容设计

### 1. Markdown 模板示例

**文件**: `templates/markdown/daily-report-premium.md`

```markdown
<div align="center">

# 📅 {{.Date}} 工作日报
### {{.UserName}} · {{.Department}}

</div>

---

## ✅ 今日完成

{{range .CompletedTasks}}
| 任务 | 状态 | 产出 | 进度 |
|------|------|------|------|
| {{.Name}} | {{.StatusIcon}} | {{.Output}} | {{.ProgressBar}} |
{{end}}

**完成率**: {{.CompletionRate}}% {{.CompletionBadge}}

## 📊 关键数据

| 指标 | 今日 | 目标 | 状态 |
|------|------|------|------|
{{range .Metrics}}
| {{.Name}} | {{.Today}} | {{.Target}} | {{.StatusIcon}} |
{{end}}

## 📋 明日计划

{{range .TomorrowPlans}}
### {{.PriorityIcon}} {{.Name}}
- ⏰ 截止时间: {{.Deadline}}
- 📝 备注: {{.Notes}}
{{end}}

## 💡 关键洞察

{{range .Insights}}
> **{{.Type}}**: {{.Content}}
{{end}}

---
<div align="right">

🤖 由 {{.BotName}} 自动生成 · {{.GenerateTime}}

</div>
```

### 2. HTML 模板示例

**文件**: `templates/html/news-digest.html`

```html
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        {{.StyleConfig.CSS}}
    </style>
</head>
<body>
    <div class="news-container">
        <header class="news-header">
            <h1>📰 {{.Title}}</h1>
            <p class="subtitle">{{.Subtitle}} · {{.Date}}</p>
        </header>

        <main class="news-content">
            {{range .Articles}}
            <article class="news-item">
                <div class="news-image" style="background-image: url('{{.ImageUrl}}')"></div>
                <div class="news-body">
                    <span class="news-tag">{{.Category}}</span>
                    <h2>{{.Title}}</h2>
                    <p class="news-summary">{{.Summary}}</p>
                    <a href="{{.Url}}" class="read-more">阅读全文 →</a>
                </div>
            </article>
            {{end}}
        </main>

        <footer class="news-footer">
            <p>{{.FooterText}}</p>
        </footer>
    </div>
</body>
</html>
```

### 3. Office 模板示例

**文件**: `templates/office/weekly-report.docx` (模板文件)

使用占位符标记：
- `{{Date}}` - 日期
- `{{UserName}}` - 用户名
- `{{Table:CompletedTasks}}` - 表格区域

### 4. 图表模板示例

**文件**: `templates/chart/sales-dashboard.json`

```json
{
  "type": "echarts",
  "option": {
    "title": {
      "text": "{{.Title}}",
      "subtext": "{{.Subtitle}}"
    },
    "tooltip": {
      "trigger": "axis"
    },
    "xAxis": {
      "type": "category",
      "data": {{.XAxisData | json}}
    },
    "yAxis": {
      "type": "value"
    },
    "series": [{
      "data": {{.SeriesData | json}},
      "type": "line",
      "smooth": true,
      "areaStyle": {{.AreaStyle | json}}
    }]
  }
}
```

---

## API 设计

### 模板管理 API

```go
// 模板管理 Handler
// POST   /message-templates                    创建模板
// GET    /message-templates                    列出模板
// GET    /message-templates/:id                获取模板详情
// PUT    /message-templates/:id                更新模板
// DELETE /message-templates/:id                删除模板
// POST   /message-templates/:id/render         渲染模板
// GET    /message-templates/categories         获取模板分类
// GET    /message-templates/formats            获取支持的格式类型
```

### Bot 模板关联 API

```go
// Bot 模板配置 Handler
// GET    /bots/:bot_id/message-templates               获取 Bot 的模板列表
// POST   /bots/:bot_id/message-templates               为 Bot 添加模板
// PUT    /bots/:bot_id/message-templates/:template_id  更新 Bot 模板配置
// DELETE /bots/:bot_id/message-templates/:template_id  移除 Bot 模板
```

### 核心数据结构

```go
// message_template.go

type MessageTemplate struct {
    ID          string                 `json:"id"`
    Name        string                 `json:"name"`
    Code        string                 `json:"code"`
    Description string                 `json:"description"`
    Category    TemplateCategory       `json:"category"`
    FormatType  OutputFormat           `json:"format_type"`
    Tags        []string               `json:"tags"`
    Content     string                 `json:"content"`
    StyleConfig StyleConfig            `json:"style_config"`
    Variables   []TemplateVariable     `json:"variables"`
    MatchRules  MatchRules             `json:"match_rules"`
    IsSystem    bool                   `json:"is_system"`
    OwnerUserID *string                `json:"owner_user_id,omitempty"`
    IsActive    bool                   `json:"is_active"`
    Version     int                    `json:"version"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}

type TemplateCategory string
const (
    CategoryDaily     TemplateCategory = "daily"      // 日报/周报
    CategoryReport    TemplateCategory = "report"     // 报告
    CategoryChart     TemplateCategory = "chart"      // 图表
    CategoryDocument  TemplateCategory = "document"   // 文档
    CategoryNews      TemplateCategory = "news"       // 资讯
    CategoryCard      TemplateCategory = "card"       // 卡片
    CategoryCustom    TemplateCategory = "custom"     // 自定义
)

type OutputFormat string
const (
    FormatMarkdown  OutputFormat = "markdown"   // Markdown 文本
    FormatHTML      OutputFormat = "html"       // HTML
    FormatWord      OutputFormat = "word"       // Word 文档
    FormatExcel     OutputFormat = "excel"      // Excel 表格
    FormatPPT       OutputFormat = "ppt"        // PPT 演示文稿
    FormatPDF       OutputFormat = "pdf"        // PDF 文档
    FormatChart     OutputFormat = "chart"      // 数据图表
    FormatImage     OutputFormat = "image"      // 图片
    FormatJSON      OutputFormat = "json"       // JSON 数据
)

type TemplateVariable struct {
    Name        string `json:"name"`
    Type        string `json:"type"`        // string, number, boolean, array, object
    Required    bool   `json:"required"`
    Description string `json:"description"`
    Default     any    `json:"default,omitempty"`
}

type MatchRules struct {
    Keywords    []string `json:"keywords,omitempty"`     // 关键词匹配
    DataTypes   []string `json:"data_types,omitempty"`   // 数据类型匹配
    IntentTypes []string `json:"intent_types,omitempty"` // 意图类型匹配
    Conditions  []Condition `json:"conditions,omitempty"` // 复杂条件
}

type StyleConfig struct {
    Theme       string            `json:"theme,omitempty"`       // 主题：default, dark, colorful
    PrimaryColor string           `json:"primary_color,omitempty"`
    FontFamily  string            `json:"font_family,omitempty"`
    CSS         string            `json:"css,omitempty"`         // 自定义 CSS
    EChartsTheme string          `json:"echarts_theme,omitempty"`
}
```

---

## 渲染引擎实现

### 渲染流程

```go
// 渲染请求
 type RenderRequest struct {
     TemplateID   string                 `json:"template_id"`   // 模板ID
     TemplateCode string                 `json:"template_code"` // 或模板编码
     Data         map[string]interface{} `json:"data"`          // 数据
     Variables    map[string]interface{} `json:"variables"`     // 变量（覆盖数据中的值）
     OutputFormat OutputFormat           `json:"output_format"` // 目标输出格式（可选，默认使用模板格式）
 }

 // 渲染响应
 type RenderResponse struct {
     Content     string `json:"content"`      // 渲染后的内容
     Format      string `json:"format"`       // 实际输出格式
     FileURL     string `json:"file_url,omitempty"` // 如果是文件输出，返回文件URL
     FilePath    string `json:"file_path,omitempty"` // 文件路径
     RenderTime  int    `json:"render_time_ms"` // 渲染耗时
 }
```

### 渲染器接口

```go
// Renderer 渲染器接口
type Renderer interface {
    // 支持的格式
    SupportedFormats() []OutputFormat

    // 渲染
    Render(template *MessageTemplate, data map[string]interface{}, options RenderOptions) (*RenderResult, error)

    // 验证模板
    Validate(template *MessageTemplate) error
}

// MarkdownRenderer Markdown 渲染器
type MarkdownRenderer struct{}

// HTMLRenderer HTML 渲染器
type HTMLRenderer struct{}

// OfficeRenderer Office 文档渲染器（使用 unioffice/unioffice 库）
type OfficeRenderer struct{}

// ChartRenderer 图表渲染器（生成 ECharts/Chart.js 配置或图片）
type ChartRenderer struct{}

// ImageRenderer 图片渲染器（使用 Puppeteer/Playwright 截图 HTML）
type ImageRenderer struct{}
```

---

## 实现优先级

用户确认：先做 Phase 1 和 Phase 2（核心功能），高级格式（Office/Chart/Image）后续逐步添加。

## 实现任务清单

### Phase 1: 基础架构（核心功能）- 优先实现

#### Task 1.1: 数据库迁移
- 创建 `message_templates` 表
- 创建 `bot_message_templates` 表
- 创建 `message_template_renders` 表

#### Task 1.2: 核心数据结构
- 定义模板相关的类型和常量
- 定义渲染器接口

#### Task 1.3: Markdown 渲染器
- 实现 Markdown 渲染器
- 支持 Go template 语法
- 支持内置函数（json, date, progressBar 等）

#### Task 1.4: 模板管理 API
- 实现模板 CRUD API
- 实现模板渲染 API

### Phase 2: Bot 集成

#### Task 2.1: Bot 模板关联
- 实现 Bot 模板关联 API
- 在 Bot 配置中添加模板选择

#### Task 2.2: 自动场景匹配
- 实现场景匹配器
- 支持关键词、数据类型、意图识别匹配

#### Task 2.3: 对话中应用模板
- 修改对话流程，在适当时机应用模板
- 支持技能触发模板渲染

### Phase 3: 高级格式支持

#### Task 3.1: HTML 渲染器
- 实现 HTML 渲染器
- 支持 CSS 样式注入

#### Task 3.2: Office 渲染器
- 集成 unioffice 库
- 实现 Word/Excel/PPT 渲染

#### Task 3.3: 图表渲染器
- 集成 ECharts/Chart.js
- 支持生成图表配置和图片

#### Task 3.4: 图片渲染器
- 集成 Playwright/Puppeteer
- 支持 HTML 转图片

### Phase 4: 前端界面

#### Task 4.1: 模板管理界面
- 模板列表页面
- 模板编辑器（支持变量定义、预览）
- 模板分类浏览

#### Task 4.2: Bot 模板配置界面
- Bot 详情中添加模板配置标签页
- 模板选择和场景规则配置

#### Task 4.3: 模板市场（可选）
- 系统预置模板展示
- 用户分享模板

---

## 关键文件路径

### 后端
```
/data2/memoh-v2/internal/message_templates/
├── models.go              # 数据模型
├── service.go             # 业务逻辑
├── handler.go             # HTTP API
├── renderer/
│   ├── interface.go       # 渲染器接口
│   ├── markdown.go        # Markdown 渲染器
│   ├── html.go            # HTML 渲染器
│   ├── office.go          # Office 渲染器
│   ├── chart.go           # 图表渲染器
│   └── image.go           # 图片渲染器
├── matcher.go             # 场景匹配器
└── builtin/               # 内置模板
    ├── daily/
    ├── report/
    ├── chart/
    └── ...
```

### 前端
```
/data2/memoh-v2/packages/web/src/pages/
├── message-templates/         # 模板管理页面
│   ├── index.vue
│   ├── editor.vue
│   └── preview.vue
└── bots/
    └── templates.vue          # Bot 模板配置页面
```

### 数据库迁移
```
/data2/memoh-v2/db/migrations/00XX_message_templates.up.sql
/data2/memoh-v2/db/migrations/00XX_message_templates.down.sql
```

---

## 预置模板清单

### Markdown 模板
1. `daily-report-simple` - 简洁日报
2. `daily-report-premium` - 精美日报
3. `weekly-report` - 周报
4. `news-digest` - 资讯简报
5. `data-table` - 数据表格
6. `checklist` - 待办清单
7. `meeting-notes` - 会议纪要

### HTML 模板
1. `newsletter` - 电子邮件简报
2. `dashboard` - 数据仪表盘
3. `portfolio` - 作品展示

### Office 模板
1. `weekly-report.docx` - Word 周报
2. `project-tracker.xlsx` - Excel 项目追踪表
3. `quarterly-review.pptx` - PPT 季度汇报

### 图表模板
1. `line-chart` - 折线图
2. `bar-chart` - 柱状图
3. `pie-chart` - 饼图
4. `dashboard-chart` - 组合仪表盘

---

## 技术选型

### 依赖库

```go
// Go 模板引擎（已内置）
import "text/template"

// Office 文档生成
github.com/unidoc/unioffice v1.36.0

// HTML 转 PDF
github.com/chromedp/chromedp v0.12.0

// 图表生成（服务端渲染）
github.com/wcharczuk/go-chart v2.1.0

// Markdown 扩展语法
github.com/yuin/goldmark v1.7.8
```

### TypeScript/前端

```json
{
  "echarts": "^5.5.0",
  "monaco-editor": "^0.52.0",
  "handlebars": "^4.7.8"
}
```

---

## 验证方式

1. **模板管理**:
   ```bash
   curl -X POST http://localhost:8080/message-templates \
     -H "Content-Type: application/json" \
     -d '{"name":"测试模板","code":"test","format_type":"markdown","content":"# {{.Title}}"}'
   ```

2. **模板渲染**:
   ```bash
   curl -X POST http://localhost:8080/message-templates/:id/render \
     -H "Content-Type: application/json" \
     -d '{"data":{"Title":"Hello"}}'
   ```

3. **端到端测试**:
   - 创建模板 → 关联 Bot → 触发对话 → 验证输出格式

---

## 扩展性考虑

1. **新格式支持**：只需实现 Renderer 接口即可添加新格式
2. **第三方模板**：支持导入外部模板（如 Office 模板文件上传）
3. **AI 辅助生成**：未来可集成 AI 根据内容自动生成模板
4. **模板版本控制**：支持模板版本管理和回滚

---

*计划文件保存于*: `/data2/memoh-v2/docs/plan-message-templates.md`
*创建日期*: 2026-03-19
