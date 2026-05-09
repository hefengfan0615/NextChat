package config

import (
	"fmt"
	"github.com/spf13/viper"
	"strings"
)

type ProxyConfig struct {
	Port         string                 `mapstructure:"port"`
	Host         string                 `mapstructure:"host"`
	ReadTimeout  int                    `mapstructure:"read_timeout"`
	WriteTimeout int                    `mapstructure:"write_timeout"`
	Providers    map[string]Provider    `mapstructure:"providers"`
	Rules        []RoutingRule          `mapstructure:"rules"`
}

type Provider struct {
	Name         string   `mapstructure:"name"`
	BaseURL      string   `mapstructure:"base_url"`
	APIKeyEnv    string   `mapstructure:"api_key_env"`
	Timeout      int      `mapstructure:"timeout"`
	Weight       int      `mapstructure:"weight"`
	Enabled      bool     `mapstructure:"enabled"`
	Regions      []string `mapstructure:"regions"`
	Capabilities []string `mapstructure:"capabilities"`
}

type RoutingRule struct {
	PathPrefix  string   `mapstructure:"path_prefix"`
	Provider    string   `mapstructure:"provider"`
	LoadBalance string   `mapstructure:"load_balance"`
	Regions     []string `mapstructure:"regions"`
	Priority    int      `mapstructure:"priority"`
}

type Config struct {
	Proxy ProxyConfig `mapstructure:"proxy"`
	Health HealthConfig `mapstructure:"health"`
	Log   LogConfig   `mapstructure:"log"`
}

type HealthConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	CheckInterval  int  `mapstructure:"check_interval"`
	Timeout        int  `mapstructure:"timeout"`
	MaxRetries     int  `mapstructure:"max_retries"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	OutputPath string `mapstructure:"output_path"`
}

var AppConfig *Config

func LoadConfig(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("配置文件读取失败: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}

	if err := validateConfig(AppConfig); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	return nil
}

func validateConfig(cfg *Config) error {
	if cfg.Proxy.Port == "" {
		cfg.Proxy.Port = "8080"
	}
	if cfg.Proxy.Host == "" {
		cfg.Proxy.Host = "0.0.0.0"
	}
	if cfg.Proxy.ReadTimeout == 0 {
		cfg.Proxy.ReadTimeout = 30
	}
	if cfg.Proxy.WriteTimeout == 0 {
		cfg.Proxy.WriteTimeout = 60
	}
	if cfg.Health.CheckInterval == 0 {
		cfg.Health.CheckInterval = 30
	}
	if cfg.Health.Timeout == 0 {
		cfg.Health.Timeout = 5
	}
	if cfg.Health.MaxRetries == 0 {
		cfg.Health.MaxRetries = 3
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}

	return nil
}

func GetProviderByName(name string) (*Provider, error) {
	if provider, ok := AppConfig.Proxy.Providers[name]; ok {
		return &provider, nil
	}
	return nil, fmt.Errorf("未找到提供商: %s", name)
}

func GetMatchingRule(path string, region string) *RoutingRule {
	var bestRule *RoutingRule
	highestPriority := -1

	for i := range AppConfig.Proxy.Rules {
		rule := &AppConfig.Proxy.Rules[i]
		
		if !strings.HasPrefix(path, rule.PathPrefix) {
			continue
		}

		if len(rule.Regions) > 0 {
			found := false
			for _, r := range rule.Regions {
				if strings.EqualFold(r, region) || r == "*" {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if rule.Priority > highestPriority {
			highestPriority = rule.Priority
			bestRule = rule
		}
	}

	return bestRule
}
