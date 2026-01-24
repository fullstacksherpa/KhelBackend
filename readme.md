# Khalti Payment Integration: Understanding `return_url` vs `website_url`

## Flow Overview

text

Mobile App (WebView / WebBrowser)
↓
Khalti Hosted Payment Page
↓
GET /v1/store/payments/khalti ← Redirect with Query Params
↓
Your Backend Processing:

1. Parse query parameters
2. Find payment by `pidx`
3. Call Khalti LOOKUP API
4. Apply atomic database transaction
5. Redirect user to app deep link

## Critical Configuration Parameters

### 🔄 `return_url` → Technical Redirect (MANDATORY)

Purpose: The URL where Khalti redirects users after payment completion.

#### Key Characteristics:

- HTTP Method: Must support `GET` requests

- Accessibility: Must be reachable by Khalti servers

- Control: Must be backend-controlled endpoint

- Function: Payment callback entry point for your system

#### Example Configuration:

json

{
"return_url": "https://api.yourdomain.com/v1/store/payments/khalti"
}

#### What Happens:

text

User completes payment on Khalti
↓
Khalti redirects browser to your return_url
↓
GET /v1/store/payments/khalti?pidx=...&status=Completed&transaction_id=...
↓
Your backend processes and finalizes the order

#### Backend Responsibilities:

1.  Parse query parameters (`pidx`, `status`, `transaction_id`, etc.)

2.  Find payment record using the `pidx`

3.  Call Khalti LOOKUP API to verify payment authenticity

4.  Apply atomic database transaction to update order status

5.  Redirect user to appropriate app deep link or success page

⚠️ Critical: This is business-critical for payment verification and order fulfillment.

---

### 🌐 `website_url` → Merchant Identity (CONTEXT ONLY)

Purpose: Your official website URL for merchant identification and compliance.

#### Key Characteristics:

- NOT a callback URL

- NOT used for redirects

- NOT required to handle payment requests

- Should point to your public-facing website

#### Example Configuration:

json

{
"website_url": "https://yourdomain.com"
}

#### Used by Khalti for:

- Merchant information display during payment

- Risk & fraud assessment (verifying legitimate businesses)

- Compliance & trust (regulatory requirements)

- UI context within Khalti app/web interfaces

---

## Implementation Summary

| Parameter     | Purpose                   | Required | Example                                  | Criticality       |
| ------------- | ------------------------- | -------- | ---------------------------------------- | ----------------- |
| `return_url`  | Payment callback endpoint | ✅ Yes   | `https://api.domain.com/payments/khalti` | BUSINESS-CRITICAL |
| `website_url` | Merchant identification   | ✅ Yes   | `https://domain.com`                     | Informational     |

## Best Practices

### For `return_url`:

- Use HTTPS endpoint

- Implement proper error handling

- Add idempotency checks for duplicate callbacks

- Log all incoming requests for debugging

- Keep processing time minimal (< 5 seconds)

### For `website_url`:

- Use your primary domain (not subdomains)

- Ensure the website is publicly accessible

- Keep it updated if your domain changes

- Use a professional, legitimate business website

## Common Pitfalls to Avoid

1.  ❌ DON'T use the same URL for both parameters

2.  ❌ DON'T use frontend/app URLs for `return_url`

3.  ❌ DON'T use API endpoints for `website_url`

4.  ✅ DO test both URLs in sandbox environment

5.  ✅ DO monitor `return_url` endpoints for failures

## Security Considerations

- Validate all incoming parameters from Khalti

- Always verify payments using Khalti LOOKUP API before updating database

- Implement CSRF protection if applicable

- Use HTTPS for all endpoints

- Monitor for unusual payment patterns

Below is a **clear internal guide** (like a mini-spec) for how your checkout + payment system works end-to-end: **order creation, cart locking, gateway initiation, redirects, verification, idempotency, and app UX**. You can drop this into your repo as `docs/payments.md`.

---

# Checkout & Payments System Guide (Khel)

0. Goals

---

This system is designed to ensure:

- **No "paid but order not updated"** bugs

- **No "cart converted without payment"** bugs

- **Idempotency** (safe with retries, duplicate redirects, user refresh, etc.)

- **Gateway payloads are NOT trusted** (redirect params can be tampered)

