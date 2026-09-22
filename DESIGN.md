# DESIGN.md

## 1. Design intent

Wayshard should feel like a focused developer control surface, not an enterprise workflow dashboard and not a model playground. The interface should preserve the restraint, density, project/session navigation model, and terminal-friendly character that made the imported OpenCode 2 client/TUI foundations appealing, while replacing their single-agent assumptions with Wayshard's explicit orchestration model.

Wayshard architecture always wins over inherited UI structure. If an OpenCode-derived component conflicts with Wayshard's domain, adapt or replace the component rather than bending the product architecture around it.

Wayshard is fully rebranded. Normal product UI must not present OpenCode naming, icons, product copy, or runtime concepts as Wayshard concepts. Required third-party attribution belongs in notices/developer/legal surfaces.

## 2. Clients

Wayshard has four full clients:

- Web;
- CLI / TUI;
- Desktop;
- Android.

“Full client” means the user can control the core Wayshard product from that client. It does not mean every client must render every capability with identical widgets.

Web/Desktop/Android should share graphical client logic and visual language where practical. CLI/TUI uses a terminal-appropriate presentation derived from the imported OpenCode 2 terminal foundation.

Do not add convenience systems purely for parity. Preserve/adapt existing command-palette, accessibility, keyboard, editor, terminal, or similar systems if the imported foundation already provides them and they remain appropriate. Do not create new systems solely because another client has them.

## 3. Overall visual direction

The graphical clients should remain low-chrome and developer-oriented:

- dark-first visual language with light/system support where inherited infrastructure permits;
- compact information density;
- restrained use of cards and borders;
- rounded surfaces used sparingly and consistently;
- strong typography hierarchy without oversized marketing UI inside the product;
- project/session navigation that remains usable on narrow screens;
- progressive disclosure for advanced routing/context/diagnostic information.

Do not transform the product into a workflow board full of large status cards. The session remains the main working surface.

## 4. Primary information architecture

The primary project workspace should preserve the simple core surfaces:

```text
Session | Changes | Files | Terminal
```

Where a client cannot reasonably present one of these in the same form, provide an appropriate equivalent without adding unrelated UI complexity.

Advanced information should normally live in overflow/detail surfaces rather than permanent top-level tabs:

- Usage;
- Run details;
- Routing;
- Context;
- Artifacts;
- Project knowledge;
- Recovery/diagnostics.

## 5. Home and project switching

Home should make server and project state understandable without looking like an administration console.

Users can:

- choose/switch Wayshard servers;
- open recently used projects;
- add/open/clone/create projects on the selected server;
- see projects requiring attention;
- enter an existing session or create a new session.

A project belongs to a server. Android and remote clients browse the server's project filesystem, not the phone/client machine's filesystem.

Missing/moved projects remain visible as unavailable with actions such as Locate and Remove from Wayshard. Do not silently forget history.

## 6. Project onboarding

### 6.1 Existing project

Open immediately after basic passive discovery. Deeper indexing may continue in the background.

Do not show a blocking “AI setup wizard” merely because knowledge discovery is still running. Surface project-knowledge status subtly and allow the user to inspect detected authoritative/scoped documents.

### 6.2 Clone

Clone happens on the selected server, then follows the existing-project flow.

### 6.3 New project

Ask only for necessary project creation details such as name/location/Git initialization and optional initial description. Do not force a long questionnaire.

Open a normal session after creation. Brainstorming may be artifact-only and should not require file creation.

When the user later asks to generate project canonicals, Wayshard creates the six-file canonical set coherently as a normal run.

## 7. Session experience

The session timeline is the center of the product.

A user message is followed by compact stage activity that makes routing visible without overwhelming the conversation:

```text
Planning · <harness/model>
Executing · <harness/model>
Validating
Reviewing · <harness/model>
Repairing · <harness/model>   (when necessary)
Integrating
Complete
```

Each stage is expandable for detail. Default collapsed presentation should show enough to understand progress, route, duration/status, and meaningful warnings.

