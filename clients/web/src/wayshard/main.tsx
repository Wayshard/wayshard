// Wayshard Web entry point. Web mounts the same shared graphical client that
// Desktop (Tauri 2) and Android (Tauri 2) host.
import { render } from "solid-js/web"
import { WayshardApp } from "@wayshard/gui"
import "@wayshard/gui/styles"

render(() => <WayshardApp />, document.getElementById("root")!)
