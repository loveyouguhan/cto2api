package main

import (
	"cto2api/config"
	"cto2api/handlers"
	"cto2api/models"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

//go:embed web/*
var webFS embed.FS

func main() {
	// 初始化数据存储
	store := models.GetStore("data.json")

	// 创建Gin引擎
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 创建API处理器
	apiHandler := handlers.NewAPIHandler(store)

	// 静态文件服务（管理前端）
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	r.StaticFS("/admin", http.FS(webContent))

	// 管理API路由
	admin := r.Group("/api/admin")
	{
		admin.GET("/check-setup", apiHandler.CheckSetup)
		admin.POST("/setup", apiHandler.Setup)
		admin.POST("/login", apiHandler.Login)

		// 需要认证的路由（简化版，实际应该使用中间件）
		admin.GET("/cookies", apiHandler.ListCookies)
		admin.POST("/cookies", apiHandler.AddCookie)
		admin.PUT("/cookies/:id", apiHandler.UpdateCookie)
		admin.DELETE("/cookies/:id", apiHandler.DeleteCookie)
		admin.POST("/cookies/:id/test", apiHandler.TestCookie)
		admin.GET("/cookies/:id/usage", apiHandler.GetCookieUsage)
		admin.GET("/api-key", apiHandler.GetAPIKey)
		admin.PUT("/api-key", apiHandler.UpdateAPIKey)
		admin.GET("/usage", apiHandler.GetUsage)
	}

	// OpenAI兼容API路由
	v1 := r.Group("/v1")
	{
		v1.GET("/models", apiHandler.ListModels)
		v1.POST("/chat/completions", apiHandler.ChatCompletions)
	}

	// 根路径
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "CTO2API Server",
			"admin":   "/admin",
			"api":     "/v1/chat/completions",
		})
	})

	// 加载配置
	cfg := config.Load()

	// 获取服务器URL（用于日志显示）
	serverURL := getServerURL(cfg.Port)

	// 启动服务器
	log.Println("============================================================")
	log.Printf("服务器启动在 %s", serverURL)
	log.Printf("管理页面: %s/admin", serverURL)
	log.Printf("API端点: %s/v1/chat/completions", serverURL)
	log.Println("============================================================")

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// getServerURL 获取服务器URL用于日志显示
func getServerURL(port int) string {
	// 优先使用 Zeabur 提供的域名
	if zeaburURL := os.Getenv("ZEABUR_URL"); zeaburURL != "" {
		return "https://" + zeaburURL
	}

	// 其他云平台的域名环境变量
	if renderURL := os.Getenv("RENDER_EXTERNAL_URL"); renderURL != "" {
		return renderURL
	}

	if railwayURL := os.Getenv("RAILWAY_STATIC_URL"); railwayURL != "" {
		return "https://" + railwayURL
	}

	// 默认使用本地地址
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}
