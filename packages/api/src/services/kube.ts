import {
  AuthenticationV1Api,
  CoreV1Api,
  CustomObjectsApi,
  KubeConfig,
} from '@kubernetes/client-node';
import type { Env } from '../env.js';

export interface KubeClients {
  config: KubeConfig;
  core: CoreV1Api;
  custom: CustomObjectsApi;
  /**
   * Authentication v1 client used to validate Bearer tokens from
   * in-cluster service accounts (disk-agent, storage-meta, …) via
   * TokenReview. See packages/api/src/auth/tokenreview.ts.
   */
  authn: AuthenticationV1Api;
}

export function createKubeClients(env: Env): KubeClients {
  const config = new KubeConfig();
  if (env.KUBECONFIG_PATH) {
    config.loadFromFile(env.KUBECONFIG_PATH);
  } else if (process.env.KUBERNETES_SERVICE_HOST) {
    config.loadFromCluster();
  } else {
    config.loadFromDefault();
  }
  return {
    config,
    core: config.makeApiClient(CoreV1Api),
    custom: config.makeApiClient(CustomObjectsApi),
    authn: config.makeApiClient(AuthenticationV1Api),
  };
}
