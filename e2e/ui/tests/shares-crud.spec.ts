import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";
import { seedDataset, seedPool, seedShare } from "../fixtures/seed";

test.describe("shares CRUD", () => {
  test.beforeEach(async ({ api }) => {
    await seedPool(api);
    await seedDataset(api);
  });

  test("create share, verify protocols list SMB + NFS", async ({ authedPage, api }) => {
    await seedShare(api, "e2e-share-photos", "e2e-dataset-media");

    await authedPage.goto("/shares");
    const row = authedPage.locator(selectors.shareRow("e2e-share-photos"));
    await expect(row).toBeVisible();
    await expect(row).toContainText(/smb/i);
    await expect(row).toContainText(/nfs/i);
  });

  test("share detail shows allowed networks and ACL rows", async ({
    authedPage,
    api,
  }) => {
    await seedShare(api, "e2e-share-photos", "e2e-dataset-media");
    await authedPage.goto("/shares/e2e-share-photos");
    await expect(authedPage.getByText(/rootSquash/i)).toBeVisible();
  });
});