Avoid large “enterprise workflow” stage cards. The conversation should still feel like a coding session.

Meaningful recovery can appear as a timeline marker such as “Recovered after server restart,” while routine transient retries remain in stage detail unless they require attention.

## 8. Composer and routing controls

The composer should remain familiar and low-friction.

Automatic routing is the default. Do not make model selection the dominant control as in a single-agent product.

A compact control such as `Auto` or `Routing: Auto` may expose per-task overrides including:

- force a harness/model where available;
- select a routing profile such as Balanced, Quality, Economy, or Speed;
- set task-specific budget/route preferences;
- override planner/executor/reviewer route where advanced users need it.

The system must clearly reject impossible/forbidden overrides rather than pretending they were accepted.

The inherited `+`/attachment/action affordance may expose images/files, context references, commands, or shell-related actions where appropriate.

## 9. Changes

Changes is primarily a review/diff surface, not a complete Git client.

Within a run/session, make provenance explicit:

```text
Run | Workspace
```

- **Run** shows only the run-produced delta from the immutable starting snapshot.
- **Workspace** shows current source working-tree changes, including pre-existing user work and later manual edits.

When helpful, badges can distinguish agent-modified, pre-existing, canonical, generated, or conflicted files.

Use the inherited OpenCode-style diff viewer and changed-file list as the visual foundation where suitable.

Do not add a full branch/staging/pull/push/rebase workbench unless inherited functionality already exists and remains directly useful. Manual Git stays available through Terminal and external tools.

Run-specific orchestration actions such as retry integration or revert run may exist because they are Wayshard lifecycle operations, not generic Git controls.

## 10. Files

Files should preserve the familiar project tree and straightforward viewing/editing model.

Graphical clients may:

- browse the server-side project tree;
- open text files;
- edit/save text where appropriate;
- use basic editor behavior inherited from the client foundation;
- show binary-file metadata/preview when supported.

Do not turn the Wayshard file viewer into a full IDE editor project.

File saves must detect concurrent modification. If the file changed since it was opened, show a conflict/reload/compare flow rather than overwriting unseen changes.

Canonical documents are normal repository files and should not live in a separate hidden editor.

## 11. Terminal

Graphical clients expose interactive server-side terminal sessions for the project's source workspace.

Important behavior:

- a terminal is server-owned, not browser-window-owned;
- closing/reloading a client does not automatically terminate the PTY;
- users can have multiple terminal sessions/tabs where inherited UI supports it;
- a server restart may terminate sessions and should display that honestly;
- the default terminal is the user's source workspace, not a hidden agent run workspace.

Advanced run diagnostics may offer a way to inspect a run workspace terminal if useful, but it should not replace the primary project terminal.

The CLI is already running in a terminal, so do not force a nested terminal widget. Preserve/adapt the imported terminal client's natural shell/escape behavior.

## 12. Approvals

Approvals are server-owned and may be answered from any authenticated client.

An approval should communicate:

- what boundary is being crossed;
- the requested resource/destination/action;
- why it is needed;
- available scopes such as once/run/project where supported;
- clear Allow/Deny actions.

Examples include tool network access, imported external inputs, project secret use, Git push, or other policy boundaries.

A denial is a real outcome, not an infrastructure failure: the protected operation must not execute and must not be automatically retried. Only the first resolution of an approval takes effect. Cancelling a run, or losing the server while an approval is pending, invalidates the approval rather than silently approving it.

Reading a notification does not resolve the approval. Resolution changes the underlying attention item.

## 13. Validation and review presentation

Keep validation compact in the main timeline:

```text
Validating
✓ Build
✓ Typecheck
✓ 142 tests
⚠ 2 pre-existing failures
```

Tap/expand for command-level evidence.

Review can summarize acceptance criteria and unresolved findings:

```text
Reviewing · <harness/model>
✓ 7/7 acceptance criteria
✓ No blocking findings
```

If a deterministic hard gate fails, the interface must not present a contradictory “review passed” completion state.

Unknown/unverified criteria should be labeled honestly.

