# Architecture Document: Music Video Streaming Subscription System

เอกสารฉบับนี้อธิบายโครงสร้างสถาปัตยกรรมของระบบแพลตฟอร์มสตรีมมิ่งเพลงและวิดีโอ (Streaming Platform) รวมถึงส่วนประกอบ (Containers) และการเชื่อมต่อกับระบบภายนอก (External Systems)

---

```
workspace "Streaming Platform" "Music Video Streaming Subscription System" {

    model {
        user = person "User" "ผู้ใช้บริการ"

        paymentGw = softwareSystem "Payment Gateway" "Omise PayPal Bank API" "External"
        bankApp = softwareSystem "Bank App" "Mobile Banking" "External"
        cdn = softwareSystem "Amazon CloudFront" "CDN กระจายในทุกภูมิภาค" "External"

        platform = softwareSystem "Streaming Platform" "ระบบสตรีมเพลงและวิดีโอ" {

            client = container "Client App" "หน้าจอผู้ใช้" "Smartphone Tablet Web"
            gateway = container "API Gateway" "จัดการ Request ผู้ใช้ไปยัง Backend" "Kong"

            lb = container "Elastic Load Balancing" "กระจาย traffic" "AWS ELB"
            k8s = container "Kubernetes" "จัดการ Container Workload" "K8s"

            userSvc = container "User Service" "Registration FR1.1 และ Profile FR1.2" "Node.js"
            authSvc = container "Authentication Service" "Login Logout JWT Refresh" "Node.js"
            subSvc = container "Subscription Service" "จัดการแพ็กเกจสมาชิก" "Node.js"
            subTracker = container "Subscription Status Tracker" "ตรวจสถานะ pending Active expired cancel" "Cron Worker"
            webhook = container "Webhook Handler" "รับ callback จาก Payment Gateway" "Node.js"
            paySvc = container "Payment Service" "ติดตามสถานะการชำระเงิน" "Node.js"
            contentSvc = container "Content Service" "จัดการวิดีโอ เพลง สตรีมมิ่ง ตำแหน่งการเล่น และประวัติการดู" "Node.js"

            // --- [ปรับปรุง] แยก Message Broker ออกเป็น 2 รูปแบบการทำงาน ---
            eventBus = container "Amazon SNS" "กระจาย Event แบบ Pub/Sub (เช่น ContentDeleted)"
            taskQueue = container "RabbitMQ" "รับคิวงานแบบ Point-to-Point (เช่น PlaybackTick"

            contentWorker = container "Content Worker" "ดึง Event ไปลบไฟล์และข้อมูล" "Node.js"
            playbackWorker = container "Playback Worker" "ดึง Heartbeat จาก Queue ไปทำ Bulk Upsert ลง DB" "Node.js/Go"

            userDb = container "User DB" "users และ password hash" "MySQL" "Database"
            authDb = container "Auth DB" "credentials lookup" "MySQL" "Database"
            subDb = container "Subscription DB" "pending Active expired cancel" "MySQL" "Database"
            payDb = container "Payment DB" "pending failed success" "MySQL" "Database"
            contentDb = container "Content DB" "metadata ของ content" "MySQL" "Database"
            watchDb = container "Watch History DB" "ที่ดูค้าง (High Write Throughput)" "MongoDB" "Database"
            redis = container "Redis" "Session, Content, และ Watch History Cache" "Cache" "Database"
            s3 = container "Amazon S3" "ไฟล์วิดีโอและเพลง" "Object Storage" "Database"
        }

        // --- Core Flow ---
        user -> client "ใช้งาน"
        client -> gateway "API Request" "HTTPS"
        gateway -> lb "forward"
        lb -> k8s "distribute"

        // --- Kubernetes Routing to Services ---
        k8s -> userSvc "route"
        k8s -> authSvc "route"
        k8s -> subSvc "route"
        k8s -> paySvc "route"
        k8s -> contentSvc "route"

        // --- User & Auth ---
        userSvc -> userDb "Read Write" "MySQL Protocol"
        authSvc -> authDb "verify credentials" "MySQL Protocol"
        authSvc -> redis "session store" "Redis Protocol"

        // --- Subscription & Payment ---
        subSvc -> subDb "Read Write status" "MySQL Protocol"
        subSvc -> paymentGw "เริ่มชำระเงิน" "HTTPS"
        subTracker -> subDb "update status by cron" "MySQL Protocol"
        paymentGw -> bankApp "redirect ผู้ใช้" "HTTPS"
        bankApp -> paymentGw "ยืนยันชำระเงิน" "HTTPS"
        paymentGw -> webhook "callback" "HTTPS"
        webhook -> subDb "Update Status"
        webhook -> payDb "Update Status"
        paySvc -> payDb "UPDATE status" "MySQL Protocol"
        subSvc -> paySvc "trigger active" "gRPC/HTTP"

        // --- Content & Playback Flow (Sync) ---
        contentSvc -> contentDb "Query metadata / Hydrate metadata" "MySQL Protocol"
        contentSvc -> redis "Read/Write cache + lastPosition Cache" "Redis Protocol"
        contentSvc -> watchDb "Query history" "MongoDB Protocol"

        // --- Direct Upload Flow ---
        contentSvc -> s3 "Generate Pre-signed URL" "AWS SDK"
        client -> s3 "Direct upload via Pre-signed URL" "HTTPS"

        // --- [ปรับปรุง] การ Publish และ Consume ที่แยกรูปแบบกันชัดเจน ---
        contentSvc -> eventBus "Publish ContentDeleted Event" "HTTPS"
        contentSvc -> taskQueue "Send PlaybackTick Task" "AMQP"

        eventBus -> contentWorker "Consume Event" "HTTPS"
        playbackWorker -> taskQueue "Consume Task (Batch)" "AMQP"

        // --- Worker Flows ---
        contentWorker -> contentDb "Save/Delete metadata" "MySQL Protocol"
        contentWorker -> watchDb "Update/Delete history" "MongoDB Protocol"
        contentWorker -> s3 "Store/Physical Delete media" "HTTPS"
        contentWorker -> cdn "Invalidate Cache" "HTTPS"
        playbackWorker -> watchDb "Bulk Upsert history" "MongoDB Protocol"

        // --- Content Delivery ---
        s3 -> cdn "Origin source"
        cdn -> client "Stream media" "HTTPS"
    }

    views {
        systemContext platform "Context" {
            include *
            autolayout lr
        }

        container platform "Containers" {
            include *
            autolayout lr
        }

        styles {
            element "Person" {
                background #08427B
                color #ffffff
                shape Person
            }
            element "Software System" {
                background #1168BD
                color #ffffff
            }
            element "External" {
                background #999999
                color #ffffff
            }
            element "Container" {
                background #438DD5
                color #ffffff
            }
            element "Database" {
                shape Cylinder
                background #F7A072
                color #000000
            }
            relationship "Relationship" {
                color #ffffff
                thickness 3
                fontSize 22
                dashed false
            }
        }
    }
}
```

