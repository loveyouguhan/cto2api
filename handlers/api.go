package handlers

import (
	"cto2api/models"
	"cto2api/services"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// APIHandler API处理器
type APIHandler struct {
	store        *models.DataStore
	usageManager *services.UsageManager
}

// NewAPIHandler 创建API处理器
func NewAPIHandler(store *models.DataStore) *APIHandler {
	return &APIHandler{
		store:        store,
		usageManager: services.NewUsageManager(),
	}
}

// Message 消息结构
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice 选择项
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage 使用统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []StreamDelta `json:"choices"`
}

// StreamDelta 流式增量
type StreamDelta struct {
	Index        int          `json:"index"`
	Delta        DeltaContent `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

// DeltaContent 增量内容
type DeltaContent struct {
	Content string `json:"content,omitempty"`
}

// 模型映射
var modelMapping = map[string]string{
	"gpt-5":             "GPT5",
	"claude-sonnet-4-5": "ClaudeSonnet4_5",
}

// ChatCompletions 聊天完成接口
func (h *APIHandler) ChatCompletions(c *gin.Context) {
	// 验证API密钥
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少Authorization头"})
		return
	}

	// 提取Bearer token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的Authorization格式"})
		return
	}

	apiKey := parts[1]
	expectedKey := h.store.GetAPIKey()
	
	if expectedKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "API密钥未设置，请先在管理页面设置"})
		return
	}

	if apiKey != expectedKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的API密钥"})
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取可用的cookie
	cookieInfo := h.store.GetNextCookie()
	if cookieInfo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "没有可用的Cookie"})
		return
	}

	// 提取用户消息
	var prompt string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			prompt = req.Messages[i].Content
			break
		}
	}

	if prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有找到用户消息"})
		return
	}

	// 创建CTO客户端
	client := services.NewCTOClient(cookieInfo.Cookie)

	// 获取认证信息
	clerkInfo, err := client.GetClerkInfo()
	if err != nil {
		h.store.RecordError(cookieInfo.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取认证信息失败: " + err.Error()})
		return
	}

	jwt, err := client.GetJWT(clerkInfo.SessionID)
	if err != nil {
		h.store.RecordError(cookieInfo.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取JWT失败: " + err.Error()})
		return
	}

	// 确定adapter
	adapter := modelMapping[req.Model]
	if adapter == "" {
		adapter = "ClaudeSonnet4_5"
	}

	// 创建聊天
	chatID := uuid.New().String()
	if err := client.CreateChat(jwt, prompt, adapter, chatID); err != nil {
		h.store.RecordError(cookieInfo.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建聊天失败: " + err.Error()})
		return
	}

	// 流式响应
	if req.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		responseChan := make(chan services.StreamResponse, 100)
		go client.StreamChat(chatID, clerkInfo.UserID, responseChan)

		for resp := range responseChan {
			if resp.Error != nil {
				h.store.RecordError(cookieInfo.ID)
				break
			}

			if resp.Done {
				chunk := StreamChunk{
					ID:      "chatcmpl-" + chatID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []StreamDelta{{
						Index:        0,
						Delta:        DeltaContent{},
						FinishReason: stringPtr("stop"),
					}},
				}
				c.SSEvent("", chunk)
				c.SSEvent("", "[DONE]")
				break
			}

			if resp.Content != "" {
				chunk := StreamChunk{
					ID:      "chatcmpl-" + chatID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []StreamDelta{{
						Index: 0,
						Delta: DeltaContent{Content: resp.Content},
					}},
				}
				c.SSEvent("", chunk)
			}
		}
		return
	}

	// 非流式响应
	fullResponse, err := client.GetFullResponse(chatID, clerkInfo.UserID)
	if err != nil {
		h.store.RecordError(cookieInfo.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取响应失败: " + err.Error()})
		return
	}

	response := ChatResponse{
		ID:      "chatcmpl-" + chatID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Role: "assistant", Content: fullResponse},
			FinishReason: "stop",
		}},
		Usage: Usage{
			PromptTokens:     len(prompt) / 4,
			CompletionTokens: len(fullResponse) / 4,
			TotalTokens:      (len(prompt) + len(fullResponse)) / 4,
		},
	}

	c.JSON(http.StatusOK, response)
}

// ListModels 列出模型
func (h *APIHandler) ListModels(c *gin.Context) {
	models := []gin.H{}
	for model := range modelMapping {
		models = append(models, gin.H{
			"id":       model,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "cto-new",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

// SetupRequest 初始设置请求
type SetupRequest struct {
	Password string `json:"password" binding:"required"`
	APIKey   string `json:"api_key" binding:"required"`
}

// Setup 首次设置密码和API密钥
func (h *APIHandler) Setup(c *gin.Context) {
	// 检查是否已设置密码
	if h.store.GetPasswordHash() != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "已完成初始设置"})
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 生成密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	if err := h.store.SetPasswordHash(string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存密码失败"})
		return
	}

	if err := h.store.SetAPIKey(req.APIKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存API密钥失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "设置成功"})
}

// LoginRequest 登录请求
type LoginRequest struct {
	Password string `json:"password" binding:"required"`
}

// Login 登录验证
func (h *APIHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash := h.store.GetPasswordHash()
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先完成初始设置"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}

	// 生成简单的token（实际项目中应使用JWT）
	token := uuid.New().String()
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// CheckSetup 检查是否已完成初始设置
func (h *APIHandler) CheckSetup(c *gin.Context) {
	hasPassword := h.store.GetPasswordHash() != ""
	c.JSON(http.StatusOK, gin.H{"setup_completed": hasPassword})
}

// UpdateAPIKeyRequest 更新API密钥请求
type UpdateAPIKeyRequest struct {
	APIKey string `json:"api_key" binding:"required"`
}

// UpdateAPIKey 更新API密钥
func (h *APIHandler) UpdateAPIKey(c *gin.Context) {
	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.SetAPIKey(req.APIKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API密钥更新成功"})
}

// GetAPIKey 获取当前API密钥（仅显示部分）
func (h *APIHandler) GetAPIKey(c *gin.Context) {
	key := h.store.GetAPIKey()
	if key == "" {
		c.JSON(http.StatusOK, gin.H{"api_key": ""})
		return
	}

	// 只显示前8位和后4位
	masked := key
	if len(key) > 12 {
		masked = key[:8] + "..." + key[len(key)-4:]
	}

	c.JSON(http.StatusOK, gin.H{"api_key": masked, "full_key": key})
}

// AddCookieRequest 添加Cookie请求
type AddCookieRequest struct {
	Name   string `json:"name" binding:"required"`
	Cookie string `json:"cookie" binding:"required"`
}

// AddCookie 添加Cookie
func (h *APIHandler) AddCookie(c *gin.Context) {
	var req AddCookieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cookie := &models.CookieInfo{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Cookie:    req.Cookie,
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	if err := h.store.AddCookie(cookie); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "添加失败"})
		return
	}

	c.JSON(http.StatusOK, cookie)
}

// UpdateCookieRequest 更新Cookie请求
type UpdateCookieRequest struct {
	Name    *string `json:"name"`
	Cookie  *string `json:"cookie"`
	Enabled *bool   `json:"enabled"`
}

// UpdateCookie 更新Cookie
func (h *APIHandler) UpdateCookie(c *gin.Context) {
	id := c.Param("id")
	
	var req UpdateCookieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Cookie != nil {
		updates["cookie"] = *req.Cookie
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.store.UpdateCookie(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteCookie 删除Cookie
func (h *APIHandler) DeleteCookie(c *gin.Context) {
	id := c.Param("id")
	
	if err := h.store.DeleteCookie(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// ListCookies 列出所有Cookie（带用量信息）
func (h *APIHandler) ListCookies(c *gin.Context) {
	cookies := h.store.ListCookies()
	
	// 异步获取每个Cookie的用量信息
	for _, cookie := range cookies {
		if cookie.Enabled {
			go h.fetchCookieUsage(cookie)
		}
	}
	
	c.JSON(http.StatusOK, cookies)
}

// fetchCookieUsage 异步获取Cookie用量信息
func (h *APIHandler) fetchCookieUsage(cookie *models.CookieInfo) {
	client := services.NewCTOClient(cookie.Cookie)
	
	clerkInfo, err := client.GetClerkInfo()
	if err != nil {
		return
	}
	
	jwt, err := client.GetJWT(clerkInfo.SessionID)
	if err != nil {
		return
	}
	
	billing, err := h.usageManager.GetBillingInfo(client, jwt)
	if err != nil {
		return
	}
	
	// 更新Cookie的用量信息（不保存到文件）
	cookie.Usage = &models.UsageInfo{
		TaskCreditsUsage:     billing.TaskCreditsUsage,
		TaskCreditsLimit:     billing.TaskCreditsLimit,
		TaskConcurrencyUsage: billing.TaskConcurrencyUsage,
		TaskConcurrencyLimit: billing.TaskConcurrencyLimit,
		LastUpdate:           time.Now().Format("2006-01-02 15:04:05"),
	}
}

// TestCookie 测试Cookie连通性
func (h *APIHandler) TestCookie(c *gin.Context) {
	id := c.Param("id")
	
	// 获取Cookie信息
	cookieInfo := h.store.GetCookie(id)
	if cookieInfo == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cookie不存在"})
		return
	}

	// 创建CTO客户端
	client := services.NewCTOClient(cookieInfo.Cookie)

	// 测试获取认证信息
	clerkInfo, err := client.GetClerkInfo()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取认证信息失败: " + err.Error(),
		})
		return
	}

	// 测试获取JWT
	jwt, err := client.GetJWT(clerkInfo.SessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取JWT失败: " + err.Error(),
		})
		return
	}

	// 如果能获取到JWT，说明Cookie有效
	if jwt != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Cookie有效，连接正常",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "Cookie可能已失效",
	})
}

// GetUsage 获取用量信息（总览）
func (h *APIHandler) GetUsage(c *gin.Context) {
	// 获取任意一个可用的Cookie来获取用量信息
	cookieInfo := h.store.GetNextCookie()
	if cookieInfo == nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "没有可用的Cookie，无法获取用量信息",
		})
		return
	}

	// 创建CTO客户端
	client := services.NewCTOClient(cookieInfo.Cookie)

	// 获取认证信息
	clerkInfo, err := client.GetClerkInfo()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "获取认证信息失败: " + err.Error(),
		})
		return
	}

	jwt, err := client.GetJWT(clerkInfo.SessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "获取JWT失败: " + err.Error(),
		})
		return
	}

	// 使用缓存机制获取用量信息
	billing, err := h.usageManager.GetBillingInfo(client, jwt)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "获取用量信息失败: " + err.Error(),
		})
		return
	}

	// 异步刷新（如果缓存快过期）
	if h.usageManager.IsStale() {
		go h.usageManager.RefreshAsync(client, jwt)
	}

	c.JSON(http.StatusOK, billing)
}

// GetCookieUsage 获取指定Cookie的用量信息
func (h *APIHandler) GetCookieUsage(c *gin.Context) {
	id := c.Param("id")
	
	// 获取Cookie信息
	cookieInfo := h.store.GetCookie(id)
	if cookieInfo == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cookie不存在"})
		return
	}

	// 创建CTO客户端
	client := services.NewCTOClient(cookieInfo.Cookie)

	// 获取认证信息
	clerkInfo, err := client.GetClerkInfo()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "获取认证信息失败: " + err.Error(),
		})
		return
	}

	jwt, err := client.GetJWT(clerkInfo.SessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "获取JWT失败: " + err.Error(),
		})
		return
	}

	// 获取用量信息
	billing, err := client.GetBillingInfo(jwt)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "获取用量信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, billing)
}

func stringPtr(s string) *string {
	return &s
}