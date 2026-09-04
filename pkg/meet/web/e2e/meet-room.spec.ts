import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import net from "node:net";
import { appendFileSync, cpSync, readFileSync, writeFileSync } from "node:fs";

import { expect, test, type Page } from "@playwright/test";

const webDir = path.resolve(import.meta.dirname, "..");
const repoRoot = path.resolve(webDir, "../../..");
// Deterministic fixtures, not fleet members: addressing a real agent would exec
// its CLI, which needs a network and a key and answers differently every run.
// See e2e/fleet/ — the launch template echoes the prompt back.
const primaryAgent = "echoback-fixed";
const invitedAgent = "echoback-alt";
const facilitatorAgent = "echoback-facilitator";

let server: ChildProcess | undefined;
let baseURL = "";
let meetDir = "";
let binDir = "";
let serverOutput = "";
let fleetDir = "";

/** materializeFleet copies the fixture fleet into a temp dir and renders the
 * tool template, which needs the absolute path of the echoback script. */
function materializeFleet(): string {
  const dir = mkdtempSync(path.join(tmpdir(), "bashy-meet-fleet-"));
  const src = path.join(import.meta.dirname, "fleet");
  cpSync(src, dir, { recursive: true });
  const tmpl = path.join(dir, "tools", "echoback.yaml.tmpl");
  writeFileSync(
    path.join(dir, "tools", "echoback.yaml"),
    readFileSync(tmpl, "utf8").replaceAll("__DIR__", dir),
  );
  rmSync(tmpl, { force: true });
  return dir;
}

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  test.setTimeout(180_000);

  fleetDir = materializeFleet();
  binDir = mkdtempSync(path.join(tmpdir(), "bashy-meet-bin-"));
  meetDir = mkdtempSync(path.join(tmpdir(), "bashy-meet-state-"));
  const binary = path.join(binDir, "bashy-meet-e2e");

  execFileSync("npm", ["run", "build"], {
    cwd: webDir,
    stdio: "inherit",
  });
  // The harness's own server (pkg/meet/web/e2e/serve), not a product binary:
  // cmd/coreutils is the multi-call applet binary and has no meet command, and
  // `bashy` lives in a sibling repo that is not present in a coreutils
  // checkout. Building here is also what makes the test test THIS SPA — the
  // meetspa tag embeds the dist that `npm run build` just produced.
  execFileSync(
    "go",
    ["build", "-tags", "meetspa", "-o", binary, "./pkg/meet/web/e2e/serve"],
    {
      cwd: repoRoot,
      stdio: "inherit",
    },
  );

  const port = await freePort();
  baseURL = `http://127.0.0.1:${port}`;
  server = spawn(
    binary,
    ["-addr", `127.0.0.1:${port}`],
    {
      // A browser-created room defaults its minutes to ./docs/meetings. Keep
      // that real behavior inside the disposable test root: closing a room in
      // an end-to-end test must never dirty the source checkout.
      cwd: meetDir,
      env: {
        ...process.env,
        BASHY_MEET_DIR: meetDir,
        BASHY_FLEET_DIR: fleetDir,
      },
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  server.stdout?.on("data", (chunk) => {
    serverOutput += chunk.toString();
  });
  server.stderr?.on("data", (chunk) => {
    serverOutput += chunk.toString();
  });

  await waitForHealth();
});

test.afterAll(async () => {
  server?.kill();
  rmSync(meetDir, { recursive: true, force: true });
  rmSync(binDir, { recursive: true, force: true });
  rmSync(fleetDir, { recursive: true, force: true });
});