- Source of truth is always the **gateway verification API**

- Mobile app gets a clean result via **deep links**: `khel://checkout/<state>?...`

---

1. Data Model & Key Fields

---

### `payments` table (core)

Each payment attempt is a row:

- `id` (internal ID)

- `order_id` (FK)

- `provider` (`khalti`, `esewa`, ...)

- `provider_ref` (gateway reference)
  - Khalti: `pidx`

  - eSewa: `transaction_uuid`

- `amount_cents`

- `currency` default `NPR`

- `status` enum: `pending | paid | failed | refunded...`

- `gateway_response` JSONB: raw gateway payload we want to keep

- Unique: `(provider, provider_ref)` to avoid duplicates when ref exists

### `payment_logs` table (support/debug)

Stores events:

- `payment_id`

- `log_type`: `redirect | webhook | response | error`

- `payload`: JSONB

This is important for diagnosing real-world issues.

---

2. Checkout Flow Overview

---

### A) Cart → Checkout Start

User clicks "Checkout".

Server does:

1.  **Validate cart**
    - items available, totals correct, etc.

2.  **Create Order** (if not exists) and link to cart
    - `orders` row created with `status = awaiting_payment` (or similar)

    - `orders.payment_status = pending`

    - record totals, shipping info, etc.

3.  **Lock cart for checkout**
    - cart becomes `checkout_pending`

    - cart is locked to this order:
      - `checkout_order_id = order.id`

    - prevents:
      - double checkouts

      - multiple orders from same cart

      - cart edits while payment is happening

Result: at this point the system has a stable "checkout snapshot".

---

3. Creating a Payment Attempt

---

### B) Choose payment method (khalti/esewa)

When user selects provider and presses "Pay":

Server does:

1.  Create `payments` row
    - `provider = 'khalti'` or `provider = 'esewa'`

    - `status = pending`

    - `amount_cents`, `currency`

    - returns `payment.id`

2.  Call provider adapter: `InitiatePayment(...)`
    - **Khalti** → returns `payment_url` + `pidx`

    - **eSewa** → returns `form_url` + form fields including `transaction_uuid`

3.  Save `provider_ref` immediately (best-effort)
    - `SetProviderRef(payment.id, ref, rawResponse)`

    - For Khalti: `ref = pidx`

    - For eSewa: `ref = transaction_uuid`

Why save it now?

- So return handlers can locate the payment from the redirect using `provider_ref`

1.  Return to mobile:
    - either a URL to open

    - or form fields (for eSewa)

---

4. Gateway Redirect/Return (Browser → API → App)

---

Both Khalti and eSewa are **redirect based**: after payment they send user back to your backend (Return URL). That endpoint is hit by the **user's browser/webview**, not a true webhook.

### C) Return URLs (GET endpoints)

#### Khalti

`GET /v1/store/payments/khalti?pidx=...&status=...&...`

#### eSewa

`GET /v1/store/payments/esewa/return?result=success|failure&data=<base64_json>`

These endpoints do the same high-level steps:

1.  Parse required ref from query:
    - Khalti: `pidx`

    - eSewa: decode base64 JSON and read `transaction_uuid`

2.  Lookup internal payment:
    - `Payments.GetByProviderRef(provider, ref)`

3.  Log redirect payload:
    - `PayLogs.InsertPaymentLog(payment.id, "redirect", query/payload)`

4.  **Verify with gateway API**
    - Do not trust redirect status alone.

    - Khalti: `lookup` API with `pidx`

    - eSewa: `status` API with
      - `product_code`

      - `total_amount`

      - `transaction_uuid`

5.  Apply state transition atomically in DB (transaction)

6.  Redirect back to app using `redirectPaymentResult()`
    - deep link: `khel://checkout/<state>?...`

    - fallback to web if app not installed

---

5. Verification Is the Source of Truth

---

### D) Why verify?

Redirect payload can be:

- tampered (user can change query params)

- incomplete

- wrong in edge cases (pending/ambiguous)

Therefore, **we always call**:

- Khalti lookup API

- eSewa transaction status API

### E) Verification response contract (your internal normalized response)

Your adapters return:

- `Success` (true only when actually paid)

- `Terminal`
  - false when gateway says "still pending"

  - true when gateway says "failed/canceled/expired/not_found/refund"