## 1. System Context (ภาพรวมของระบบ)

ผู้ใช้งาน (User) เข้าใช้บริการระบบสตรีมมิ่ง โดยตัวระบบหลักมีการเชื่อมต่อกับระบบภายนอกดังนี้:

- **Payment Gateway:** ระบบรับชำระเงินภายนอก (เช่น Omise, PayPal, Bank API)
- **Bank App:** แอปพลิเคชัน Mobile Banking สำหรับยืนยันการชำระเงิน
- **Amazon CloudFront (CDN):** เครือข่ายกระจายเนื้อหา (Content Delivery Network) เพื่อให้ผู้ใช้สตรีมมีเดียได้รวดเร็วในทุกภูมิภาค

---

## 2. Infrastructure & Entry Points (โครงสร้างพื้นฐานและจุดรับ Request)

- **Client App:** หน้าจอผู้ใช้ รองรับ Smartphone, Tablet และ Web
- **API Gateway (Kong):** ด่านหน้าสำหรับจัดการ Request ทั้งหมดจากผู้ใช้งาน และทำหน้าที่ Routing ไปยัง Backend
- **Elastic Load Balancing (AWS ELB):** กระจาย Traffic ให้สมดุล
- **Kubernetes (K8s):** ระบบจัดการ Container Workload สำหรับ Microservices ทั้งหมด

---

## 3. Microservices (บริการย่อย)

ระบบหลังบ้านถูกแบ่งออกเป็น Service ย่อยๆ (พัฒนาด้วย Node.js) ดังนี้:

- **User Service:** จัดการการสมัครสมาชิก (Registration) และข้อมูลโปรไฟล์ผู้ใช้
- **Authentication Service:** จัดการการ Login, Logout และระบบ Token (JWT & Refresh Token)
- **Subscription Service:** จัดการแพ็กเกจสมาชิกและการเรียกชำระเงิน
- **Payment Service:** จัดการและติดตามสถานะการชำระเงิน
- **Webhook Handler:** จุดรับข้อมูล Callback (Webhook) จาก Payment Gateway เมื่อผู้ใช้ชำระเงินสำเร็จหรือล้มเหลว
- **Subscription Status Tracker:** Cron Worker สำหรับตรวจสอบและอัปเดตสถานะแพ็กเกจ (Pending, Active, Expired, Canceled)
- **Content Service:** บริการหลักสำหรับจัดการวิดีโอ, เพลง, สตรีมมิ่ง, ตำแหน่งการเล่น และประวัติการรับชม

