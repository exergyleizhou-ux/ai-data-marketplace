# Oasis Shared Runtime Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the authoritative Lumen Lab Run, approval, recovery, and artifact state in the existing Oasis `/workspace` shell without duplicating the Agent runtime inside React.

**Architecture:** The embedded Lab publishes a small versioned same-origin `postMessage` snapshot containing only routing identity and counters. Oasis validates that bridge message, then reads the authoritative Lab Run and Artifact REST APIs and renders a parent-level runtime drawer; cancellation remains the existing Run API. The iframe stays the interaction surface and Lumen remains the only owner of lifecycle state.

**Tech Stack:** Go 1.23 tests, shipped vanilla JavaScript, Next.js 16, React 19, TypeScript, Vitest, Testing Library.

---

## File map

- Modify `/Users/lei/lumen/.worktrees/lumen-production-runtime/internal/science/lab/static/app.js`: build and publish versioned parent snapshots after project, Run, and approval changes.
- Modify `/Users/lei/lumen/.worktrees/lumen-production-runtime/internal/science/lab/static/labui_test.mjs`: exercise the shipped bridge payload builder.
- Modify `/Users/lei/lumen/.worktrees/lumen-production-runtime/internal/science/lab/static_helpers_test.go`: require the bridge contract in the Go verification gate.
- Create `frontend/lib/workbench-runtime.ts`: strict bridge parser, Run/Artifact response parsing, URL-safe API client, terminal-state helper.
- Create `frontend/lib/workbench-runtime.test.ts`: invalid-message, API encoding, response-shape, and cancellation tests.
- Create `frontend/components/WorkbenchRuntimePanel.tsx`: accessible status summary and details drawer driven only by validated props.
- Create `frontend/components/WorkbenchRuntimePanel.test.tsx`: idle/running/failure/approval/artifact/cancel behavior.
- Modify `frontend/app/workspace/page.tsx`: validate iframe source/origin, fetch authoritative runtime details, wire cancel, render the panel.
- Modify `frontend/lib/next-config.test.ts`: narrow the optional `rewrites` function before invocation so the existing proxy contract passes typecheck.

### Task 1: Publish a versioned Lab-to-Oasis bridge contract

**Files:**
- Modify: `/Users/lei/lumen/.worktrees/lumen-production-runtime/internal/science/lab/static/app.js`
- Modify: `/Users/lei/lumen/.worktrees/lumen-production-runtime/internal/science/lab/static/labui_test.mjs`
- Modify: `/Users/lei/lumen/.worktrees/lumen-production-runtime/internal/science/lab/static_helpers_test.go`

- [ ] **Step 1: Write a failing shipped-JavaScript test**

Add assertions that `LabUI.buildWorkbenchSnapshot` returns exactly:

```js
{
  kind: "lumen.workbench.snapshot",
  version: 1,
  surface: "lab",
  project: { id: "project-a", title: "Project A" },
  run: { id: "run_1", last_seq: 7, terminal: false },
  pending_approvals: 2,
}
```

Also assert invalid/empty inputs become `project: null`, `run: null`, `pending_approvals: 0` and that the Go source-contract test requires `window.parent.postMessage`, the snapshot kind, and version.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
go test ./internal/science/lab -run 'TestLabUIPureHelpersJS|TestLabUIRunReplayContract' -count=1
```

Expected: FAIL because `buildWorkbenchSnapshot` and the publishing contract do not exist.

- [ ] **Step 3: Implement the minimal bridge**

Add a pure `buildWorkbenchSnapshot(project, runID, lastSeq, terminal, pendingApprovals)` above the LabUI export. Add a runtime `postWorkbenchSnapshot()` below global state that reads `activeProject`, `currentRunId`, `currentRunSeq`, `sseState.terminal`, and `pendingApprovalCards().length`, then calls:

```js
window.parent.postMessage(snapshot, window.location.origin);
```

Only post when embedded (`window.parent !== window`). Invoke it after project selection/loading, after each handled SSE event, after approval resolution/banner updates, when a Run is reset, and after restore completes. Do not put prompt text, tool args, secrets, or file contents in the message.

- [ ] **Step 4: Verify GREEN and repeat for stability**

Run:

```bash
go test ./internal/science/lab -run 'TestLabUIPureHelpersJS|TestLabUIRunReplayContract' -count=3
```

Expected: PASS three times.

- [ ] **Step 5: Commit the Lumen bridge**

```bash
git add internal/science/lab/static/app.js internal/science/lab/static/labui_test.mjs internal/science/lab/static_helpers_test.go
git commit -m "feat(lab-ui): publish workbench runtime snapshots"
```

### Task 2: Add a strict Oasis runtime client

**Files:**
- Create: `frontend/lib/workbench-runtime.ts`
- Create: `frontend/lib/workbench-runtime.test.ts`
- Modify: `frontend/lib/next-config.test.ts`

- [ ] **Step 1: Write failing parser and API tests**

Cover these behaviors with real values and a stub `fetch`:

```ts
parseWorkbenchSnapshot(valid) // accepted
parseWorkbenchSnapshot({ ...valid, version: 2 }) // null
parseWorkbenchSnapshot({ ...valid, run: { id: "../x" } }) // null
loadLabRuntime(validSnapshot, fetcher) // GET encoded Run, events, artifacts
cancelLabRun("run_1", fetcher) // POST /api/lab/runs/run_1/cancel
```

Responses must be shape-checked before use. Run IDs accept only `[A-Za-z0-9_-]`; project IDs are encoded with `URLSearchParams`. Failed responses surface a generic `WorkbenchRuntimeError` containing status, never response bodies or secrets.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
npm test -- lib/workbench-runtime.test.ts
```

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement the minimal typed client**

