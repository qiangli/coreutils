import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import net from "node:net";
import { cpSync, readFileSync, writeFileSync } from "node:fs";

import { expect, test, type Page } from "@playwright/test";

const webDir = path.resolve(import.meta.dirname, "..");
const repoRoot = path.resolve(webDir, "../../..");
// Deterministic fixtures, not fleet members: addressing a real agent would exec
// its CLI, which needs a network and a key and answers differently every run.
// See e2e/fleet/ — the launch template echoes the prompt back.
const primaryAgent = "echoback-fixed";
const invitedAgent = "echoback-alt";

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
  const sidebar = page.locator("aside").filter({ hasText: "bashy meet" }).first();
  await expect(sidebar.getByText("connecting")).toHaveCount(0);
  await expect(sidebar.getByText("open", { exact: true })).toBeVisible();
});

test("creates a room from the sidebar and selects it with one agent seated", async ({ page }) => {
  const topic = unique("Browser created room");
  await openMeet(page);

  await page.getByRole("button", { name: "New room" }).click();
  await page.getByLabel("Topic").fill(topic);
  await page.getByLabel("Agents").fill(primaryAgent);
  await page.getByRole("button", { name: "Open room" }).click();

  const roomButton = page.getByRole("button", { name: new RegExp(topic) });
  const sidebar = page.locator("aside").filter({ hasText: "bashy meet" }).first();
  await expect(roomButton).toBeVisible();
  await expect(sidebar.getByText(primaryAgent)).toBeVisible();
  await expect(page.getByLabel("Message the room")).toBeEnabled();
});

test("invites an agent from room details and shows it in the roster", async ({ page }) => {
  const topic = unique("Browser invite room");
  await openMeet(page);
  await createRoomFromUI(page, topic, primaryAgent);

  // RoomDetails is mounted twice — the desktop panel and the mobile sheet — so
  // "Invite" matches two buttons. Submitting the form with Enter targets the one
  // field we filled instead of disambiguating a duplicated control.
  const inviteField = page.getByLabel("Agent to invite").first();
  await inviteField.fill(invitedAgent);
  await inviteField.press("Enter");

  const sidebar = page.locator("aside").filter({ hasText: "bashy meet" }).first();
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
  await expect(page.getByRole("button", { name: "Room closed" }).first()).toBeDisabled();
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

async function openMeet(page: Page) {
  await page.goto(`${baseURL}/?mock=0`);
  await expect(page.getByText("bashy meet")).toBeVisible();
}

async function createRoomFromUI(page: Page, topic: string, agents: string) {
  await page.getByRole("button", { name: "New room" }).click();
  await page.getByLabel("Topic").fill(topic);
  await page.getByLabel("Agents").fill(agents);
  await page.getByRole("button", { name: "Open room" }).click();
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
