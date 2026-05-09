package main

import (
	"ai-reverse-proxy/config"
	"ai-reverse-proxy/health"
	"ai-reverse-proxy/proxy"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	configPath string
	port       string
	host       string
	showVersion bool
)

func init() {
	flag.StringVar(&configPath, "config", "config.yaml", "配置文件路径")
	flag.StringVar(&port, "port", "", "监听端口 (覆盖配置文件)")
	flag.StringVar(&host, "host", "", "监听地址 (覆盖配置文件)")
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Println("AI Reverse Proxy v1.0.0")
		fmt.Println("基于Go语言的高性能AI服务反向代理")
		return
	}

	if err := run(); err != nil {
		log.Fatalf("程序启动失败: %v", err)
	}
}

func run() error {
	if err := config.LoadConfig(configPath); err != nil {
		return fmt.Errorf("配置加载失败: %w", err)
	}

	cfg := config.AppConfig

	if port != "" {
		cfg.Proxy.Port = port
	}
	if host != "" {
		cfg.Proxy.Host = host
	}

	bindAddr := fmt.Sprintf("%s:%s", cfg.Proxy.Host, cfg.Proxy.Port)
	
	log.Printf("=" + strings.Repeat("=", 60))
	log.Printf("AI Reverse Proxy 启动中...")
	log.Printf("监听地址: %s", bindAddr)
	log.Printf("读取超时: %d秒", cfg.Proxy.ReadTimeout)
	log.Printf("写入超时: %d秒", cfg.Proxy.WriteTimeout)
	log.Printf("健康检查: %v", cfg.Health.Enabled)
	log.Printf("=" + strings.Repeat("=", 60))

	reverseProxy := proxy.NewReverseProxy(&cfg.Proxy)
	
	healthChecker := health.NewHealthChecker(reverseProxy, &cfg.Health)
	healthChecker.Start()
	defer healthChecker.Stop()

	mux := http.NewServeMux()
	
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/v1/", reverseProxy.ServeHTTP)
	mux.HandleFunc("/health", healthChecker.ServeHTTP)
	mux.HandleFunc("/healthz", healthChecker.ServeHTTP)
	mux.HandleFunc("/health/ready", healthChecker.ServeHTTP)
	mux.HandleFunc("/health/live", healthChecker.ServeHTTP)
	mux.HandleFunc("/stats", handleStats)
	mux.HandleFunc("/reload", handleReload)

	server := &http.Server{
		Addr:         bindAddr,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  time.Duration(cfg.Proxy.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Proxy.WriteTimeout) * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("服务器已在 %s 启动", bindAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器错误: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")
	
	return nil
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AI Reverse Proxy</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
        }
        .container {
            background: white;
            border-radius: 20px;
            padding: 60px;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
        }
        h1 { color: #333; margin-bottom: 20px; font-size: 2.5em; }
        .subtitle { color: #666; font-size: 1.2em; margin-bottom: 40px; }
        .features {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 20px;
            max-width: 600px;
            margin: 0 auto;
        }
        .feature {
            background: #f8f9fa;
            padding: 20px;
            border-radius: 10px;
        }
        .feature h3 { color: #667eea; margin-bottom: 10px; }
        .feature p { color: #666; font-size: 0.9em; }
        .version { margin-top: 40px; color: #999; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🤖 AI Reverse Proxy</h1>
        <p class="subtitle">高性能AI服务反向代理 - 加速国内外AI响应</p>
        <div class="features">
            <div class="feature">
                <h3>🌐 智能路由</h3>
                <p>根据用户地区自动选择最优节点</p>
            </div>
            <div class="feature">
                <h3>⚡ 负载均衡</h3>
                <p>多节点负载均衡，提高可用性</p>
            </div>
            <div class="feature">
                <h3>❤️ 健康检查</h3>
                <p>实时监控节点状态，自动故障转移</p>
            </div>
            <div class="feature">
                <h3>🔒 安全代理</h3>
                <p>API Key安全注入，保护隐私</p>
            </div>
        </div>
        <p class="version">Version 1.0.0</p>
    </div>
</body>
</html>
	`))
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","message":"统计功能开发中"}`))
}

func handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if err := config.LoadConfig(configPath); err != nil {
		http.Error(w, fmt.Sprintf("配置重载失败: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Write([]byte(`{"status":"ok","message":"配置已重载"}`))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		if r.URL.Path != "/health" && r.URL.Path != "/healthz" && 
		   r.URL.Path != "/health/live" && r.URL.Path != "/health/ready" {
			log.Printf("[%s] %s %s", r.Method, r.URL.Path, r.RemoteAddr)
		}
		
		next.ServeHTTP(w, r)
		
		if r.URL.Path != "/health" && r.URL.Path != "/healthz" && 
		   r.URL.Path != "/health/live" && r.URL.Path != "/health/ready" {
			log.Printf("[完成] %s %s - 耗时: %v", r.Method, r.URL.Path, time.Since(start))
		}
	})
}
