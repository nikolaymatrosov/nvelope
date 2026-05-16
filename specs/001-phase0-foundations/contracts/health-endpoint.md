# Contract: Health / Readiness Endpoint (API service)

The only externally observable runtime interface in Phase 0. The Worker and Scheduler
services expose no network interface this phase.

## `GET /healthz`

Liveness + readiness probe for the API service. Unauthenticated, no tenant scope.

### Request

- Method: `GET`
- Path: `/healthz`
- No headers, query, or body required.

### Response — healthy

- Status: `200 OK`
- Content-Type: `application/json`
- Body:

  ```json
  {
    "status": "ok",
    "service": "api",
    "version": "<build-version>"
  }
  ```

### Response — not ready

- Status: `503 Service Unavailable`
- Content-Type: `application/json`
- Body:

  ```json
  {
    "status": "unavailable",
    "service": "api",
    "version": "<build-version>"
  }
  ```

  Returned while the service is still starting or is draining after a shutdown signal.

### Behavior contract

- The endpoint MUST NOT require authentication and MUST NOT touch tenant data.
- It MUST respond `200` only when the process has completed startup (config loaded and
  validated, logger and DB pool initialized).
- During graceful drain it MUST respond `503` so Kubernetes stops routing traffic.
- Response time MUST be well under any reasonable probe timeout (target < 100 ms).
- The Kubernetes `Deployment` for the API service references this path for both its
  `livenessProbe` and `readinessProbe`.
