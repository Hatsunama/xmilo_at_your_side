# Bridge HTTP Contract — working starter version

Base URL: `http://127.0.0.1:42817`

All routes require:

- `Authorization: Bearer <localhost_token>`

## Implemented routes in v2 starter

- `GET /health`
- `GET /ready`
- `POST /bootstrap`
- `POST /auth/refresh`
- `POST /task/start`
- `GET /task/current`
- `POST /task/interrupt`
- `POST /task/cancel`
- `POST /task/choice` (stub)
- `POST /task/resume_queue` (stub)
- `POST /thought/request`
- `GET /state`
- `GET /storage/stats`
- `POST /reset` (guarded partial implementation)
- `POST /trophy/conjure` (stub)
- `POST /inspector/open` (stub)
- `POST /inspector/close` (stub)

## Notes

The route set matches the authority docs as closely as possible, but some are currently present as stubs so the app and sidecar can evolve without contract churn.