## 14. Integration states

For source-changing tasks, distinguish implementation success from source publication.

Possible user-visible states include:

- Ready to integrate;
- Integrating;
- Integration blocked;
- Complete.

If the user switched branches, do not silently apply work to the new branch. Show the run's original target and current source branch and require an explicit target decision when necessary.

If integration conflicts, preserve the run result and make the conflict actionable without implying the implementation was lost.

## 15. Usage

Usage should be richer than a simple token total.

Normal view may show:

- total cost;
- input/output/cache tokens where available;
- duration;
- validation time;
- stage breakdown.

Advanced usage can show:

- planner/executor/reviewer/Jev usage;
- provider/harness/model;
- pricing snapshot/estimate vs actual;
- retries and attempts;
- context/token estimates;
- latency.

Actual provider/harness-reported usage/cost wins over estimates. Estimated values must be labeled.

## 16. Run details

Run Details is the deep inspection surface. Recommended sections:

- Overview;
- Stages;
- Validation;
- Routing;
- Context;
- Artifacts;
- Recovery;
- Diagnostics.

Overview should be concise: status, duration, cost, files changed, validation status, review status.

Stages shows route and attempt history, including infrastructure fallback/retry information.

Routing exposes the Jev/routing inspector for advanced users: assessment dimensions/probabilities, chosen route, policy rule, fallback chain, model/harness health, and decision versions.

Context exposes the ContextManifest/provenance rather than merely a giant pasted prompt.

Diagnostics can expose sanitized ACP/system detail but must remain out of the normal workflow.

## 17. Project Knowledge UI

Provide a project knowledge surface showing detected authority, scope, and status without forcing users to understand the entire graph.

Example grouping:

```text
Authoritative
  AGENTS.md
  SPEC.md
  ARCHITECTURE.md
  DESIGN.md

Harness-specific
  CLAUDE.md
  .cursor/rules/...

Scoped
  backend/AGENTS.md
  frontend/AGENTS.md

Supporting
  README.md
  CONTRIBUTING.md
```

A document detail can show:

- inferred/declared role;
- authority domain;
- scope;
- references;
- why it was discovered;
- current hash/status.

Unresolved conflicts appear as warnings but should not block project opening. Interrupt the user only when the conflict materially affects a task.

## 18. Notifications and attention

A compact notification/attention surface is sufficient; it does not need to become a major permanent workspace tab.

Useful default notification events:

- approval required;
- manual validation required;
- run completed;
- run blocked;
- run failed;
- integration conflict;
- server/harness requires attention.

Distinguish:

- notification history/read state;
- unresolved attention that still requires action.

Push delivery is optional infrastructure. Reconnecting a client must still reveal pending server-side attention even without push.

## 19. Settings

Graphical settings should remain restrained and use a familiar list/two-pane or stacked-mobile layout.

Recommended categories:

- Preferences;
- Appearance;
- Notifications;
- Servers;
- Projects;
- Harnesses;
- Models;
- Routing;
- Decision Engine;
- Security;
- Storage & Backups;
- Diagnostics;
- About.

Project-local settings may expose:

- General;
- Knowledge;
- Validation;
- Routing;
- Permissions;
- Git & Workspaces;
- Retention.

Do not create a generic provider-credentials experience that suggests Wayshard owns credentials actually managed by harnesses.

### 19.1 Harnesses

Show discovered executable path, version, ACP compatibility/capabilities, authentication state, model/options visibility, health, and diagnostics. Support rescan and explicitly configured custom ACP executable paths. Discovery probes run in isolation and cannot read a harness's real configuration, so an authentication state that cannot be determined is shown as unknown rather than implied healthy or authenticated.

### 19.2 Models

Separate discovered availability from the user's automatic-routing pool. Search/filter is important when a harness exposes a large catalog.

### 19.3 Routing

Default view should be understandable without Jev internals. Advanced settings may expose thresholds, role pools, fallbacks, and budgets.

### 19.4 Decision Engine

