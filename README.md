# tramas

## Technology Stack

| Layer | Technology |
|---|---|
| Mobile | Flutter / Dart (iOS and Android) |
| API | Go / Echo |
| Admin Web | Go / templ / htmx |
| Background Worker | Go |
| Database | PostgreSQL |
| Reverse Proxy | Caddy |
| Containers | Docker Compose |

## Configuration

| File | Purpose |
|---|---|
| `compose.yaml` | Shared service definitions |
| `compose.dev.yaml` | Development overrides; HTTP on `127.0.0.1:8080` by default |
| `compose.prod.yaml` | Production overrides; HTTP/HTTPS on ports 80 and 443 |
| `.env.example` | Environment variable template |
| `backend/go.mod` / `backend/go.sum` | Go module and dependency versions |
| `mobile/.fvmrc` | Flutter SDK version |
| `mobile/pubspec.yaml` / `mobile/pubspec.lock` | Flutter dependencies and locked versions |
| `mobile/config/` | Environment-specific API endpoints |

## Directory Structure

```text
.
├── backend/
│   ├── cmd/
│   │   ├── api/
│   │   └── worker/
│   ├── internal/
│   │   ├── auth/
│   │   ├── admin/
│   │   ├── users/
│   │   ├── baseball/
│   │   ├── social/
│   │   ├── content/
│   │   ├── ai/
│   │   └── platform/
│   ├── web/
│   │   ├── components/
│   │   ├── pages/
│   │   └── static/
│   ├── migrations/
│   ├── go.mod
│   └── go.sum
├── mobile/
│   ├── android/
│   ├── ios/
│   ├── lib/
│   │   ├── api/
│   │   └── config/
│   ├── config/
│   ├── test/
│   ├── pubspec.yaml
│   └── pubspec.lock
├── infra/
│   ├── docker/
│   │   ├── api/
│   │   ├── worker/
│   │   ├── postgres/
│   │   └── caddy/
│   └── caddy/
├── docs/
│   ├── architecture/
│   ├── database/
│   └── api/
├── scripts/
├── compose.yaml
├── compose.dev.yaml
├── compose.prod.yaml
└── .env.example
```
