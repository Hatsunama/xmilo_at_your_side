# Planned Relay Access API — xMilo_v3

These routes are not all implemented yet.
They are the next build-facing contract surface implied by the locked product rules.

## Email verification

### `POST /email/send-code`
Request:
```json
{
  "email": "user@example.com"
}
```

Response:
```json
{
  "sent": true,
  "cooldown_seconds": 60,
  "expires_in_seconds": 600
}
```

### `POST /email/verify-code`
Request:
```json
{
  "email": "user@example.com",
  "code": "123456",
  "device_name": "Seeker",
  "app_version": "0.1.0"
}
```

Response:
```json
{
  "verified": true,
  "access_token": "...",
  "refresh_token": "...",
  "trial_used": false,
  "access_state": "verified_no_access"
}
```

## Access codes

### `POST /access/redeem-code`
Request:
```json
{
  "email": "user@example.com",
  "code": "XMILO-ABC123"
}
```

Response:
```json
{
  "redeemed": true,
  "access_state": "access_code_active",
  "expires_at": "2026-03-24T12:00:00Z"
}
```

### `POST /admin/access-codes/batch-create`
Creates a batch of single-use access codes.

## Trial state

### `POST /access/start-trial`
Called once when the first real task/request begins.

### `GET /access/state`
Returns current verified-email access state, including whether the one-time trial is already used.

## Billing / entitlement

### `POST /billing/restore`
Manual restore-purchases fallback.

### `POST /billing/webhook/revenuecat`
Receives entitlement changes from RevenueCat.

## Admin support

### `GET /admin/installs`
Returns the basic install health metadata list.

### `GET /admin/errors`
Returns unresolved error reports.

### `POST /admin/errors/{id}/clear`
Moves an unresolved error to the cleared list.
