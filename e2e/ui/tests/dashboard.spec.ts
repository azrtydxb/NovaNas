import { test, expect } from "../fixtures/auth";
import { selectors } from "../lib/selectors";

test.describe("dashboard", () => {
  test("loads with health pill, capacity cards, activity feed", async ({
    authedPage,
    api,
  }) => {
    // Smoke the API first so we know the backend is up before driving the UI.
    const health = await api.health();
    expect(health.status).toMatch(/ok|ready|healthy/i);

    await authedPage.goto("/");
    await expect(authedPage.locator(selectors.healthPill)).toBeVisible();
    await expect(authedPage.locator(selectors.capacityCard).first()).toBeVisible();
    await expect(authedPage.locator(selectors.activityFeed)).toBeVisible();
  });

  test("health pill reflects backend /health state", async ({ authedPage }) => {
    await authedPage.goto("/");
    const pill = authedPage.locator(selectors.healthPill);
    await expect(pill).toHaveAttribute("data-status", /ok|degraded|error/);
  });
});
