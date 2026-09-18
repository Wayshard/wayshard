# Android (Tauri 2)

Wayshard Android is the same Solid graphical client packaged with Tauri 2.

```sh
cd clients
bun install
bun run --cwd web build
cd desktop
bunx tauri android init
bunx tauri android build --apk
```

The application identifier is `dev.wayshard.app`. Android does not provision a local server; pair to an existing Wayshard Server.
