# Security Policy

## Reporting a Vulnerability

If you discover a security issue, please contact the maintainer directly instead of opening a public issue. We will acknowledge receipt within 48 hours and work on a fix before public disclosure.

## Scope

The following areas are in scope for security review:

- **Go backend** — API gateway, authentication, session management, task queue, storage abstraction
- **SQL injection, command injection, path traversal** — all input paths
- **JWT token handling, session hijacking, cookie security**
- **File upload** — validation, storage isolation, extension whitelist

Out of scope:

- **`core-tts-example/`** — example stub, not a production engine
- **Third-party dependencies** — report vulnerabilities upstream
- **Deployment environment** — network, reverse proxy, OS hardening are the operator's responsibility

## Built-in Security Features

| Feature | Implementation |
|---------|---------------|
| Password hashing | bcrypt |
| Session management | JWT with HttpOnly cookies, short-lived access tokens (15 min), refresh token rotation (30 days) |
| Rate limiting | Login/register: 10 requests per minute per IP |
| CSRF protection | SameSite=Lax cookies, CORS restricted to frontend origin |
| SQL injection | Parameterized queries via sqlc — no string concatenation |
| File upload | Extension whitelist, size limit, stored outside web root |
| Docker | Non-root user (`appuser`), no shell in runtime image |
| Request body limit | Configurable max request size (default 10 MB) |
| Concurrency limit | Ceiling on heavy operations (e.g., file extraction: max 8 concurrent) |

## Deployment Checklist

Before running in production:

1. **Change `SECRET_KEY`** — generate a random 64-character string
2. **Enable HTTPS** — use a reverse proxy (nginx, Caddy, Traefik) with TLS termination
3. **Set a strong `REDIS_PASSWORD`** — at least 32 random characters
4. **Enable cookie Secure flag** — `COOKIE_SECURE=1` when behind HTTPS
5. **Use S3 storage** — recommended for multi-replica deployments; local disk is single-node only
6. **Restrict database access** — PostgreSQL should not be exposed to the public network
7. **Review user registration** — new accounts are `pending` by default; approve via `/api/admin/users/{id}/approve`