/**
 * Cloudflare Worker - AI API 通用代理
 * 
 * 功能：
 * - 代理任意 AI API 端点（OpenAI, Anthropic, Google, DeepSeek 等）
 * - 支持流式响应 (SSE)
 * - 自动处理认证头
 * - CORS 跨域支持
 * 
 * 使用方法：
 * 1. 部署到 Cloudflare Workers
 * 2. 在前端配置此 Worker 的 URL 作为 API 代理地址
 * 
 * Cloudflare Workers 免费计划每日 100,000 请求，100ms CPU 时间
 */

// ============== 可配置区域 ==============
// 用户需要修改这些配置指向想要代理的 API 地址

const CONFIG = {
  // 主 API 基础 URL（留空则使用请求头中的 x-base-url）
  BASE_URL: "",
  
  // OpenAI
  OPENAI_URL: "",
  OPENAI_API_KEY: "",
  
  // Anthropic (Claude)
  ANTHROPIC_URL: "",
  ANTHROPIC_API_KEY: "",
  
  // Google Gemini
  GOOGLE_URL: "",
  GOOGLE_API_KEY: "",
  
  // DeepSeek
  DEEPSEEK_URL: "",
  DEEPSEEK_API_KEY: "",
  
  // Azure OpenAI
  AZURE_URL: "",
  AZURE_API_KEY: "",
  
  // 其他自定义端点
  CUSTOM_URL: "",
  CUSTOM_API_KEY: "",
  
  // 是否启用认证检查
  REQUIRE_AUTH: false,
  ALLOWED_API_KEYS: [], // 允许的客户端 API Keys
  
  // CORS 配置
  ALLOWED_ORIGINS: "*", // 或指定域名如: "https://your-app.workers.dev"
  
  // 请求超时（毫秒）
  TIMEOUT_MS: 60000,
};

// ============== 常量定义 ==============
const OPENAI_BASE_URL = "https://api.openai.com";
const ANTHROPIC_BASE_URL = "https://api.anthropic.com";
const GOOGLE_BASE_URL = "https://generativelanguage.googleapis.com";
const DEEPSEEK_BASE_URL = "https://api.deepseek.com";

const SKIP_HEADERS = [
  "host",
  "connection",
  "content-length",
  "content-encoding",
  "transfer-encoding",
  "via",
  "vvia",
  "proxy-connection",
  "upgrade",
];

const AUTH_HEADERS = [
  "authorization",
  "x-api-key",
  "x-goog-api-key",
  "api-key",
];

// ============== 工具函数 ==============
function getTargetUrl(url, baseUrl) {
  if (baseUrl) {
    const target = baseUrl.endsWith("/") ? baseUrl.slice(0, -1) : baseUrl;
    const path = url.pathname.startsWith("/") ? url.pathname : "/" + url.pathname;
    return target + path + (url.search || "");
  }
  return url.href;
}

function getApiKeyForProvider(provider, headers, baseUrl) {
  // 1. 检查自定义端点
  if (CONFIG.CUSTOM_API_KEY && baseUrl?.includes(CONFIG.CUSTOM_URL)) {
    return CONFIG.CUSTOM_API_KEY;
  }
  
  // 2. 检查请求头中的 API Key
  const authHeader = headers.get("authorization");
  if (authHeader) {
    return authHeader.replace(/^Bearer\s+/i, "").trim();
  }
  
  for (const header of AUTH_HEADERS) {
    const value = headers.get(header);
    if (value) return value;
  }
  
  // 3. 根据提供商返回配置的 Key
  const url = baseUrl?.toLowerCase() || "";
  
  if (url.includes("anthropic") || url.includes("api.anthropic")) {
    return CONFIG.ANTHROPIC_API_KEY || "";
  }
  if (url.includes("google") || url.includes("generativelanguage")) {
    return CONFIG.GOOGLE_API_KEY || "";
  }
  if (url.includes("deepseek")) {
    return CONFIG.DEEPSEEK_API_KEY || "";
  }
  if (url.includes("azure") || url.includes("openai.azure")) {
    return CONFIG.AZURE_API_KEY || "";
  }
  if (url.includes("openai") || url.includes("api.openai")) {
    return CONFIG.OPENAI_API_KEY || "";
  }
  
  return "";
}

