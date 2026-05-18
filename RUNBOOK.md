# heb-inventory Runbook

On-call reference for the inventory service. Each section answers: **how do I see what's wrong, and how do I fix it.**

---

## Quick links

```bash
# pod status
kubectl -n heb-inventory get pods -l app.kubernetes.io/name=heb-inventory

# tail logs (JSON; pipe to jq)
kubectl -n heb-inventory logs -l app.kubernetes.io/name=heb-inventory --tail=200 -f | jq

# recent restarts / events
kubectl -n heb-inventory get events --sort-by=.lastTimestamp | tail -30

# rollout status
kubectl -n heb-inventory rollout history deploy/heb-inventory

# port-forward to hit the API directly
kubectl -n heb-inventory port-forward svc/heb-inventory 8080:80

# port-forward to pprof on a single pod
POD=$(kubectl -n heb-inventory get pod -l app.kubernetes.io/name=heb-inventory -o jsonpath='{.items[0].metadata.name}')
kubectl -n heb-inventory port-forward "$POD" 6060:6060
```

## Health endpoints

| Endpoint   | Meaning                          | Probe         |
|------------|----------------------------------|---------------|
| `/healthz` | Process is alive                 | liveness      |
| `/readyz`  | Process can reach Postgres       | readiness     |
| `/metrics` | Prometheus scrape                | (scrape only) |

If `/healthz` is 200 but `/readyz` is 503, the app is up but the DB is unreachable from this pod — see "Database unreachable" below.

## Common failure modes

### 1. Pods CrashLoopBackOff at startup

**Symptoms:** `kubectl get pods` shows restart count climbing. Logs end with `"DATABASE_URL is required"` or `failed to connect to <host>`.

**Diagnose:**
```bash
kubectl -n heb-inventory describe pod <pod>
kubectl -n heb-inventory get secret heb-inventory-secrets -o jsonpath='{.data.DATABASE_URL}' | base64 -d
```

**Resolve:**
- Missing/invalid `DATABASE_URL` → fix the Secret, then `kubectl rollout restart deploy/heb-inventory`.
- Postgres not yet ready → wait for `postgres-0` to be Ready, the app will recover on its next probe.

### 2. Readiness flapping (503 from `/readyz`)

**Symptoms:** Pods toggle between Ready and NotReady. `http_requests_total{status="Service Unavailable"}` increases on `/readyz`.

**Diagnose:**
```bash
kubectl -n heb-inventory exec -it postgres-0 -- pg_isready -U inventory -d inventory
kubectl -n heb-inventory logs -l app.kubernetes.io/name=heb-inventory | jq 'select(.level=="ERROR")'
```

**Resolve:**
- DB pool exhausted (look for `acquire connection: timeout`): bump `MaxConns` in `internal/store/store.go` or right-size the workload.
- DB OOMKilled: `kubectl -n heb-inventory describe pod postgres-0` → look at last-state reason. Raise Postgres limits in `k8s/postgres-statefulset.yaml`.

### 3. High 5xx rate

**Symptoms:** `InventoryHighErrorRate` alert firing.

**Diagnose:**
```bash
# top error paths from logs in the last 10 minutes
kubectl -n heb-inventory logs -l app.kubernetes.io/name=heb-inventory --since=10m \
  | jq -r 'select(.level=="ERROR") | "\(.msg)\t\(.err)"' | sort | uniq -c | sort -rn | head
```

**Resolve:** If a single handler dominates, check the SQL it runs against `pg_stat_statements`. If errors are spread across handlers, suspect DB or downstream.

### 4. p95 latency regression

**Symptoms:** `InventoryP95LatencyHigh` alert.

**Diagnose:**
```bash
# capture a 30s CPU profile from a single pod
POD=$(kubectl -n heb-inventory get pod -l app.kubernetes.io/name=heb-inventory -o jsonpath='{.items[0].metadata.name}')
kubectl -n heb-inventory port-forward "$POD" 6060:6060 &
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=30
```

**Resolve:** Usually a slow query. Check `EXPLAIN ANALYZE` against `pg_stat_statements`; add an index in a new migration. Don't add indexes blindly — verify with the profile and explain plan first.

### 5. Memory leak / OOMKilled

**Symptoms:** `InventoryPodOOMKilled` alert, or pod restart with `OOMKilled` reason.

**Diagnose:**
```bash
# heap snapshot
kubectl -n heb-inventory port-forward "$POD" 6060:6060 &
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/heap
```

**Resolve:** Common cause is unbounded slices / leaked goroutines. Use the `goroutine` profile (`/debug/pprof/goroutine?debug=2`) to find stuck goroutines.

## Rolling back

```bash
# inspect history
kubectl -n heb-inventory rollout history deploy/heb-inventory

# roll back one revision
kubectl -n heb-inventory rollout undo deploy/heb-inventory

# roll back to a specific revision
kubectl -n heb-inventory rollout undo deploy/heb-inventory --to-revision=<N>

# verify
kubectl -n heb-inventory rollout status deploy/heb-inventory
```

The Deployment uses `maxUnavailable: 0` and a PodDisruptionBudget of `minAvailable: 1`, so rollbacks are zero-downtime as long as ≥2 replicas are running.

## Scaling

```bash
# manual override (HPA will resume control after stabilization window)
kubectl -n heb-inventory scale deploy/heb-inventory --replicas=4

# inspect HPA decisions
kubectl -n heb-inventory describe hpa heb-inventory
```

## Database operations

```bash
# psql in cluster
kubectl -n heb-inventory exec -it postgres-0 -- psql -U inventory -d inventory

# top slow queries
kubectl -n heb-inventory exec -it postgres-0 -- \
  psql -U inventory -d inventory -c "SELECT query, calls, mean_exec_time FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

> `pg_stat_statements` needs to be loaded via `shared_preload_libraries`. The in-cluster Postgres in this chart does not enable it by default — enable it in `k8s/postgres-statefulset.yaml` or rely on managed Postgres tooling in real environments.

## Escalation

1. Page primary on-call (PagerDuty schedule: *fill in*).
2. After 15 min unresolved with customer-facing impact, page secondary.
3. Open an incident channel `#inc-inventory-<date>`.
4. Within 48h of resolution, write the RCA and link it from the alert post-mortem.

## RCA template

Every incident gets one file in `docs/rca/<YYYY-MM-DD>-<slug>.md` with these sections:

- **Summary** (one paragraph)
- **Impact** (who, what, how long, in customer-facing units)
- **Timeline** (UTC timestamps, what we did)
- **Root cause** (technical, not "human error")
- **Detection** (how we knew — was it the alert that should have caught it?)
- **Action items** (each with an owner and a ticket)
