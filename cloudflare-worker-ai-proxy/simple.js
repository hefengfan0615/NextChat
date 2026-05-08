/**
 * Cloudflare Worker - 简化版 AI 代理
 * 
 * 使用方法：只需修改下方的 TARGET_URL 和 API_KEY
 */

const TARGET_URL = "https://api.openai.com";  // ← 修改为你要代理的地址
const API_KEY = "";                          // ← 你的 API Key（留空则使用请求头中的）

export default {
  async fetch(request) {
    // CORS 预检
    if (request.method === "OPTIONS") {
      return new Response(null, {
        status: 204,
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
          "Access-Control-Allow-Headers": "Content-Type, Authorization, x-api-key, anthropic-version",
        },
      });
    }

    const url = new URL(request.url);
    const path = url.pathname + url.search;
    
    // 构建目标 URL
    const target = TARGET_URL.endsWith("/") ? TARGET_URL.slice(0, -1) : TARGET_URL;
    const targetPath = path.startsWith("/") ? path : "/" + path;
    const targetUrl = target + targetPath;

    // 获取 API Key
    let authKey = API_KEY;
    if (!authKey) {
      authKey = request.headers.get("authorization")?.replace(/^Bearer\s+/i, "").trim() || "";
    }

    // 构建请求头
    const headers = new Headers();
    headers.set("Content-Type", "application/json");
    
    if (authKey) {
      if (target.includes("anthropic")) {
        headers.set("x-api-key", authKey);
      } else {
        headers.set("Authorization", `Bearer ${authKey}`);
      }
    }

    // Anthropic 特殊头
    if (target.includes("anthropic")) {
      headers.set("anthropic-version", request.headers.get("anthropic-version") || "2023-06-01");
      headers.set("anthropic-dangerous-direct-browser-access", "true");
    }

    // 获取请求体
    let body = null;
    if (request.method !== "GET" && request.method !== "HEAD") {
      body = await request.text();
    }

    console.log(`[Proxy] ${request.method} ${targetUrl}`);

    try {
      const response = await fetch(targetUrl, {
        method: request.method,
        headers,
        body,
        redirect: "manual",
      });

      // 构建响应
      const responseHeaders = new Headers();
      for (const [key, value] of response.headers.entries()) {
        if (!["www-authenticate", "content-encoding"].includes(key.toLowerCase())) {
          responseHeaders.set(key, value);
        }
      }
      responseHeaders.set("Access-Control-Allow-Origin", "*");
      responseHeaders.set("Cache-Control", "no-store");

      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: responseHeaders,
      });

    } catch (error) {
      console.error("[Error]", error);
      return new Response(JSON.stringify({ error: { message: error.message } }), {
        status: 502,
        headers: { "Content-Type": "application/json", "Access-Control-Allow-Origin": "*" },
      });
    }
  }
};
