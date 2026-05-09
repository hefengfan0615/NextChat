package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
)

type Config struct {
	ListenAddr     string
	NextChatURL    string
	ApiKey         string
	CacheDir       string
	CacheTTL       time.Duration
	ConnPoolSize   int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MaxRetries     int
	CircuitThreshold int
	CircuitTimeout   time.Duration
}

var cfg = &Config{
	ListenAddr:      ":8080",
	NextChatURL:     "http://localhost:3000",
	CacheDir:        "./cache",
	CacheTTL:        10 * time.Minute,
	ConnPoolSize:    100,
	ReadTimeout:     60 * time.Second,
	WriteTimeout:    120 * time.Second,
	IdleTimeout:     90 * time.Second,
	MaxRetries:      3,
	CircuitThreshold: 5,
	CircuitTimeout:   30 * time.Second,
}

type Backend struct {
	URL           *url.URL
	Weight        int
	Latency       time.Duration
	Healthy       bool
	FailCount     int32
	SuccessCount  int32
	TotalRequests int64
	TotalErrors   int64
	mu            sync.RWMutex
}

type LoadBalancer struct {
	backends []*Backend
	current  int32
	mu       sync.RWMutex
}

func NewLoadBalancer(urls []string) *LoadBalancer {
	backends := make([]*Backend, len(urls))
	for i, u := range urls {
		parsed, _ := url.Parse(u)
		backends[i] = &Backend{
			URL:     parsed,
			Weight:  1,
			Healthy: true,
		}
	}
	return &LoadBalancer{backends: backends}
}

func (lb *LoadBalancer) GetBackend() *Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	best := lb.backends[0]
	bestScore := float64(math.MaxFloat64)

	for _, be := range lb.backends {
		if !be.Healthy {
			continue
		}
		latency := be.Latency.Seconds()
		failRatio := float64(atomic.LoadInt64(&be.TotalErrors)) / math.Max(1, float64(atomic.LoadInt64(&be.TotalRequests)))
		score := latency + failRatio*10

		if score < bestScore {
			bestScore = score
			best = be
		}
	}

	atomic.AddInt64(&best.TotalRequests, 1)
	return best
}

func (lb *LoadBalancer) RecordResult(backend *Backend, latency time.Duration, isError bool) {
	backend.mu.Lock()
	defer backend.mu.Unlock()

	if isError {
		atomic.AddInt64(&backend.TotalErrors, 1)
		backend.FailCount++
		atomic.AddInt32(&backend.SuccessCount, 0)
		if backend.FailCount >= int32(cfg.CircuitThreshold) {
			backend.Healthy = false
			go func() {
				time.Sleep(cfg.CircuitTimeout)
				backend.mu.Lock()
				backend.FailCount = 0
				backend.Healthy = true
				backend.mu.Unlock()
			}()
		}
	} else {
		if backend.Latency == 0 {
			backend.Latency = latency
		} else {
			backend.Latency = (backend.Latency*7 + latency) / 8
		}
		atomic.AddInt32(&backend.SuccessCount, 1)
		backend.FailCount = 0
	}
}

type CircuitBreaker struct {
	failures     int32
	lastFailure  time.Time
	state        int32
	maxFailures  int32
	timeout      time.Duration
	mu           sync.RWMutex
}

const (
	StateClosed   = 0
	StateOpen     = 1
	StateHalfOpen = 2
)

func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: int32(maxFailures),
		timeout:     timeout,
		state:      StateClosed,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	state := atomic.LoadInt32(&cb.state)
	if state == StateClosed {
		return true
	}
	if state == StateOpen {
		cb.mu.RLock()
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.mu.RUnlock()
			atomic.CompareAndSwapInt32(&cb.state, StateOpen, StateHalfOpen)
			return true
		}
		cb.mu.RUnlock()
		return false
	}
	return true
}

func (cb *CircuitBreaker) RecordSuccess() {
	atomic.StoreInt32(&cb.state, StateClosed)
	atomic.StoreInt32(&cb.failures, 0)
}

