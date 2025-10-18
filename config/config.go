package config

import (
	"encoding/json"
	"os"
	"sync"
)

// Config 应用配置
type Config struct {
	Port         int    `json:"port"`
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
			Port:     8000,
			DataFile: "data.json",
		}
		
		// 尝试从文件加载
		if data, err := os.ReadFile("config.json"); err == nil {
			json.Unmarshal(data, cfg)
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