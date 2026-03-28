# Example Project Structure
``` text
student-api/
├── cmd/
│   └── server/
│       └── main.go          # จุดเริ่มต้น (Entry Point) และการทำ Dependency Injection
├── internal/
│   └── student/             # Student Domain
│       ├── handler.go       # รับ HTTP Request / ส่ง JSON Response (Controller)
│       ├── service.go       # Business Logic (คำนวณเกรด, ตรวจสอบเงื่อนไขธุรกิจ)
│       ├── repository.go    # ติดต่อ Database (SQL, NoSQL)
│       └── model.go         # Data Structures (Structs) สำหรับ Domain นี้
├── pkg/
│   └── logger/              # Shared Utility (ถ้าต้องการแชร์ให้โปรเจกต์อื่นใช้ด้วย)
│       └── logger.go
├── api/
│   └── swagger.yaml         # API Documentation (ถ้ามี)
├── bin/                     # เก็บไฟล์ Binary ที่ Build แล้ว (มักถูก ignore ใน git)
├── go.mod                   # Module definitions
├── go.sum                   # Checksums สำหรับ dependencies
└── Makefile                 # สคริปต์สำหรับ Build/Run/Test
```