test("renders room list from the real server and opens a live observe socket", async ({ page }) => {
  const first = unique("Browser seeded alpha");
  const second = unique("Browser seeded beta");
  const seeded = await createRoom(first, [primaryAgent]);
  await postMessage(seeded.id, "e2e-human", "seeded transcript from setup");
  await createRoom(second, [primaryAgent]);

  await openMeet(page);

  await expect(
    page.getByRole("button", { name: new RegExp(first) }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: new RegExp(second) }),
  ).toBeVisible();
  await expect(page.getByText("Demo workspace")).toHaveCount(0);
  const sidebar = page.locator("aside").first();
  await expect(sidebar.locator('[title="open"]')).toBeVisible();
  await expect(page).toHaveTitle("Meet — bashy");
  await expect(page.getByText("bashymeet", { exact: true })).toBeVisible();
  await expect(page.getByText(/Relay/)).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Back to all apps" })).toBeVisible();
});

test("creates a room from the sidebar and selects it with one agent seated", async ({ page }) => {
  const topic = unique("Browser created room");
  await openMeet(page);

  await page.getByRole("button", { name: "New meeting" }).click();
  await page.getByLabel("Topic").fill(topic);
  await page.getByLabel("Facilitator").selectOption(facilitatorAgent);
  await page.getByLabel("Participants").selectOption(primaryAgent);
  await page.getByRole("button", { name: "Open meeting" }).click();

  const roomButton = page.getByRole("button", { name: new RegExp(topic) });
  const sidebar = page.locator("aside").first();
  await expect(roomButton).toBeVisible();
  await expect(sidebar.getByText(primaryAgent)).toBeVisible();
  await expect(page.getByLabel("Message the room")).toBeEnabled();
});

test("invites an agent from room details and shows it in the roster", async ({ page }) => {
  const topic = unique("Browser invite room");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  // RoomDetails is mounted twice — the desktop panel and the mobile sheet — so
  // "Invite" matches two buttons, so target the visible desktop details copy.
  const inviteField = page.getByLabel("Agent to invite").first();
  await inviteField.selectOption(invitedAgent);
  await page.getByRole("button", { name: "Invite", exact: true }).first().click();

  const sidebar = page.locator("aside").first();
  await expect(sidebar.getByText(invitedAgent)).toBeVisible();

  await page.getByRole("button", { name: `Remove ${invitedAgent}` }).first().click();
  await expect(sidebar.getByText(invitedAgent)).toHaveCount(0);
});

test("room actions call their exact API routes and carry the draft", async ({ page }) => {
  const topic = unique("Browser actions room");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  const cases = [
    { label: "Hear from everyone", action: "round", body: undefined },
    {
      label: "Take a quick pulse",
      action: "poll",
      body: { question: "Ship today?", choices: ["Yes", "Needs more discussion"] },
    },
    {
      label: "Ask the room",
      action: "ask",
      body: { question: "Ship today?" },
    },
    { label: "Find common ground", action: "converge", body: undefined },
  ];

  await page.getByLabel("Message the room").fill("Ship today?");
  for (const item of cases) {
    await page.route(`**/api/rooms/*/${item.action}`, async (route) => {
      await route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job: `test-${item.action}`, room: "test" }),
      });
    }, { times: 1 });
    const requestPromise = page.waitForRequest((request) =>
      request.url().endsWith(`/${item.action}`),
    );
    await page.getByRole("button", { name: "Room actions" }).click();
    await page.getByRole("menuitem", { name: new RegExp(item.label) }).click();
    const request = await requestPromise;
    expect(request.method()).toBe("POST");
    expect(request.postDataJSON()).toEqual(item.body ?? null);
  }
});

test("records outcomes and reflects room closure without a reload", async ({ page }) => {
  const topic = unique("Browser lifecycle room");
  const decision = unique("Ship the browser actions");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  await page.getByLabel("Message the room").fill(decision);
  await page.getByRole("button", { name: "Room actions" }).click();
  await page.getByRole("menuitem", { name: /Record decision/ }).click();
  await expect(page.getByText(decision).last()).toBeVisible();

  await page.getByRole("button", { name: "Close room" }).first().click();
  await page.getByRole("dialog").getByRole("button", { name: "Close room" }).click();
  await expect(page.getByLabel("Message the room")).toBeDisabled();
  const reopen = page.getByRole("button", { name: "Open room" }).first();
  await expect(reopen).toBeEnabled();
  await reopen.click();
  await expect(page.getByLabel("Message the room")).toBeEnabled();

  // Closing ends the room's WebSocket normally. Reopening must establish a
  // fresh observe stream; otherwise this reply exists only after a page reload.
  await page.getByLabel("Message the room").fill(`@${primaryAgent} after reopen`);
  await page.getByRole("button", { name: "Send message" }).click();
  await expect(page.getByText(/Current question: after reopen/).first()).toBeVisible({
    timeout: 60_000,
  });
});

