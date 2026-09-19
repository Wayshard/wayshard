# Android (Tauri 2)

Wayshard Android is the same Solid graphical client packaged with Tauri 2
(`clients/desktop`, identifier `dev.wayshard.app`). It pairs to an existing
Wayshard Server; it does not host execution.

Official signed APKs are produced by GitHub Actions (`.github/workflows/release.yml`
job `android`) using Environment `release` secrets. See [`docs/release.md`](../../docs/release.md).

Local iteration (not an official signed release):

```sh
cd clients
bun install --frozen-lockfile
bun run --cwd web build
cd desktop
bunx tauri android init
bunx tauri android build --apk
```
