# Security Audit — 2026-07-27

## Scope

The audit covered Go and npm dependencies, application code, Dockerfiles, runtime images, reverse-proxy configuration, and the deployed Compose services.

## Remediation

- Upgraded to Go 1.26, current Go dependencies, Svelte 5, Vite 8, and Node 24.
- Restricted browser requests to an explicit origin allowlist, removed wildcard CORS, limited trusted proxies, bound the production backend to loopback, and added HTTP timeouts and input limits.
- Added CSP and browser security headers, rejected unknown host names, and limited request and signaling field sizes.
- Updated and pinned container base versions, installed OS security updates, and changed all production containers to non-root users with dropped capabilities and no-new-privileges. Nginx filesystems are read-only.
- Added explicit bounds checks and error handling identified by static analysis.

## Verification

- `npm audit`: 0 vulnerabilities.
- `govulncheck`: 0 reachable or imported-package vulnerabilities.
- `gosec`: 0 findings.
- Trivy repository scan: 0 dependency vulnerabilities, misconfigurations, or secrets.
- Trivy runtime scan with fixes available: 0 medium, high, or critical findings across backend, frontend, and nginx.
- Go tests, `go vet`, 15 frontend tests, Svelte checks, and production builds pass.
- A headless Chrome smoke test rendered the production UI without CSP or application errors.
- Deployed health, program-guide dependencies, container health checks, and a real native NVENC initialization probe are healthy.

## Upstream-only Findings

Ubuntu 24.04 reports seven unique medium-severity CVEs without vendor fixes. `GO-2026-5932` also flags the unused `x/crypto/openpgp` package as unsafe by design; the application neither imports nor calls it. Neither category currently has an actionable update. Re-run the scans when upstream fixes become available.
