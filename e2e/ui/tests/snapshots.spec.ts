import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";
import { seedDataset, seedPool } from "../fixtures/seed";

test.describe("snapshots", () => {
  test.beforeEach(async ({ api }) => {
    await seedPool(api);
    await seedDataset(api);
  });

  test("take manual snapshot, appears in list", async ({ authedPage, api }) => {
    await authedPage.goto("/data-protection/snapshots?source=e2e-dataset-media");
    await authedPage.locator(selectors.takeSnapshotButton).click();
    await authedPage
      .getByRole("dialog")
      .getByRole("button", { name: /create|take/i })
      .click();

    await expect(authedPage.locator(selectors.snapshotList)).toBeVisible();
    await expect(authedPage.locator(selectors.snapshotList)).toContainText(
      /e2e-dataset-media/,
    );

    const snaps = await api.listSnapshots();
    expect(Array.isArray(snaps.items)).toBe(true);
  });
});
