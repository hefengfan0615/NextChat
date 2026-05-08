#!/usr/bin/env node

/**
 * 测试脚本 - 验证 Cloudflare Worker 代理
 * 
 * 使用方法：
 * node test.js <worker-url> [api-key]
 */

const WORKER_URL = process.argv[2] || "http://localhost:8787";
const API_KEY = process.argv[3] || "";

async function testHealth() {
  console.log("\n📡 测试健康检查...");
  try {
    const res = await fetch(`${WORKER_URL}/health`);
    const data = await res.json();
    console.log("✅ 健康检查通过:", data);
    return true;
  } catch (e) {
    console.log("❌ 健康检查失败:", e.message);
    return false;
  }
}

async function testOpenAI() {
  console.log("\n🔵 测试 OpenAI API...");
  try {
    const res = await fetch(`${WORKER_URL}/v1/openai/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${API_KEY}`,
      },
      body: JSON.stringify({
        model: "gpt-4o-mini",
        messages: [{ role: "user", content: "Say 'Hello from proxy!' in exactly those words." }],
        max_tokens: 50,
      }),
    });
    
    const data = await res.json();
    
    if (res.ok) {
      console.log("✅ OpenAI 请求成功!");
      console.log("📝 响应:", data.choices?.[0]?.message?.content || data);
    } else {
      console.log("❌ OpenAI 请求失败:", data);
    }
    return res.ok;
  } catch (e) {
    console.log("❌ OpenAI 请求失败:", e.message);
    return false;
  }
}

async function testAnthropic() {
  console.log("\n🟡 测试 Anthropic (Claude) API...");
  try {
    const res = await fetch(`${WORKER_URL}/v1/anthropic/v1/messages`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "x-api-key": API_KEY,
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify({
        model: "claude-3-5-haiku-20241022",
        max_tokens: 100,
        messages: [{ role: "user", content: "Say 'Hello from proxy!' in exactly those words." }],
      }),
    });
    
    const data = await res.json();
    
    if (res.ok) {
      console.log("✅ Anthropic 请求成功!");
      console.log("📝 响应:", data.content?.[0]?.text || data);
    } else {
      console.log("❌ Anthropic 请求失败:", data);
    }
    return res.ok;
  } catch (e) {
    console.log("❌ Anthropic 请求失败:", e.message);
    return false;
  }
}

async function testStream() {
  console.log("\n🔄 测试流式响应...");
  try {
    const res = await fetch(`${WORKER_URL}/v1/openai/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${API_KEY}`,
      },
      body: JSON.stringify({
        model: "gpt-4o-mini",
        messages: [{ role: "user", content: "Count from 1 to 3, one number per message." }],
        max_tokens: 50,
        stream: true,
      }),
    });
    
    let chunks = 0;
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      
      chunks++;
      const chunk = decoder.decode(value);
      console.log("📦 流式数据块:", chunk.slice(0, 100) + (chunk.length > 100 ? "..." : ""));
    }
    
    console.log(`✅ 流式响应成功，共 ${chunks} 个数据块`);
    return true;
  } catch (e) {
    console.log("❌ 流式响应测试失败:", e.message);
    return false;
  }
}

async function testGenericProxy() {
  console.log("\n🌐 测试通用代理 (x-base-url header)...");
  try {
    const res = await fetch(`${WORKER_URL}/proxy/v1/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${API_KEY}`,
        "x-base-url": "https://api.openai.com",
      },
      body: JSON.stringify({
        model: "gpt-4o-mini",
        messages: [{ role: "user", content: "Say 'Generic proxy works!' in exactly those words." }],
        max_tokens: 50,
      }),
    });
    
    const data = await res.json();
    
    if (res.ok) {
      console.log("✅ 通用代理请求成功!");
      console.log("📝 响应:", data.choices?.[0]?.message?.content || data);
    } else {
      console.log("❌ 通用代理请求失败:", data);
    }
    return res.ok;
  } catch (e) {
    console.log("❌ 通用代理请求失败:", e.message);
    return false;
  }
}

async function main() {
  console.log("🚀 Cloudflare Worker AI 代理测试工具");
  console.log("=====================================");
  console.log("Worker URL:", WORKER_URL);
  console.log("API Key:", API_KEY ? "已提供 (隐藏)" : "未提供");
  
  const results = [];
  
  // 健康检查
  results.push({ name: "健康检查", pass: await testHealth() });
  
  if (!API_KEY) {
    console.log("\n⚠️  未提供 API Key，跳过 API 测试");
    console.log("   使用方式: node test.js <worker-url> <api-key>");
    return;
  }
  
  // API 测试
  results.push({ name: "OpenAI", pass: await testOpenAI() });
  results.push({ name: "Anthropic", pass: await testAnthropic() });
  results.push({ name: "流式响应", pass: await testStream() });
  results.push({ name: "通用代理", pass: await testGenericProxy() });
  
  // 汇总
  console.log("\n=====================================");
  console.log("📊 测试结果汇总:");
  
  results.forEach(r => {
    console.log(`  ${r.pass ? "✅" : "❌"} ${r.name}`);
  });
  
  const passed = results.filter(r => r.pass).length;
  console.log(`\n通过: ${passed}/${results.length}`);
  
  if (passed === results.length) {
    console.log("🎉 所有测试通过!");
  } else {
    console.log("⚠️  部分测试失败，请检查配置");
    process.exit(1);
  }
}

main().catch(console.error);
