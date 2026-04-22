import { expect, test } from '@playwright/test';

test.describe('login screen', () => {
  test('renders a sign-in button', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('button', { name: /sign in/i })).toBeVisible();
  });
});
