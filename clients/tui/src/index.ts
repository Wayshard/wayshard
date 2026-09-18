#!/usr/bin/env bun
/**
 * Wayshard interactive TUI. Uses the Wayshard HTTP API, not ACP and not OpenCode.
 * Terminal-native: no nested terminal, $EDITOR for file edits.
 */
import { WayshardClient, type Project, type Run } from "@wayshard/sdk";
import * as readline from "node:readline";
import { spawnSync } from "node:child_process";

const base = process.env.WAYSHARD_SERVER || "http://127.0.0.1:7420";
const token = process.env.WAYSHARD_TOKEN;
const client = new WayshardClient(base, token);

let project: Project | undefined;
let convId = "";
let run: Run | undefined;

async function main() {
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
  const ask = (q: string) => new Promise<string>((res) => rl.question(q, res));
  console.log(`Wayshard TUI  ${base}`);
  console.log("commands: projects | open <path> | session | send <text> | stages | changes | files | usage | knowledge | approvals | allow <id> | deny <id> | notes | cancel | retry | integrate | settings | help | quit");
  for (;;) {
    const line = (await ask("wayshard> ")).trim();
    const [cmd, ...rest] = line.split(/\s+/);
    const arg = rest.join(" ");
    try {
      switch (cmd) {
        case "":
        case "help":
          console.log("projects open session send stages changes files usage knowledge approvals notes cancel retry integrate settings quit");
          break;
        case "quit":
        case "exit":
          rl.close();
          return;
        case "projects":
          console.log(JSON.stringify(await client.projects(), null, 2));
          break;
        case "open": {
          project = await client.openProject(arg);
          const c = await client.createConversation(project.id, "TUI");
          convId = c.id;
          console.log("opened", project.name, project.status);
          break;
        }
        case "session":
          if (!convId) {
            console.log("open a project first");
            break;
          }
          console.log(JSON.stringify(await client.messages(convId), null, 2));
          break;
        case "send": {
          if (!convId) throw new Error("no session");
          const r = await client.sendMessage(convId, arg, { idempotencyKey: crypto.randomUUID() });
          run = r.run;
          console.log("run", run.id, run.status);
          break;
        }
        case "stages":
          if (!run) break;
          run = await client.run(run.id);
          console.log(run.status, run.blockedReason || "");
          console.log(JSON.stringify(await client.stages(run.id), null, 2));
          break;
        case "changes":
          if (run) console.log("RUN\n", JSON.stringify(await client.runChanges(run.id), null, 2));
          if (project) console.log("WORKSPACE\n", JSON.stringify(await client.workspaceChanges(project.id), null, 2));
          break;
        case "files":
          if (!project) break;
          if (arg && process.env.EDITOR) {
            const f = await client.readFile(project.id, arg);
            const tmp = `/tmp/wayshard-edit-${Date.now()}`;
            await Bun.write(tmp, f.content);
            spawnSync(process.env.EDITOR, [tmp], { stdio: "inherit" });
            const next = await Bun.file(tmp).text();
            await client.writeFile(project.id, arg, next, f.hash);
            console.log("saved");
          } else {
            console.log(JSON.stringify(await client.listFiles(project.id, arg), null, 2));
          }
          break;
        case "usage":
          if (run) console.log(JSON.stringify(await client.usage(run.id), null, 2));
          break;
        case "knowledge":
          if (project) console.log(JSON.stringify(await client.knowledge(project.id), null, 2));
          break;
        case "approvals":
          console.log(JSON.stringify(await client.approvals(), null, 2));
          break;
        case "allow":
          await client.resolveApproval(arg, "allowed");
          break;
        case "deny":
          await client.resolveApproval(arg, "denied");
          break;
        case "notes":
          console.log(JSON.stringify(await client.notifications(), null, 2));
          break;
        case "cancel":
          if (run) await client.cancel(run.id);
          break;
        case "retry":
          if (run) console.log(await client.retry(run.id));
          break;
        case "integrate":
          if (run) await client.integrate(run.id);
          break;
        case "settings":
          console.log(JSON.stringify({ settings: await client.settings(), storage: await client.storage(), sandbox: await client.sandbox() }, null, 2));
          break;
        default:
          console.log("unknown command");
      }
    } catch (e) {
      console.error(e);
    }
  }
}

main();
