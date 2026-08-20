#!/usr/bin/env pwsh
# Windows wrapper mirroring the Makefile targets, for shells without make.
#
#   ./make.ps1 test
#   ./make.ps1 check
#   ./make.ps1 record

param([Parameter(Position = 0)][string]$Target = "help")

switch ($Target) {
    "build"  { docker compose build }
    "setup"  { docker compose run --rm setup }
    "doctor" { docker compose run --rm doctor }
    "reset"  { docker compose down -v; docker compose build --no-cache }
    "test"   { docker compose run --rm test }
    "check"  { docker compose run --rm check }
    "record" { docker compose run --rm record }
    "vendor" { docker compose run --rm vendor }
    "deps"   { docker compose run --rm deps }
    "shell"  { docker compose run --rm shell }
    "clean"  { docker compose down -v }
    "native" { go test -race -count=1 ./... }
    default  {
        Write-Host "targets: build setup doctor test check record vendor deps shell clean reset native"
    }
}
