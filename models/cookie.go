package models

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// CookieInfo Cookie信息
type CookieInfo struct {
	ID           string    `json:"id"`
	Cookie       string    `json:"cookie"`
	Name         string    `json:"name"`          // 用户自定义名称
	Enabled      bool      `json:"enabled"`       // 是否启用
	RequestCount int       `json:"request_count"` // 请求次数
	ErrorCount   int       `json:"error_count"`   // 错误次数
	LastUsedAt   time.Time `json:"last_used_at"`  // 最近使用时间
	CreatedAt    time.Time `json:"created_at"`    // 创建时间
	Usage        *UsageInfo `json:"usage,omitempty"` // 用量信息（不保存到文件）
}

// UsageInfo 用量信息（临时数据，不保存）
type UsageInfo struct {
	TaskCreditsUsage     int    `json:"task_credits_usage"`
	TaskCreditsLimit     int    `json:"task_credits_limit"`
	TaskConcurrencyUsage int    `json:"task_concurrency_usage"`
	TaskConcurrencyLimit int    `json:"task_concurrency_limit"`
	LastUpdate           string `json:"last_update"`
}

// AppData 应用数据（包含密码、API密钥和所有cookie）
type AppData struct {
	PasswordHash string        `json:"password_hash"` // bcrypt hash
	APIKey       string        `json:"api_key"`       // OpenAI API密钥
	Cookies      []*CookieInfo `json:"cookies"`
}

// DataStore 数据存储
type DataStore struct {
	mu           sync.RWMutex
	data         *AppData
	cookies      map[string]*CookieInfo
	enabledList  []string // 启用的cookie ID列表
	currentIndex int
	dataFile     string
}

var (
	store *DataStore
	once  sync.Once
)

// GetStore 获取数据存储单例
func GetStore(dataFile string) *DataStore {
	once.Do(func() {
		store = &DataStore{
			data: &AppData{
				Cookies: []*CookieInfo{},
			},
			cookies:  make(map[string]*CookieInfo),
			dataFile: dataFile,
		}
		store.Load()
	})
	return store
}

// Load 从文件加载数据
func (s *DataStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在，使用空数据
		}
		return err
	}

	if err := json.Unmarshal(data, s.data); err != nil {
		return err
	}

	// 重建索引
	s.cookies = make(map[string]*CookieInfo)
	s.enabledList = []string{}
	
	for _, c := range s.data.Cookies {
		s.cookies[c.ID] = c
		if c.Enabled {
			s.enabledList = append(s.enabledList, c.ID)
		}
	}

	return nil
}

// Save 保存数据到文件
func (s *DataStore) save() error {
	// 更新cookies列表
	s.data.Cookies = make([]*CookieInfo, 0, len(s.cookies))
	for _, c := range s.cookies {
		s.data.Cookies = append(s.data.Cookies, c)
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.dataFile, data, 0644)
}

// GetPasswordHash 获取密码哈希
func (s *DataStore) GetPasswordHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.PasswordHash
}

// SetPasswordHash 设置密码哈希
func (s *DataStore) SetPasswordHash(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.PasswordHash = hash
	return s.save()
}

// GetAPIKey 获取API密钥
func (s *DataStore) GetAPIKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.APIKey
}

// SetAPIKey 设置API密钥
func (s *DataStore) SetAPIKey(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.APIKey = key
	return s.save()
}

// AddCookie 添加Cookie
func (s *DataStore) AddCookie(cookie *CookieInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cookies[cookie.ID] = cookie
	if cookie.Enabled {
		s.enabledList = append(s.enabledList, cookie.ID)
	}

	return s.save()
}

// UpdateCookie 更新Cookie
func (s *DataStore) UpdateCookie(id string, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cookie, exists := s.cookies[id]
	if !exists {
		return nil
	}

	// 更新字段
	if name, ok := updates["name"].(string); ok {
		cookie.Name = name
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		oldEnabled := cookie.Enabled
		cookie.Enabled = enabled
		
		// 更新启用列表
		if enabled && !oldEnabled {
			s.enabledList = append(s.enabledList, id)
		} else if !enabled && oldEnabled {
			s.removeFromEnabledList(id)
		}
	}
	if cookieStr, ok := updates["cookie"].(string); ok {
		cookie.Cookie = cookieStr
	}

	return s.save()
}

// DeleteCookie 删除Cookie
func (s *DataStore) DeleteCookie(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cookie, exists := s.cookies[id]; exists {
		if cookie.Enabled {
			s.removeFromEnabledList(id)
		}
		delete(s.cookies, id)
	}

	return s.save()
}

// GetNextCookie 获取下一个可用的Cookie（轮询）
func (s *DataStore) GetNextCookie() *CookieInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.enabledList) == 0 {
		return nil
	}

	id := s.enabledList[s.currentIndex]
	s.currentIndex = (s.currentIndex + 1) % len(s.enabledList)

	cookie := s.cookies[id]
	cookie.RequestCount++
	cookie.LastUsedAt = time.Now()

	// 异步保存，避免阻塞
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.save()
	}()

	return cookie
}

// RecordError 记录错误
func (s *DataStore) RecordError(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cookie, exists := s.cookies[id]; exists {
		cookie.ErrorCount++
		go func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.save()
		}()
	}
}

// ListCookies 获取所有Cookie列表
func (s *DataStore) ListCookies() []*CookieInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*CookieInfo, 0, len(s.cookies))
	for _, c := range s.cookies {
		result = append(result, c)
	}
	return result
}

// GetCookie 获取指定Cookie
func (s *DataStore) GetCookie(id string) *CookieInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cookies[id]
}

// removeFromEnabledList 从启用列表中移除
func (s *DataStore) removeFromEnabledList(id string) {
	for i, cid := range s.enabledList {
		if cid == id {
			s.enabledList = append(s.enabledList[:i], s.enabledList[i+1:]...)
			break
		}
	}
}