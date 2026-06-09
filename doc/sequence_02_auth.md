# Sequence 02 — Auth Flows (FR 2.1–2.4)

## 2.1 Login (FR 2.1, 2.2, 2.3)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant SS as SubscriptionService
    participant DB as Database
    participant Cache as Redis Cache

    Note over Client,API: Login เข้าสู่ระบบ

    Client->>API: POST /login {email, password}
    API->>AM: login(email, password)

    Note over AM,DB: ตรวจสอบผู้ใช้
    AM->>DB: Get user by email
    DB-->>AM: user / not found

    alt ไม่พบ user
        AM-->>API: InvalidCredentialError
        API-->>Client: 401 Unauthorized {error}
    else พบ user
        Note over AM: ตรวจสอบรหัสผ่าน
        AM->>AM: compare(password, passwordHash)

        alt รหัสผ่านไม่ถูกต้อง
            AM-->>API: InvalidCredentialError
            API-->>Client: 401 Unauthorized {error}
        else รหัสผ่านถูกต้อง

            Note over AM,SS: ตรวจสอบสถานะ Subscription
            AM->>SS: getSubscription(userId)
            SS->>DB: Query subscription by userId
            DB-->>SS: subscription / null
            SS-->>AM: subscription

            AM->>AM: hasActiveSub = (status=ACTIVE && endDate > now)

            Note over AM: สร้าง Access Token (อายุ 15 นาที)
            AM->>AM: create accessToken (JWT with jti, userId, hasActiveSub)

            Note over AM: สร้าง Refresh Token (อายุ 15 วัน)
            AM->>AM: create refreshToken (JWT)
            AM->>AM: hash(refreshToken)

            Note over AM,Cache: เก็บ Refresh Token ลง Redis
            AM->>Cache: SET user:{userId}:refresh_token (hashed) EX 15d
            Cache-->>AM: OK

            AM-->>API: {accessToken, refreshToken}
            API-->>Client: 200 OK {accessToken, refreshToken}
        end
    end
```

## 2.2 Token Verification Middleware (FR 2.4)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as Middleware (API Gateway)
    participant AM as AuthManager
    participant Cache as Redis Cache
    participant H as Handler

    Note over Client,API: เรียก API ที่ต้องใช้ Authentication

    Client->>API: Request + Authorization: Bearer <accessToken>

    API->>AM: verifyToken(accessToken)

    Note over AM: ตรวจสอบ Signature และ Expiration ก่อน

    alt ลายเซ็นไม่ถูกต้อง (Signature invalid)
        AM-->>API: SignatureError
        API-->>Client: 401 Unauthorized {error}
    else token หมดอายุ (exp < now)
        AM-->>API: TokenExpiredError
        API-->>Client: 401 Unauthorized {error}
    else token ถูกต้อง
        Note over AM: ดึงข้อมูลจาก token (claims, jti)
        AM->>AM: extract claims (userId, jti, roles, etc.)

        AM->>Cache: Check blacklist by jti
        Cache-->>AM: found / not found

        alt token ถูก revoke (อยู่ใน blacklist)
            AM-->>API: TokenRevokedError
            API-->>Client: 401 Unauthorized {error}
        else token ใช้งานได้
            AM-->>API: principal (userId, roles, permissions)

            API->>H: handleRequest(principal, request)
            H-->>API: response

            API-->>Client: 200 OK {data}
        end
    end
```

