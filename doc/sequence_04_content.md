# Sequence 04 — Content Flows (FR 5.1, 5.2, 5.3)

## 4.1 Upload Content (FR 5.1)

```mermaid
sequenceDiagram
    actor Creator
    participant API
    participant AM as AuthManager
    participant DB
    participant OS as ObjectStorage (S3)

    Note over Creator,OS: Phase 1: Request Upload URL
    Creator->>API: POST /content/request-upload (Bearer, {type, extension})
    API->>AM: verify(token)
    AM-->>API: principal

    alt !principal.hasActiveSubscription
        API-->>Creator: 403 Forbidden
    else มีสิทธิ์
        API->>OS: Generate Pre-signed URL (PUT, expiry=15m)
        OS-->>API: presignedUrl, objectKey
        API-->>Creator: 200 OK {presignedUrl, objectKey}
    end

    Note over Creator,OS: Phase 2: Direct Upload
    Creator->>OS: PUT presignedUrl (Binary File)
    OS-->>Creator: 200 OK (Upload Success)

    Note over Creator,DB: Phase 3: Confirm & Save Metadata
    Creator->>API: POST /content/confirm (Bearer, {objectKey, title, metadata})
    API->>DB: BEGIN TX
    API->>DB: INSERT contents(contentId, objectKey, ...)
    alt type=video
        API->>DB: INSERT videos(...)
    end
    API->>DB: COMMIT
    alt Commit Success
        DB-->>API: OK (Transaction Committed)
        API-->>Creator: 201 Created (Content Ready for Transcoding)
    else Commit Failed (เช่น ขัดข้องที่ DB)
        DB-->>API: Error
        API->>DB: ROLLBACK
        API-->>Creator: 500 Internal Server Error
    end
```

## 4.2 List Contents (FR 5.2)

```mermaid
sequenceDiagram
    actor Client
    participant API
    participant AM as AuthManager
    participant Cache as Redis (Cache)
    participant CS as ContentService
    participant DB

    Note over Client,API: เพิ่ม cursor (หรือ page) และ limit เสมอ
    Client->>API: GET /contents?type=video&cursor=abc&limit=20
    API->>AM: verify(token)
    AM-->>API: principal

    API->>Cache: GET cache:contents:video:abc:20

    alt Cache Hit (มีข้อมูลใน Cache)
        Cache-->>API: JSON List<Content>
        API-->>Client: 200 OK [contents] (ตอบกลับทันที)
    else Cache Miss (ไม่มีข้อมูล)
        Cache-->>API: null
        API->>CS: getContentsByType("video", cursor="abc", limit=20)

        Note over CS,DB: เลิกใช้ SELECT * และบังคับ LIMIT
        CS->>DB: SELECT c.id, c.title, v.thumbnailUrl FROM contents c JOIN videos v ... WHERE id > cursor LIMIT 20
        DB-->>CS: List<Content>
        CS-->>API: list (id, title, thumbnailUrl)

        Note over API,Cache: เซฟลง Cache เพื่อให้คนต่อไปโหลดเร็วขึ้น (ตั้งเวลาหมดอายุ TTL)
        API->>Cache: SETEX cache:contents:video:abc:20 (list, TTL=5 mins)

        API-->>Client: 200 OK [contents]
    end
```

## 4.3 Stream Content with Subscription Check (FR 5.3)

```mermaid
sequenceDiagram
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
