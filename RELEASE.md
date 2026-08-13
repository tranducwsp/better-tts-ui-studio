# Release Checklist

Checklist for tagging and publishing a new version of the AI Voice Studio platform.

---

## Before tagging `v1.0.0`

- [ ] `go vet ./backend/...` — no warnings
- [ ] `go build ./backend/...` — no errors
- [ ] `go test -count=1 -short ./backend/tests/unit/...` — all pass
- [ ] `npm test` — all pass
- [ ] `npm run check` — no type errors
- [ ] `scripts/check-test-layout.sh` — clean
- [ ] `.env.example` regenerated: `cd backend && go generate ./config`
- [ ] Core engine contract (`core-tts-example/`) aligns with `docs/GATEWAY.md`
- [ ] All documentation in `docs/` is up to date
- [ ] No uncommitted changes that should be committed

## Release steps

1. Bump version:
   - `frontend/package.json` — set `"version": "1.0.0"`
   - `k8s/Chart.yaml` — set `version: 1.0.0`
2. Update `CHANGELOG.md` with the new version
3. Commit: `git commit -m "chore: bump to v1.0.0"`
4. Tag: `git tag v1.0.0`
5. Push: `git push && git push --tags`

## Post-release

- Build Docker images: `docker compose build`
- Push to registry (if applicable): `docker compose push`

## Version history

| Version | Date | Notes |
|---------|------|-------|
| 1.0.0   |      | First release |