- `State` raw gateway state string

- `ProviderRef` (pidx/transaction_uuid)

#### Khalti states

- Success only if `Completed`

- Pending: `Pending`, `Initiated`

- Terminal fail: `Expired`, `User canceled`, `Refunded`, `Partially refunded`

#### eSewa states

- Success only if `COMPLETE`

- Pending: `PENDING`, `AMBIGUOUS`

- Terminal fail: `NOT_FOUND`, `CANCELED`, `FULL_REFUND`, `PARTIAL_REFUND`

---

6. Atomic DB Transitions (Most Important Part)

---

### F) When verification says Paid (Success=true)

Inside a single DB transaction:

1.  Re-read payment row (idempotency guard)
    - if already `paid`, stop

2.  `Payments.MarkPaid(paymentID)`
    - sets `payments.status = paid`

    - updates `orders.payment_status = paid`

    - updates `orders.status = processing`

    - sets `orders.paid_at = now()`

3.  Convert cart (strict)
    - `Carts.ConvertCheckoutCart(orderID)`

    - should only convert if:
      - cart `status = checkout_pending`

      - `checkout_order_id = orderID`

Outcome: paid order + converted cart, always consistent.

---

### G) When verification is Pending (Success=false, Terminal=false)

We do **nothing**:

- keep payment `pending`

- keep cart locked

Mobile should show Pending and keep polling.

This avoids failing a payment that may complete a moment later.

---

### H) When verification is Terminal failure (Success=false, Terminal=true)

Inside DB transaction:

1.  Re-check payment (idempotency)

2.  `payments.status = failed`

3.  Optionally update order status: `payment_failed`

4.  `Carts.UnlockCheckoutCart(orderID)`
    - user can retry checkout or choose another payment

Outcome: user is not stuck.

---

7. Deep Link Response to App

---

### `redirectPaymentResult(...)`

Instead of returning JSON errors to a browser, we send an HTML page that:

1.  tries to open `khel://checkout/<state>?query`

2.  after ~1.2s redirects to web fallback if app not installed

3.  shows a button "Open in app"

Deep link format:

`khel://checkout/<state>?order_id=...&payment_id=...&provider=...&ref=...&state=...&reason=...`

- `<state>` is one of:
  - `success`

  - `pending`

  - `failed`

Query keys:

- `order_id` (helpful for UI and fetching order)

- `payment_id` (needed for verify polling)

- `provider` (khalti/esewa)

- `ref` (pidx or transaction_uuid)

- `state` (gateway state string)

- `reason` (internal debug reason)

---

8. Mobile App Behavior

---

### Success screen

- Clears local cart

- invalidates `storeCart` query

- shows confirmation + "Continue shopping"

- optional "View Order"

### Failed screen

- shows error

- shows metadata (order_id/payment_id/provider/state/reason)

- "Try again" → back to checkout

### Pending screen (polling)

- reads deep link params: `payment_id`, `provider`, `ref`

- calls `POST /store/payments/verify`
  - body: `{ payment_id, method: provider, data: { pidx/transaction_uuid } }`

- if `success` → go success

- if `terminal` → go failed

- else keep polling

**Note:** pending is normal in real payments---avoid failing too quickly.

---

9. The Verify Endpoint (POST /verify)

---

This endpoint exists for the app to confirm status again (and for retries).

Flow:

1.  Validate payload: payment_id + method

2.  Load payment row

3.  if already paid → return success idempotent

4.  log inbound data

5.  verify with gateway (network call outside transaction)

6.  apply atomic DB transitions:
    - paid → MarkPaid + ConvertCart

    - pending → no-op

    - terminal fail → failed + UnlockCart

Return:

`{
  "success": true|false,
  "terminal": true|false,
  "state": "Completed|Pending|COMPLETE|PENDING|..."
}`

---

10. Why Some Errors Redirect Instead of JSON Errors

---

Return handlers are hit by a **browser/webview**, not your API client.

So returning `400 JSON` is not helpful; the user sees a blank page.

**Rule:**

- If the request comes from redirect/return endpoints:
  - prefer redirect back to app (failed/pending) with a `reason`

  - keep JSON errors mainly for internal API calls (POST /verify)