Jev appears as control-plane infrastructure with connection status, configured model/alias, credential status, test connection, and advanced diagnostics.

### 19.5 Security

Expose understandable policy such as workspace isolation, network, external filesystem, project secrets, Git push, and force-push behavior. Previously granted persistent permissions should be reviewable/revocable.

### 19.6 Storage

Show database, artifacts, run workspaces, attachments, and logs, plus retention controls and low-disk warnings.

## 20. CLI / TUI design

The Wayshard CLI/TUI is a full client built from the imported OpenCode 2 terminal client foundation, then adapted independently.

Running `wayshard` with no subcommand should open the interactive terminal client unless a final implementation decision requires a different entry command.

The TUI should support core product operations such as:

- server/project/session switching;
- chat/composer;
- stage timeline;
- approvals;
- run status and control;
- changes/diffs;
- project/file inspection appropriate to the inherited terminal UX;
- usage and run details;
- routing/context/knowledge inspection;
- notifications/attention.

Do not require the CLI to embed a text editor if the foundation does not already provide a good one; handoff to `$EDITOR` is acceptable. Do not build a terminal inside the terminal purely for parity.

The same binary may expose scriptable noninteractive subcommands against the Wayshard API. Exact command syntax is an implementation/specification detail, but the interactive and scriptable paths must use the same server domain model/authentication.

## 21. Android design

Android is a full client, not a read-only companion.

It can:

- connect/pair to servers;
- browse/open/clone/create server-side projects;
- create and continue sessions/tasks;
- follow all orchestration stages;
- respond to approvals;
- attach files/images;
- browse/edit source text files where appropriate;
- inspect run/workspace changes;
- use terminal capability through a mobile-appropriate presentation;
- inspect artifacts, usage, routing, context, knowledge, and settings;
- cancel/retry/revert/integrate where the product exposes those actions.

Responsive/mobile presentation may differ substantially from desktop. Capability should not be intentionally removed merely because the screen is small, except for local-server provisioning and other genuinely platform-specific functions.

## 22. Server connection UX

Desktop local onboarding should offer a simple local-server path plus connection to another server.

Android connects to an existing server via pairing invitation/QR/code.

Web is already being served by a Wayshard Server and authenticates to that server.

A server profile shows stable server identity and current endpoint. Endpoint changes do not create a new project universe.

Disconnected clients clearly show stale/disconnected state and do not optimistically claim server actions succeeded.

## 23. Branding

Use Wayshard consistently in product UI, package metadata, binary names, and examples.

Avoid forcing a literal “shard” visual metaphor into every surface. The name should support a strong identity without turning the interface into themed decoration.

Primary public identity:

```text
Wayshard
wayshard.dev
github.com/Wayshard
```

## 24. Design guardrails

- Keep the main coding/session experience simple even though the backend is sophisticated.
- Prefer progressive disclosure to permanent advanced controls.
- Do not expose raw ACP as the user mental model.
- Do not use model confidence as a visual substitute for evidence.
- Do not imply that a run is complete before required source integration.
- Do not hide meaningful recovery or conflicts.
- Do not turn Wayshard into a Git client, IDE, networking appliance, model credential manager, or workflow-management suite beyond the product requirements.

## 21. Client source lineage (Pass 1E)

The live graphical client and TUI are adapted from the imported OpenCode 2
application source, not recreated. The graphical **application composition** —
application root, Home, project/session layout and sidebar, session page,
titlebar/work-surface navigation, composer region, review, file browser and
terminal panels, run timeline, new-session flow and the narrow/mobile model —
descends from the vendored OpenCode application files and keeps their interaction
character, with Wayshard domain and server authority adapted into it. The design
system, theme, session/message rendering, diff and review surfaces, prompt
composer and terminal presentation descend from `packages/ui` and
`packages/session-ui`; the TUI descends from the OpenCode OpenTUI terminal
client. The product is fully rebranded as Wayshard and the domain is Wayshard
(projects → sessions → runs → stages → artifacts). See
`docs/client-source-lineage.md`.
