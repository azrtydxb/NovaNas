import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";

test.describe("login / OIDC flow", () => {
  test("unauthenticated user is redirected to login", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/login|\/realms\/novanas/);
    await expect(page.locator(selectors.loginButton)).toBeVisible();
  });

  test("authenticated session reaches app shell", async ({ authedPage }) => {
    await authedPage.goto("/");
    await expect(authedPage.locator(selectors.appShell)).toBeVisible();
    await expect(authedPage.locator(selectors.userMenu)).toBeVisible();
  });

  test("logout clears the session and returns to login", async ({ authedPage }) => {
    await authedPage.goto("/");
    await authedPage.locator(selectors.userMenu).click();
    await authedPage.locator(selectors.logout).click();
    await expect(authedPage).toHaveURL(/\/login|\/realms\/novanas/);
  });
});
