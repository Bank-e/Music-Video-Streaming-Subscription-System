# Sequence 03 — Subscription + Payment Flows (FR 3.1–3.4, FR 4.1–4.2)

## 3.1 Subscribe + Initiate Payment (FR 3.1, FR 4.1)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant PG as PaymentGateway
  participant Ext as Stripe/Omise
  participant DB

  Client->>API: POST /subscribe (Bearer) {billingCycle}
  API->>AM: verify(token)
  AM-->>API: principal
  API->>SS: subscribe(principal, billingCycle)
  SS->>DB: SELECT * FROM subscriptions WHERE userId=? AND status IN (ACTIVE, PENDING)
  alt มีการจ่าย subscriptions แล้ว
    SS-->>API: DuplicateSubscriptionError
    API-->>Client: 409 Conflict
  else ยังไม่มี
    SS->>DB: INSERT subscriptions(status=PENDING, userId, billingCycle, price)
    DB-->>SS: subscriptionId
    SS->>PG: createPaymentIntent(amount, currency)
    PG->>Ext: POST /checkout/session {amount, currency, metadata: subscriptionId}
    Ext-->>PG: {checkoutUrl, intentId}
    PG->>DB: INSERT payments(subscriptionId, externalTransactionId=intentId, status=PENDING, amount)
    PG-->>SS: checkoutUrl
    SS-->>API: checkoutUrl
    API-->>Client: 200 OK {redirectUrl=checkoutUrl}
  end
  Client->>Ext: redirect to checkoutUrl + ชำระเงิน
```

## 3.2 Payment Webhook → Activate Subscription (FR 3.4, FR 4.2)

```mermaid
sequenceDiagram
  participant Ext as Stripe/Omise
  participant API as API Gateway
  participant PG as PaymentGateway
  participant SS as SubscriptionService
  participant DB

  Ext->>API: POST /webhook/payment {payload, signature}
  API->>PG: handleWebhook(payload, signature)
  PG->>PG: verify HMAC signature
  alt signature invalid
    PG-->>API: 400 Bad Request
    API-->>Ext: 400
  else valid
    PG->>DB: SELECT status FROM payments WHERE externalTransactionId=?
    alt already COMPLETED (idempotency)
      Note right of PG: NFR Reliability — กัน webhook ซ้ำ
      PG-->>API: 200 OK (noop)
      API-->>Ext: 200
    else PENDING
      PG->>DB: UPDATE payments SET status=COMPLETED, paidAt=now WHERE externalTransactionId=?
      PG->>SS: onPaymentSuccess(externalTransactionId)
      SS->>DB: SELECT subscription via payment.subscriptionId
      DB-->>SS: subscription
      SS->>SS: subscription.activate() — status=ACTIVE, startDate=now, endDate=now+cycle
      SS->>DB: UPDATE subscriptions SET status=ACTIVE, ...
      SS-->>PG: true
      PG-->>API: 200 OK
      API-->>Ext: 200
    end
  end
```

## 3.3 Cancel Subscription (FR 3.1)

```mermaid
sequenceDiagram
  actor Client
  participant API as API Gateway
  participant AM as AuthManager
  participant SS as SubscriptionService
  participant DB

  Client->>API: POST /subscription/cancel (Bearer)
  API->>AM: verify(token)
  AM-->>API: principal
  API->>SS: cancel(principal)
  SS->>DB: SELECT * FROM subscriptions WHERE userId=? AND status=ACTIVE
  DB-->>SS: result
  alt ไม่มี active sub
    SS-->>API: NotFoundError
    API-->>Client: 404
  else พบ
    SS->>SS: subscription.cancel() — status=CANCELLED
    SS->>DB: UPDATE subscriptions SET status=CANCELLED
    DB-->>SS: success
    SS-->>API: true
    API-->>Client: 200 OK
  end

```

## 3.4 Subscription Expiration Check (FR 3.3 — Scheduled)

```mermaid
sequenceDiagram
  participant Cron
  participant SS as SubscriptionService
  participant DB

  Note over Cron: รันทุก 1 ชั่วโมง
  Cron->>SS: checkExpirations()
  SS->>DB: SELECT * FROM subscriptions WHERE status=ACTIVE AND endDate < now
  DB-->>SS: List<Subscription>
  loop each subscription
    SS->>SS: subscription.expire() — status=EXPIRED
    SS->>DB: UPDATE subscriptions SET status=EXPIRED WHERE subscriptionId=?
    DB-->>SS: success
  end
  SS-->>Cron: done
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