func (cb *CircuitBreaker) RecordFailure() {
	atomic.AddInt32(&cb.failures, 1)
	cb.mu.Lock()
	cb.lastFailure = time.Now()
	cb.mu.Unlock()
	if atomic.LoadInt32(&cb.failures) >= cb.maxFailures {
		atomic.StoreInt32(&cb.state, StateOpen)
	}
}

type ResponseCache struct {
	data     map[string]*CacheEntry
	mu       sync.RWMutex
	maxSize  int
	evictMu  sync.Mutex
}

type CacheEntry struct {
	Data      []byte
	ExpiresAt time.Time
	CreatedAt time.Time
	Key       string
}

func NewResponseCache(maxSize int) *ResponseCache {
	c := &ResponseCache{
		data:    make(map[string]*CacheEntry),
		maxSize: maxSize,
	}
	go c.evictionWorker()
	return c
}

func (c *ResponseCache) evictionWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		c.evict()
	}
}

func (c *ResponseCache) evict() {
	c.evictMu.Lock()
	defer c.evictMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, entry := range c.data {
		if now.After(entry.ExpiresAt) {
			delete(c.data, k)
		}
	}

	if len(c.data) > c.maxSize {
		type kv struct {
			key    string
			access time.Time
		}
		entries := make([]kv, 0, len(c.data))
		for k, e := range c.data {
			entries = append(entries, kv{k, e.CreatedAt})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].access.Before(entries[j].access)
		})
		toDelete := len(c.data) - c.maxSize/2
		for i := 0; i < toDelete && i < len(entries); i++ {
			delete(c.data, entries[i].key)
		}
	}
}

func (c *ResponseCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.data[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	c.mu.Lock()
	if entry != c.data[key] {
		c.mu.Unlock()
		return nil, false
	}
	entry.CreatedAt = time.Now()
	c.mu.Unlock()

	return entry.Data, true
}

func (c *ResponseCache) Set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	c.data[key] = &CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
		Key:       key,
	}
	c.mu.Unlock()
}

func (c *ResponseCache) Clear() {
	c.mu.Lock()
	c.data = make(map[string]*CacheEntry)
	c.mu.Unlock()
}

type ProxyServer struct {
	lb              *LoadBalancer
	cache           *ResponseCache
	circuitBreaker  *CircuitBreaker
	httpClient      *http.Client
	stats           *Stats
	cacheKeySecret  []byte
	compressBuffer  *sync.Pool
}

type Stats struct {
	Requests       int64
	CacheHits      int64
	CacheMisses    int64
	Errors         int64
	AvgLatency     time.Duration
	TotalLatency   time.Duration
	LatencyCount   int64
	ActiveRequests int32
	mu             sync.RWMutex
}

