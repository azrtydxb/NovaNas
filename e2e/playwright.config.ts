import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for NovaNas E2E.
 *
 * The target deployment is expected to be reachable at NOVANAS_BASE_URL
 * (default: https://localhost:8443). In CI, `scripts/bootstrap-cluster.sh`
 * brings up a kind cluster and installs the NovaNas umbrella helm chart,
 * then port-forwards the UI to the expected URL.
 */

const baseURL = process.env.NOVANAS_BASE_URL ?? "https://localhost:8443";
const isCI = !!process.env.CI;

export default defineConfig({
  testDir: "./ui/tests",
  fullyParallel: false,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  workers: isCI ? 2 : undefined,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: "playwright-report" }],
    ["junit", { outputFile: "test-results/junit.xml" }],
  ],
  use: {
    baseURL,
    ignoreHTTPSErrors: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
  },
  projects: [
    {
      name: "ui",
      use: { ...devices["Desktop Chrome"] },
      testMatch: /.*\.spec\.ts/,
    },
  ],
  outputDir: "test-results",
});
