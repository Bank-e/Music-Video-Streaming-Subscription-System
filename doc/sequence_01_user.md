# Sequence 01 — User Flows (FR 1.1, 1.2)

## 1.1 User Registration (FR 1.1)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant US as UserService
    participant DB as Database

    Note over Client,API: สมัครสมาชิกใหม่ (Register)

    Client->>API: POST /register {email, password, displayName}
    API->>US: register(email, password, displayName)

    Note over US: ตรวจสอบข้อมูลเบื้องต้น
    US->>US: validate email format & password length

    alt ข้อมูลไม่ถูกต้อง
        US-->>API: ValidationError
        API-->>Client: 400 Bad Request {error}
    else ข้อมูลถูกต้อง
        US->>DB: Check email exists
        DB-->>US: exists / not found

        alt email ซ้ำ
            US-->>API: DuplicateEmailError
            API-->>Client: 409 Conflict {error: "Email already used"}
        else email ใช้ได้
            Note over US: เข้ารหัสรหัสผ่าน
            US->>US: hashPassword(password)

            US->>DB: Insert new user (email, hash, displayName)
            DB-->>US: userId (new record id)

            US-->>API: userId
            API-->>Client: 201 Created {userId}
        end
    end
```

## 1.2 Update Profile (FR 1.2)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant US as UserService
    participant DB as Database

    Note over Client,API: อัปเดตข้อมูลโปรไฟล์ผู้ใช้

    Client->>API: PUT /me/profile {displayName, avatarUrl} + Bearer Token

    API->>AM: verifyToken(accessToken)
    AM-->>API: principal / invalid

    alt token ไม่ถูกต้อง หรือหมดอายุ
        API-->>Client: 401 Unauthorized {error}
    else token ถูกต้อง
        API->>US: updateProfile(userId, displayName, avatarUrl)

        Note over US: ตรวจสอบข้อมูลก่อนอัปเดต
        US->>US: validate(displayName, avatarUrl)

        alt ข้อมูลไม่ถูกต้อง
            US-->>API: ValidationError
            API-->>Client: 400 Bad Request {error}
        else ข้อมูลถูกต้อง
            US->>DB: Update user profile
            DB-->>US: success / fail

            alt ไม่พบ user
                US-->>API: NotFoundError
                API-->>Client: 404 Not Found
            else อัปเดตสำเร็จ
                US-->>API: success
                API-->>Client: 200 OK {message: "Profile updated"}
            end
        end
    end
```

## 1.3 Change Password (FR 1.2)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant US as UserService
    participant DB as Database
    participant Cache as Redis Cache

    Note over Client,API: เปลี่ยนรหัสผ่านผู้ใช้

    Client->>API: POST /me/password {oldPassword, newPassword} + Bearer Token

    API->>AM: verifyToken(accessToken)
    AM-->>API: principal / invalid

    alt token ไม่ถูกต้อง หรือหมดอายุ
        API-->>Client: 401 Unauthorized {error}
    else token ถูกต้อง
        API->>US: changePassword(userId, oldPassword, newPassword)

        Note over US: ตรวจสอบรูปแบบรหัสผ่านใหม่
        US->>US: validate(newPassword)

        alt รหัสผ่านใหม่ไม่ผ่านเงื่อนไข
            US-->>API: ValidationError
            API-->>Client: 400 Bad Request {error}
        else ผ่าน validation
            US->>DB: Get current password hash
            DB-->>US: passwordHash / not found

            alt ไม่พบ user
                US-->>API: NotFoundError
                API-->>Client: 404 Not Found
            else พบ user
                US->>US: compare(oldPassword, passwordHash)

                alt รหัสผ่านเดิมไม่ถูกต้อง
                    US-->>API: InvalidCredentialError
                    API-->>Client: 401 Unauthorized {error}
                else รหัสผ่านเดิมถูกต้อง
                    Note over US: เข้ารหัสรหัสผ่านใหม่
                    US->>US: newHash = hash(newPassword)

                    US->>DB: Update password
                    DB-->>US: success

                    Note over US,Cache: บังคับ logout ทุก session (security)
                    US->>Cache: DEL user:{userId}:refresh_token
                    Cache-->>US: OK

                    US-->>API: success
                    API-->>Client: 200 OK {message: "Password changed"}
                end
            end
        end
    end
```

## 1.4 Change Email (FR 1.2)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant US as UserService
    participant DB as Database

    Note over Client,API: เปลี่ยน Email (ไม่ใช้ verification)

    Client->>API: POST /me/email {newEmail} + Bearer Token

    API->>AM: verifyToken(accessToken)
    AM-->>API: principal / invalid

    alt token ไม่ถูกต้อง
        API-->>Client: 401 Unauthorized {error}
    else token ถูกต้อง
        API->>US: changeEmail(userId, newEmail)

        US->>US: validateEmail(newEmail)
        alt email ไม่ถูกต้อง
            US-->>API: ValidationError
            API-->>Client: 400 Bad Request
        else email ถูกต้อง
            US->>DB: Check email exists
            DB-->>US: exists / not found

            alt email ซ้ำ
                US-->>API: DuplicateEmailError
                API-->>Client: 409 Conflict
            else ใช้ได้
                US->>DB: Update email
                DB-->>US: success

                US-->>API: success
                API-->>Client: 200 OK {message: "Email updated"}
            end
        end
    end
```
