import { KubeConfig, CoreV1Api, CustomObjectsApi } from '@kubernetes/client-node';
import type { Env } from '../env.js';

export interface KubeClients {
  config: KubeConfig;
  core: CoreV1Api;
  custom: CustomObjectsApi;
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
  };
}
