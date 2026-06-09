# Sequence 05 — Playback & Watch History (FR 5.4)

## 5.1 Record Watch Position (heartbeat ขณะเล่น)

```mermaid
sequenceDiagram
    autonumber
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

        alt ไม่มีแพ็กเกจ
            API-->>Client: 403 Forbidden
        else มีแพ็กเกจปกติ
            Note over API,Cache: 1. อัปเดตลง RAM (เร็วระดับ ms, Redis รับ 100k TPS สบาย)
            API->>Cache: SET cache:watch_history:{userId}:{contentId} = position
            Cache-->>API: OK

            Note over API,MQ: 2. โยนใส่คิวไว้ (Fire and Forget)
            API->>MQ: Publish "PlaybackTick" {userId, contentId, position}
            MQ-->>API: Published

            API-->>Client: 204 No Content
        end
    end

    Note over Worker,DB: 3. โฟลว์หลังบ้าน ทำงานแยกส่วน (Asynchronous)
    loop ทุกๆ 1 นาที (หรือเมื่อคิวเต็ม)
        Worker->>MQ: Consume "PlaybackTick" (ดึงมาทีละ 5,000 คิว)
        MQ-->>Worker: Messages
        Note over Worker: Worker ทำการรวบข้อมูล (Aggregate)<br/>ถ้า User A ส่งมา 6 ครั้งใน 1 นาที ให้เหลือแค่ "วินาทีล่าสุด" อันเดียว

        Note over Worker,DB: 4. บันทึกลง DB ก้อนใหญ่ทีเดียว (Bulk Upsert)
        Worker->>DB: INSERT INTO watch_history ... ON CONFLICT DO UPDATE (5,000 rows in 1 query)
        DB-->>Worker: OK

        Worker->>MQ: ACK
        MQ-->>Worker: Acknowledged
    end
```

## 5.2 Resume Playback เมื่อเปิด content ค้างไว้ (FR 5.4)

```mermaid
sequenceDiagram
    autonumber
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

    alt พบข้อมูลใน Cache
        Cache-->>API: lastPosition
        API-->>Client: 200 OK {resumeAt: lastPosition}
    else ไม่พบข้อมูลใน Cache
        Cache-->>API: null
        API->>PS: getResumePosition(principal, contentId)
        PS->>DB: SELECT lastPosition FROM watch_history WHERE userId=? AND contentId=?

        alt มีประวัติใน DB
            DB-->>PS: lastPosition
            PS-->>API: lastPosition
        else ไม่มีประวัติใน DB
            DB-->>PS: empty
            PS-->>API: 0
        end

        Note over API,Cache: 2. นำค่าที่ได้มาอัปเดตลง Cache (ตั้ง TTL เผื่อไว้)
        API->>Cache: SETEX cache:watch_history:{userId}:{contentId} (lastPosition, TTL=7 days)
        Cache-->>API: OK

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

        alt พบข้อมูลใน Cache
            Cache-->>PS: contentMetadata (JSON)
        else ไม่พบข้อมูลใน Cache
            Cache-->>PS: null
            PS->>DB: Query Content Detail (SELECT title, baseCdnUrl, thumb FROM contents)
            DB-->>PS: dbMetadata
            PS->>Cache: SETEX to Redis (Write-Through for next time)
            Cache-->>PS: OK
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

        alt พบข้อมูลใน Cache
            Cache-->>CS: contentData
        else ไม่พบข้อมูลใน Cache
            Cache-->>CS: null
            CS->>DB: SELECT title, baseCdnUrl FROM contents WHERE id=?
            DB-->>CS: contentData
            CS->>Cache: SETEX cache:content:{id}:meta
            Cache-->>CS: OK
        end

        CS->>CS: Generate Signed URL (HMAC + 5m Expiry)
        CS-->>API: {signedUrl, metadata}
    and Phase 2: Get Playback Resume Point
        API->>PS: GetResumePosition(userId, id)
        PS->>Cache: GET cache:resume:{userId}:{id}

        alt พบข้อมูลใน Cache
            Cache-->>PS: lastPosition
        else ไม่พบข้อมูลใน Cache
            Cache-->>PS: null
            PS->>DB: SELECT lastPosition FROM watch_history WHERE ...
            DB-->>PS: lastPosition
            PS->>Cache: SETEX cache:resume:{userId}:{id}
            Cache-->>PS: OK
        end

        PS-->>API: lastPosition
    end

    API-->>Client: 200 OK {cdnUrl, resumeAt, metadata}

    Note over Client,CDN: [Client-side Streaming Initiation]
    Client->>CDN: GET signedUrl (Range Header: bytes=resumeAt-)
    CDN-->>Client: 206 Partial Content (Streaming Start)

    Note over Client,PS: [Periodic Sync] Every 10s -> POST /playback/tick
```