---

## 4. Message Brokers & Asynchronous Workers (ระบบคิวและเบื้องหลัง)

มีการแบ่งแยก Message Broker ตามรูปแบบการใช้งานที่ชัดเจน:

- **Amazon SNS (Event Bus):** กระจาย Event แบบ **Pub/Sub** (เช่น `ContentDeleted` Event)
- **RabbitMQ (Task Queue):** รับจัดการคิวงานแบบ **Point-to-Point** ที่มีปริมาณสูง (เช่น `PlaybackTick` จาก Heartbeat)

**Background Workers:**

- **Content Worker (Node.js):** ดึง Event จาก SNS ไปประมวลผล เช่น ลบไฟล์สื่อออกจาก S3, ลบข้อมูลใน DB, และสั่ง Invalidate Cache บน CDN
- **Playback Worker (Node.js/Go):** ดึงข้อมูล Heartbeat (ตำแหน่งการเล่นวิดีโอ) จาก RabbitMQ เพื่อนำไปบันทึกลง Database แบบ Bulk Upsert

---

## 5. Databases & Storage (ฐานข้อมูลและพื้นที่จัดเก็บ)

เพื่อประสิทธิภาพสูงสุด ระบบเลือกใช้ฐานข้อมูลที่เหมาะสมกับแต่ละ Domain:

- **User DB (MySQL):** เก็บข้อมูลผู้ใช้งานและรหัสผ่าน (Password Hash)
- **Auth DB (MySQL):** เก็บข้อมูลสำหรับการยืนยันตัวตน (Credentials lookup)
- **Subscription DB (MySQL):** เก็บข้อมูลสถานะแพ็กเกจ
- **Payment DB (MySQL):** เก็บข้อมูลและสถานะ Transaction การชำระเงิน
- **Content DB (MySQL):** เก็บข้อมูล Metadata ของเนื้อหา (เพลง/วิดีโอ)
- **Watch History DB (MongoDB):** จัดเก็บประวัติการดูและจุดที่ดูค้างไว้ (รองรับ High Write Throughput)
- **Redis (Cache):** จัดเก็บ Session, Content Metadata และ Watch History (ตำแหน่งล่าสุดแบบ Real-time)
- **Amazon S3 (Object Storage):** พื้นที่จัดเก็บไฟล์วิดีโอและไฟล์เพลงต้นฉบับ

---

## 6. Key Data Flows (กระแสข้อมูลสำคัญ)

### 6.1 Authentication Flow

1. API Gateway ส่ง Request ไปยัง `Auth Service`
2. `Auth Service` ตรวจสอบข้อมูลกับ `Auth DB`
3. จัดการเก็บหรืออ่าน Session/Token ผ่าน `Redis`

### 6.2 Payment & Subscription Flow

1. ผู้ใช้ทำรายการผ่าน `Subscription Service` ระบบจะ Redirect ไปที่ `Payment Gateway` และ `Bank App`
2. เมื่อชำระเงินเสร็จ `Payment Gateway` จะส่ง Callback กลับมาที่ `Webhook Handler`
3. `Webhook` ทำการอัปเดตสถานะใน `Subscription DB` และ `Payment DB`
4. `Subscription Service` ส่งคำสั่งไปที่ `Payment Service` เพื่อเริ่มเปิดใช้งานแพ็กเกจ (Trigger Active)

### 6.3 Direct Media Upload Flow

1. `Client` ขอสิทธิ์อัปโหลดจาก `Content Service`
2. `Content Service` ออก Pre-signed URL ของ `Amazon S3` ให้
3. `Client` อัปโหลดไฟล์ตรงเข้า `Amazon S3` ผ่าน Pre-signed URL (ลดภาระแบนด์วิดท์ของ Backend)

### 6.4 Content Playback & Sync Flow

1. สตรีมไฟล์มีเดียจาก `Amazon S3` ผ่าน `CloudFront (CDN)` ไปยัง `Client`
2. `Client` ส่ง Heartbeat (ตำแหน่งการเล่น) มาที่ `Content Service`
3. `Content Service` อัปเดตข้อมูลลง `Redis` เพื่อให้ดึงข้อมูลได้ทันที และส่ง Task ลง `RabbitMQ`
4. `Playback Worker` ดึงข้อมูลจาก `RabbitMQ` ไปทำ Bulk Upsert ลง `Watch History DB (MongoDB)` เพื่อบันทึกถาวร
