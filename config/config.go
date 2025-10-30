package config

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"
)

// Config 应用配置
type Config struct {
	Port         int    `json:"port"`
	Host         string `json:"host"`
	DataFile     string `json:"data_file"`
	PasswordHash string `json:"password_hash"` // bcrypt hash
}

var (
	cfg  *Config
	once sync.Once
)

// Load 加载配置
func Load() *Config {
	once.Do(func() {
		cfg = &Config{
			Port:     7032,
			Host:     "0.0.0.0",
			DataFile: "data.json",
		}

		// 尝试从文件加载
		if data, err := os.ReadFile("config.json"); err == nil {
			json.Unmarshal(data, cfg)
		}

		// 从环境变量读取端口（优先级最高）
		if portStr := os.Getenv("PORT"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				cfg.Port = port
			}
		}

		// 从环境变量读取主机地址
		if host := os.Getenv("HOST"); host != "" {
			cfg.Host = host
		}
	})
	return cfg
}

// Save 保存配置
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("config.json", data, 0644)
}

// Get 获取配置实例
func Get() *Config {
	if cfg == nil {
		return Load()
	}
	return cfg
}