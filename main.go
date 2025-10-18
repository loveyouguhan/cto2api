package main

import (
	"cto2api/handlers"
	"cto2api/models"
	"embed"
	"io/fs"
	"log"
	"net/http"

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
		admin.GET("/api-key", apiHandler.GetAPIKey)
		admin.PUT("/api-key", apiHandler.UpdateAPIKey)
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

	// 启动服务器
	log.Println("服务器启动在 http://localhost:8000")
	log.Println("管理页面: http://localhost:8000/admin")
	log.Println("API端点: http://localhost:8000/v1/chat/completions")
	
	if err := r.Run(":8000"); err != nil {
		log.Fatal(err)
	}
}