In your latest code, you switched to redirecting for missing params --- that's the right UX.

---

11. Idempotency & Race Conditions

---

Handled at 2 levels:

1.  **Early check**
    - if payment already paid, return early

2.  **Inside transaction**
    - re-read payment status before writing

    - prevents double MarkPaid due to:
      - duplicate redirects

      - user refresh

      - repeated callback hits

      - multiple app polls

Also, `(provider, provider_ref)` uniqueness prevents duplicate provider refs.

---

12. Operational Notes / Best Practices

---

- Keep gateway response in `gateway_response` (payments table) for visibility

- Keep all inbound redirect payloads in `payment_logs`

- Treat network verify errors as **pending**, not failed (better UX)

- Always do verification network call **outside** DB tx

- Always do DB transitions **inside** tx

---

13. Quick "Flow Diagram"

---

1.  Mobile: checkout → server creates order + locks cart

2.  Mobile: choose provider → server creates payment(pending)

3.  Server: calls gateway initiate → saves provider_ref

4.  User pays on gateway → gateway redirects browser to your return endpoint

5.  Return handler:
    - get provider_ref

    - find payment

    - verify via gateway API

    - tx: mark paid + convert cart OR fail + unlock cart OR keep pending

    - deep link back to app

6.  Mobile:
    - success → clear cart

    - pending → poll /verify

    - failed → retry

# A) Developer checklist (common mistakes + "done right")

1. Gateway return URLs

---

✅ Must be **GET** endpoints\
✅ Must be reachable from public internet\
✅ Must match what you configured in Khalti/eSewa dashboard

**Khalti**

- `return_url` must be your GET endpoint:\
  `/v1/store/payments/khalti`

- Expect query params like `pidx`, `status`, `transaction_id`, etc.

- You still must call `lookup API` after redirect.

**eSewa**

- `success_url` & `failure_url` must point to your endpoint:\
  `/v1/store/payments/esewa/return`

- eSewa sends base64 `data`, don't trust it alone.

- Always call status-check API after redirect.

✅ Your current approach is correct: redirect → verify → DB tx → deep link.

---

2. Provider reference (provider_ref) mapping

---

This is critical because your return handlers depend on it.

✅ Save provider_ref **as soon as initiate returns**:

- Khalti: `provider_ref = pidx`

- eSewa: `provider_ref = transaction_uuid`

If you don't save provider_ref, you'll get:

- return handler receives ref but can't match payment → "payment not found"

✅ You already do this:

`switch method {
case "khalti": transaction_uuid/pidx ...
case "esewa": ...
}`

---

3. Never trust redirect payload as success

---

Redirect params can be:

- tampered

- missing

- "pending" even if user paid

- "success" even if later reversed

✅ Always verify using gateway API:

- Khalti `lookup`

- eSewa status-check

✅ In UX:

- if verify fails due to network → show **pending** not failed

---

4. Do network calls outside DB transaction

---

✅ Correct:

- verify with gateway outside TX

- then TX only for state transitions

Otherwise you risk:

- long DB locks

- deadlocks

- slowdowns under load

---

5. Atomic transitions (must be in ONE TX)

---

When payment is actually paid, inside one transaction:

- `payments.status = paid`

- `orders.payment_status = paid`

- `orders.status = processing`

- `orders.paid_at = now()`

- `cart converted` (only if locked to this order)

If these happen in separate transactions you can get:

- paid but cart not converted

- cart converted but payment not marked paid

✅ Your MarkPaid + ConvertCheckoutCart approach is correct.

---

6. Idempotency (duplicate redirects / polling)

---

You MUST assume:

- user refreshes return URL

- browser hits return twice

- app polls /verify multiple times

- gateway retries

✅ Guardrails:

- return handler: if already paid, just redirect success

- inside TX: re-check status before updating

---

7. Pending states are normal

---

Do NOT mark failed just because it's not immediately "Complete".

✅ eSewa:

- `PENDING`, `AMBIGUOUS` => keep pending and keep cart locked

✅ Khalti:

- `Pending`, `Initiated` => keep pending

Terminal failure only for:

- canceled / expired / not_found / refund states

---

8. Return endpoints should redirect, not JSON error

---

Return handlers are opened in browser/SFSafariViewController.

If you respond with JSON 400/500, user will see:

- blank page

- confusing error

✅ Best practice:

- Always `redirectPaymentResult(...)` for return endpoints

- Reserve JSON error responses for internal API endpoints like POST `/verify`

---

# B) Debugging playbook (how to diagnose real payment issues)

## Step 1: Find the payment row

Query `payments` by:

- `id`

- OR `(provider, provider_ref)`

Example:

- Khalti pidx: `provider='khalti' AND provider_ref='<pidx>'`

- eSewa uuid: `provider='esewa' AND provider_ref='<transaction_uuid>'`

Check:

- status: pending/paid/failed

- gateway_response contains initiate data

## Step 2: Check payment_logs

Look at `payment_logs` for that payment_id:

- `redirect` log: did return handler receive query/payload?

- `error` log: did verification fail? why?

- any `webhook` logs (if you log them)

This tells you if the problem is:

- missing provider_ref saved earlier

- return handler not being hit

- verify call failing

- DB tx failing

## Step 3: Re-run verify manually

Call your verify endpoint:\
`POST /v1/store/payments/verify`

Payload:

- Khalti:

`{ "payment_id": 123, "method": "khalti", "data": { "pidx": "..." } }`

- eSewa:

`{ "payment_id": 123, "method": "esewa", "data": { "transaction_uuid": "...", "product_code":"...", "total_amount":"..." } }`

If verify succeeds but DB didn't update:

- problem in transaction logic

- cart convert/unlock logic

- MarkPaid update queries

## Step 4: Check cart lock state

Confirm cart is:

- `checkout_pending` when payment pending

- `converted` after paid

- unlocked after terminal failure

If cart stays locked forever:

- verify never reached terminal state

- OR you're not unlocking on terminal fail

- OR status-check API is failing and you're not rechecking later

---

# C) Config / env README snippet (copy-paste)

## Payment environment variables

### Core

- `FRONTEND_URL=https://web.gocloudnepal.com`\
  Used for fallback when deep link fails.

### App deep link scheme

- `APP_SCHEME=khel`\
  Deep link is: `khel://checkout/<state>?...`

---

## Khalti

### Dev / Prod keys

- `KHALTI_SECRET_KEY=...`

- `KHALTI_IS_PROD=false|true`

### Redirect URLs

- `KHALTI_RETURN_URL=https://api.gocloudnepal.com/v1/store/payments/khalti`\
  Must be GET supported.

### Website URL (required by Khalti)

- `KHALTI_WEBSITE_URL=https://gocloudnepal.com`\
  This is your public site, used by Khalti to validate merchant context.

---

## eSewa

### Merchant secrets

- `ESEWA_MERCHANT_CODE=...`

- `ESEWA_SECRET_KEY=...`

- `ESEWA_IS_PROD=false|true`

### Return URLs

- `ESEWA_SUCCESS_URL=https://api.gocloudnepal.com/v1/store/payments/esewa/return`

- `ESEWA_FAILURE_URL=https://api.gocloudnepal.com/v1/store/payments/esewa/return`

---

# D) Checkout flow in depth (your system step-by-step)

Below is the "full story" in the exact order things happen.

---

1. User builds cart (client + server consistency)

---

- You have a local cart store (`useCartStore`) for UI speed

- You also maintain a server cart (`storeCart` query)

- At checkout time, server becomes the source of truth for totals and stock

---

2. User clicks "Place order" (Checkout request)

---

### Server does:

1.  Validate user + cart

2.  Validate shipping info exists (address, city, phone)

3.  Create an **Order**
    - order totals, shipping data stored

    - status likely `awaiting_payment`

4.  Lock cart:
    - cart becomes `checkout_pending`

    - cart points to `checkout_order_id = order.id`

5.  Create Payment row:
    - payment.status = pending

    - payment.provider = chosen method

Return to client:

- `order_id`, `payment_id`

- if COD: done

- if online: payment_url + payment_data

---

3. COD flow (simple)

---

If payment method is `cash_on_delivery`:

1.  clear cart

2.  order becomes "processing"

3.  redirect to `/checkout/success?order_id=...`

No gateway involved.

---

4. Online flow (Khalti/eSewa)

---

Client navigates to payment UI (WebView or expo-web-browser).

### 4A) For Khalti