function filterHeaders(headers, skipList = []) {
  const filtered = new Headers();
  for (const [key, value] of headers.entries()) {
    const lowerKey = key.toLowerCase();
    if (!SKIP_HEADERS.includes(lowerKey) && !skipList.includes(lowerKey)) {
      filtered.set(key, value);
    }
  }
  return filtered;
}

function createCORSHeaders(origin = "*") {
  return {
    "Access-Control-Allow-Origin": origin,
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type, Authorization, x-api-key, anthropic-version, x-goog-api-key, openai-organization",
    "Access-Control-Max-Age": "86400",
  };
}

function jsonResponse(data, status = 200, cors = true) {
  const headers = {
    "Content-Type": "application/json",
    ...(cors ? createCORSHeaders() : {}),
  };
  return new Response(JSON.stringify(data), { status, headers });
}

function errorResponse(message, status = 400) {
  return jsonResponse({ error: { message, type: "invalid_request" } }, status);
}

// ============== 代理处理器 ==============
async function handleProxyRequest(request) {
  const url = new URL(request.url);
  
  // 获取目标基础 URL
  let baseUrl = CONFIG.BASE_URL;
  
  // 优先使用请求头中的 x-base-url
  const headerBaseUrl = request.headers.get("x-base-url");
  if (headerBaseUrl) {
    baseUrl = headerBaseUrl;
  }
  
  // 如果都没配置，返回错误
  if (!baseUrl) {
    return errorResponse("No BASE_URL configured and x-base-url header not provided", 500);
  }
  
  // 确保 baseUrl 格式正确
  if (!baseUrl.startsWith("http")) {
    baseUrl = "https://" + baseUrl;
  }
  
  // 构建目标 URL
  const targetUrl = getTargetUrl(url, baseUrl);
  
  // 获取 API Key
  const apiKey = getApiKeyForProvider("", request.headers, baseUrl);
  
  // 过滤请求头
  const headers = filterHeaders(request.headers);
  
  // 添加认证头（如果需要且没有的话）
  if (apiKey && !headers.has("authorization") && !headers.has("x-api-key") && !headers.has("x-goog-api-key")) {
    if (targetUrl.includes("anthropic")) {
      headers.set("x-api-key", apiKey);
    } else if (targetUrl.includes("google")) {
      headers.set("x-goog-api-key", apiKey);
    } else if (targetUrl.includes("azure")) {
      headers.set("api-key", apiKey);
    } else {
      headers.set("Authorization", `Bearer ${apiKey}`);
    }
  }
  
  // 设置 Content-Type
  headers.set("Content-Type", "application/json");
  
  // 获取请求体
  let body = null;
  if (request.method !== "GET" && request.method !== "HEAD") {
    body = await request.text();
  }
  
  console.log(`[Proxy] ${request.method} ${targetUrl}`);
  
  // 创建超时控制器
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), CONFIG.TIMEOUT_MS);
  
  try {
    // 发起请求
    const response = await fetch(targetUrl, {
      method: request.method,
      headers,
      body,
      redirect: "manual",
      signal: controller.signal,
    });
    
    clearTimeout(timeout);
    
    // 处理重定向
    if (response.status >= 300 && response.status < 400) {
      const location = response.headers.get("location");
      if (location) {
        // 可以选择处理重定向或返回给客户端
        const newResponse = new Response(response.body, {
          status: response.status,
          statusText: response.statusText,
          headers: {
            ...Object.fromEntries(response.headers.entries()),
            location: location,
          },
        });
        return newResponse;
      }
    }
    
    // 构建响应头
    const responseHeaders = new Headers();
    
    // 复制原始响应头（过滤敏感头）
    for (const [key, value] of response.headers.entries()) {
      const lowerKey = key.toLowerCase();
      if (!["www-authenticate", "content-security-policy"].includes(lowerKey)) {
        responseHeaders.set(key, value);
      }
    }
    
    // 添加 CORS 头
    const origin = request.headers.get("origin") || "*";
    Object.entries(createCORSHeaders(origin)).forEach(([key, value]) => {
      responseHeaders.set(key, value);
    });
    
    // 禁用缓存
    responseHeaders.set("Cache-Control", "no-store");
    
    // 处理流式响应
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("text/event-stream") || contentType.includes("stream")) {
      // 流式响应 - 直接传递
      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: responseHeaders,
      });
    }
    
    // 普通响应
    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });
    
  } catch (error) {
    clearTimeout(timeout);
    
    if (error.name === "AbortError") {
      return errorResponse("Request timeout", 504);
    }
    
    console.error("[Proxy Error]", error);
    return errorResponse(`Proxy error: ${error.message}`, 502);
  }
}