## 2.3 Refresh Token (FR 2.3)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant SS as SubscriptionService
    participant Cache as Redis Cache

    Note over Client,API: ขอ Access Token ใหม่ด้วย Refresh Token

    Client->>API: POST /refresh {refreshToken}
    API->>AM: refresh(refreshToken)

    Note over AM: ตรวจสอบความถูกต้องของ Refresh Token

    AM->>AM: verify(refreshToken)

    alt token ไม่ถูกต้อง หรือหมดอายุ
        AM-->>API: InvalidTokenError
        API-->>Client: 401 Unauthorized {error, action: "re-login"}
    else token ถูกต้อง
        Note over AM: ดึง userId จาก token
        AM->>AM: extract userId
        AM->>AM: hashedInput = hash(refreshToken)

        Note over AM,Cache: ตรวจสอบกับ Redis (Token ล่าสุดของ user)
        AM->>Cache: GET user:{userId}:refresh_token
        Cache-->>AM: storedHash / not found

        alt ไม่พบ หรือ hash ไม่ตรง (Reuse Attack)
            Note over AM: ตรวจพบการใช้ token ซ้ำ (อาจโดนขโมย)
            Note over AM,Cache: ลบ refreshToken ออกจาก redis
            AM->>Cache: DEL user:{userId}:refresh_token
            Cache-->>AM: OK

            AM-->>API: RevokedTokenError
            API-->>Client: 401 Unauthorized {error, action: "re-login"}
        else token ถูกต้อง
            Note over AM,SS: โหลดข้อมูล Subscription ล่าสุด
            AM->>SS: getSubscription(userId)
            SS-->>AM: subscription / null

            AM->>AM: hasActiveSub = (status=ACTIVE && endDate > now)

            Note over AM: สร้าง Access Token ใหม่ (15 นาที)
            AM->>AM: newAccessToken = signJWT(userId, jti, hasActiveSub)

            Note over AM: สร้าง Refresh Token ใหม่ (Rotation)
            AM->>AM: newRefreshToken = signJWT(userId, exp=30d)
            AM->>AM: newHash = hash(newRefreshToken)

            Note over AM,Cache: บันทึก Refresh Token ตัวใหม่
            AM->>Cache: SET user:{userId}:refresh_token newHash EX 30d
            Cache-->>AM: OK

            AM-->>API: {newAccessToken, newRefreshToken}
            API-->>Client: 200 OK {accessToken, refreshToken}
        end
    end
```

## 2.4 Logout (FR 2.1 — Client-side)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant Cache as Redis Cache

    Note over Client,API: Logout ออกจากระบบ

    Client->>API: POST /logout + Authorization: Bearer <accessToken>

    API->>AM: logout(accessToken)

    Note over AM: ตรวจสอบ token ก่อน logout
    AM->>AM: verify(accessToken)
    AM-->>API: valid / invalid

    alt token ไม่ถูกต้อง หรือหมดอายุ
        API-->>Client: 401 Unauthorized {error}
    else token ถูกต้อง
        Note over AM: ดึง userId, jti และ exp จาก token
        AM->>AM: extract userId, jti, exp
        AM->>AM: remainingTime = exp - now

        Note over AM,Cache: 1. ใส่ Access Token ลง blacklist
        AM->>Cache: SET blacklist:token:{jti} "1" EX remainingTime
        Cache-->>AM: OK

        Note over AM,Cache: 2. ลบ Refresh Token ของ user
        AM->>Cache: DEL user:{userId}:refresh_token
        Cache-->>AM: OK

        AM-->>API: success
        API-->>Client: 204 No Content
    end

    Note over Client: Client ลบ token ออกจาก storage (localStorage / cookie)
```

---

**หมายเหตุ**: ทุก flow ที่ตามมา (subscription, content, playback) ใช้ 2.2 เป็น step แรกเสมอ ในไดอะแกรมอื่นจะย่อเหลือ `API → AM.verify → principal`

---

# Redis Architecture for Authentication System

อธิบายโครงสร้างการจัดเก็บข้อมูลใน Redis สำหรับระบบ Hybrid Authentication (JWT + Redis) เพื่อรองรับการทำ **Instant Logout (Blacklist)** และ **Refresh Token Rotation (Reuse Detection)**

---

## 1. Access Token Blacklist

ใช้สำหรับบันทึก Access Token ที่ถูกยกเลิกการใช้งานก่อนหมดอายุ (เช่น ผู้ใช้กด Logout หรือถูกสั่งระงับสิทธิ์)

