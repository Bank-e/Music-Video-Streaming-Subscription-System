# Sequence 03 — Subscription + Payment Flows (FR 3.1–3.4, FR 4.1–4.2)

## 3.1 Subscribe + Initiate Payment (FR 3.1, FR 4.1)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant SS as SubscriptionService
    participant PG as PaymentGateway
    participant Ext as External Payment (Stripe/Omise)
    participant DB as Database

    Client->>API: POST /subscribe {billingCycle} + Bearer Token
    API->>AM: verifyToken
    AM-->>API: principal

    API->>SS: subscribe(userId, billingCycle)

    Note over SS,DB: ตรวจสอบว่าผู้ใช้มี subscription ที่ยังใช้งาน/รอจ่ายอยู่หรือไม่
    SS->>DB: Find subscription (ACTIVE หรือ PENDING)
    DB-->>SS: found / not found

    alt มีอยู่แล้ว
        SS-->>API: DuplicateSubscriptionError
        API-->>Client: 409 Conflict
    else ยังไม่มี

        Note over SS: 1. สร้าง "รายการสมัคร" สถานะรอจ่าย (PENDING)
        SS->>DB: Create subscription record\n(status=PENDING, userId, plan, price)
        DB-->>SS: subscriptionId

        Note over SS,PG: 2. ขอสร้าง "รายการชำระเงิน" กับ Payment Gateway
        SS->>PG: requestPayment(amount, subscriptionId)

        Note over PG,Ext: ส่งคำขอไปผู้ให้บริการ (Stripe/Omise)
        PG->>Ext: Create checkout session\n(amount, currency, metadata=subscriptionId)
        Ext-->>PG: checkoutUrl, transactionId

        Note over PG,DB: 3. บันทึกว่า "กำลังรอจ่ายเงินจริง"
        PG->>DB: Create payment record\n(subscriptionId, transactionId, status=PENDING, amount)
        DB-->>PG: success

        PG-->>SS: checkoutUrl
        SS-->>API: checkoutUrl
        API-->>Client: 200 OK {redirectUrl}

        Note over Client,Ext: ผู้ใช้ถูกพาไปหน้าชำระเงินจริง
        Client->>Ext: Open checkout page และจ่ายเงิน
    end
```

## 3.2 Payment Webhook → Activate Subscription (FR 3.4, FR 4.2)

```mermaid
sequenceDiagram
    autonumber
    participant Ext as External Payment (Stripe/Omise)
    participant API as API Gateway
    participant PG as PaymentGateway
    participant SS as SubscriptionService
    participant DB as Database

    Note over Ext,API: Payment Provider แจ้งผลการจ่ายเงิน (Webhook)

    Ext->>API: POST /webhook/payment {payload, signature}

    API->>PG: handleWebhook(payload, signature)

    Note over PG: 1. ตรวจสอบว่า webhook มาจาก provider จริง
    PG->>PG: verifySignature(payload, signature)

    alt signature ไม่ถูกต้อง
        PG-->>API: Reject (invalid signature)
        API-->>Ext: 400 Bad Request
    else signature ถูกต้อง

        Note over PG,DB: 2. ตรวจสอบสถานะ payment เดิม (กัน webhook ซ้ำ)
        PG->>DB: Find payment by transactionId (จาก payload ที่ได้รับ)
        DB-->>PG: payment (status)

        alt payment ถูกประมวลผลแล้ว (COMPLETED)
            Note right of PG: Idempotency — กัน event ซ้ำ
            PG-->>API: 200 OK (ignore)
            API-->>Ext: 200 OK
        else payment ยังเป็น PENDING

            Note over PG: 3. ยืนยันว่า "จ่ายเงินสำเร็จ"
            PG->>DB: Mark payment as COMPLETED\n(set paidAt = now)
            DB-->>PG: success

            Note over PG,SS: 4. แจ้งระบบ subscription ให้ activate
            PG->>SS: onPaymentSuccess(transactionId)

            SS->>DB: Load subscription (via payment.subscriptionId)
            DB-->>SS: subscription

            Note over SS: เปลี่ยนสถานะเป็นใช้งานได้
            SS->>SS: activate subscription\n(status=ACTIVE, start=now, end=now+plan)

            SS->>DB: Update subscription status
            DB-->>SS: success

            SS-->>PG: success
            PG-->>API: 200 OK
            API-->>Ext: 200 OK
        end
    end
```

## 3.3 Cancel Subscription (FR 3.1)

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Gateway
    participant AM as AuthManager
    participant SS as SubscriptionService
    participant DB as Database

    Note over Client,API: ยกเลิกการต่ออายุ (Cancel at period end)

    Client->>API: POST /subscription/cancel + Bearer Token

    API->>AM: verifyToken
    AM-->>API: principal / invalid

    alt token ไม่ถูกต้อง
        API-->>Client: 401 Unauthorized
    else token ถูกต้อง
        API->>SS: cancelSubscription(userId)

        SS->>DB: Find ACTIVE subscription
        DB-->>SS: subscription / not found

        alt ไม่มี subscription
            SS-->>API: NotFoundError
            API-->>Client: 404
        else พบ subscription

            Note over SS: ตั้งค่า "ไม่ต่ออายุรอบถัดไป"
            SS->>SS: cancelAtPeriodEnd = true

            SS->>DB: Update subscription (cancelAtPeriodEnd = true)
            DB-->>SS: success

            SS-->>API: success
            API-->>Client: 200 OK {message: "Will not renew next cycle"}
        end
    end
```

## 3.4 Subscription Expiration Check (FR 3.3 — Scheduled)

```mermaid
sequenceDiagram
    autonumber
    participant Cron
    participant SS as SubscriptionService
    participant DB as Database

    Note over Cron: รันทุก 1 ชั่วโมง

    Cron->>SS: checkExpirations()

    Note over SS,DB: หา subscription ที่หมดอายุแล้ว
    SS->>DB: Find subscriptions\n(status=ACTIVE AND endDate < now)
    DB-->>SS: expiredSubscriptions

    alt ไม่มีรายการ
        SS-->>Cron: done (no-op)
    else มีรายการ
        loop แต่ละ subscription
            Note over SS: สิทธิ์หมดอายุแล้ว
            SS->>SS: mark status = EXPIRED

            SS->>DB: Update subscription (status=EXPIRED)
            DB-->>SS: success
        end

        SS-->>Cron: done
    end
```

## 3.5 Renew Subscription

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant PG as PaymentGateway
  participant Ext as Stripe/Omise
  participant DB

  Client->>API: POST /subscription/renew (Bearer)
  API->>AM: verify(token)
  AM-->>API: principal
  API->>SS: renew(principal)
  SS->>DB: SELECT subscription (ACTIVE or EXPIRED) WHERE userId=?
  DB-->>SS: sub
  SS->>PG: createPaymentIntent(amount, currency)
  PG->>Ext: POST /checkout/session
  Ext-->>PG: {checkoutUrl, intentId}
  PG->>DB: INSERT payments(subscriptionId=sub.id, externalTransactionId=intentId, PENDING)
  DB-->>PG: success
  PG-->>SS: checkoutUrl
  SS-->>API: checkoutUrl
  API-->>Client: 200 OK {redirectUrl}
  Note over Client,Ext: หลัง webhook success → onPaymentSuccess ต่อ endDate
```
