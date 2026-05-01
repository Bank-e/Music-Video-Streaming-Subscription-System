# Sequence 04 — Content Flows (FR 5.1, 5.2, 5.3)

## 4.1 Upload Content (FR 5.1)

```mermaid
sequenceDiagram
    autonumber
    actor Creator
    participant API
    participant AM as AuthManager
    participant OS as ObjectStorage (S3)
    participant DB as Database

    Note over Creator,OS: Phase 1: ขอ URL สำหรับอัปโหลด

    Creator->>API: POST /content/request-upload {type, extension} + Bearer
    API->>AM: verifyToken
    AM-->>API: principal / invalid

    alt token ไม่ถูกต้อง
        API-->>Creator: 401 Unauthorized
    else token ถูกต้อง
        alt ไม่มีสิทธิ์ (เช่น ไม่มี subscription)
            API-->>Creator: 403 Forbidden
        else มีสิทธิ์
            Note over API: สร้าง objectKey แบบ unique (เช่น UUID + extension)
            API->>OS: Generate Pre-signed URL (PUT, expiry=15m, content-type)
            OS-->>API: presignedUrl, objectKey

            Note over API: บันทึกสถานะ "รอ confirm"
            API->>DB: INSERT upload_session (objectKey, userId, status=PENDING)
            DB-->>API: success

            API-->>Creator: 200 OK {presignedUrl, objectKey}
        end
    end

    Note over Creator,OS: Phase 2: อัปโหลดไฟล์จริง

    Creator->>OS: PUT presignedUrl (Binary File)
    OS-->>Creator: 200 OK

    Note over Creator,API: Phase 3: ยืนยันและบันทึกข้อมูล

    Creator->>API: POST /content/confirm {objectKey, title, metadata} + Bearer

    API->>AM: verifyToken
    AM-->>API: principal

    Note over API,DB: ตรวจสอบว่า objectKey นี้เป็นของ user จริง
    API->>DB: Find upload_session by objectKey + userId
    DB-->>API: session / not found

    alt ไม่พบ หรือไม่ใช่เจ้าของ
        API-->>Creator: 403 Forbidden
    else valid

        Note over API,OS: (สำคัญ) ตรวจสอบว่าไฟล์มีอยู่จริง
        API->>OS: HEAD objectKey
        OS-->>API: exists / not found

        alt file ไม่อยู่ (upload fail)
            API-->>Creator: 400 Bad Request {error: "file not uploaded"}
        else file อยู่จริง

            API->>DB: BEGIN TX

            Note over API: สร้าง content หลัก
            API->>DB: INSERT contents(contentId, objectKey, title, ...)

            alt type = video
                API->>DB: INSERT videos(...)
            end

            Note over API: อัปเดต upload_session = COMPLETED
            API->>DB: UPDATE upload_session SET status=COMPLETED

            API->>DB: COMMIT

            alt commit สำเร็จ
                DB-->>API: OK
                API-->>Creator: 201 Created
            else commit ล้มเหลว
                DB-->>API: Error
                API->>DB: ROLLBACK
                API-->>Creator: 500 Internal Server Error
            end
        end
    end
```

## 4.2 List Contents (FR 5.2)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API
    participant AM as AuthManager
    participant Cache as Redis
    participant CS as ContentService
    participant DB as Database

    Note over Client,API: ดึงรายการ content (รองรับ pagination)

    Client->>API: GET /contents?type=video&cursor=abc&limit=20

    API->>AM: verifyToken
    AM-->>API: principal / invalid

    alt token ไม่ถูกต้อง
        API-->>Client: 401 Unauthorized
    else valid

        Note over API: สร้าง cacheKey จาก parameter ทั้งหมด
        API->>Cache: GET cache:contents:video:abc:20

        alt Cache Hit
            Cache-->>API: cachedList
            API-->>Client: 200 OK {items, nextCursor}

        else Cache Miss
            Cache-->>API: null

            API->>CS: getContents(type, cursor, limit)

            Note over CS,DB: ใช้ cursor-based pagination (ไม่ใช้ OFFSET)
            CS->>DB: SELECT id, title, thumbnailUrl\nFROM contents\nWHERE type='video' AND id > cursor\nORDER BY id ASC\nLIMIT 20
            DB-->>CS: rows

            Note over CS: คำนวณ nextCursor จาก item สุดท้าย
            CS-->>API: {items, nextCursor}

            Note over API,Cache: เก็บ cache พร้อม TTL
            API->>Cache: SETEX cache:contents:video:abc:20 {items, nextCursor} TTL=300s

            API-->>Client: 200 OK {items, nextCursor}
        end
    end
