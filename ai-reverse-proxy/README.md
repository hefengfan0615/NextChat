# AI Reverse Proxy

基于Go语言的高性能AI服务反向代理，支持智能路由、负载均衡和健康检查。

## 功能特性

- **智能路由**: 根据用户地区自动选择最优AI服务提供商
- **负载均衡**: 支持轮询、最小延迟、加权等多种负载均衡策略
- **健康检查**: 实时监控AI服务商状态，自动故障转移
- **安全代理**: API Key自动注入，保护用户隐私
- **高性能**: 基于Go语言标准库实现，支持高并发
- **易于配置**: YAML配置文件，灵活可扩展

## 支持的AI服务商

- OpenAI (GPT-4, GPT-3.5)
- Anthropic (Claude)
- DeepSeek
- Google Gemini
- Moonshot AI
- Alibaba Qwen
- ByteDance Doubao

## 快速开始

### 1. 编译程序

```bash
cd ai-reverse-proxy
go mod download
go build -o ai-proxy main.go
```

### 2. 配置环境变量

```bash
export OPENAI_API_KEY="your-openai-api-key"
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export DEEPSEEK_API_KEY="your-deepseek-api-key"
export GOOGLE_API_KEY="your-google-api-key"
export MOONSHOT_API_KEY="your-moonshot-api-key"
export DASHSCOPE_API_KEY="your-dashscope-api-key"
export DOUBAO_API_KEY="your-doubao-api-key"
```

### 3. 启动代理

```bash
./ai-proxy -config config.yaml
```

### 4. 使用代理

使用代理发送请求：

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dummy-key" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## 配置说明

### 配置文件结构

```yaml
proxy:
  port: "8080"           # 监听端口
  host: "0.0.0.0"        # 监听地址
  read_timeout: 30       # 读取超时(秒)
  write_timeout: 60      # 写入超时(秒)
  
  providers:
    provider_name:
      name: "显示名称"
      base_url: "API基础URL"
      api_key_env: "环境变量名"
      timeout: 30        # 超时时间(秒)
      weight: 10         # 权重
      enabled: true      # 是否启用
      regions:           # 支持的地区
        - "CN"          # 中国大陆
        - "INTL"        # 国际
      capabilities:     # 支持的能力
        - "chat"
        - "completions"
  
  rules:
    - path_prefix: "/v1/chat/completions"  # 路径前缀
      provider: "deepseek"                  # 提供商名称
      load_balance: "weighted"              # 负载均衡策略
      regions: ["CN", "*"]                  # 匹配的地区
      priority: 10                         # 优先级

health:
  enabled: true
  check_interval: 30      # 检查间隔(秒)
  timeout: 5             # 超时时间(秒)
  max_retries: 3         # 最大重试次数

log:
  level: "info"          # 日志级别
  format: "json"         # 日志格式
  output_path: "logs/proxy.log"
```

### 路由规则

规则按照优先级(priority)匹配，优先级高的规则优先匹配。

负载均衡策略：
- `roundrobin`: 轮询
- `leastlatency`: 最小延迟
- `weighted`: 加权轮询

## API端点

- `GET /` - 首页
- `POST /v1/chat/completions` - ChatGPT兼容接口
- `POST /v1/completions` - Completions接口
- `GET /v1/models` - 模型列表
- `GET /health` - 健康检查
- `GET /healthz` - Kubernetes健康检查
- `GET /health/ready` - 就绪检查
- `GET /health/live` - 存活检查
- `POST /reload` - 重载配置

## Docker部署

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o ai-proxy main.go

FROM alpine
WORKDIR /app
COPY --from=builder /app/ai-proxy .
COPY config.yaml.example config.yaml
CMD ["./ai-proxy"]
```

构建和运行：

```bash
docker build -t ai-proxy .
docker run -p 8080:8080 \
  -e OPENAI_API_KEY="your-key" \
  -e ANTHROPIC_API_KEY="your-key" \
  ai-proxy
```

## 工作原理

### 反向代理原理

1. **客户端请求**: 客户端向代理服务器发送请求
2. **智能路由**: 代理根据请求路径、客户端地区等因素选择目标提供商
3. **请求转发**: 代理将请求转发到选定的AI服务商
4. **响应返回**: AI服务商的响应通过代理返回给客户端

### 智能路由

代理通过以下方式识别客户端地区：
- X-Forwarded-For 头
- X-Real-IP 头
- CF-Connecting-IP 头
- 客户端IP地址

国内IP(10.x.x.x, 172.16-31.x.x, 192.168.x.x)标记为CN区域。

### 负载均衡

当有多个提供商时，代理根据配置的负载均衡策略选择提供商：
- **轮询**: 依次选择每个提供商
- **最小延迟**: 选择响应延迟最低的提供商
- **加权轮询**: 根据权重比例选择提供商

### 健康检查

代理定期检查各提供商的状态：
- 检查间隔: 默认30秒
- 超时时间: 默认5秒
- 失败阈值: 连续3次失败标记为不健康

## 性能优化

- 连接池复用
- 高并发支持
- 请求体复用
- 异步健康检查
- 日志异步写入

## 安全建议

1. 使用HTTPS
2. 配置API Key环境变量
3. 限制访问IP
4. 定期更新依赖
5. 监控日志异常

## 故障排除

### 连接超时

检查：
- 网络连接
- API Key是否正确
- 防火墙设置

### 路由失败

检查：
- 配置文件是否正确
- 规则是否匹配请求路径
- 提供商是否启用

### 健康检查失败

检查：
- 提供商服务是否可用
- API Key是否有权限
- 网络是否正常

## License

MIT License
