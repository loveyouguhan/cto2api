package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// ClerkInfo Clerk会话信息
type ClerkInfo struct {
	SessionID string
	UserID    string
}

// CTOClient CTO.NEW客户端
type CTOClient struct {
	cookie string
	client *http.Client
}

// NewCTOClient 创建客户端
func NewCTOClient(cookie string) *CTOClient {
	return &CTOClient{
		cookie: cookie,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetClerkInfo 获取Clerk会话信息
func (c *CTOClient) GetClerkInfo() (*ClerkInfo, error) {
	url := "https://clerk.cto.new/v1/me/organization_memberships?paginated=true&limit=10&offset=0&__clerk_api_version=2025-04-10&_clerk_js_version=5.102.0"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clerk API返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	client, ok := result["client"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("无效的响应格式")
	}

	sessionID, _ := client["last_active_session_id"].(string)
	
	sessions, _ := client["sessions"].([]interface{})
	if len(sessions) == 0 {
		return nil, fmt.Errorf("没有活动会话")
	}
	
	session, _ := sessions[0].(map[string]interface{})
	user, _ := session["user"].(map[string]interface{})
	userID, _ := user["id"].(string)

	return &ClerkInfo{
		SessionID: sessionID,
		UserID:    userID,
	}, nil
}

// GetJWT 获取JWT token
func (c *CTOClient) GetJWT(sessionID string) (string, error) {
	url := fmt.Sprintf("https://clerk.cto.new/v1/client/sessions/%s/tokens?__clerk_api_version=2025-04-10&_clerk_js_version=5.101.1", sessionID)
	
	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte{}))
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取JWT失败: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	jwt, _ := result["jwt"].(string)
	if jwt == "" {
		return "", fmt.Errorf("JWT为空")
	}

	return jwt, nil
}

// CreateChat 创建聊天会话
func (c *CTOClient) CreateChat(jwt, prompt, adapter, chatID string) error {
	url := "https://api.enginelabs.ai/engine-agent/chat"
	
	data := map[string]interface{}{
		"prompt":        prompt,
		"chatHistoryId": chatID,
		"adapterName":   adapter,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://cto.new")
	req.Header.Set("Referer", "https://cto.new")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	// 接受200和202状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("创建聊天失败: HTTP %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// StreamResponse 流式响应结构
type StreamResponse struct {
	Content      string
	Done         bool
	Error        error
}

// StreamChat 流式获取聊天响应
func (c *CTOClient) StreamChat(chatID, wsUserToken string, responseChan chan<- StreamResponse) {
	defer close(responseChan)

	wsURL := fmt.Sprintf("wss://api.enginelabs.ai/engine-agent/chat-histories/%s/buffer/stream?token=%s", chatID, wsUserToken)
	
	// 添加请求头
	headers := http.Header{}
	headers.Set("Origin", "https://cto.new")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	
	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}
	
	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		responseChan <- StreamResponse{Error: fmt.Errorf("WebSocket连接失败: %v", err)}
		return
	}
	defer conn.Close()

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// 如果是正常关闭，不报错
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				responseChan <- StreamResponse{Done: true}
				return
			}
			responseChan <- StreamResponse{Error: fmt.Errorf("读取WebSocket消息失败: %v", err)}
			return
		}

		// 重置读取超时
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		var data map[string]interface{}
		if err := json.Unmarshal(message, &data); err != nil {
			continue
		}

		// 处理更新消息
		if data["type"] == "update" {
			if buffer, ok := data["buffer"].(string); ok {
				var inner map[string]interface{}
				if err := json.Unmarshal([]byte(buffer), &inner); err != nil {
					continue
				}

				if inner["type"] == "chat" {
					if chat, ok := inner["chat"].(map[string]interface{}); ok {
						if content, ok := chat["content"].(string); ok && content != "" {
							responseChan <- StreamResponse{Content: content}
						}
					}
				}
			}
		}

		// 处理状态消息
		if data["type"] == "state" {
			if state, ok := data["state"].(map[string]interface{}); ok {
				if inProgress, ok := state["inProgress"].(bool); ok && !inProgress {
					responseChan <- StreamResponse{Done: true}
					return
				}
			}
		}
	}
}

// GetFullResponse 获取完整响应（非流式）
func (c *CTOClient) GetFullResponse(chatID, wsUserToken string) (string, error) {
	responseChan := make(chan StreamResponse, 100)
	go c.StreamChat(chatID, wsUserToken, responseChan)

	var fullResponse string
	for resp := range responseChan {
		if resp.Error != nil {
			return "", resp.Error
		}
		if resp.Done {
			break
		}
		fullResponse += resp.Content
	}

	return fullResponse, nil
}

// BillingInfo 用量信息
type BillingInfo struct {
	Active                        bool    `json:"active"`
	CurrentBillingTier            int     `json:"currentBillingTier"`
	TaskCreditsPeriod             string  `json:"taskCreditsPeriod"`
	TaskCreditsLimit              int     `json:"taskCreditsLimit"`
	TaskCreditsUsage              int     `json:"taskCreditsUsage"`
	TaskCreditsNonExpiringBalance int     `json:"taskCreditsNonExpiringBalance"`
	TaskConcurrencyLimit          int     `json:"taskConcurrencyLimit"`
	TaskConcurrencyUsage          int     `json:"taskConcurrencyUsage"`
	StartDate                     string  `json:"startDate"`
	Seats                         int     `json:"seats"`
	IsAnnualPlan                  bool    `json:"isAnnualPlan"`
	BillingPeriod                 *string `json:"billingPeriod"`
	PendingPlanChanges            *string `json:"pendingPlanChanges"`
	TotalPlanCost                 *string `json:"totalPlanCost"`
	NextInvoiceAmount             *string `json:"nextInvoiceAmount"`
}

// GetBillingInfo 获取用量信息
func (c *CTOClient) GetBillingInfo(jwt string) (*BillingInfo, error) {
	url := "https://api.enginelabs.ai/billing"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://cto.new")
	req.Header.Set("Referer", "https://cto.new")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取用量信息失败: HTTP %d, 响应: %s", resp.StatusCode, string(body))
	}

	var billing BillingInfo
	if err := json.NewDecoder(resp.Body).Decode(&billing); err != nil {
		return nil, err
	}

	return &billing, nil
}