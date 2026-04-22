import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";
import { seedDataset, seedPool } from "../fixtures/seed";

test.describe("datasets CRUD", () => {
  test.beforeEach(async ({ api }) => {
    await seedPool(api, "e2e-pool-main");
  });

  test("create a dataset, appears in list", async ({ authedPage, api }) => {
    await seedDataset(api, "e2e-dataset-media", "e2e-pool-main");
    await authedPage.goto("/storage/datasets");
    await expect(
      authedPage.locator(selectors.datasetRow("e2e-dataset-media")),
    ).toBeVisible();
  });

  test("edit dataset protection policy and save", async ({ authedPage, api }) => {
    await seedDataset(api, "e2e-dataset-media", "e2e-pool-main");
    await authedPage.goto("/storage/datasets/e2e-dataset-media");
    await authedPage.getByRole("button", { name: /edit|protection/i }).first().click();
    await authedPage.locator(selectors.protectionSelect).selectOption("mirror");
    await authedPage.getByRole("button", { name: /save|apply/i }).click();
    await expect(authedPage.getByText(/saved|updated/i)).toBeVisible();

    const ds = await api.get<{ spec: { protection: { mode: string } } }>(
      "/api/v1/datasets/e2e-dataset-media",
    );
    expect(ds.spec.protection.mode).toBe("mirror");
  });
});