test("manage participants opens the hidden room details panel", async ({ page }) => {
  const topic = unique("Browser manage room");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  await page.getByRole("button", { name: "Hide room details" }).click();
  await expect(page.getByLabel("Agent to invite").first()).not.toBeVisible();
  await page.getByRole("button", { name: "Room actions" }).click();
  await page.getByRole("menuitem", { name: /Manage participants/ }).click();
  await expect(page.getByLabel("Agent to invite").first()).toBeVisible();
});

test("posts a human message through the composer and reloads it from the transcript", async ({ page }) => {
  const topic = unique("Browser transcript room");
  const message = unique("human browser message");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  await page.getByLabel("Message the room").fill(message);
  await page.getByRole("button", { name: "Send message" }).click();

  await expect(page.getByText(message)).toBeVisible();
  await page.reload();
  await expect(page.getByText(message)).toBeVisible();
});

test("normalizes a legacy transport turn and reveals raw JSON only on request", async ({ page }) => {
  const topic = unique("Browser legacy transport");
  const seeded = await createRoom(topic, [primaryAgent]);
  const prose = unique("legacy codex answer");
  const raw = [
    JSON.stringify({ type: "thread.started", thread_id: "thread-e2e" }),
    JSON.stringify({
      type: "item.completed",
      item: { id: "item-e2e", type: "agent_message", text: prose },
    }),
    JSON.stringify({
      type: "turn.completed",
      usage: { input_tokens: 10, output_tokens: 3 },
    }),
  ].join("\n");
  appendFileSync(
    path.join(meetDir, seeded.id, "transcript.jsonl"),
    `${JSON.stringify({
      round: 1,
      speaker: primaryAgent,
      role: "participant",
      kind: "turn",
      text: raw,
      ts: new Date().toISOString(),
    })}\n`,
  );

  await openMeet(page);
  await page.getByRole("button", { name: new RegExp(topic) }).click();
  await expect(page.getByText(prose, { exact: true })).toBeVisible();
  await expect(page.getByText(/thread\.started/)).toHaveCount(0);
  await expect(page.getByText(/Raw transport/)).toHaveCount(0);

  await page.getByRole("button", { name: "Show the raw agent transport" }).click();
  await expect(page.getByText(/Raw transport \(3 lines\)/)).toBeVisible();
  await page.getByText(/Raw transport \(3 lines\)/).click();
  await expect(page.getByText(/thread\.started/)).toBeVisible();

  await page.getByRole("button", { name: "Hide the raw agent transport" }).click();
  await expect(page.getByText(/Raw transport/)).toHaveCount(0);
});

// The one that needed a deterministic agent: addressing runs a real turn through
// chat.Invoke, so the fixture in e2e/fleet stands in for an agent CLI and echoes
// the prompt back. What is asserted is the browser, not the API — the reply has
// to arrive over /observe and be painted, which is the half no Go test can see.
// This is the assertion the other four cannot make: they prove the room can be
// listed, opened, staffed and posted to, but only this one proves an AGENT
// ANSWERS and a human sees it. Two defects hid behind it, both silent, and both
// found only by running the whole chain in a browser:
//
//   1. liveEventSchema required role/status, which LiveEvent marks omitempty —
//      so the SPA received every live frame and discarded all of them.
//   2. the fixture's `#!/usr/bin/env bash` shebang was intercepted by the agent
//      shell shim (see e2e/fleet/echoback), so the turn re-executed the harness
//      server instead of the fixture and never returned.
//
// Neither showed up as an error anywhere. Keep this test running.
test("addressed agent replies render in the browser", async ({ page }) => {
  const topic = unique("Browser reply room");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  await page.getByLabel("Message the room").fill(`@${primaryAgent} say hello`);
  await page.getByRole("button", { name: "Send message" }).click();

  // ECHO[<model>] is the fixture's signature; the agent's turn is what renders.
  await expect(page.getByText(/ECHO\[fixed\]/).first()).toBeVisible({
    timeout: 60_000,
  });
});