// ============== 专门的处理函数 ==============

// OpenAI 兼容 API 处理
async function handleOpenAI(request) {
  const url = new URL(request.url);
  const path = url.pathname.replace("/v1/openai", "");
  
  const baseUrl = CONFIG.OPENAI_URL || CONFIG.BASE_URL || OPENAI_BASE_URL;
  const targetUrl = `${baseUrl}${path}${url.search}`;
  
  return proxyRequest(request, targetUrl, "openai");
}

// Anthropic (Claude) API 处理
async function handleAnthropic(request) {
  const url = new URL(request.url);
  const path = url.pathname.replace("/v1/anthropic", "");
  
  const baseUrl = CONFIG.ANTHROPIC_URL || CONFIG.BASE_URL || ANTHROPIC_BASE_URL;
  const targetUrl = `${baseUrl}${path}${url.search}`;
  
  return proxyRequest(request, targetUrl, "anthropic");
}

// Google Gemini API 处理
async function handleGoogle(request) {
  const url = new URL(request.url);
  const path = url.pathname.replace("/v1/google", "");
  
  const baseUrl = CONFIG.GOOGLE_URL || CONFIG.BASE_URL || GOOGLE_BASE_URL;
  const targetUrl = `${baseUrl}${path}${url.search}`;
  
  return proxyRequest(request, targetUrl, "google");
}

// DeepSeek API 处理
async function handleDeepSeek(request) {
  const url = new URL(request.url);
  const path = url.pathname.replace("/v1/deepseek", "");
  
  const baseUrl = CONFIG.DEEPSEEK_URL || CONFIG.BASE_URL || DEEPSEEK_BASE_URL;
  const targetUrl = `${baseUrl}${path}${url.search}`;
  
  return proxyRequest(request, targetUrl, "deepseek");
}

