# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: meet-room.spec.ts >> renders room list from the real server and opens a live observe socket
- Location: e2e/meet-room.spec.ts:100:1

# Error details

```
Error: Command failed: go build -tags meetspa -o /var/folders/vg/nlsn8n8x77n1xgg2nlpnvz180000gn/T/bashy-meet-bin-xnn65Z/bashy-meet-e2e ./pkg/meet/web/e2e/serve
```

# Test source

```ts
  1   | import { execFileSync, spawn, type ChildProcess } from "node:child_process";
  2   | import { mkdtempSync, rmSync } from "node:fs";
  3   | import { tmpdir } from "node:os";
  4   | import path from "node:path";
  5   | import net from "node:net";
  6   | import { cpSync, readFileSync, writeFileSync } from "node:fs";
  7   | 
  8   | import { expect, test, type Page } from "@playwright/test";
  9   | 
  10  | const webDir = path.resolve(import.meta.dirname, "..");
  11  | const repoRoot = path.resolve(webDir, "../../..");
  12  | // Deterministic fixtures, not fleet members: addressing a real agent would exec
  13  | // its CLI, which needs a network and a key and answers differently every run.
  14  | // See e2e/fleet/ — the launch template echoes the prompt back.
  15  | const primaryAgent = "echoback-fixed";
  16  | const invitedAgent = "echoback-alt";
  17  | 
  18  | let server: ChildProcess | undefined;
  19  | let baseURL = "";
  20  | let meetDir = "";
  21  | let binDir = "";
  22  | let serverOutput = "";
  23  | let fleetDir = "";
  24  | 
  25  | /** materializeFleet copies the fixture fleet into a temp dir and renders the
  26  |  * tool template, which needs the absolute path of the echoback script. */
  27  | function materializeFleet(): string {
  28  |   const dir = mkdtempSync(path.join(tmpdir(), "bashy-meet-fleet-"));
  29  |   const src = path.join(import.meta.dirname, "fleet");
  30  |   cpSync(src, dir, { recursive: true });
  31  |   const tmpl = path.join(dir, "tools", "echoback.yaml.tmpl");
  32  |   writeFileSync(
  33  |     path.join(dir, "tools", "echoback.yaml"),
  34  |     readFileSync(tmpl, "utf8").replaceAll("__DIR__", dir),
  35  |   );
  36  |   rmSync(tmpl, { force: true });
  37  |   return dir;
  38  | }
  39  | 
  40  | test.describe.configure({ mode: "serial" });
  41  | 
  42  | test.beforeAll(async () => {
  43  |   test.setTimeout(180_000);
  44  | 
  45  |   fleetDir = materializeFleet();
  46  |   binDir = mkdtempSync(path.join(tmpdir(), "bashy-meet-bin-"));
  47  |   meetDir = mkdtempSync(path.join(tmpdir(), "bashy-meet-state-"));
  48  |   const binary = path.join(binDir, "bashy-meet-e2e");
  49  | 
  50  |   execFileSync("npm", ["run", "build"], {
  51  |     cwd: webDir,
  52  |     stdio: "inherit",
  53  |   });
  54  |   // The harness's own server (pkg/meet/web/e2e/serve), not a product binary:
  55  |   // cmd/coreutils is the multi-call applet binary and has no meet command, and
  56  |   // `bashy` lives in a sibling repo that is not present in a coreutils
  57  |   // checkout. Building here is also what makes the test test THIS SPA — the
  58  |   // meetspa tag embeds the dist that `npm run build` just produced.
> 59  |   execFileSync(
      |   ^ Error: Command failed: go build -tags meetspa -o /var/folders/vg/nlsn8n8x77n1xgg2nlpnvz180000gn/T/bashy-meet-bin-xnn65Z/bashy-meet-e2e ./pkg/meet/web/e2e/serve
  60  |     "go",
  61  |     ["build", "-tags", "meetspa", "-o", binary, "./pkg/meet/web/e2e/serve"],
  62  |     {
  63  |       cwd: repoRoot,
  64  |       stdio: "inherit",
  65  |     },
  66  |   );
  67  | 
  68  |   const port = await freePort();
  69  |   baseURL = `http://127.0.0.1:${port}`;
  70  |   server = spawn(
  71  |     binary,
  72  |     ["-addr", `127.0.0.1:${port}`],
  73  |     {
  74  |       cwd: repoRoot,
  75  |       env: {
  76  |         ...process.env,
  77  |         BASHY_MEET_DIR: meetDir,
  78  |         BASHY_FLEET_DIR: fleetDir,
  79  |       },
  80  |       stdio: ["ignore", "pipe", "pipe"],
  81  |     },
  82  |   );
  83  |   server.stdout?.on("data", (chunk) => {
  84  |     serverOutput += chunk.toString();
  85  |   });
  86  |   server.stderr?.on("data", (chunk) => {
  87  |     serverOutput += chunk.toString();
  88  |   });
  89  | 
  90  |   await waitForHealth();
  91  | });
  92  | 
  93  | test.afterAll(async () => {
  94  |   server?.kill();
  95  |   rmSync(meetDir, { recursive: true, force: true });
  96  |   rmSync(binDir, { recursive: true, force: true });
  97  |   rmSync(fleetDir, { recursive: true, force: true });
  98  | });
  99  | 
  100 | test("renders room list from the real server and opens a live observe socket", async ({ page }) => {
  101 |   const first = unique("Browser seeded alpha");
  102 |   const second = unique("Browser seeded beta");
  103 |   const seeded = await createRoom(first, [primaryAgent]);
  104 |   await postMessage(seeded.id, "e2e-human", "seeded transcript from setup");
  105 |   await createRoom(second, [primaryAgent]);
  106 | 
  107 |   await openMeet(page);
  108 | 
  109 |   await expect(
  110 |     page.getByRole("button", { name: new RegExp(first) }),
  111 |   ).toBeVisible();
  112 |   await expect(
  113 |     page.getByRole("button", { name: new RegExp(second) }),
  114 |   ).toBeVisible();
  115 |   await expect(page.getByText("Demo workspace")).toHaveCount(0);
  116 |   const sidebar = page.locator("aside").filter({ hasText: "bashy meet" }).first();
  117 |   await expect(sidebar.getByText("connecting")).toHaveCount(0);
  118 |   await expect(sidebar.getByText("open", { exact: true })).toBeVisible();
  119 | });
  120 | 
  121 | test("creates a room from the sidebar and selects it with one agent seated", async ({ page }) => {
  122 |   const topic = unique("Browser created room");
  123 |   await openMeet(page);
  124 | 
  125 |   await page.getByRole("button", { name: "New room" }).click();
  126 |   await page.getByLabel("Topic").fill(topic);
  127 |   await page.getByLabel("Agents").fill(primaryAgent);
  128 |   await page.getByRole("button", { name: "Open room" }).click();
  129 | 
  130 |   const roomButton = page.getByRole("button", { name: new RegExp(topic) });
  131 |   const sidebar = page.locator("aside").filter({ hasText: "bashy meet" }).first();
  132 |   await expect(roomButton).toBeVisible();
  133 |   await expect(sidebar.getByText(primaryAgent)).toBeVisible();
  134 |   await expect(page.getByLabel("Message the room")).toBeEnabled();
  135 | });
  136 | 
  137 | test("invites an agent from room details and shows it in the roster", async ({ page }) => {
  138 |   const topic = unique("Browser invite room");
  139 |   await openMeet(page);
  140 |   await createRoomFromUI(page, topic, primaryAgent);
  141 | 
  142 |   // RoomDetails is mounted twice — the desktop panel and the mobile sheet — so
  143 |   // "Invite" matches two buttons. Submitting the form with Enter targets the one
  144 |   // field we filled instead of disambiguating a duplicated control.
  145 |   const inviteField = page.getByLabel("Agent to invite").first();
  146 |   await inviteField.fill(invitedAgent);
  147 |   await inviteField.press("Enter");
  148 | 
  149 |   const sidebar = page.locator("aside").filter({ hasText: "bashy meet" }).first();
  150 |   await expect(sidebar.getByText(invitedAgent)).toBeVisible();
  151 | });
  152 | 
  153 | test("posts a human message through the composer and reloads it from the transcript", async ({ page }) => {
  154 |   const topic = unique("Browser transcript room");
  155 |   const message = unique("human browser message");
  156 |   await openMeet(page);
  157 |   await createRoomFromUI(page, topic, primaryAgent);
  158 | 
  159 |   await page.getByLabel("Message the room").fill(message);
```