```

## 4.3 Stream Content with Subscription Check (FR 5.3)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API
    participant AM as AuthManager
    participant Cache as Redis (Cache)
    participant CS as ContentService
    participant DB
    participant CDN

    Client->>API: GET /content/{id}/stream (Bearer)
    API->>AM: verify(token)
    AM-->>API: principal

    alt !principal.hasActiveSubscription
        API-->>Client: 403 No Active Subscription
    else ok
        API->>Cache: GET cache:content:{id}:base_url

        alt Cache Miss
            API->>CS: getContentById(id)
            CS->>DB: SELECT cdnBaseUrl, isPublished FROM contents WHERE contentId=?
            DB-->>CS: content

            alt content ไม่พบ หรือถูกซ่อน
                CS-->>API: NotFoundError
                API-->>Client: 404
            else พบ
                API->>Cache: SETEX cache:content:{id}:base_url (TTL 1 hour)
            end
        end

        Note right of Cache: ตรงนี้ API จะได้ Base URL เสมอ (เร็วมาก)
        Cache-->>API: baseCdnUrl

        API->>API: signedUrl = generateCdnUrl(baseCdnUrl, principal.userId) -- HMAC + TTL 5m
        API-->>Client: 302 Redirect → signedUrl

        Client->>CDN: GET signedUrl
        Note over CDN: CDN Edge ตรวจสอบ HMAC Signature
        CDN-->>Client: stream bytes (NFR Performance <3s TTFB)
    end
```

## 4.4 Update Own Content (FR 5.1)

```mermaid
sequenceDiagram
    autonumber
    actor Creator
    participant API
    participant AM as AuthManager
    participant Cache as Redis (Cache)
    participant CS as ContentService
    participant DB

    Creator->>API: PATCH /content/{id} (Bearer) {title?, metadata?}
    API->>AM: verify(token)
    AM-->>API: principal

    alt !principal.hasActiveSubscription
        API-->>Creator: 403 Forbidden (No Subscription)
    else มีสิทธิ์ใช้งาน
        API->>CS: updateContent(principal, id, data)

        CS->>DB: SELECT creatorId FROM contents WHERE contentId=?
        DB-->>CS: content

        alt content == null
            CS-->>API: NotFoundError
            API-->>Creator: 404 Not Found
        else content.creatorId != principal.userId
            CS-->>API: ForbiddenError (Not Owner)
            API-->>Creator: 403 Forbidden
        else เจ้าของจริง
            CS->>DB: UPDATE contents SET title=?, metadata=? WHERE contentId=?

            Note left of CS: Cache Invalidation (สำคัญมาก!)
            CS->>Cache: DEL cache:content:{id}:* (ลบข้อมูลเก่าทิ้ง)
            CS-->>API: true
            API-->>Creator: 200 OK
        end
    end
```

## 4.5 Delete Own Content

```mermaid
sequenceDiagram
    autonumber
    actor Creator
    participant API
    participant AM as AuthManager
    participant Cache as Redis
    participant CS as ContentService
    participant DB
    participant MQ as Message Queue (RabbitMQ/Kafka)

    Creator->>API: DELETE /content/{id} (Bearer)
    API->>AM: verify(token)
    AM-->>API: principal

    API->>CS: deleteContent(principal, id)

    Note over CS,DB: 1. ดึงข้อมูลที่ยังไม่ถูกลบ
    CS->>DB: SELECT creatorId FROM contents WHERE contentId=? AND deleted_at IS NULL
    DB-->>CS: content

    alt content ไม่พบ
        CS-->>API: NotFound
        API-->>Creator: 404
    else ไม่ใช่เจ้าของ
        CS-->>API: Forbidden
        API-->>Creator: 403
    else เจ้าของจริง
        Note over CS,DB: 2. Soft Delete (เปลี่ยนสถานะอย่างเดียว)
        CS->>DB: UPDATE contents SET deleted_at=NOW() WHERE contentId=?

        Note over CS,Cache: 3. Invalidate Cache
        CS->>Cache: DEL cache:content:{id}:*

        Note over CS,MQ: 4. โยนงานหนักไปให้ Worker ทำเบื้องหลัง
        CS->>MQ: Publish Event "ContentDeleted" {contentId, cdnUrl}

        CS-->>API: true
        API-->>Creator: 204 No Content
    end
```

## 4.6 Delete Content Using Message Queue In Background

```mermaid
sequenceDiagram
    autonumber
    participant MQ as Message Queue
    participant Worker as Background Worker
    participant OS as Object Storage (S3)
    participant CDN as CDN Network
    participant DB as Database

    MQ-->>Worker: Consume Event: {contentId: 123, fileKey: "vid_123.mp4"}

    Note over Worker,OS: 1. ลบไฟล์จริง (Physical Delete)
    Worker->>OS: DELETE /bucket/vid_123.mp4
    OS-->>Worker: 204 Deleted

    Note over Worker,CDN: 2. สั่งล้างแคชทั่วโลก (Cache Invalidation)
    Worker->>CDN: POST /invalidation {paths: ["/stream/123/*"]}
    CDN-->>Worker: Invalidation ID (รอประมวลผล 1-2 นาที)

    Note over Worker,DB: 3. ทยอยลบข้อมูลที่ผูกกันอยู่ (Chunked Delete)
    loop จนกว่าข้อมูลจะหมด
        Worker->>DB: DELETE FROM watch_history WHERE contentId=123 LIMIT 1000
        DB-->>Worker: OK
    end

    Worker->>DB: DELETE FROM contents WHERE contentId=123
    DB-->>Worker: OK

    Note over Worker,MQ: 4. ยืนยันการทำงานจบ
    Worker->>MQ: ACK (Acknowledge) Message
```