- Server initiate returns `payment_url` and `pidx`

- Save provider_ref = `pidx`

- Client opens `payment_url`

After payment:

- Khalti redirects to your `KHALTI_RETURN_URL` with query params

- Your `khaltiReturnHandler`:
  1.  reads `pidx`

  2.  finds payment by provider_ref

  3.  calls Khalti lookup API

  4.  TX:
      - paid → MarkPaid + ConvertCart

      - pending → no-op

      - terminal fail → fail + unlock cart

  5.  deep link back to app

---

### 4B) For eSewa

- Server initiate returns `PaymentURL` + form fields including `transaction_uuid`

- Save provider_ref = `transaction_uuid`

- Client opens form via POST

After payment:

- eSewa redirects to your return url with `data` base64

- `esewaReturnHandler`:
  1.  decode base64

  2.  optional signature check (integrity)

  3.  find payment by provider_ref=transaction_uuid

  4.  call eSewa status-check API

  5.  TX:
      - COMPLETE → MarkPaid + ConvertCart

      - PENDING/AMBIGUOUS → keep pending

      - terminal fail → fail + unlock cart

  6.  deep link back to app

---

5. Mobile app receives deep link → routes to screens

---

Deep link format:\
`khel://checkout/<state>?order_id=...&payment_id=...&provider=...&ref=...&state=...&reason=...`

Your screens should:

- **success**: show confirmation, clear local cart, optionally fetch order

- **failed**: show reason + "Try again"

- **pending**: poll `/verify` using `payment_id + provider + ref`

---

6. Pending screen (poll /verify)

---

Your polling logic is good in concept.

But now, since your return handlers already verify and transition,\
pending is mainly needed when:

- gateway returns pending/ambiguous

- your server verification temporarily fails

- user closed browser too fast

### Polling should send:

- method: `provider`

- data should include:
  - Khalti: `{ pidx: ref }`

  - eSewa: at minimum `{ transaction_uuid: ref }`
    - (your server can fetch total_amount/product_code from DB if you store them)

---

# E) Strong recommendation (small improvement)

For eSewa verify, you currently require:

- product_code

- total_amount

- transaction_uuid

If the app is polling and only has `transaction_uuid`, you can make your server smarter:

✅ Store `product_code` and `total_amount` in `gateway_response` during initiate, so verify can load them from DB by payment_id.

That way the client only sends:

- payment_id + transaction_uuid\
  and your server fills the rest.

This makes your polling **much more reliable**.

Payment flow:
paymentProvider(esewa, khalti):

- Opens in system browser
- Provider redirect to backend happens in browser
- backend redirects after verification to khel://checkout/status
- if app is not open fallback to website

Khalti:

- return_url → backend handler
  Backend:
- logs redirect
- calls lookup API
- applies atomic DB transaction
- redirects to deep link
- App opens automatically

note for self

- Use provider_ref for the stable identifier:
  - Khalti => pidx
  - eSewa => transaction_uuid
- Use gateway_response JSONB to store everything else (init response, lookup raw, etc)

/store/payments/esewa/start?payment_id=xxx (handler to auto-post HTML)

- this is the best way to use expo-web-browser because system browser can't do a raw post body easily like webview hack can but html can auto-submit.

What it does

- validate payment_id
- loads payment
- calls initiatePayment() again (fresh uuid each attempt)
- overrides success and failure url to include payment_id
- save provider_ref = transaction_uuid
- gateway_response = form fields
- returns an html page that auto-submits to esewa form url

payment callback handler

- redirectToAppReturn is correct and matches your app redirect target:
- khel://payments/return?...
- eSewa start: re-initiate with a fresh transaction_uuid ✅
- eSewa return: decode base64 → verify signature → call status-check API (source of truth) ✅
- Khalti return: lookup by pidx → call lookup API → apply DB tx atomically ✅

Quick mental model

Order table constraint, only these states are allowed:

✅ active + checkout_order_id NULL
✅ checkout_pending + checkout_order_id SET
✅ checkout_pending + checkout_order_id NULL (not typical but allowed)
✅ converted + checkout_order_id NULL
✅ abandoned + checkout_order_id NULL

❌ converted + checkout_order_id SET (your current convert does this)
❌ active + checkout_order_id SET (if you set order id before status)
