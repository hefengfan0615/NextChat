package proxy

import (
	"ai-reverse-proxy/config"
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type ReverseProxy struct {
	config      *config.ProxyConfig
	transport   *http.Transport
	providerMgr *ProviderManager
}

type ProviderManager struct {
	providers map[string]*ProviderInstance
	mu        sync.RWMutex
}

type ProviderInstance struct {
	Name       string
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	Weight     int
	Enabled    bool
	Healthy    bool
	Latency    time.Duration
	FailCount  int
	mu         sync.RWMutex
}

type ProxyResponse struct {
	Provider   string
	Latency    time.Duration
	StatusCode int
	Error      error
}

func NewReverseProxy(cfg *config.ProxyConfig) *ReverseProxy {
	transport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}

	manager := &ProviderManager{
		providers: make(map[string]*ProviderInstance),
	}

	for name, provider := range cfg.Providers {
		apiKey := os.Getenv(provider.APIKeyEnv)
		if apiKey == "" && name != "mock" {
			log.Printf("警告: 未找到环境变量 %s for provider %s", provider.APIKeyEnv, name)
			continue
		}

		timeout := time.Duration(provider.Timeout) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		manager.providers[name] = &ProviderInstance{
			Name:    name,
			BaseURL: provider.BaseURL,
			APIKey:  apiKey,
			Timeout: timeout,
			Weight:  provider.Weight,
			Enabled: provider.Enabled,
			Healthy: true,
		}
	}

	return &ReverseProxy{
		config:      cfg,
		transport:   transport,
		providerMgr: manager,
	}
}

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	
	clientIP := GetClientIP(r)
	region := DetectRegion(clientIP)
	
	log.Printf("[请求] %s %s | IP: %s | Region: %s", r.Method, r.URL.Path, clientIP, region)

	rule := config.GetMatchingRule(r.URL.Path, region)
	if rule == nil {
		http.Error(w, "未找到匹配的路由规则", http.StatusNotFound)
		return
	}

	providerName := rule.Provider
	if providerName == "" {
		http.Error(w, "路由规则未指定提供商", http.StatusInternalServerError)
		return
	}

	provider := p.providerMgr.GetProvider(providerName)
	if provider == nil {
		http.Error(w, "指定的提供商不存在", http.StatusBadGateway)
		return
	}

	targetURL, err := url.Parse(provider.BaseURL)
	if err != nil {
		log.Printf("[错误] 目标URL解析失败: %v", err)
		http.Error(w, "代理服务器配置错误", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = p.transport
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[错误] 代理请求失败: %v", err)
		p.providerMgr.RecordFailure(providerName)
		http.Error(w, "上游服务器错误", http.StatusBadGateway)
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Forwarded-Proto", r.URL.Scheme)
		req.Header.Set("X-Real-IP", clientIP)
		req.Header.Set("X-Proxy-Provider", providerName)
		
		if apiKey := provider.APIKey; apiKey != "" {
			authHeader := req.Header.Get("Authorization")
			if authHeader == "" {
				req.Header.Set("Authorization", "Bearer "+apiKey)
			}
		}
		
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	proxy.ServeHTTP(w, r)

	latency := time.Since(startTime)
	p.providerMgr.RecordLatency(providerName, latency)
	
	log.Printf("[响应] Provider: %s | Status: 200 | Latency: %v", providerName, latency)
}

func (pm *ProviderManager) GetProvider(name string) *ProviderInstance {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.providers[name]
}

func (pm *ProviderManager) GetHealthyProvider(names []string, balanceMethod string) *ProviderInstance {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var candidates []*ProviderInstance
	for _, name := range names {
		if provider, ok := pm.providers[name]; ok && provider.Enabled && provider.Healthy {
			candidates = append(candidates, provider)
		}
	}

	if len(candidates) == 0 {
		for _, name := range names {
			if provider, ok := pm.providers[name]; ok && provider.Enabled {
				return provider
			}
		}
		return nil
	}

	switch balanceMethod {
	case "roundrobin":
		return candidates[time.Now().UnixNano()%int64(len(candidates))]
	case "leastlatency":
		return pm.selectLeastLatency(candidates)
	case "weighted":
		return pm.selectWeighted(candidates)
	default:
		return candidates[0]
	}
}

func (pm *ProviderManager) selectLeastLatency(candidates []*ProviderInstance) *ProviderInstance {
	var best *ProviderInstance
	minLatency := time.Hour

	for _, c := range candidates {
		if c.Latency < minLatency {
			minLatency = c.Latency
			best = c
		}
	}
	return best
}

func (pm *ProviderManager) selectWeighted(candidates []*ProviderInstance) *ProviderInstance {
	totalWeight := 0
	for _, c := range candidates {
		totalWeight += c.Weight
	}

	r := time.Now().UnixNano() % int64(totalWeight)
	sum := 0
	for _, c := range candidates {
		sum += c.Weight
		if int64(sum) > r {
			return c
		}
	}
	return candidates[0]
}

func (pm *ProviderManager) RecordLatency(name string, latency time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if provider, ok := pm.providers[name]; ok {
		provider.mu.Lock()
		provider.Latency = latency
		if latency > 5*time.Second {
			provider.FailCount++
			if provider.FailCount > 5 {
				provider.Healthy = false
				log.Printf("[健康检查] 提供商 %s 被标记为不健康", name)
			}
		} else {
			provider.FailCount = 0
			provider.Healthy = true
		}
		provider.mu.Unlock()
	}
}

func (pm *ProviderManager) RecordFailure(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if provider, ok := pm.providers[name]; ok {
		provider.mu.Lock()
		provider.FailCount++
		if provider.FailCount > 3 {
			provider.Healthy = false
			log.Printf("[健康检查] 提供商 %s 失败次数过多，标记为不健康", name)
		}
		provider.mu.Unlock()
	}
}

func GetClientIP(r *http.Request) string {
	headers := []string{
		"X-Forwarded-For",
		"X-Real-IP",
		"CF-Connecting-IP",
		"True-Client-IP",
	}

	for _, header := range headers {
		if ip := r.Header.Get(header); ip != "" {
			if idx := strings.Index(ip, ","); idx != -1 {
				ip = strings.TrimSpace(ip[:idx])
			}
			return ip
		}
	}

	return r.RemoteAddr
}

func DetectRegion(clientIP string) string {
	if strings.HasPrefix(clientIP, "10.") ||
	   strings.HasPrefix(clientIP, "172.16.") || strings.HasPrefix(clientIP, "172.17.") ||
	   strings.HasPrefix(clientIP, "172.18.") || strings.HasPrefix(clientIP, "172.19.") ||
	   strings.HasPrefix(clientIP, "172.20.") || strings.HasPrefix(clientIP, "172.21.") ||
	   strings.HasPrefix(clientIP, "172.22.") || strings.HasPrefix(clientIP, "172.23.") ||
	   strings.HasPrefix(clientIP, "172.24.") || strings.HasPrefix(clientIP, "172.25.") ||
	   strings.HasPrefix(clientIP, "172.26.") || strings.HasPrefix(clientIP, "172.27.") ||
	   strings.HasPrefix(clientIP, "172.28.") || strings.HasPrefix(clientIP, "172.29.") ||
	   strings.HasPrefix(clientIP, "172.30.") || strings.HasPrefix(clientIP, "172.31.") ||
	   strings.HasPrefix(clientIP, "192.168.") ||
	   clientIP == "127.0.0.1" || clientIP == "::1" || clientIP == "localhost" {
		return "CN"
	}

	return "INTL"
}