func NewProxyServer() *ProxyServer {
	transport := &http.Transport{
		MaxIdleConns:        cfg.ConnPoolSize,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     cfg.IdleTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: cfg.ReadTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.WriteTimeout,
	}

	secret := make([]byte, 32)
	rand.Read(secret)

	return &ProxyServer{
		lb:             NewLoadBalancer([]string{cfg.NextChatURL}),
		cache:          NewResponseCache(10000),
		circuitBreaker: NewCircuitBreaker(cfg.CircuitThreshold, cfg.CircuitTimeout),
		httpClient:     client,
		stats:          &Stats{},
		cacheKeySecret: secret,
		compressBuffer: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (s *ProxyServer) generateCacheKey(r *http.Request, body []byte) string {
	method := r.Method
	path := r.URL.Path
	query := r.URL.RawQuery

	auth := r.Header.Get("Authorization")
	stream := r.Header.Get("Accept")

	combined := fmt.Sprintf("%s|%s|%s|%s|%s|%x", method, path, query, auth, stream, sha256.Sum256(body))

	h := hmac.New(sha256.New, s.cacheKeySecret)
	h.Write([]byte(combined))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *ProxyServer) shouldCache(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}

	path := r.URL.Path
	if strings.Contains(path, "/chat/completions") {
		if r.Header.Get("Accept") == "text/event-stream" {
			return false
		}
		return true
	}

	if strings.Contains(path, "/models") && r.Method == http.MethodGet {
		return true
	}

	return false
}

func (s *ProxyServer) forwardRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	atomic.AddInt32(&s.stats.ActiveRequests, 1)
	defer atomic.AddInt32(&s.stats.ActiveRequests, -1)

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	if s.shouldCache(r) && len(body) > 0 {
		cacheKey := s.generateCacheKey(r, body)

		if cached, ok := s.cache.Get(cacheKey); ok {
			atomic.AddInt64(&s.stats.CacheHits, 1)
			w.Header().Set("X-Cache", "HIT")
			w.Write(cached)
			return
		}
		atomic.AddInt64(&s.stats.CacheMisses, 1)
		w.Header().Set("X-Cache", "MISS")
	}

	if !s.circuitBreaker.Allow() {
		s.recordLatency(time.Since(start), false)
		http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	backend := s.lb.GetBackend()

	var resp *http.Response
	var err error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * 100 * time.Millisecond
			backoff += time.Duration(rand.Intn(100)) * time.Millisecond
			time.Sleep(backoff)
		}

		resp, err = s.doRequest(backend.URL, r, body)
		if err == nil {
			break
		}

		log.Printf("Request attempt %d failed: %v", attempt+1, err)

		if attempt == cfg.MaxRetries {
			s.recordLatency(time.Since(start), false)
			s.circuitBreaker.RecordFailure()
			s.lb.RecordResult(backend, time.Since(start), true)
			http.Error(w, "Failed to proxy request", http.StatusBadGateway)
			return
		}
	}

	s.recordLatency(time.Since(start), false)
	s.circuitBreaker.RecordSuccess()
	s.lb.RecordResult(backend, time.Since(start), false)

	if resp.StatusCode >= 500 {
		s.lb.RecordResult(backend, time.Since(start), true)
	}

	s.copyResponse(w, r, resp, body)
}

func (s *ProxyServer) doRequest(target *url.URL, r *http.Request, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(r.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	for k, v := range r.Header {
		if k == "Connection" || k == "Host" || k == "Origin" || k == "Referer" {
			continue
		}
		req.Header[k] = v
	}

	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set("X-Real-IP", getIP(r))
	req.Header.Set("X-Forwarded-Proto", "https")

	if cfg.ApiKey != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+cfg.ApiKey)
	}

	req.Header.Set("Accept-Encoding", "gzip, deflate, br")

	return s.httpClient.Do(req)
}

