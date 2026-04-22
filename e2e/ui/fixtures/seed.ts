import { ApiClient } from "../lib/api-client";

/**
 * Seed helpers — create a minimal set of NovaNas CRs so UI specs can assert
 * against non-empty lists. Each helper is idempotent (ignores 409 conflict).
 *
 * Called from global setup / per-test fixtures. All created resources are
 * prefixed with `e2e-` so teardown can purge them.
 */

const PREFIX = "e2e-";

async function idempotent<T>(fn: () => Promise<T>): Promise<T | null> {
  try {
    return await fn();
  } catch (err) {
    if (err instanceof Error && /409|already exists/.test(err.message)) return null;
    throw err;
  }
}

export async function seedPool(api: ApiClient, name = `${PREFIX}pool-main`) {
  return idempotent(() =>
    api.post("/api/v1/pools", {
      metadata: { name },
      spec: {
        tier: "warm",
        deviceFilter: { preferredClass: "hdd", minSize: "500Gi" },
        recoveryRate: "balanced",
        rebalanceOnAdd: "manual",
      },
    }),
  );
}

export async function seedDataset(
  api: ApiClient,
  name = `${PREFIX}dataset-media`,
  pool = `${PREFIX}pool-main`,
) {
  return idempotent(() =>
    api.post("/api/v1/datasets", {
      metadata: { name },
      spec: {
        pool,
        size: "1Ti",
        filesystem: "xfs",
        protection: {
          mode: "erasureCoding",
          erasureCoding: { dataShards: 4, parityShards: 2 },
        },
        compression: "zstd",
        aclMode: "nfsv4",
      },
    }),
  );
}

export async function seedShare(
  api: ApiClient,
  name = `${PREFIX}share-photos`,
  dataset = `${PREFIX}dataset-media`,
) {
  return idempotent(() =>
    api.post("/api/v1/shares", {
      metadata: { name },
      spec: {
        dataset,
        path: "/photos",
        protocols: {
          smb: { server: "main-smb", shadowCopies: true, caseSensitive: false },
          nfs: { server: "main-nfs", squash: "rootSquash" },
        },
      },
    }),
  );
}

export async function seedSnapshotSchedule(
  api: ApiClient,
  name = `${PREFIX}sched-hourly`,
  datasetName = `${PREFIX}dataset-media`,
) {
  return idempotent(() =>
    api.post("/api/v1/snapshots/schedules", {
      metadata: { name },
      spec: {
        source: { kind: "Dataset", name: datasetName },
        cron: "0 * * * *",
        retention: { hourly: 24, daily: 7 },
      },
    }),
  );
}

export async function seedAll(api: ApiClient): Promise<void> {
  await seedPool(api);
  await seedDataset(api);
  await seedShare(api);
  await seedSnapshotSchedule(api);
}

export async function purgeSeed(api: ApiClient): Promise<void> {
  const paths = [
    "/api/v1/snapshots/schedules",
    "/api/v1/shares",
    "/api/v1/datasets",
    "/api/v1/pools",
  ];
  for (const base of paths) {
    const list = await api.get<{ items: Array<{ metadata: { name: string } }> }>(base);
    for (const item of list.items ?? []) {
      if (item.metadata?.name?.startsWith(PREFIX)) {
        await api.del(`${base}/${item.metadata.name}`).catch(() => {});
      }
    }
  }
}