test("opens a Chat-backed direct message and streams its reply", async ({ page }) => {
  await createDM(primaryAgent);
  await openMeet(page);
  await openChat(page, primaryAgent);
  await expect(page.getByText("Direct message", { exact: true }).first()).toBeVisible();

  await page.locator("textarea").fill("say hello privately");
  await page.getByRole("button", { name: "Send message" }).click();
  await expect(page.getByText(/ECHO\[fixed\]/).first()).toBeVisible({ timeout: 60_000 });
});

test("a long Chat turn visibly shows the agent working", async ({ page }) => {
  await createDM(primaryAgent);
  await openMeet(page);
  await openChat(page, primaryAgent);
  await page.route("**/api/dms/*/messages", async (route) => {
    await route.fulfill({
      status: 202,
      contentType: "application/json",
      body: JSON.stringify({ agent: primaryAgent, status: "working" }),
    });
  }, { times: 1 });
  await page.locator("textarea").fill("take your time");
  await page.getByRole("button", { name: "Send message" }).click();
  await expect(page.getByLabel(`${primaryAgent} is typing`)).toBeVisible();
  await expect(page.getByText(/reply will appear here/i)).toBeVisible();
});

test("Start work reaches server policy and fails closed without trusted Bashy containment", async ({ page }) => {
  await createDM(primaryAgent);
  await openMeet(page);
  await openChat(page, primaryAgent);
  const startWork = page.getByRole("button", { name: "Start work" });
  await expect(startWork).toBeVisible();
  await page.locator("textarea").fill("edit and test this change");
  await startWork.click();
  await expect(page.getByText(/no trusted Bashy containment provenance/i)).toBeVisible();
  await expect(page.locator("textarea")).toHaveValue("edit and test this change");
});

test("Meet tabs navigate between channels and Chat direct messages", async ({ page }) => {
  const topic = unique("Meet tab channel");
  await createRoom(topic, [primaryAgent]);
  await createDM(primaryAgent);
  await openMeet(page);

  await openChat(page, primaryAgent);
  await expect(page.getByText("Past conversations", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Meetings", { exact: true }).first()).toHaveCount(0);
  await expect(page.getByRole("heading", { name: primaryAgent })).toBeVisible();
  await expect(page.locator("textarea")).toBeEnabled();

  await page.getByRole("tab", { name: /Meet/ }).click();
  await expect(page.getByText("Meetings", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Past conversations", { exact: true }).first()).toHaveCount(0);
  await expect(page.getByLabel("Message the room")).toBeEnabled();
});

test("an empty legacy DM transcript never renders raw schema JSON", async ({ page }) => {
  await createDM(primaryAgent);
  await openMeet(page);
  await page.route("**/api/dms/echoback-fixed", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        state: {
          agent: primaryAgent,
          human: "e2e-human",
          created: new Date().toISOString(),
          updated: new Date().toISOString(),
        },
        events: null,
      }),
    });
  }, { times: 1 });
  await openChat(page, primaryAgent);
  await expect(page.locator("textarea")).toBeEnabled();
  await expect(page.getByText(/expected.*array|invalid_type/i)).toHaveCount(0);
});

