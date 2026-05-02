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
            DB-->>API: OK

            Note over API: สร้าง content หลัก
            API->>DB: INSERT contents(contentId, objectKey, title, ...)
            DB-->>API: success

            alt type = video
                API->>DB: INSERT videos(...)
                DB-->>API: success
            end

            Note over API: อัปเดต upload_session = COMPLETED
            API->>DB: UPDATE upload_session SET status=COMPLETED
            DB-->>API: success

            API->>DB: COMMIT

            alt commit สำเร็จ
                DB-->>API: OK
                API-->>Creator: 201 Created
            else commit ล้มเหลว
                DB-->>API: Error
                API->>DB: ROLLBACK
                DB-->>API: OK
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

    alt ผู้ใช้ไม่มีแพ็กเกจสมาชิก (!hasActiveSubscription)
        API-->>Client: 403 No Active Subscription
    else มีแพ็กเกจสมาชิกและพร้อมใช้งาน
        API->>Cache: GET cache:content:{id}:base_url

        alt มีข้อมูลในแคช (Cache Hit)
            Cache-->>API: baseCdnUrl
        else ไม่มีข้อมูลในแคช (Cache Miss)
            Cache-->>API: null

            API->>CS: getContentById(id)

            CS->>DB: SELECT cdnBaseUrl, isPublished FROM contents WHERE contentId=?
            DB-->>CS: content

            alt หาคอนเทนต์ในระบบไม่พบ หรือถูกตั้งค่าซ่อนไว้
                CS-->>API: NotFoundError
                API-->>Client: 404 Not Found
            else พบข้อมูลคอนเทนต์
                CS-->>API: content

                API->>Cache: SETEX cache:content:{id}:baseCdnUrl (TTL 1 hour)
                Cache-->>API: OK
            end
        end

        Note right of Cache: ตรงนี้ API จะได้ Base URL เสมอ
        API->>API: signedUrl = generateCdnUrl(baseCdnUrl, principal.userId)

        API-->>Client: 302 Redirect → signedUrl

        Client->>CDN: GET signedUrl
        CDN-->>Client: stream bytes
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

    alt ผู้ใช้ไม่มีแพ็กเกจสมาชิก (!hasActiveSubscription)
        API-->>Creator: 403 Forbidden (No Subscription)
    else มีแพ็กเกจสมาชิกและพร้อมใช้งาน
        API->>CS: updateContent(principal, id, data)

        CS->>DB: SELECT creatorId FROM contents WHERE contentId=?
        DB-->>CS: content

        alt หาคอนเทนต์ในระบบไม่พบ (content == null)
            CS-->>API: NotFoundError
            API-->>Creator: 404 Not Found
        else ผู้ใช้ไม่ใช่เจ้าของคอนเทนต์ (creatorId != userId)
            CS-->>API: ForbiddenError (Not Owner)
            API-->>Creator: 403 Forbidden
        else ผู้ใช้เป็นเจ้าของคอนเทนต์ตัวจริง
            CS->>DB: UPDATE contents SET title=?, metadata=? WHERE contentId=?
            DB-->>CS: OK (Updated)

            Note left of CS: ลบแคชเก่าทิ้ง เพื่อให้ระบบดึงข้อมูลใหม่
            CS->>Cache: DEL cache:content:{id}:*
            Cache-->>CS: OK (Deleted)

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

    alt หาคอนเทนต์ในระบบไม่พบ (content == null)
        CS-->>API: NotFound
        API-->>Creator: 404
    else ผู้ใช้ไม่ใช่เจ้าของคอนเทนต์ (creatorId != userId)
        CS-->>API: Forbidden
        API-->>Creator: 403
    else ผู้ใช้เป็นเจ้าของคอนเทนต์ตัวจริง
        Note over CS,DB: 2. Soft Delete (เปลี่ยนสถานะเป็นถูกลบ)
        CS->>DB: UPDATE contents SET deleted_at=NOW() WHERE contentId=?
        DB-->>CS: OK (Updated)

        Note over CS,Cache: 3. ล้างข้อมูลแคชทิ้ง
        CS->>Cache: DEL cache:content:{id}:*
        Cache-->>CS: OK (Deleted)

        Note over CS,MQ: 4. ส่งงานไปลบไฟล์จริงเบื้องหลัง
        CS->>MQ: Publish Event "ContentDeleted" {contentId, cdnUrl}
        MQ-->>CS: OK (Published)

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

---

# Redis Architecture for Content System

