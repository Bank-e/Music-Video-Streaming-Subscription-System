# Sequence 02 — Auth Flows (FR 2.1–2.4)

## 2.1 Login (FR 2.1, 2.2, 2.3)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant DB as Database
  participant Cache as Redis Cache

  Client->>API: POST /login {email, pass}
  API->>AM: login(email, pass)
  AM->>DB: SELECT userId, passwordHash FROM users WHERE email=?

  alt ไม่พบ user
    DB-->>AM: empty
    AM-->>API: InvalidCredentialError
    API-->>Client: 401 Unauthorized
  else พบ user
    DB-->>AM: user
    alt bcrypt.compare(pass, passwordHash) = false
      AM-->>API: InvalidCredentialError
      API-->>Client: 401 Unauthorized
    else รหัสผ่านถูกต้อง
      AM->>SS: getSubscriptionByUserId(userId)
      SS->>DB: SELECT status, endDate FROM subscriptions WHERE userId=?
      DB-->>SS: sub (or null)
      SS-->>AM: sub
      AM->>AM: hasActiveSub = (sub.status=ACTIVE && sub.endDate > now)

      Note over AM: 1. สร้าง Access Token (อายุ 15 นาที + ฝัง jti เพื่อทำ Blacklist)
      AM->>AM: claims = {jti=uuid, sub=userId, hasActiveSubscription, exp=now+15m}
      AM->>AM: accessToken = JWT.sign(claims, HS256)

      Note over AM: 2. สร้าง Refresh Token เป็น JWT (อายุ 15 วัน + ฝัง userId)
      AM->>AM: refreshToken = JWT.sign({sub=userId, exp=now+15d}, SecretKey)
      AM->>AM: hashedRT = hash(refreshToken)

      Note over AM,Cache: 3. เก็บ Hash ของ Refresh Token ลง Redis (ใช้ Key ให้ตรงกับตอน Refresh)
      AM->>Cache: SET user:{userId}:active_refresh_token {hashedRT} EX 2592000

      AM-->>API: {accessToken, refreshToken}
      API-->>Client: 200 OK {accessToken, refreshToken}
    end
  end
```

## 2.2 Token Verification Middleware (FR 2.4)

```mermaid
sequenceDiagram
  actor Client
  participant API as Middleware
  participant AM as AuthManager
  participant Cache as Redis Cache
  participant H as Handler

  Client->>API: any protected request + Authorization: Bearer <token>
  API->>AM: verify(accessToken)

  Note over AM: 1. ตรวจสอบลายเซ็น (Signature) และ Expiration ก่อนเสมอ
  alt signature invalid
    AM-->>API: SignatureError
    API-->>Client: 401 Unauthorized
  else exp < now
    AM-->>API: TokenExpiredError
    API-->>Client: 401 Unauthorized
  else valid (Token ของจริงและยังไม่หมดอายุ)
    Note over AM: 2. เมื่อมั่นใจว่าเป็น Token แท้ ค่อยแกะ jti ไปถาม Redis
    AM->>AM: extract jti, claims
    AM->>Cache: EXISTS blacklist:token:{jti}
    Cache-->>AM: result (0 or 1)

    alt result = 1 (อยู่ใน Blacklist)
      AM-->>API: TokenRevokedError
      API-->>Client: 401 Unauthorized
    else result = 0 (ใช้งานได้ปกติ)
      AM-->>API: AccessPrincipal(userId, hasActiveSubscription)
      API->>H: handle(principal, requestBody)
      H-->>API: result
      API-->>Client: 200 OK
    end
  end
```

## 2.3 Refresh Token (FR 2.3)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant Cache as Redis Cache

  Client->>API: POST /refresh {refreshToken}
  API->>AM: refresh(refreshToken)

  Note over AM: 1. ตรวจสอบความถูกต้องของ Token ก่อน
  AM->>AM: verify(refreshToken, SecretKey)
  alt Signature ผิด / หมดอายุ (30 วัน)
    AM-->>API: InvalidTokenError
    API-->>Client: 401 Unauthorized (Force Re-login)
  else Token ถูกต้อง
    Note over AM: 2. แกะข้อมูล userId ออกมาจาก Payload ของ Refresh Token
    AM->>AM: decode -> extract userId
    AM->>AM: hashedInputRT = hash(refreshToken)

    Note over AM,Cache: 3. เช็กกับ Redis ว่า Token นี้คือตัวล่าสุดของ User คนนี้ใช่ไหม
    AM->>Cache: GET user:{userId}:active_refresh_token
    Cache-->>AM: storedHashedRT

    alt storedHashedRT != hashedInputRT หรือไม่พบ
      Note over AM: ตรวจพบคนแอบนำ Token เก่ามาใช้ (Reuse Detection)
      AM->>Cache: DEL user:{userId}:active_refresh_token (แบนทุกอุปกรณ์)
      AM-->>API: RevokedTokenError
      API-->>Client: 401 Unauthorized (Force Re-login)
    else ตรงกัน (Valid)
      AM->>SS: getSubscriptionByUserId(userId)
      SS-->>AM: sub
      AM->>AM: recompute hasActiveSubscription

      Note over AM: 4. ออกชุด Token ใหม่ (Rotation)
      AM->>AM: newAccessToken = JWT.sign({jti=new_uuid, sub=userId...}, 15m)
      AM->>AM: newRefreshToken = JWT.sign({sub=userId}, 30d)
      AM->>AM: newHashedRT = hash(newRefreshToken)

      Note over AM,Cache: 5. บันทึก Refresh Token ตัวใหม่ทับของเดิม
      AM->>Cache: SET user:{userId}:active_refresh_token {newHashedRT} EX 2592000

      AM-->>API: {accessToken, refreshToken}
      API-->>Client: 200 OK {accessToken: newAccessToken, refreshToken: newRefreshToken}
    end
  end
```

## 2.4 Logout (FR 2.1 — Client-side)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant Cache as Redis Cache

  Client->>API: POST /logout + Authorization: Bearer <token>
  API->>AM: logout(accessToken, userId)

  AM->>AM: decode -> extract jti, exp
  AM->>AM: remainingTime = exp - now

  Note over AM,Cache: 1. แบน Access Token ใบนี้ทันที
  AM->>Cache: SET blacklist:token:{jti} "1" EX {remainingTime}

  Note over AM,Cache: 2. ทำลาย Refresh Token ป้องกันการขอใหม่
  AM->>Cache: DEL user:{userId}:refresh_tokens

  AM-->>API: success
  API-->>Client: 204 No Content

  Note over Client: Client ลบ Token ออกจาก localStorage / Cookie
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
