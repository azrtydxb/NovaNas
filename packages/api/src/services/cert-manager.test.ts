import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { Certificate as NovaCertificate } from '@novanas/schemas';
import { describe, expect, it, vi } from 'vitest';
import { createCertManagerClient } from './cert-manager.js';

function fakeApi() {
  return {
    createNamespacedCustomObject: vi.fn().mockResolvedValue({}),
    patchNamespacedCustomObject: vi.fn().mockResolvedValue({}),
    deleteNamespacedCustomObject: vi.fn().mockResolvedValue({}),
  } as unknown as CustomObjectsApi & {
    createNamespacedCustomObject: ReturnType<typeof vi.fn>;
    patchNamespacedCustomObject: ReturnType<typeof vi.fn>;
    deleteNamespacedCustomObject: ReturnType<typeof vi.fn>;
  };
}

const baseCert: NovaCertificate = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Certificate',
  metadata: { name: 'web' },
  spec: {
    provider: 'acme',
    commonName: 'example.com',
    dnsNames: ['example.com', 'www.example.com'],
    acme: { issuer: 'letsencrypt' },
    renewBeforeDays: 15,
  },
};

describe('cert-manager projection', () => {
  it('creates a cert-manager Certificate with the right ClusterIssuer for acme/letsencrypt', async () => {
    const api = fakeApi();
    const cm = createCertManagerClient(api);
    await cm.ensureCertificate(baseCert, 'novanas-system');
    expect(api.createNamespacedCustomObject).toHaveBeenCalledTimes(1);
    const [, , ns, , body] = api.createNamespacedCustomObject.mock.calls[0]!;
    expect(ns).toBe('novanas-system');
    expect(body).toMatchObject({
      apiVersion: 'cert-manager.io/v1',
      kind: 'Certificate',
      metadata: { name: 'web', namespace: 'novanas-system' },
      spec: {
        secretName: 'web-tls',
        commonName: 'example.com',
        dnsNames: ['example.com', 'www.example.com'],
        issuerRef: { name: 'novanas-acme-prod', kind: 'ClusterIssuer' },
        renewBefore: '360h',
      },
    });
  });

  it('upload provider is a no-op (user supplied material directly)', async () => {
    const api = fakeApi();
    const cm = createCertManagerClient(api);
    await cm.ensureCertificate(
      {
        ...baseCert,
        spec: {
          provider: 'upload',
          commonName: 'x',
          upload: {
            certSecret: { secretName: 'c' },
            keySecret: { secretName: 'k' },
          },
        },
      },
      'novanas-system'
    );
    expect(api.createNamespacedCustomObject).not.toHaveBeenCalled();
  });

  it('patches when create returns 409', async () => {
    const api = fakeApi();
    api.createNamespacedCustomObject.mockRejectedValueOnce({ statusCode: 409 });
    const cm = createCertManagerClient(api);
    await cm.ensureCertificate(baseCert, 'novanas-system');
    expect(api.patchNamespacedCustomObject).toHaveBeenCalledTimes(1);
  });

  it('delete swallows 404', async () => {
    const api = fakeApi();
    api.deleteNamespacedCustomObject.mockRejectedValueOnce({ statusCode: 404 });
    const cm = createCertManagerClient(api);
    await expect(cm.deleteCertificate('gone', 'ns')).resolves.toBeUndefined();
  });

  it('internalPki maps to selfsigned issuer', async () => {
    const api = fakeApi();
    const cm = createCertManagerClient(api);
    await cm.ensureCertificate(
      { ...baseCert, spec: { ...baseCert.spec, provider: 'internalPki', acme: undefined } },
      'novanas-system'
    );
    const [, , , , body] = api.createNamespacedCustomObject.mock.calls[0]!;
    const spec = (body as { spec: { issuerRef: { name: string } } }).spec;
    expect(spec.issuerRef.name).toBe('novanas-selfsigned');
  });
});