Define `WorkbenchSnapshot`, `LabRun`, `LabEvent`, `LabArtifact`, `LabRuntimeDetail`, `parseWorkbenchSnapshot`, `loadLabRuntime`, `cancelLabRun`, and `isTerminalRunStatus`. Fetch Run/events only when a Run ID exists and artifacts only when a project ID exists. Cap displayed events/artifacts in the client to 100 items.

Narrow `nextConfig.rewrites` before calling it:

```ts
expect(nextConfig.rewrites).toBeTypeOf("function");
const rewrites = await nextConfig.rewrites!();
```

- [ ] **Step 4: Verify tests and typecheck GREEN**

Run:

```bash
npm test -- lib/workbench-runtime.test.ts lib/next-config.test.ts
npm run typecheck
```

Expected: PASS.

- [ ] **Step 5: Commit the client contract**

```bash
git add frontend/lib/workbench-runtime.ts frontend/lib/workbench-runtime.test.ts frontend/lib/next-config.test.ts
git commit -m "feat(workbench): consume shared runtime contract"
```

### Task 3: Render authoritative Run, approval, and artifact state

**Files:**
- Create: `frontend/components/WorkbenchRuntimePanel.tsx`
- Create: `frontend/components/WorkbenchRuntimePanel.test.tsx`
- Modify: `frontend/app/workspace/page.tsx`

- [ ] **Step 1: Write failing accessible component tests**

Render the panel through `LocaleProvider` and assert:

- idle snapshot says no active Run;
- running Run exposes the full Run ID, status, event count, approval count, and enabled cancel button;
- succeeded Run hides cancel and lists artifacts as safe same-origin Lab download links;
- failed/exhausted Run shows `stop_reason` and the server-provided Run error as text, never HTML;
- drawer button uses `aria-expanded` and can be operated with `userEvent`.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
npm test -- components/WorkbenchRuntimePanel.test.tsx
```

Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement the presentational panel**

Use semantic `<button>`, `<section>`, `<dl>`, and `<ul>` elements. Keep data fetching out of this component. Artifact links must use `encodeURIComponent`/`URLSearchParams` and `target="_blank" rel="noopener"`. Disable cancel while the request is pending.

- [ ] **Step 4: Wire the parent workspace securely**

In `workspace/page.tsx`, hold an iframe ref and register one `message` listener. Reject messages unless all are true:

```ts
event.origin === window.location.origin
event.source === iframeRef.current?.contentWindow
parseWorkbenchSnapshot(event.data) !== null
```

Use an `AbortController` plus monotonically increasing request counter so stale fetches cannot overwrite a newer project/Run. Fetch details after an accepted message, call the existing cancel endpoint from the panel, and refresh after cancellation. Clear Lab runtime UI when switching away from the Lab tab.

- [ ] **Step 5: Verify component and full frontend gates**

Run:

```bash
npm test -- components/WorkbenchRuntimePanel.test.tsx lib/workbench-runtime.test.ts
npm test
npm run typecheck
npm run lint
npm run build
```

Expected: tests/typecheck/build PASS; lint has no new warnings in changed files.

- [ ] **Step 6: Commit the Oasis UI**

```bash
git add frontend/components/WorkbenchRuntimePanel.tsx frontend/components/WorkbenchRuntimePanel.test.tsx frontend/app/workspace/page.tsx
git commit -m "feat(workbench): show durable Lab runtime state"
```

### Task 4: Cross-repository completion gate

**Files:**
- Verify only.

- [ ] **Step 1: Run Lumen correctness gates**

```bash
go test -race ./internal/science/lab ./internal/runstate
go vet ./...
go test ./...
git diff --check main...HEAD
```

Expected: PASS and clean worktree.

- [ ] **Step 2: Run Oasis correctness gates**

```bash
npm test
npm run typecheck
npm run lint
npm run build
git diff --check feat/oasis-agent-workbench-production...HEAD
```

Expected: PASS, with any pre-existing lint warnings recorded separately and no warnings from changed files.

- [ ] **Step 3: Audit security-sensitive changes**

Confirm no bridge payload contains prompt/tool args/file contents, every message checks origin and source, paths are encoded, React never injects server HTML, cancel uses POST, and no credentials were added. Run `npm audit --omit=dev` and report unresolved dependency findings without applying forced upgrades.

- [ ] **Step 4: Confirm both worktrees are clean and summarize commits**

```bash
git status --short
git log --oneline -3
```

Expected: both worktrees clean; all changes committed only to their feature branches.
