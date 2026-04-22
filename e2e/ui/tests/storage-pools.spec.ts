import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";
import { seedPool } from "../fixtures/seed";

test.describe("storage pools", () => {
  test("create a pool via API, see it in the UI list", async ({ authedPage, api }) => {
    await seedPool(api, "e2e-pool-main");

    await authedPage.goto("/storage/pools");
    await expect(authedPage.locator(selectors.poolList)).toBeVisible();
    await expect(
      authedPage.locator(selectors.poolRow("e2e-pool-main")),
    ).toBeVisible();
  });

  test("pool detail page renders tier and recovery rate", async ({ authedPage, api }) => {
    await seedPool(api, "e2e-pool-main");
    await authedPage.goto("/storage/pools/e2e-pool-main");
    await expect(authedPage.getByText(/warm/i)).toBeVisible();
    await expect(authedPage.getByText(/balanced/i)).toBeVisible();
  });
});
