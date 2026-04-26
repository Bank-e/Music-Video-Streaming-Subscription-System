# Sequence 05 — Playback & Watch History (FR 5.4)

## 5.1 Record Watch Position (heartbeat ขณะเล่น)

```mermaid
sequenceDiagram
    actor Client
    participant API
    participant AM as AuthManager
    participant Cache as Redis
    participant MQ as MessageBroker
    participant Worker as Background Worker
    participant DB

    Note over Client: ส่งทุก 10 วินาทีขณะกำลังเล่น
    loop while playing
        Client->>API: POST /playback/tick (Bearer) {contentId, position}
        API->>AM: verify(token)
        AM-->>API: principal

        alt !principal.hasActiveSubscription
            API-->>Client: 403 Forbidden
        else ok
            Note over API,Cache: 1. อัปเดตลง RAM (เร็วระดับ ms, Redis รับ 100k TPS สบาย)
            API->>Cache: SET cache:watch_history:{userId}:{contentId} = position

            Note over API,MQ: 2. โยนใส่คิวไว้ (Fire and Forget)
            API->>MQ: Publish "PlaybackTick" {userId, contentId, position}

            API-->>Client: 204 No Content
        end
    end

    Note over Worker,DB: 3. โฟลว์หลังบ้าน ทำงานแยกส่วน (Asynchronous)
    loop ทุกๆ 1 นาที (หรือเมื่อคิวเต็ม)
        Worker->>MQ: Consume "PlaybackTick" (ดึงมาทีละ 5,000 คิว)

        Note over Worker: Worker ทำการรวบข้อมูล (Aggregate)<br/>ถ้า User A ส่งมา 6 ครั้งใน 1 นาที ให้เหลือแค่ "วินาทีล่าสุด" อันเดียว

        Note over Worker,DB: 4. บันทึกลง DB ก้อนใหญ่ทีเดียว (Bulk Upsert)
        Worker->>DB: INSERT INTO watch_history ... ON CONFLICT DO UPDATE (5,000 rows in 1 query)
        DB-->>Worker: OK
        Worker->>MQ: ACK
    end
```

## 5.2 Resume Playback เมื่อเปิด content ค้างไว้ (FR 5.4)

```mermaid
sequenceDiagram
    actor Client
    participant API
    participant AM as AuthManager
    participant Cache as Redis
    participant PS as PlaybackService
    participant DB

    Client->>API: GET /content/{id}/resume (Bearer)
    API->>AM: verify(token)
    AM-->>API: principal

    Note over API,Cache: 1. เช็คจาก Cache ก่อนเสมอ เร็วที่สุด!
    API->>Cache: GET cache:watch_history:{userId}:{contentId}

    alt Cache Hit (เพิ่งดูไปไม่นาน)
        Cache-->>API: lastPosition
        API-->>Client: 200 OK {resumeAt: lastPosition}
    else Cache Miss (ดูนานมาแล้ว หรือเปิดครั้งแรก)
        Cache-->>API: null
        API->>PS: getResumePosition(principal, contentId)
        PS->>DB: SELECT lastPosition FROM watch_history WHERE userId=? AND contentId=?

        alt พบ record ใน DB
            DB-->>PS: lastPosition
            PS-->>API: lastPosition
        else ไม่พบ (เปิดครั้งแรก)
            DB-->>PS: empty
            PS-->>API: 0
        end

        Note over API,Cache: 2. นำค่าที่ได้มาอัปเดตลง Cache (ตั้ง TTL เผื่อไว้)
        API->>Cache: SETEX cache:watch_history:{userId}:{contentId} (lastPosition, TTL=7 days)

        API-->>Client: 200 OK {resumeAt: lastPosition}
    end
```

## 5.3 View Watch History

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client (Frontend/App)
    participant API as API Controller (Backend)
    participant AM as AuthManager (Identity)
    participant PS as PlaybackService (Logic)
    participant Cache as Redis (Cache Layer)
    participant DB as Database (Source of Truth)

    Note over Client,API: GET /me/history?cursor=1714130000&limit=50
    Client->>API: Request Watch History (With JWT Bearer)

    API->>AM: Verify Token (Authentication)
    AM-->>API: Principal (userId)

    API->>PS: GetHistory(userId, cursor, limit)

    Note over PS,DB: [Phase 1: Database Lookup]
    PS->>DB: Query History (WHERE userId=? AND watchedAt < ? LIMIT 50)
    Note right of DB: Use Index on (userId, watchedAt) for speed
    DB-->>PS: historyRows (List of contentId, lastPosition, watchedAt)

    Note over PS,Cache: [Phase 2: Data Hydration & Cache-Aside]
    loop Each item in historyRows
        PS->>Cache: MGET cache:content:{id}:meta

        alt Cache Hit (Found in Redis)
            Cache-->>PS: contentMetadata (JSON)
        else Cache Miss (Not in Redis)
            PS->>DB: Query Content Detail (SELECT title, baseCdnUrl, thumb FROM contents)
            DB-->>PS: dbMetadata
            PS->>Cache: SETEX to Redis (Write-Through for next time)
        end

        Note over PS: [Phase 3: Security & Transformation]
        PS->>PS: Generate Signed URL (HMAC Signature + 5m Expiry)
        PS->>PS: Map to DTO (Combine Metadata + Resume Position)
    end

    Note over PS: Calculate nextCursor (watchedAt of the 50th item)
    PS-->>API: List<WatchHistoryDTO> + nextCursor
    API-->>Client: 200 OK (Paginated JSON Response)

    Note over Client: Client uses nextCursor for Infinite Scroll
```

## 5.4 Combined — Open Content → Resume → Record

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client (Video Player)
    participant API as API Gateway / Controller
    participant AM as AuthManager (Identity)
    participant CS as ContentService (Metadata)
    participant PS as PlaybackService (History)
    participant Cache as Redis (Speed Layer)
    participant DB as Database (Source of Truth)
    participant CDN as CDN (Content Delivery)

    Client->>API: GET /content/{id}/play (Bearer)
    API->>AM: Verify Token (Authentication)
    AM-->>API: Principal (userId, hasSubscription)

    Note over API,DB: [Parallel Execution to Minimize Latency]
    par Phase 1: Get Content & Security
        API->>CS: GetContentDetail(id, userId)
        CS->>Cache: MGET cache:content:{id}:meta
        alt Cache Miss
            CS->>DB: SELECT title, baseCdnUrl FROM contents WHERE id=?
            DB-->>CS: contentData
            CS->>Cache: SETEX cache:content:{id}:meta
        end
        CS->>CS: Generate Signed URL (HMAC + 5m Expiry)
        CS-->>API: {signedUrl, metadata}
    and Phase 2: Get Playback Resume Point
        API->>PS: GetResumePosition(userId, id)
        PS->>Cache: GET cache:resume:{userId}:{id}
        alt Cache Miss
            PS->>DB: SELECT lastPosition FROM watch_history WHERE ...
            DB-->>PS: lastPosition
            PS->>Cache: SETEX cache:resume:{userId}:{id}
        end
        PS-->>API: lastPosition
    end

    API-->>Client: 200 OK {cdnUrl, resumeAt, metadata}

    Note over Client,CDN: [Client-side Streaming Initiation]
    Client->>CDN: GET signedUrl (Range Header: bytes=resumeAt-)
    CDN-->>Client: 206 Partial Content (Streaming Start)

    Note over Client,PS: [Periodic Sync] Every 10s -> POST /playback/tick
```
