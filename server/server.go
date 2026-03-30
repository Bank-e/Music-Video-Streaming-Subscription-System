package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	
	// สมมติฐานการ import ตามโฟลเดอร์โปรเจกต์
	// "streaming-subscription-system/auth/service"
	// "streaming-subscription-system/identity/handler"
	// "streaming-subscription-system/identity/repository"
	// ...
)

func main() {
	// สร้าง Gin engine
	r := gin.Default()

	// ------------------------------------------------------------------------
	// 1. Dependency Injection (ประกอบร่าง Repository -> Service -> Controller)
	// ------------------------------------------------------------------------
	// สมมติการเชื่อมต่อ Database
	// db := setupDatabase()

	// -- Layer: Repository --
	// userRepo := repository.NewUserRepository(db)
	// sessionRepo := repository.NewSessionRepository(db)
	// ...

	// -- Layer: Service --
	// authService := service.NewAuthService(sessionRepo, userRepo)
	// userService := service.NewUserService(userRepo)
	// ...

	// -- Layer: Controller (Handler) --
	// userHandler := handler.NewUserController(userService)
	// authHandler := handler.NewAuthController(authService)
	// ...

	// ------------------------------------------------------------------------
	// 2. Middleware สำหรับตรวจสอบ Token
	// ------------------------------------------------------------------------
	authMiddleware := func() gin.HandlerFunc {
		return func(c *gin.Context) {
			path := c.Request.URL.Path
			method := c.Request.Method

			// ข้อยกเว้น: 3 Endpoint ที่ไม่ต้องใช้ Authorization Header
			isBypassRoute := (path == "/api/identity/users" && method == "POST") ||
				(path == "/api/auth/login" && method == "POST") ||
				(path == "/api/billing/payments/webhook" && method == "POST")

			if isBypassRoute {
				c.Next() // ปล่อยผ่าน
				return
			}

			// ดึง Token จาก Header
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
				c.Abort()
				return
			}

			// ตัดคำว่า "Bearer " ออกเพื่อเอาแค่ token
			// token := strings.TrimPrefix(authHeader, "Bearer ")

			// ตรวจสอบ Token ผ่าน AuthService
			// principal, err := authService.Verify(token)
			// if err != nil {
			// 	c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Token expired or revoked"})
			// 	c.Abort()
			// 	return
			// }

			// ส่งข้อมูล user ผ่าน Context ไปให้ Controller ใช้งานต่อ
			// c.Set("userId", principal.UserID)

			c.Next()
		}
	}

	// ------------------------------------------------------------------------
	// 3. Setup Routes (ผูก Controller เข้ากับ Endpoints)
	// ------------------------------------------------------------------------
	api := r.Group("/api")
	api.Use(authMiddleware()) // เปิดใช้งาน Middleware กับทุกเส้นทางใน /api

	// -- Identity Routes --
	identity := api.Group("/identity")
	{
		identity.POST("/users", mockHandler)               // สมัครสมาชิกใหม่ (Bypass Token)
		identity.GET("/users/me", mockHandler)             // ดึงข้อมูลตัวเอง
		identity.PATCH("/users/me/email", mockHandler)     // เปลี่ยนอีเมล
		identity.PATCH("/users/me/password", mockHandler)  // เปลี่ยนรหัสผ่าน
		identity.GET("/users/me/profile", mockHandler)     // ดึงโปรไฟล์
		identity.PATCH("/users/me/profile", mockHandler)   // แก้ไขโปรไฟล์
		identity.GET("/users/me/preferences", mockHandler) // ดึง preference ทั้งหมด
	}

	// -- Auth Routes --
	auth := api.Group("/auth")
	{
		auth.POST("/login", mockHandler)   // ล็อกอิน (Bypass Token)
		auth.POST("/logout", mockHandler)  // ล็อกเอาท์
		auth.POST("/refresh", mockHandler) // ขอ token ใหม่
		auth.GET("/verify", mockHandler)   // ตรวจสอบ token
	}

	// -- Billing Routes --
	billing := api.Group("/billing")
	{
		billing.GET("/plans", mockHandler)
		billing.POST("/subscriptions", mockHandler)
		billing.POST("/payments/webhook", mockHandler) // Webhook (Bypass Token)
		// ... เพิ่มเส้นทาง billing อื่นๆ
	}

	// -- Content Routes --
	content := api.Group("/content")
	{
		content.GET("/contents", mockHandler)
		content.POST("/contents", mockHandler)
		content.POST("/watch-history", mockHandler)
		// ... เพิ่มเส้นทาง content อื่นๆ
	}

	// ------------------------------------------------------------------------
	// 4. Start Server
	// ------------------------------------------------------------------------
	log.Println("Server is starting on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// mockHandler ทำหน้าที่เป็น Placeholder ชั่วคราวก่อนที่คุณจะนำ Handler ของจริงมาใส่
func mockHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Endpoint is working"})
}