- **Key Pattern:** `blacklist:token:{jti}`
  - `{jti}`: JWT ID ซึ่งเป็น Unique ID ที่ฝังอยู่ใน Payload ของ Access Token
- **Value:** `Timestamp (Epoch Time)`
  - ตัวอย่าง: `1714476000` (เวลาที่ Token ถูกสั่ง Revoke เพื่อประโยชน์ในการทำ Audit Log)
- **TTL (Time-To-Live):** `Remaining Expiry Time` ของ Access Token ใบนั้น
  - หน่วยเป็นวินาที (Seconds)
  - **เหตุผล:** เพื่อให้ Redis ลบข้อมูลทิ้งอัตโนมัติเมื่อ Token นั้นหมดอายุตามธรรมชาติไปแล้ว ช่วยประหยัด Memory
- **การใช้งาน (Usage):**
  - **Write (SET):** ถูกสร้างเมื่อผู้ใช้เรียก API `/logout`
  - **Read (EXISTS):** ถูกตรวจสอบใน **Token Verification Middleware** (FR 2.2) ทุกครั้งที่มี Request เข้ามา โดยจะตรวจสอบ _หลังจาก_ ที่ Verify Signature ผ่านแล้วเท่านั้น เพื่อป้องกัน DoS Attack

---

## 2. Active Refresh Token (Session Management)

ใช้สำหรับตรวจสอบสิทธิ์ในการออก Access Token ใบใหม่ ป้องกันการนำ Token เก่าที่ถูกขโมยมาใช้ซ้ำ (Reuse Detection) ปัจจุบันรองรับแบบ **Single Session (1 บัญชี ล็อกอินได้ 1 อุปกรณ์)**

- **Key Pattern:** `user:{userId}:active_refresh_token`
  - `{userId}`: รหัสประจำตัวของผู้ใช้งาน
- **Value:** `Hashed Refresh Token`
  - ตัวอย่าง: `"$2b$10$EixZaYVK1fsbw1ZfbX3OXePaWxn96p36WQoeG6Lruj3vjGQx40IGN"`
  - **คำเตือน:** ห้ามเก็บ Refresh Token เป็น Plain Text เด็ดขาด ต้องผ่านฟังก์ชัน Hash (เช่น SHA-256 หรือ bcrypt) ก่อนจัดเก็บ เพื่อป้องกันกรณี Redis ถูกเจาะข้อมูล
- **TTL (Time-To-Live):** `30 Days` (2,592,000 วินาที)
  - ตั้งค่าให้เท่ากับอายุเต็มของ Refresh Token
- **การใช้งาน (Usage):**
  - **Write (SET):** ถูกอัปเดตค่าใหม่ทุกครั้งที่เรียก API `/login` (FR 2.1) หรือ `/refresh` (FR 2.3) สำเร็จ
  - **Read (GET):** ถูกเรียกอ่านตอนผู้ใช้ส่ง Refresh Token มาขอต่ออายุ (FR 2.3) เพื่อตรวจสอบว่าค่า Hash ตรงกับที่เก็บไว้หรือไม่
  - **Delete (DEL):** ถูกลบทิ้งเมื่อผู้ใช้กด `/logout` (FR 2.4) หรือเมื่อระบบตรวจพบการพยายามใช้ Refresh Token เก่าที่ถูกหมุนเวียน (Rotation) ไปแล้ว เพื่อเตะผู้ใช้ออกจากระบบทุกอุปกรณ์

---

## 💡 Security Considerations

1. **Verification Order:** ใน Middleware ต้องทำ `JWT.verify(signature)` ก่อนนำ `jti` ไปคิวรีลง Redis เสมอ
2. **Reuse Detection:** หากผู้ใช้ส่ง Refresh Token มา แต่ Hash ไม่ตรงกับใน `active_refresh_token` ให้สันนิษฐานว่า Token รั่วไหล ต้องสั่ง `DEL user:{userId}:active_refresh_token` ทันที
