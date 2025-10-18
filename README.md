# CTO2API - Go版本

将CTO.NEW的AI服务转换为OpenAI兼容的API接口，提供Web管理界面。

## 功能特点

- ✅ OpenAI兼容的API接口 (`/v1/chat/completions`)
- ✅ 支持流式和非流式响应
- ✅ Web管理界面
- ✅ Cookie轮询机制（自动负载均衡）
- ✅ Cookie统计（请求次数、错误次数、最近使用时间）
- ✅ Cookie启用/禁用功能
- ✅ API密钥验证
- ✅ 管理密码保护
- ✅ 数据持久化（JSON文件）

## 项目结构

```
cto2api/
├── main.go              # 主程序入口
├── go.mod              # Go模块依赖
├── data.json           # 数据存储（自动生成）
├── config/
│   └── config.go       # 配置管理
├── models/
│   └── cookie.go       # 数据模型和存储
├── services/
│   └── cto_client.go   # CTO.NEW客户端
├── handlers/
│   └── api.go          # API处理器
└── web/
    └── index.html      # 管理前端
```

## 快速开始

### 1. 安装依赖

```bash
go mod download
```

### 2. 编译运行

```bash
# 直接运行
go run main.go

# 或编译后运行
go build -o cto2api
./cto2api  # Windows: cto2api.exe
```

### 3. 初始设置

首次访问 `http://localhost:8000/admin` 会要求设置：

1. **管理密码**：用于登录管理界面
2. **API密钥**：用于验证API请求（类似OpenAI的API Key）

### 4. 添加Cookie

1. 登录管理界面
2. 在"Cookie管理"区域添加Cookie
3. Cookie获取方法：
   - 登录 https://cto.new
   - 打开浏览器开发者工具（F12）
   - 找到请求 `https://clerk.cto.new/v1/client/sessions/...`
   - 复制请求头中的完整Cookie（以`__client=`开头）

### 5. 使用API

使用你设置的API密钥调用接口：

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

## API接口

### OpenAI兼容接口

#### 聊天完成
```
POST /v1/chat/completions
Authorization: Bearer YOUR_API_KEY
```

支持的模型：
- `gpt-5` - GPT5
- `claude-sonnet-4-5` - Claude Sonnet 4.5

#### 列出模型
```
GET /v1/models
```

### 管理接口

#### 检查设置状态
```
GET /api/admin/check-setup
```

#### 初始设置
```
POST /api/admin/setup
{
  "password": "管理密码",
  "api_key": "API密钥"
}
```

#### 登录
```
POST /api/admin/login
{
  "password": "管理密码"
}
```

#### Cookie管理
```
GET    /api/admin/cookies          # 列出所有Cookie
POST   /api/admin/cookies          # 添加Cookie
PUT    /api/admin/cookies/:id      # 更新Cookie
DELETE /api/admin/cookies/:id      # 删除Cookie
```

#### API密钥管理
```
GET /api/admin/api-key              # 获取当前API密钥
PUT /api/admin/api-key              # 更新API密钥
```

## 数据存储

所有数据保存在 `data.json` 文件中，包括：
- 管理密码（bcrypt加密）
- API密钥
- 所有Cookie及其统计信息

数据格式：
```json
{
  "password_hash": "bcrypt哈希",
  "api_key": "你的API密钥",
  "cookies": [
    {
      "id": "uuid",
      "name": "Cookie名称",
      "cookie": "完整Cookie字符串",
      "enabled": true,
      "request_count": 100,
      "error_count": 2,
      "last_used_at": "2024-01-01T00:00:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

## Cookie轮询机制

- 自动轮询所有启用的Cookie
- 记录每个Cookie的使用统计
- 自动跳过禁用的Cookie
- 支持动态添加/删除Cookie（无需重启）

## 配置

默认配置：
- 端口：8000
- 数据文件：data.json

可以通过修改 `config/config.go` 自定义配置。

## 注意事项

1. **Cookie安全**：Cookie包含敏感信息，请妥善保管 `data.json` 文件
2. **API密钥**：建议使用强密钥，并定期更换
3. **管理密码**：首次设置后无法通过界面修改，如需修改请删除 `data.json` 重新设置
4. **Cookie有效期**：Cookie可能会过期，需要定期更新

## 与Python版本的区别

1. **性能更好**：Go的并发性能优于Python
2. **单文件部署**：编译后只需一个可执行文件
3. **Web管理界面**：无需手动编辑文件
4. **统计功能**：自动记录Cookie使用情况
5. **API密钥验证**：增加了安全性

## 开发

### 添加新功能

1. 修改 `models/cookie.go` 添加数据模型
2. 在 `handlers/api.go` 添加API处理器
3. 在 `web/index.html` 添加前端界面

### 调试

```bash
# 启用详细日志
GIN_MODE=debug go run main.go
```

## 许可证

本项目仅供个人学习使用。

## 常见问题

**Q: Cookie从哪里获取？**  
A: 登录cto.new后，在浏览器开发者工具的Network标签中找到clerk.cto.new的请求，复制Cookie请求头。

**Q: API密钥忘记了怎么办？**  
A: 在管理界面可以查看和更新API密钥。

**Q: 如何重置所有设置？**  
A: 删除 `data.json` 文件，重启程序即可重新设置。

**Q: 支持多少个Cookie？**  
A: 理论上无限制，建议3-5个即可实现良好的负载均衡。

**Q: Cookie失效了怎么办？**  
A: 在管理界面更新或删除失效的Cookie，添加新的Cookie。