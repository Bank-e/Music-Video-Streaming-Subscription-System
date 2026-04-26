# Sequence 02 — Auth Flows (FR 2.1–2.4)

## 2.1 Login (FR 2.1, 2.2, 2.3)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant DB

  Client->>API: POST /login {email, pass}
  API->>AM: login(email, pass)
  AM->>DB: SELECT userId, passwordHash FROM users WHERE email=?
  alt ไม่พบ user
    DB-->>AM: empty
    AM-->>API: InvalidCredentialError
    API-->>Client: 401 Unauthorized
  else พบ
    DB-->>AM: user
    alt bcrypt.compare(pass, passwordHash) = false
      AM-->>API: InvalidCredentialError
      API-->>Client: 401
    else ถูกต้อง
      AM->>SS: getSubscriptionByUserId(userId)
      SS->>DB: SELECT status, endDate FROM subscriptions WHERE userId=?
      DB-->>SS: sub (or null)
      SS-->>AM: sub
      AM->>AM: hasActiveSub = (sub.status=ACTIVE && sub.endDate > now)
      AM->>AM: claims = {sub=userId, hasActiveSubscription, iat, exp=now+15m}
      AM->>AM: accessToken = JWT.sign(claims, HS256)
      AM->>AM: refreshToken = JWT.sign({sub=userId, exp=now+30d})
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
  participant H as Handler

  Client->>API: any protected request + Authorization: Bearer <token>
  API->>AM: verify(accessToken)
  alt signature invalid
    AM-->>API: SignatureError
    API-->>Client: 401 Unauthorized
  else exp < now
    AM-->>API: TokenExpiredError
    API-->>Client: 401 Unauthorized
  else valid
    AM->>AM: decode claims
    AM-->>API: AccessPrincipal(userId, hasActiveSubscription, expiresAt)
    API->>H: handle(principal, requestBody)
    H-->>API: result
    API-->>Client: 200 OK
  end
```

## 2.3 Refresh Token (FR 2.3)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant DB

  Client->>API: POST /refresh {refreshToken}
  API->>AM: refresh(refreshToken)
  alt refresh invalid/expired
    AM-->>API: 401
    API-->>Client: 401 Unauthorized
  else valid
    AM->>AM: decode → userId
    AM->>SS: getSubscriptionByUserId(userId)
    SS->>DB: SELECT ...
    DB-->>SS: sub
    SS-->>AM: sub
    AM->>AM: recompute hasActiveSubscription
    AM->>AM: accessToken = JWT.sign({userId, hasActiveSubscription, exp=now+15m})
    AM-->>API: accessToken
    API-->>Client: 200 OK {accessToken}
  end
```

## 2.4 Logout (FR 2.1 — Client-side)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway

  Note over Client: เลือกวิธีใดวิธีหนึ่ง
  Client->>Client: ลบ accessToken + refreshToken จาก localStorage
  Note over Client,API: token ที่ยังไม่หมดอายุใช้ได้จนถึง exp เพราะ stateless ไม่มี blacklist
  Client->>API: (ไม่จำเป็น) POST /logout — server ตอบ 204 เฉย ๆ
```

---

**หมายเหตุ**: ทุก flow ที่ตามมา (subscription, content, playback) ใช้ 2.2 เป็น step แรกเสมอ ในไดอะแกรมอื่นจะย่อเหลือ `API → AM.verify → principal`
