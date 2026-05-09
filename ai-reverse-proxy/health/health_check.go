package health

import (
	"ai-reverse-proxy/config"
	"ai-reverse-proxy/proxy"
	"log"
	"net/http"
	"sync"
	"time"
)

type HealthChecker struct {
	proxy      *proxy.ReverseProxy
	config    *config.HealthConfig
	stopChan   chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
	checks     map[string]*HealthStatus
}

type HealthStatus struct {
	Name       string
	Healthy    bool
	Latency    time.Duration
	LastCheck  time.Time
	FailCount  int
	TotalReqs  int64
	SuccessReqs int64
	mu         sync.RWMutex
}

type HealthResponse struct {
	Status     string                 `json:"status"`
	Timestamp  time.Time              `json:"timestamp"`
	Uptime     time.Duration           `json:"uptime"`
	Providers  map[string]ProviderHealth `json:"providers"`
}

type ProviderHealth struct {
	Healthy    bool          `json:"healthy"`
	Latency    string        `json:"latency"`
	LastCheck  time.Time     `json:"last_check"`
	FailCount  int           `json:"fail_count"`
	SuccessRate float64      `json:"success_rate"`
}

func NewHealthChecker(p *proxy.ReverseProxy, cfg *config.HealthConfig) *HealthChecker {
	return &HealthChecker{
		proxy:    p,
		config:  cfg,
		stopChan: make(chan struct{}),
		checks:   make(map[string]*HealthStatus),
	}
}

func (hc *HealthChecker) Start() {
	if !hc.config.Enabled {
		log.Println("[健康检查] 健康检查已禁用")
		return
	}

	hc.wg.Add(1)
	go hc.runChecker()
	
	log.Printf("[健康检查] 健康检查已启动，检查间隔: %d秒", hc.config.CheckInterval)
}

func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
	log.Println("[健康检查] 健康检查已停止")
}

func (hc *HealthChecker) runChecker() {
	defer hc.wg.Done()
	
	ticker := time.NewTicker(time.Duration(hc.config.CheckInterval) * time.Second)
	defer ticker.Stop()

	hc.checkAllProviders()

	for {
		select {
		case <-ticker.C:
			hc.checkAllProviders()
		case <-hc.stopChan:
			return
		}
	}
}

func (hc *HealthChecker) checkAllProviders() {
	cfg := config.AppConfig
	if cfg == nil {
		return
	}

	for name, provider := range cfg.Proxy.Providers {
		hc.wg.Add(1)
		go hc.checkProvider(name, provider)
	}
}

func (hc *HealthChecker) checkProvider(name string, provider config.Provider) {
	defer hc.wg.Done()

	status := &HealthStatus{
		Name:      name,
		Healthy:   true,
		LastCheck: time.Now(),
	}

	healthURL := provider.BaseURL + "/v1/models"
	if provider.BaseURL == "mock" {
		hc.updateStatus(name, status)
		return
	}

	startTime := time.Now()
	
	client := &http.Client{
		Timeout: time.Duration(hc.config.Timeout) * time.Second,
	}

	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		status.Healthy = false
		hc.updateStatus(name, status)
		return
	}

	if provider.APIKeyEnv != "" {
		req.Header.Set("Authorization", "Bearer test")
	}

	resp, err := client.Do(req)
	latency := time.Since(startTime)

	if err != nil {
		status.Healthy = false
		status.Latency = latency
		hc.updateStatus(name, status)
		log.Printf("[健康检查] 提供商 %s 检查失败: %v", name, err)
		return
	}
	defer resp.Body.Close()

	status.Latency = latency

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status.Healthy = true
	} else {
		status.Healthy = false
		status.FailCount++
	}

	hc.updateStatus(name, status)
	
	if status.Healthy {
		log.Printf("[健康检查] 提供商 %s 健康 | 延迟: %v", name, latency)
	}
}

func (hc *HealthChecker) updateStatus(name string, status *HealthStatus) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks[name] = status
}

func (hc *HealthChecker) GetHealthStatus() *HealthResponse {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	response := &HealthResponse{
		Timestamp: time.Now(),
		Providers: make(map[string]ProviderHealth),
	}

	healthyCount := 0
	totalCount := 0

	for name, status := range hc.checks {
		totalCount++
		successRate := 0.0
		if status.TotalReqs > 0 {
			successRate = float64(status.SuccessReqs) / float64(status.TotalReqs) * 100
		}

		providerHealth := ProviderHealth{
			Healthy:    status.Healthy,
			Latency:    status.Latency.String(),
			LastCheck:  status.LastCheck,
			FailCount:  status.FailCount,
			SuccessRate: successRate,
		}

		response.Providers[name] = providerHealth

		if status.Healthy {
			healthyCount++
		}
	}

	if totalCount == 0 {
		response.Status = "unknown"
	} else if healthyCount == totalCount {
		response.Status = "healthy"
	} else if healthyCount > 0 {
		response.Status = "degraded"
	} else {
		response.Status = "unhealthy"
	}

	return response
}

func (hc *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" || r.URL.Path == "/healthz" {
		hc.handleHealth(w, r)
	} else if r.URL.Path == "/health/ready" {
		hc.handleReadiness(w, r)
	} else if r.URL.Path == "/health/live" {
		hc.handleLiveness(w, r)
	}
}

func (hc *HealthChecker) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := hc.GetHealthStatus()
	
	w.Header().Set("Content-Type", "application/json")
	
	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else if status.Status == "degraded" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	w.Write([]byte(`{"status":"` + status.Status + `","timestamp":"` + status.Timestamp.Format(time.RFC3339) + `"}`))
}

func (hc *HealthChecker) handleReadiness(w http.ResponseWriter, r *http.Request) {
	hc.mu.RLock()
	hasHealthy := false
	for _, status := range hc.checks {
		if status.Healthy {
			hasHealthy = true
			break
		}
	}
	hc.mu.RUnlock()

	if hasHealthy {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not_ready"}`))
	}
}

func (hc *HealthChecker) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"alive"}`))
}
