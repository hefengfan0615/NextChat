/**
 * Cloudflare Worker - 极简 AI API 代理
 * 
 * 使用方法：
 * 1. 修改下方的 TARGET_URL 为你要代理的 API 地址
 * 2. 部署到 Cloudflare Workers
 * 3. 在前端使用此 Worker 的 URL 作为 API 地址
 */

// ★★★★★ 在这里修改你要代理的地址 ★★★★★
const TARGET_URL = "https://api.openai.com";
// 例如：
// const TARGET_URL = "https://api.anthropic.com";
// const TARGET_URL = "https://generativelanguage.googleapis.com";
// const TARGET_URL = "https://api.deepseek.com";
// const TARGET_URL = "https://your-proxy.com";

export default {
  async fetch(request) {
    const url = new URL(request.url);
    
    const target = TARGET_URL.endsWith("/") ? TARGET_URL.slice(0, -1) : TARGET_URL;
    const path = url.pathname + url.search;
    const fullUrl = target + path;

    const headers = new Headers();
    headers.set("Content-Type", "application/json");
    
    const auth = request.headers.get("authorization");
    if (auth) headers.set("Authorization", auth);
    
    const apiKey = request.headers.get("x-api-key");
    if (apiKey) headers.set("x-api-key", apiKey);
    
    const anthropicVersion = request.headers.get("anthropic-version");
    if (anthropicVersion) headers.set("anthropic-version", anthropicVersion);

    let body = null;
    if (request.method !== "GET") {
      body = await request.text();
    }

    try {
      const res = await fetch(fullUrl, {
        method: request.method,
        headers,
        body,
      });

      const resHeaders = new Headers();
      for (const [k, v] of res.headers) {
        if (!["www-authenticate", "content-encoding"].includes(k.toLowerCase())) {
          resHeaders.set(k, v);
        }
      }
      resHeaders.set("Access-Control-Allow-Origin", "*");

      return new Response(res.body, {
        status: res.status,
        headers: resHeaders,
      });
    } catch (e) {
      return new Response(JSON.stringify({ error: e.message }), {
        status: 502,
        headers: { "Content-Type": "application/json", "Access-Control-Allow-Origin": "*" },
      });
    }
  }
};
