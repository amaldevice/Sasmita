# Packaged build — integration tip `7963d09`

Performed after all 16 tickets and the code-review fixes were merged.

## Command

```sh
go install github.com/egoist/mygo/cmd/mygo@v0.10.0   # CLI into a temp GOBIN (outside the repo)
cd shell/mygo && mygo build                           # runs `npm --prefix ../.. run build`, embeds frontend-dist
```

MyGo v0.10.0, Go 1.27.3 toolchain, Vite 7.3.8, darwin/arm64.

## Result

| Artifact | Size |
|---|---|
| `build/darwin-arm64/Sasmita.app` | 22.4 MB |
| `build/darwin-arm64/sasmita-0.1.0-arm64.dmg` | 6.6 MB |

- `hdiutil verify` on the disk image: **checksum VALID**.
- `codesign -dv`: `Identifier=dev.sasmita.app`, `Signature=adhoc`, no TeamIdentifier.
- `mygo build` regenerated `frontend/src/mygo.ts`; `npm run check` (0 errors/0 warnings) and `npm run test:e2e` (50 passed) still pass on the regenerated client.

## Not verified in this pass

- Not notarized and no Developer ID signature — distribution outside the Mac App Store needs `macos.signingIdentity` in `mygo.json`.
- Idle/chat RSS, startup time, real WKWebView isolation, freeze recovery and per-iframe memory were **not** measured here. Those remain manual packaged-app checks carried over from spike S1 (see `docs/benchmarks/s1-mygo-walking-skeleton.md`, `ticket-28-artifact-isolation.md`, `ticket-29-live-artifacts.md`).
