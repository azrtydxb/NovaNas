import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";

test.describe("app catalog install", () => {
  test("install an app, status transitions to Running", async ({ authedPage, api }) => {
    await authedPage.goto("/apps/catalog");
    await expect(authedPage.locator(selectors.catalogGrid)).toBeVisible();

    // Pick a deterministic lightweight catalog entry.
    const card = authedPage.locator(selectors.catalogCard("nextcloud"));
    await card.click();

    await authedPage.locator(selectors.installAppButton).click();
    await authedPage.getByLabel(/instance name/i).fill("e2e-nextcloud");
    await authedPage.getByRole("button", { name: /install|deploy/i }).click();

    await authedPage.goto("/apps/instances");
    const status = authedPage.locator(selectors.appStatus("e2e-nextcloud"));
    await expect(status).toHaveText(/pending|installing|running/i, {
      timeout: 30_000,
    });
    await expect(status).toHaveText(/running/i, { timeout: 5 * 60_000 });

    const apps = await api.listApps();
    expect(
      (apps.items as Array<{ metadata: { name: string } }>).some(
        (a) => a.metadata.name === "e2e-nextcloud",
      ),
    ).toBe(true);
  });
});
