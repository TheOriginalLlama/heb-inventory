# heb-inventory

Inventory API for tracking SKUs and stock levels across store locations. Go HTTP service + Postgres, deployable as raw Kubernetes manifests or a Helm chart.

Built as a portfolio demo for a Senior Software Engineer (DevOps) role: production-shaped service + production-shaped deployment.

## Architecture

```
client -> [Ingress] -> [Service] -> [Deployment: heb-inventory (Go)]
                                          |
                                          v
                                    [Postgres (StatefulSet + PVC)]
```

- **Service** (`cmd/server`, `internal/`): Go 1.22, stdlib `net/http` routing, `slog` JSON logging, Prometheus metrics, graceful shutdown.
- **Store** (`internal/store`): `pgx/v5` connection pool; stock adjustments are transactional with a movements audit log.
- **Schema** (`db/migrations`): forward/reverse SQL migrations.
- **Container**: multi-stage build → distroless static, non-root.
- **Kubernetes** (`k8s/`): raw manifests + Kustomize.
- **Helm** (`charts/heb-inventory`): templated chart with optional in-cluster Postgres.
- **CI** (`.github/workflows/ci.yml`): vet, test, build, Docker push to GHCR, `helm lint`, kubeconform validation.

## API

| Method | Path                            | Description                       |
|--------|---------------------------------|-----------------------------------|
| POST   | `/items`                        | Create an item                    |
| GET    | `/items`                        | List items (`limit`, `offset`)    |
| GET    | `/items/{sku}`                  | Get a single item                 |
| PUT    | `/items/{sku}`                  | Update item fields                |
| POST   | `/items/{sku}/adjust`           | Apply stock delta at a store      |
| GET    | `/stores/{storeID}/stock`       | List stock levels at a store      |
| GET    | `/healthz`                      | Liveness                          |
| GET    | `/readyz`                       | Readiness (DB ping)               |
| GET    | `/metrics`                      | Prometheus metrics                |

## Run it

### Option 1: Docker Compose (fastest)

```bash
docker compose up --build
curl http://localhost:8080/healthz
curl -X POST http://localhost:8080/items \
  -H 'content-type: application/json' \
  -d '{"sku":"MILK-1G","name":"Whole Milk Gallon","department":"dairy","unit_price_cents":399}'
curl -X POST http://localhost:8080/items/MILK-1G/adjust \
  -H 'content-type: application/json' \
  -d '{"store_id":"AUSTIN-01","delta":50,"reason":"restock"}'
curl http://localhost:8080/stores/AUSTIN-01/stock
```

### Option 2: Local Go + Postgres

```bash
# start Postgres only
docker compose up -d postgres

export DATABASE_URL="postgres://inventory:inventory@localhost:5432/inventory?sslmode=disable"
go run ./cmd/server
```

### Option 3: Kubernetes (kind or minikube)

```bash
# build & load the image into kind
docker build -t ghcr.io/theoriginalllama/heb-inventory:latest .
kind load docker-image ghcr.io/theoriginalllama/heb-inventory:latest

# raw manifests
kubectl apply -k k8s/
kubectl -n heb-inventory rollout status deploy/heb-inventory

# port-forward and hit it
kubectl -n heb-inventory port-forward svc/heb-inventory 8080:80
curl http://localhost:8080/healthz
```

### Option 4: Helm

```bash
helm install inv charts/heb-inventory --create-namespace --namespace heb-inventory
# ...iterate...
helm upgrade inv charts/heb-inventory --namespace heb-inventory --set image.tag=v0.1.0
```

For a production deployment, point at a managed Postgres and disable the in-cluster one:

```bash
helm install inv charts/heb-inventory \
  --set postgres.enabled=false \
  --set database.url="postgres://USER:PASS@host:5432/inventory?sslmode=require"
```

## Configuration

All config via env vars (loaded from `ConfigMap` + `Secret` in cluster):

| Variable                    | Default     | Notes                              |
|-----------------------------|-------------|------------------------------------|
| `HTTP_ADDR`                 | `:8080`     | Listen address                     |
| `DATABASE_URL`              | (required)  | Postgres connection string         |
| `LOG_LEVEL`                 | `info`      |                                    |
| `SHUTDOWN_TIMEOUT_SECONDS`  | `15`        | Graceful shutdown drain window     |
| `PPROF_ADDR`                | (unset)     | If set (e.g. `:6060`), serves `/debug/pprof/*` on a separate port. Not exposed via Service. |

## Development

```bash
go vet ./...
go test -race ./...
go build ./...
helm lint charts/heb-inventory
kubectl kustomize k8s/ | kubeconform -strict -ignore-missing-schemas
```

## Layout

```
.
├── cmd/server/             # main.go, wires config + store + handlers + pprof
├── internal/
│   ├── config/             # env-var config
│   ├── handlers/           # HTTP routes + observability middleware
│   └── store/              # pgx-backed Postgres store
├── db/migrations/          # forward/reverse SQL migrations
├── k8s/                    # raw manifests + Kustomize
├── charts/heb-inventory/   # Helm chart
├── observability/          # Prometheus alert rules
├── scripts/                # PowerShell dev helpers
├── .github/workflows/      # CI
├── Dockerfile              # multi-stage, distroless
├── docker-compose.yml      # local dev
├── README.md
└── RUNBOOK.md              # on-call procedures
```

## JD coverage

This project was built against the H-E-B Senior Software Engineer - DevOps posting. Each JD bullet maps to a specific file or feature:

| JD requirement                                                  | Where in repo |
|------------------------------------------------------------------|---------------|
| Golang                                                           | `cmd/server`, `internal/` |
| Python / **PowerShell**                                          | `scripts/dev.ps1` (Windows dev helper) |
| **Kubernetes**                                                   | `k8s/` (raw + Kustomize), `charts/heb-inventory/` (Helm) |
| AWS / GCP / VMware                                               | `charts/heb-inventory/values.yaml` — set `postgres.enabled=false` and point `database.url` at managed cloud Postgres |
| **Build and deployment pipelines**                               | `.github/workflows/ci.yml` — vet, test, build, image push to GHCR, helm lint, kubeconform |
| **Production-ready code, tests, edge cases, errors**             | `internal/handlers/handlers_test.go`, error mapping in `internal/store/store.go`, transactional stock adjust in `internal/store/items.go` |
| **System and data architecture / design patterns**               | Items / stock / movements separation; movements as immutable audit log; transactional upsert |
| **Profiling tools**                                              | `cmd/server/main.go` — `net/http/pprof` on `:6060`; `scripts/dev.ps1 pprof` to port-forward |
| **System monitoring; best practices**                            | `/metrics` (Prometheus), `observability/alerts.yaml` (PrometheusRule), Deployment scrape annotations |
| **Production support / on-call / debugging**                     | `RUNBOOK.md` — failure modes, diagnostics, rollback, escalation, RCA template |
| **Architecting for performance, sustainability, reliability**    | HPA, PDB, rolling update `maxUnavailable: 0`, distroless + non-root + read-only FS, resource requests/limits, graceful shutdown |
| **Git, common eng tools**                                        | Conventional Git layout; CI gates on `go mod tidy` cleanliness |
| **Documentation / knowledge sharing**                            | This README, `RUNBOOK.md`, inline `NOTES.txt` in Helm chart |
| **Root cause analysis**                                          | `RUNBOOK.md` → RCA template |
