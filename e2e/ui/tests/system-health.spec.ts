import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";

test.describe("system pages", () => {
  const pages: Array<{ path: string; testid: keyof typeof selectors }> = [
    { path: "/system/settings", testid: "systemSettings" },
    { path: "/system/updates", testid: "updatesPage" },
    { path: "/system/certificates", testid: "certificatesPage" },
    { path: "/system/alerts", testid: "alertsPage" },
    { path: "/system/audit", testid: "auditPage" },
  ];

  for (const { path, testid } of pages) {
    test(`renders ${path}`, async ({ authedPage }) => {
      await authedPage.goto(path);
      await expect(authedPage.locator(selectors[testid])).toBeVisible();
    });
  }

  test("version is exposed on /api/version", async ({ api }) => {
    const v = await api.version();
    expect(v.version).toMatch(/^\d+\.\d+/);
  });
});