func (s *ProxyServer) copyResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, body []byte) {
	w.WriteHeader(resp.StatusCode)

	for k, v := range resp.Header {
		if k == "Content-Encoding" || k == "Transfer-Encoding" || k == "Connection" {
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Del("www-authenticate")

	contentEncoding := resp.Header.Get("Content-Encoding")
	isStreaming := r.Header.Get("Accept") == "text/event-stream"

	if isStreaming {
		flusher, ok := w.(http.Flusher)
		if ok {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
		}

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			data := scanner.Bytes()
			if len(data) > 0 {
				w.Write(data)
				w.Write([]byte("\n"))
				if ok {
					flusher.Flush()
				}
			}
		}
		return
	}

	var output io.Writer = w

	if strings.Contains(contentEncoding, "gzip") {
		gzWriter := gzip.NewWriter(w)
		defer gzWriter.Close()
		output = gzWriter
		w.Header().Set("Content-Encoding", "gzip")
	} else if strings.Contains(contentEncoding, "br") {
		brWriter := brotli.NewWriter(w)
		defer brWriter.Close()
		output = brWriter
		w.Header().Set("Content-Encoding", "br")
	}

	if s.shouldCache(r) && len(body) > 0 && resp.StatusCode == 200 {
		cacheKey := s.generateCacheKey(r, body)
		respBody, _ := io.ReadAll(resp.Body)

		if !strings.Contains(contentEncoding, "gzip") && !strings.Contains(contentEncoding, "br") {
			if gzipData, err := gzipEncode(respBody); err == nil {
				s.cache.Set(cacheKey, gzipData, cfg.CacheTTL)
				w.Header().Set("X-Cached", "true")
			}
		}

		output.Write(respBody)
		return
	}

	io.Copy(output, resp.Body)
}

func gzipEncode(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ProxyServer) recordLatency(d time.Duration, isError bool) {
	s.stats.mu.Lock()
	s.stats.TotalLatency += d
	s.stats.LatencyCount++
	if isError {
		s.stats.Errors++
	}
	if s.stats.LatencyCount > 0 {
		s.stats.AvgLatency = s.stats.TotalLatency / time.Duration(s.stats.LatencyCount)
	}
	s.stats.mu.Unlock()
}

func (s *ProxyServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.stats.mu.RLock()
	stats := struct {
		Requests       int64
		CacheHits      int64
		CacheMisses    int64
		Errors         int64
		AvgLatency     string
		ActiveRequests int32
		CacheSize      int
	}{
		Requests:       atomic.LoadInt64(&s.stats.Requests),
		CacheHits:      atomic.LoadInt64(&s.stats.CacheHits),
		CacheMisses:    atomic.LoadInt64(&s.stats.CacheMisses),
		Errors:         atomic.LoadInt64(&s.stats.Errors),
		AvgLatency:     s.stats.AvgLatency.String(),
		ActiveRequests: atomic.LoadInt32(&s.stats.ActiveRequests),
		CacheSize:      len(s.cache.data),
	}
	s.stats.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *ProxyServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	healthy := s.circuitBreaker.Allow()
	if healthy {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Degraded"))
	}
}

func (s *ProxyServer) handleClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.cache.Clear()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Cache cleared"))
}

func (s *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.stats.Requests, 1)

	if strings.HasPrefix(r.URL.Path, "/stats") {
		s.handleStats(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/health") {
		s.handleHealth(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/clear-cache") {
		s.handleClearCache(w, r)
		return
	}

	s.forwardRequest(w, r)
}

func getIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	listenAddr := flag.String("listen", ":8080", "Listen address")
	nextChatURL := flag.String("backend", "http://localhost:3000", "NextChat backend URL")
	apiKey := flag.String("apikey", "", "API key for authentication")
	cacheDir := flag.String("cachedir", "./cache", "Cache directory")
	cacheTTL := flag.Duration("ttl", 10*time.Minute, "Cache TTL")
	poolSize := flag.Int("pool", 100, "Connection pool size")
	maxRetries := flag.Int("retries", 3, "Max retries")
	flag.Parse()

	cfg.ListenAddr = *listenAddr
	cfg.NextChatURL = *nextChatURL
	cfg.ApiKey = *apiKey
	cfg.CacheDir = *cacheDir
	cfg.CacheTTL = *cacheTTL
	cfg.ConnPoolSize = *poolSize
	cfg.MaxRetries = *maxRetries

	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		log.Printf("Warning: Failed to create cache dir: %v", err)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	server := NewProxyServer()

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      server,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	log.Printf("🚀 NextChat Proxy Server starting on %s", cfg.ListenAddr)
	log.Printf("📡 Backend: %s", cfg.NextChatURL)
	log.Printf("⚡ Connection Pool: %d", cfg.ConnPoolSize)
	log.Printf("🔄 Max Retries: %d", cfg.MaxRetries)
	log.Printf("💾 Cache TTL: %s", cfg.CacheTTL)
	log.Printf("")
	log.Printf("Endpoints:")
	log.Printf("  Proxy:     %s/*", cfg.ListenAddr)
	log.Printf("  Stats:     %s/stats", cfg.ListenAddr)
	log.Printf("  Health:    %s/health", cfg.ListenAddr)
	log.Printf("  Clear:     %s/clear-cache", cfg.ListenAddr)

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