---

# Redis Architecture for Playback System

อธิบายโครงสร้างการจัดเก็บข้อมูลใน Redis สำหรับระบบ Video Streaming ในส่วนของ **Playback (Write-Heavy)** ซึ่งออกแบบมาเพื่อรับโหลดมหาศาลจากการส่งข้อมูลสถานะการเล่น (Heartbeat) ของผู้ใช้งานพร้อมกันจำนวนมาก โดยไม่ทำให้ Database หลักล่ม

---

## 1. Watch History & Resume State (Write-Heavy)

ใช้บันทึกตำแหน่งการเล่นล่าสุดของผู้ใช้ (Resume Playback) โดยอาศัย Redis เป็นด่านหน้าในการรับ Write Load เนื่องจากพฤติกรรมการเล่นวิดีโอจะมีการยิง API เข้ามาอัปเดตข้อมูลถี่มาก (High Write TPS)

- **Key Pattern:** `cache:watch_history:{userId}:{contentId}`
  - `{userId}`: รหัสประจำตัวของผู้ใช้งาน
  - `{contentId}`: รหัสประจำตัวของวิดีโอที่กำลังดู
- **Value:** `Integer`
  - ตัวอย่าง: `1250` (วินาทีล่าสุดที่ดูค้างไว้)
- **TTL (Time-To-Live):** `7 Days` (604,800 วินาที)
  - **เหตุผล:** ครอบคลุมพฤติกรรมผู้ใช้ส่วนใหญ่ที่มักจะกลับมาดูวิดีโอเดิมต่อภายในไม่กี่วัน การตั้ง TTL จะช่วยป้องกันไม่ให้ Memory ของ Redis เต็มจากประวัติการดูเก่าๆ ที่ผู้ใช้ดูจบไปนานแล้ว
- **การใช้งาน (Usage):**
  - **Write (SET):** ถูกอัปเดตอย่างต่อเนื่อง (เช่น ทุกๆ 10 วินาที) ขณะที่วิดีโอกำลังเล่นบนฝั่ง Client
  - **Read (GET):** ถูกเรียกอ่านเมื่อผู้ใช้เปิดวิดีโอขึ้นมาใหม่ เพื่อหาจุดเริ่มเล่นต่อ (หาก Cache Miss หรือข้อมูลถูกลบไปแล้ว ถึงจะ Fallback ไปคิวรีหาใน DB หลัก)

---

## 💡 Architecture & Performance Considerations

1. **Asynchronous DB Sync (Write-Behind Pattern):** ข้อมูลตำแหน่งการเล่นที่เขียนลง Redis เป็นเพียงจุดพักชั่วคราวเพื่อให้ตอบสนองเร็ว ระบบต้อง Publish ข้อมูลนี้ลง Message Queue (เช่น RabbitMQ หรือ Kafka) ด้วย เพื่อให้ Background Worker ทยอยนำข้อมูลไปรวบรวม (Aggregate) แล้วทำ Bulk Upsert ลง Database ก้อนใหญ่ทีเดียว **(ห้ามเขียนลง DB โดยตรงทุกๆ 10 วินาทีเด็ดขาด)**
2. **Eviction Policy:** ควรตั้งค่า Redis Server สำหรับ Server ที่จัดการ Playback เป็น `volatile-lru` เสมอ เพื่อเป็นตาข่ายนิรภัย หาก Memory กำลังจะเต็ม ระบบจะได้เลือกลบ Key ประวัติการดูเก่าๆ ที่มี TTL ทิ้งไปก่อน โดยไม่กระทบกับคนที่กำลังดูวิดีโออยู่ ณ วินาทีนั้น