อธิบายโครงสร้างการจัดเก็บข้อมูลใน Redis สำหรับระบบ Video Streaming ในส่วนของ **Content (Read-Heavy)** เพื่อลดความซ้ำซ้อนของข้อมูล จัดการหน่วยความจำได้อย่างมีประสิทธิภาพ และรองรับการทำ Pagination หน้า Feed อย่างถูกต้อง

---

## 1. Content Metadata (Read-Heavy)

ใช้สำหรับเก็บรายละเอียดของ Content เพื่อลดการ Query Database โดยตรง ข้อมูลนี้จะถูกเรียกใช้ทั้งในหน้า Detail ของวิดีโอ และใช้ดึงข้อมูลประกอบหน้า Feed

- **Key Pattern:** `cache:content:{id}:meta`
  - `{id}`: รหัสประจำตัวของวิดีโอ (Content ID)
- **Value:** `JSON String` (หรือ `Hash`)
  - ตัวอย่าง: `{"title": "...", "baseCdnUrl": "...", "thumbnail": "...", "isPublished": true}`
- **TTL (Time-To-Live):** `1 Hour` (3,600 วินาที)
  - **เหตุผล:** เป็นระยะเวลาที่เหมาะสมสำหรับข้อมูลที่ไม่ได้เปลี่ยนบ่อย ช่วยลดโหลด DB ได้ดี และไม่เก็บนานเกินไปจนข้อมูลล้าหลัง
- **การใช้งาน (Usage):**
  - **Write (SETEX):** ถูกอัปเดตเมื่อมีผู้ใช้เรียกดู Content นั้นเป็นครั้งแรก (Cache Miss แล้วไปดึงจาก DB มาวาง)
  - **Read (GET/MGET):** ถูกอ่านเมื่อเปิดหน้าวิดีโอ หรือใช้ `MGET` ดึงพร้อมกันหลายๆ วิดีโอเพื่อประกอบร่างหน้า Feed
  - **Delete (DEL):** ระบบหลังบ้าน (CMS) ต้องสั่งลบหรืออัปเดตค่านี้ทันที (Cache Invalidation) เมื่อมีการแก้ไขข้อมูล เช่น เปลี่ยนภาพปก หรือสั่ง Unpublish

---

## 2. Content List Cache / Feed (Pagination Management)

ใช้เก็บรายการ Content ตามหมวดหมู่ สำหรับหน้า Feed หรือระบบ Infinite Scroll โดยปรับมาใช้ Sorted Set เพื่อแก้ปัญหาข้อมูลซ้ำซ้อนและป้องกันปัญหา Pagination Shift (ข้อมูลเคลื่อนเวลาเลื่อนหน้าจอ)

- **Key Pattern:** `cache:feed:{type}`
  - `{type}`: ประเภทของ Feed เช่น `trending`, `new_releases`, `action_movies`
- **Value:** `Sorted Set (ZSET)`
  - **Score:** `Timestamp (Epoch Time)` ใช้เวลาที่เพิ่มวิดีโอเข้าหมวดหมู่นั้น เพื่อใช้ในการเรียงลำดับ
  - **Member:** `{contentId}` (เก็บแค่ ID เท่านั้น)
- **TTL (Time-To-Live):** `5 - 15 Minutes`
  - **เหตุผล:** เพื่อให้หน้า Feed มีความสดใหม่เสมอ อัปเดตเทรนด์ได้ทันสถานการณ์
- **การใช้งาน (Usage):**
  - **Write (ZADD):** ถูกสร้างและเติมข้อมูลโดย Background Job หรือเมื่อมีคนเปิดดู Feed หมวดนั้นแล้ว Cache Miss
  - **Read (ZREVRANGEBYSCORE):** ใช้ดึง `{contentId}` ออกมาทีละชุด (เช่น ครั้งละ 20 รายการ) ตามช่วง Timestamp ของ Cursor จากนั้น API จะนำ Array ของ ID ที่ได้ ไปทำ `MGET` จากข้อ 1 เพื่อประกอบข้อมูลส่งให้ Client

---

## 💡 Architecture & Performance Considerations

1. **Normalization in Cache:** การออกแบบให้ Feed (ข้อ 2) เก็บเฉพาะ ID แล้วค่อยไป `MGET` กับ Metadata (ข้อ 1) ช่วยประหยัด RAM มหาศาล และเมื่อแอดมินเปลี่ยนภาพปกวิดีโอ หน้า Feed ทุกหมวดหมู่จะเห็นภาพปกใหม่ทันทีโดยไม่ต้องไปตามลบ Cache Feed ทีละอัน