// 通用代理请求处理
async function proxyRequest(request, targetUrl, provider) {
  // 获取 API Key
  let apiKey = "";
  
  switch (provider) {
    case "openai":
    case "deepseek":
      apiKey = request.headers.get("authorization")?.replace(/^Bearer\s+/i, "").trim() 
        || CONFIG.OPENAI_API_KEY 
        || CONFIG.DEEPSEEK_API_KEY 
        || "";
      break;
    case "anthropic":
      apiKey = request.headers.get("x-api-key")
        || request.headers.get("authorization")?.replace(/^Bearer\s+/i, "").trim()
        || CONFIG.ANTHROPIC_API_KEY
        || "";
      break;
    case "google":
      apiKey = request.headers.get("x-goog-api-key")
        || CONFIG.GOOGLE_API_KEY
        || "";
      break;
  }
  
  // 过滤请求头
  const headers = new Headers();
  headers.set("Content-Type", "application/json");
  
  // 添加认证头
  if (apiKey) {
    if (provider === "anthropic") {
      headers.set("x-api-key", apiKey);
    } else if (provider === "google") {
      headers.set("x-goog-api-key", apiKey);
    } else {
      headers.set("Authorization", `Bearer ${apiKey}`);
    }
  }
  
  // 添加特定提供商的请求头
  if (provider === "anthropic") {
    const version = request.headers.get("anthropic-version") || "2023-06-01";
    headers.set("anthropic-version", version);
    headers.set("anthropic-dangerous-direct-browser-access", "true");
  }
  
  if (provider === "openai" && CONFIG.OPENAI_ORG_ID) {
    headers.set("OpenAI-Organization", CONFIG.OPENAI_ORG_ID);
  }
  
  // 获取请求体
  let body = null;
  if (request.method !== "GET" && request.method !== "HEAD") {
    body = await request.text();
  }
  
  console.log(`[${provider.toUpperCase()}] ${request.method} ${targetUrl}`);
  
  // 发起请求
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), CONFIG.TIMEOUT_MS);
  
  try {
    const response = await fetch(targetUrl, {
      method: request.method,
      headers,
      body,
      redirect: "manual",
      signal: controller.signal,
    });
    
    clearTimeout(timeout);
    
    // 构建响应
    const responseHeaders = new Headers();
    responseHeaders.set("Content-Type", "application/json");
    responseHeaders.set("Cache-Control", "no-store");
    responseHeaders.set("X-Accel-Buffering", "no");
    
    // CORS
    const origin = request.headers.get("origin") || "*";
    responseHeaders.set("Access-Control-Allow-Origin", origin);
    responseHeaders.set("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
    responseHeaders.set("Access-Control-Allow-Headers", "*");
    
    // 复制响应头
    for (const [key, value] of response.headers.entries()) {
      const lowerKey = key.toLowerCase();
      if (!["www-authenticate", "content-security-policy", "content-encoding"].includes(lowerKey)) {
        responseHeaders.set(key, value);
      }
    }
    
    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });
    
  } catch (error) {
    clearTimeout(timeout);
    
    if (error.name === "AbortError") {
      return jsonResponse({ error: { message: "Request timeout", type: "timeout" } }, 504);
    }
    
    console.error(`[${provider} Error]`, error);
    return jsonResponse({ error: { message: error.message, type: "proxy_error" } }, 502);
  }
}

// ============== 主入口 ==============
export default {
  async fetch(request, env, ctx) {
    // 处理 CORS 预检请求
    if (request.method === "OPTIONS") {
      return new Response(null, {
        status: 204,
        headers: createCORSHeaders(request.headers.get("origin") || "*"),
      });
    }
    
    const url = new URL(request.url);
    const path = url.pathname;
    
    // 路由分发
    try {
      // 通用代理端点
      if (path.startsWith("/proxy") || path === "/") {
        return handleProxyRequest(request);
      }
      
      // OpenAI 兼容端点
      if (path.startsWith("/v1/openai")) {
        return handleOpenAI(request);
      }
      
      // Anthropic 端点
      if (path.startsWith("/v1/anthropic")) {
        return handleAnthropic(request);
      }
      
      // Google 端点
      if (path.startsWith("/v1/google")) {
        return handleGoogle(request);
      }
      
      // DeepSeek 端点
      if (path.startsWith("/v1/deepseek")) {
        return handleDeepSeek(request);
      }
      
      // 健康检查
      if (path === "/health") {
        return jsonResponse({
          status: "ok",
          timestamp: new Date().toISOString(),
          version: "1.0.0",
        });
      }
      
      // 未匹配的路由
      return jsonResponse({
        error: {
          message: `Unknown endpoint: ${path}`,
          type: "not_found",
        }
      }, 404);
      
    } catch (error) {
      console.error("[Worker Error]", error);
      return jsonResponse({
        error: {
          message: error.message,
          type: "internal_error",
        }
      }, 500);
    }
  }
};

// ============== 可选：KV 存储配置（需要创建 KV Namespace） ==============
// 如果你想使用 Cloudflare KV 来存储配置（而不是硬编码），可以启用以下代码

/*
export default {
  async fetch(request, env, ctx) {
    // 从 KV 获取配置
    const storedConfig = await env.AI_PROXY_CONFIG?.get("config");
    const config = storedConfig ? JSON.parse(storedConfig) : CONFIG;
    
    // ... 其余代码同上
  }
};

// wrangler.toml 配置示例：
/*
name = "ai-proxy"
main = "index.js"
compatibility_date = "2024-01-01"

[[kv_namespaces]]
binding = "AI_PROXY_CONFIG"
id = "your-kv-namespace-id"
*/