test("routes an unoccupied permanent role instead of silently posting it", async ({ page }) => {
  const topic = unique("Browser lazy role room");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  await page.route("**/api/rooms/*/address", async (route) => {
    await route.fulfill({
      status: 202,
      contentType: "application/json",
      body: JSON.stringify({ job: "lazy-steward", room: "steward" }),
    });
  }, { times: 1 });
  const requestPromise = page.waitForRequest((request) =>
    request.url().endsWith("/address"),
  );

  await page.getByLabel("Message the room").fill("@steward what is the hostname?");
  await page.getByRole("button", { name: "Send message" }).click();

  const request = await requestPromise;
  expect(request.postDataJSON()).toEqual({
    agent: "steward",
    text: "what is the hostname?",
  });
  // Addressing is asynchronous, so the composer reports the accepted turn. The
  // original @steward text must never be recorded as a plain human message.
  await expect(page.getByText(/message to steward was accepted/i)).toBeVisible();
  await expect(page.getByText("@steward what is the hostname?", { exact: true })).toHaveCount(0);
});

async function openMeet(page: Page) {
  await page.goto(`${baseURL}/?mock=0`);
  await expect(page.getByText("bashymeet", { exact: true })).toBeVisible();
}

async function openChat(page: Page, agent: string) {
  await page.getByRole("tab", { name: /Chat/ }).click();
  const dmButton = page.locator("aside").first().getByRole("button", { name: new RegExp(agent) });
  const newChat = page.getByRole("menuitem", { name: new RegExp(agent) });
  await Promise.race([dmButton.waitFor(), newChat.waitFor()]);
  if (await newChat.isVisible()) await newChat.click();
  else await dmButton.click();
  await expect(page.getByRole("heading", { name: agent })).toBeVisible();
}

async function createRoomFromUI(page: Page, topic: string, agents: string) {
  await page.getByRole("button", { name: "New meeting" }).click();
  await page.getByLabel("Topic").fill(topic);
  await page.getByLabel("Facilitator").selectOption(facilitatorAgent);
  await page.getByLabel("Participants").selectOption(agents.split(/[,\s]+/).filter(Boolean));
  await page.getByRole("button", { name: "Open meeting" }).click();
  await expect(
    page.getByRole("button", { name: new RegExp(topic) }),
  ).toBeVisible();
}

async function createRoom(
  topic: string,
  participants: string[],
): Promise<{ id: string }> {
  const created = await api("api/rooms", {
    method: "POST",
    body: JSON.stringify({
      topic,
      participants,
      owner: facilitatorAgent,
      human: "e2e-human",
    }),
  });
  if (
    !created ||
    typeof created !== "object" ||
    !("id" in created) ||
    typeof created.id !== "string"
  ) {
    throw new Error(
      `create room response did not include an id: ${JSON.stringify(created)}`,
    );
  }
  return { id: created.id };
}

async function createDM(agent: string): Promise<void> {
  await api("api/dms", {
    method: "POST",
    body: JSON.stringify({ agent }),
  });
}

async function postMessage(ref: string, author: string, text: string) {
  return api(`api/rooms/${encodeURIComponent(ref)}/post`, {
    method: "POST",
    body: JSON.stringify({ author, text }),
  });
}

async function api(pathname: string, init?: RequestInit): Promise<unknown> {
  const response = await fetch(new URL(pathname, baseURL), {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  if (!response.ok) {
    throw new Error(
      `${init?.method ?? "GET"} ${pathname} failed with ${response.status}: ${await response.text()}`,
    );
  }
  if (response.status === 204) return undefined;
  return response.json();
}

async function waitForHealth() {
  const deadline = Date.now() + 15_000;
  let lastError: unknown;
  while (Date.now() < deadline) {
    if (server?.exitCode !== null) {
      throw new Error(`meet serve exited early with ${server?.exitCode}\n${serverOutput}`);
    }
    try {
      const response = await fetch(new URL("healthz", baseURL));
      if (response.ok) return;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`meet serve did not become healthy: ${String(lastError)}\n${serverOutput}`);
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const probe = net.createServer();
    probe.on("error", reject);
    probe.listen(0, "127.0.0.1", () => {
      const address = probe.address();
      probe.close(() => {
        if (typeof address === "object" && address) {
          resolve(address.port);
        } else {
          reject(new Error("could not allocate a TCP port"));
        }
      });
    });
  });
}

function unique(prefix: string) {
  return `${prefix} ${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
