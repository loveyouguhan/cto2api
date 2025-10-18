package services

import (
	"sync"
	"time"
)

// UsageManager 用量管理器
type UsageManager struct {
	mu            sync.RWMutex
	billingInfo   *BillingInfo
	lastUpdate    time.Time
	updateInterval time.Duration
	updating      bool
}

// NewUsageManager 创建用量管理器
func NewUsageManager() *UsageManager {
	return &UsageManager{
		updateInterval: 5 * time.Minute, // 5分钟更新一次
	}
}

// GetBillingInfo 获取用量信息（带缓存）
func (m *UsageManager) GetBillingInfo(client *CTOClient, jwt string) (*BillingInfo, error) {
	m.mu.RLock()
	// 如果缓存有效，直接返回
	if m.billingInfo != nil && time.Since(m.lastUpdate) < m.updateInterval {
		info := m.billingInfo
		m.mu.RUnlock()
		return info, nil
	}
	m.mu.RUnlock()

	// 需要更新
	m.mu.Lock()
	// 双重检查，避免重复更新
	if m.billingInfo != nil && time.Since(m.lastUpdate) < m.updateInterval {
		info := m.billingInfo
		m.mu.Unlock()
		return info, nil
	}

	// 如果正在更新，返回旧数据（如果有）
	if m.updating {
		info := m.billingInfo
		m.mu.Unlock()
		if info != nil {
			return info, nil
		}
		// 如果没有旧数据，等待更新完成
		time.Sleep(100 * time.Millisecond)
		return m.GetBillingInfo(client, jwt)
	}

	m.updating = true
	m.mu.Unlock()

	// 执行更新
	info, err := client.GetBillingInfo(jwt)
	
	m.mu.Lock()
	m.updating = false
	if err == nil {
		m.billingInfo = info
		m.lastUpdate = time.Now()
	}
	m.mu.Unlock()

	return info, err
}

// RefreshAsync 异步刷新用量信息
func (m *UsageManager) RefreshAsync(client *CTOClient, jwt string) {
	go func() {
		m.mu.RLock()
		if m.updating {
			m.mu.RUnlock()
			return
		}
		m.mu.RUnlock()

		m.mu.Lock()
		m.updating = true
		m.mu.Unlock()

		info, err := client.GetBillingInfo(jwt)
		
		m.mu.Lock()
		m.updating = false
		if err == nil {
			m.billingInfo = info
			m.lastUpdate = time.Now()
		}
		m.mu.Unlock()
	}()
}

// GetCached 获取缓存的用量信息（不触发更新）
func (m *UsageManager) GetCached() *BillingInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.billingInfo
}

// IsStale 检查缓存是否过期
func (m *UsageManager) IsStale() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.billingInfo == nil || time.Since(m.lastUpdate) >= m.updateInterval
}