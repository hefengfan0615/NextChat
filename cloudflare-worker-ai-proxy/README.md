# Cloudflare Worker AI 代理 - 部署指南

## 功能特性

- ✅ **通用代理** - 代理任意 AI API 端点
- ✅ **多服务商支持** - OpenAI, Anthropic, Google Gemini, DeepSeek 等
- ✅ **流式响应** - 完整支持 SSE 流式输出
- ✅ **CORS 支持** - 跨域请求无障碍
- ✅ **自动认证** - 自动处理各种 API 认证方式
- ✅ **轻量高效** - Cloudflare 全球边缘网络

## 快速开始

### 1. 安装 Wrangler CLI

```bash
npm install -g wrangler
# 或
yarn global add wrangler
```

### 2. 登录 Cloudflare

```bash
wrangler login
```

### 3. 配置 API 地址

编辑 `index.js` 开头的 CONFIG 对象：

```javascript
const CONFIG = {
  // 选择一个主要服务商的 URL
  
  // OpenAI (需要代理)
  OPENAI_URL: "https://api.openai.com",  // 或你的代理地址
  OPENAI_API_KEY: "sk-xxxx",
  
  // Anthropic Claude (可选)
  ANTHROPIC_URL: "https://api.anthropic.com",
  ANTHROPIC_API_KEY: "sk-ant-xxxx",
  
  // Google Gemini (可选)
  GOOGLE_URL: "https://generativelanguage.googleapis.com",
  GOOGLE_API_KEY: "your-google-api-key",
  
  // DeepSeek (可选)
  DEEPSEEK_URL: "https://api.deepseek.com",
  DEEPSEEK_API_KEY: "sk-xxxx",
};
```

### 4. 部署

```bash
cd cloudflare-worker-ai-proxy
wrangler deploy
```

部署成功后会返回 Worker URL，例如：
```
https://ai-proxy-worker.your-subdomain.workers.dev
```

## 使用方法

### 在 ChatGPT Next Web 中配置

1. 打开应用设置
2. 找到 API 代理设置
3. 填写 Worker URL：
   - OpenAI: `https://ai-proxy-worker.your-subdomain.workers.dev/v1/openai`
   - Anthropic: `https://ai-proxy-worker.your-subdomain.workers.dev/v1/anthropic`

### 直接 API 调用

#### OpenAI 兼容格式
```bash
curl -X POST "https://your-worker.workers.dev/v1/openai/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

#### Anthropic 格式
```bash
curl -X POST "https://your-worker.workers.dev/v1/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-anthropic-key" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

#### 使用 x-base-url 请求头（通用代理）
```bash
curl -X POST "https://your-worker.workers.dev/proxy/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -H "x-base-url: https://api.openai.com" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## 高级配置

### 使用 Cloudflare 环境变量

在 `wrangler.toml` 中配置：

```toml
[vars]
OPENAI_URL = "https://api.openai.com"
```

或在 Cloudflare Dashboard 中设置环境变量。

### 使用 KV 存储配置

1. 创建 KV Namespace：
```bash
wrangler kv:namespace create "AI_PROXY_CONFIG"
```

2. 取消 `index.js` 中 KV 相关的注释

3. 更新 `wrangler.toml`：
```toml
[[kv_namespaces]]
binding = "AI_PROXY_CONFIG"
id = "your-kv-namespace-id"
```

### 多区域代理

创建多个 Worker，每个指向不同的代理：

```javascript
// Worker 名称: openai-proxy
const CONFIG = {
  BASE_URL: "https://api.openai.com",
  OPENAI_API_KEY: "sk-xxx",
};

// Worker 名称: anthropic-proxy
const CONFIG = {
  BASE_URL: "https://api.anthropic.com",
  ANTHROPIC_API_KEY: "sk-ant-xxx",
};
```

## 路由说明

| 路径 | 说明 | 示例 |
|-----|------|------|
| `/` 或 `/proxy` | 通用代理 | 配合 `x-base-url` header 使用 |
| `/v1/openai/*` | OpenAI 兼容端点 | `/v1/chat/completions` |
| `/v1/anthropic/*` | Anthropic 端点 | `/v1/messages` |
| `/v1/google/*` | Google 端点 | `/v1beta/models/...` |
| `/v1/deepseek/*` | DeepSeek 端点 | `/v1/chat/completions` |
| `/health` | 健康检查 | 返回状态信息 |

## 故障排查

### 请求超时
- 检查目标 API 是否可访问
- 尝试增加 `CONFIG.TIMEOUT_MS` 值

### CORS 错误
- 确保 `ALLOWED_ORIGINS` 包含你的应用域名
- 或设置为 `*` 允许所有来源

### 认证失败
- 检查 API Key 是否正确
- 确认目标 API 的认证方式（Bearer / x-api-key 等）

### 流式响应不工作
- 确保没有中间代理拦截 SSE
- 检查 `Content-Type` 是否为 `text/event-stream`

## 成本说明

- Cloudflare Workers **免费计划**：
  - 每日 100,000 请求
  - 每次请求 10ms CPU 时间（付费版可扩展）
- AI API 调用费用由各服务商收取

## License

MIT
