<#
.SYNOPSIS
  Dev helper for heb-inventory on Windows.

.EXAMPLE
  .\scripts\dev.ps1 build
  .\scripts\dev.ps1 test
  .\scripts\dev.ps1 up        # docker compose up --build
  .\scripts\dev.ps1 down      # docker compose down -v
  .\scripts\dev.ps1 image     # docker build
  .\scripts\dev.ps1 lint      # go vet + helm lint + kustomize build
  .\scripts\dev.ps1 deploy    # apply k8s/ to current context
  .\scripts\dev.ps1 pprof     # port-forward pprof and open CPU profile
  .\scripts\dev.ps1 logs      # tail pod logs
#>
[CmdletBinding()]
param(
  [Parameter(Position = 0)]
  [ValidateSet('build', 'test', 'up', 'down', 'image', 'lint', 'deploy', 'pprof', 'logs', 'help')]
  [string]$Command = 'help'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$image = 'ghcr.io/theoriginalllama/heb-inventory:dev'
$namespace = 'heb-inventory'

function Invoke-Build {
  go build -trimpath -o bin/server.exe ./cmd/server
  Write-Host "built bin/server.exe" -ForegroundColor Green
}

function Invoke-Test {
  go vet ./...
  go test -race ./...
}

function Invoke-Up {
  docker compose up --build -d
  Write-Host "waiting for /readyz..." -ForegroundColor Yellow
  $deadline = (Get-Date).AddSeconds(60)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-WebRequest -Uri 'http://localhost:8080/readyz' -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) {
        Write-Host "ready" -ForegroundColor Green
        return
      }
    } catch { Start-Sleep -Seconds 2 }
  }
  Write-Warning "readyz did not return 200 within 60s; check 'docker compose logs app'"
}

function Invoke-Down {
  docker compose down -v
}

function Invoke-Image {
  docker build -t $image .
}

function Invoke-Lint {
  go vet ./...
  if ($?) { helm lint charts/heb-inventory }
  if ($?) { kubectl kustomize k8s/ | Out-Null }
  Write-Host "lint ok" -ForegroundColor Green
}

function Invoke-Deploy {
  kubectl apply -k k8s/
  kubectl -n $namespace rollout status deploy/heb-inventory --timeout=120s
}

function Invoke-Pprof {
  $pod = kubectl -n $namespace get pod -l app.kubernetes.io/name=heb-inventory -o jsonpath='{.items[0].metadata.name}'
  if (-not $pod) { throw "no heb-inventory pod found in namespace $namespace" }
  Write-Host "port-forwarding $pod 6060:6060 — Ctrl+C to stop" -ForegroundColor Yellow
  Write-Host "open http://localhost:6060/debug/pprof/ in a browser, or run:" -ForegroundColor Cyan
  Write-Host "  go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30" -ForegroundColor Cyan
  kubectl -n $namespace port-forward $pod 6060:6060
}

function Invoke-Logs {
  kubectl -n $namespace logs -l app.kubernetes.io/name=heb-inventory --tail=200 -f
}

switch ($Command) {
  'build'  { Invoke-Build }
  'test'   { Invoke-Test }
  'up'     { Invoke-Up }
  'down'   { Invoke-Down }
  'image'  { Invoke-Image }
  'lint'   { Invoke-Lint }
  'deploy' { Invoke-Deploy }
  'pprof'  { Invoke-Pprof }
  'logs'   { Invoke-Logs }
  default  { Get-Help $PSCommandPath -Detailed }
}
