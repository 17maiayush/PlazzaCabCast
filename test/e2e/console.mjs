// Clicks through every console flow. Run it against a freshly started server (`make run`, then `make e2e`): it expects an empty store.
import { readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { chromium } from "playwright-core";

// Every route the API registers; the walkthrough must hit each one from the page itself.
const routes = [...readFileSync(new URL("../../internal/api/api.go", import.meta.url), "utf8")
  .matchAll(/Handle(?:Func)?\("([A-Z]+) ([^"]+)"/g)]
  .map(([, method, path]) => ({ name: `${method} ${path}`, method, re: new RegExp(`^${path.replace(/\{[^}]+\}/g, "[^/]+")}$`) }));
const seen = new Set();

const base = process.argv[2] ?? "http://localhost:8080/";
// screenshots go to the temp dir unless a folder is given, so a run never litters the repo
const shots = process.argv[3] ?? tmpdir();
// Uses an installed Chrome; set CHROME_PATH if it lives somewhere else.
const browser = await chromium.launch({
  executablePath: process.env.CHROME_PATH ?? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
});
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = [];
const notFound = [];
page.on("response", (r) => { if (r.status() === 404) notFound.push(r.url()); });
page.on("request", (r) => {
  const url = new URL(r.url());
  if (url.origin !== new URL(base).origin) return;
  for (const route of routes) if (route.method === r.method() && route.re.test(url.pathname)) seen.add(route.name);
});
page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
page.on("console", (m) => { if (m.type() === "error" && !m.text().startsWith("Failed to load resource")) errors.push(`console: ${m.text()}`); });

const results = [];
async function step(name, action, expectToast) {
  await page.evaluate(() => { document.querySelector("#toast").textContent = ""; });
  await action();
  await page.waitForFunction((t) => document.querySelector("#toast").textContent.includes(t), expectToast, { timeout: 5000 })
    .then(() => results.push(`PASS ${name}: ${expectToast}`))
    .catch(async () => results.push(`FAIL ${name}: toast was "${await page.textContent("#toast")}"`));
}
const book = () => page.click("#book-form button.primary");
const card = (text) => page.locator("#ride-list .ride", { hasText: text }).first();
async function expectQuote(name, text) {
  await page.waitForFunction((t) => document.querySelector("#quote").textContent.includes(t), text, { timeout: 5000 })
    .then(() => results.push(`PASS ${name}: quote shows "${text}"`))
    .catch(async () => results.push(`FAIL ${name}: quote was "${await page.textContent("#quote")}"`));
}

await page.goto(base);
await page.waitForFunction(() => document.querySelector("#health-text").textContent === "API up");
results.push("PASS health indicator: API up");

await step("seed", () => page.click("#seed"), "Loaded 1 rider, 3 drivers");
await expectQuote("live estimate before booking", "₹50.00");
await expectQuote("estimate shows availability", "A Hatchback is free");
await step("book hatchback", book, "Booked with Ravi");
await expectQuote("estimate warns about the upgrade", "you get a Sedan at the Hatchback price");
await step("hatchback busy -> sedan upgrade", book, "Meena's Sedan at the Hatchback price");
await step("end short ride -> minimum fare", () => card("with Ravi").getByRole("button", { name: "End ride" }).click(), "₹50.00, minimum fare");
await card("with Meena").locator("select").selectOption("7");
await step("end upgraded ride -> hatchback price", () => card("with Meena").getByRole("button", { name: "End ride" }).click(), "₹96.88, charged as Hatchback");
await page.fill("#book-form [name=coupon_code]", "NOPE");
await expectQuote("estimate flags a bad coupon", "That coupon does not exist");
await step("invalid coupon", book, "INVALID_COUPON");
await page.fill("#book-form [name=coupon_code]", "");
await page.selectOption("#book-pickup", "6");
await expectQuote("estimate says no driver", "No driver within 5 km");
await step("no driver in radius", book, "NO_DRIVER_AVAILABLE");
await page.selectOption("#book-pickup", "1");
await page.fill("#book-form [name=coupon_code]", "save20");
await page.click("details.advanced summary");
await page.fill("#idem-key", "demo-k1");
await step("book with coupon + key", book, "Booked with Ravi");
await step("same key again -> replay", book, "Replayed");
await step("cancel inside grace", () => card("SAVE20").getByRole("button", { name: "Cancel" }).click(), "Cancelled free");
await page.click("#map", { position: { x: 300, y: 250 } });
await page.waitForFunction(() => document.querySelector("#toast").textContent.includes("Pin dropped"));
results.push("PASS map click drops a pin");
await page.click("#history-form button");
await page.waitForSelector("#history-list .item", { timeout: 5000 }).catch(() => {});
const historyItems = await page.locator("#history-list .item").count();
results.push(`${historyItems === 3 ? "PASS" : "FAIL"} history shows ${historyItems} rides for Asha (want 3: two completed, one cancelled)`);
const rideCards = await page.locator("#ride-list .ride").count();
results.push(`${rideCards === 3 ? "PASS" : "FAIL"} ride list has ${rideCards} cards (want 3, the replay added none)`);

await page.fill("#user-form [name=name]", "Kiran");
await step("register rider from the form", () => page.click("#user-form button"), "Registered Kiran");
await page.fill("#driver-form [name=name]", "Sunil");
await page.selectOption("#driver-form [name=car_type]", "SEDAN");
await page.selectOption("#driver-form [name=place]", "2");
await step("register driver from the form", () => page.click("#driver-form button"), "Registered Sunil");
await page.selectOption("#move-driver", { label: "Sunil" });
await page.selectOption("#move-form [name=place]", "3");
await step("move a cab", () => page.click("#move-form button"), "Sunil is now at HSR Layout");
await page.fill("#coupon-form [name=code]", "ten");
await page.selectOption("#coupon-form [name=kind]", "FLAT");
await page.fill("#coupon-form [name=value]", "10");
await step("add a coupon", () => page.click("#coupon-form button"), "Added TEN");
await step("delete a coupon", () => page.locator("#coupon-list .item", { hasText: "TEN" }).getByRole("button", { name: "Delete" }).click(), "Deleted TEN");
await page.selectOption("#history-who", { label: "Ravi" });
await page.selectOption("#history-form [name=status]", "completed");
await page.click("#history-form button");
await page.waitForFunction(() => document.querySelectorAll("#history-list .item").length === 1, null, { timeout: 5000 })
  .then(() => results.push("PASS driver history: Ravi has 1 completed ride"))
  .catch(() => results.push("FAIL driver history"));

await page.reload();
await page.waitForFunction(() => document.querySelectorAll("#ride-list .ride").length === 3 && document.querySelectorAll("#driver-list .item").length === 4, null, { timeout: 5000 })
  .then(() => results.push("PASS reload keeps every rider, driver and ride (read from the server)"))
  .catch(() => results.push("FAIL reload lost data"));
await expectQuote("quote is live again after reload", "A Hatchback is free");
await page.screenshot({ path: `${shots}/console-light.png`, fullPage: true });
await page.emulateMedia({ colorScheme: "dark" });
await page.setViewportSize({ width: 390, height: 900 });
await page.screenshot({ path: `${shots}/console-dark-mobile.png`, fullPage: false });
const overflow = await page.evaluate(() => [...document.querySelectorAll("body *")]
  .filter((el) => el.getBoundingClientRect().right > window.innerWidth + 1)
  .map((el) => `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ""}.${el.className?.baseVal ?? el.className}`)
  .slice(0, 5));
results.push(overflow.length ? `FAIL horizontal scroll at 390px, caused by ${overflow.join(", ")}` : "PASS no horizontal scroll at 390px");

const missing = routes.filter((r) => !seen.has(r.name)).map((r) => r.name);
results.push(missing.length ? `FAIL routes never called from the page: ${missing.join(", ")}` : `PASS all ${routes.length} API routes were called from the page`);
console.log(results.join("\n"));
console.log(notFound.length ? `404s: ${notFound.join(", ")}` : "no unexpected 404s");
console.log(errors.length ? `JS ERRORS:\n${errors.join("\n")}` : "no JS errors");
await browser.close();
const failed = results.filter((r) => r.startsWith("FAIL")).length + errors.length + notFound.length;
process.exit(failed ? 1 : 0);
