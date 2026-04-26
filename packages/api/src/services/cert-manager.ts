// cert-manager projection client.
//
// This is intentionally the only place where the api still WRITES
// Kubernetes objects post-CRD-migration (#51) — cert-manager IS the
// engine, so a NovaNas Certificate maps 1:1 onto a cert-manager
// Certificate CR. The api is a thin translator; cert-manager owns
// status (cert issuance, renewal, etc.) and the resulting Secret.

import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { Certificate as NovaCertificate } from '@novanas/schemas';

const GROUP = 'cert-manager.io';
const VERSION = 'v1';
const PLURAL = 'certificates';

interface CertManagerCertSpec {
  secretName: string;
  commonName: string;
  dnsNames?: string[];
  ipAddresses?: string[];
  issuerRef: {
    name: string;
    kind: 'ClusterIssuer';
    group: 'cert-manager.io';
  };
  duration?: string;
  renewBefore?: string;
}

export interface CertManagerClient {
  /** Project a NovaNas Certificate onto a cert-manager Certificate CR. */
  ensureCertificate(novaCert: NovaCertificate, namespace: string): Promise<void>;
  /** Best-effort delete. Missing Certificate is not an error. */
  deleteCertificate(name: string, namespace: string): Promise<void>;
}

export function createCertManagerClient(api: CustomObjectsApi): CertManagerClient {
  function buildSpec(c: NovaCertificate): CertManagerCertSpec | null {
    // Upload provider is a no-op — the user supplied the material
    // directly and there's nothing for cert-manager to issue.
    if (c.spec.provider === 'upload') return null;

    let issuerName = 'novanas-selfsigned';
    if (c.spec.provider === 'acme') {
      const acmeIssuer = c.spec.acme?.issuer;
      if (acmeIssuer === 'letsencrypt') issuerName = 'novanas-acme-prod';
      else if (acmeIssuer === 'letsencrypt-staging') issuerName = 'novanas-acme-staging';
      // 'zerossl' / 'custom' fall back to selfsigned until we ship
      // ClusterIssuers for them — cert-manager will surface the error
      // on its side.
    }

    const renewBefore = c.spec.renewBeforeDays
      ? `${c.spec.renewBeforeDays * 24}h`
      : undefined;

    return {
      secretName: `${c.metadata.name}-tls`,
      commonName: c.spec.commonName,
      dnsNames: c.spec.dnsNames,
      ipAddresses: c.spec.ipAddresses,
      issuerRef: { name: issuerName, kind: 'ClusterIssuer', group: 'cert-manager.io' },
      renewBefore,
    };
  }

  return {
    async ensureCertificate(novaCert, namespace) {
      const spec = buildSpec(novaCert);
      if (!spec) return; // upload provider — nothing to project
      const name = novaCert.metadata.name;
      const body = {
        apiVersion: `${GROUP}/${VERSION}`,
        kind: 'Certificate',
        metadata: { name, namespace },
        spec,
      };
      try {
        await api.createNamespacedCustomObject(GROUP, VERSION, namespace, PLURAL, body);
        return;
      } catch (err) {
        const status = (err as { statusCode?: number }).statusCode;
        if (status !== 409) throw err;
      }
      // Already exists — patch.
      await api.patchNamespacedCustomObject(
        GROUP,
        VERSION,
        namespace,
        PLURAL,
        name,
        { spec },
        undefined,
        undefined,
        undefined,
        { headers: { 'Content-Type': 'application/merge-patch+json' } }
      );
    },

    async deleteCertificate(name, namespace) {
      try {
        await api.deleteNamespacedCustomObject(GROUP, VERSION, namespace, PLURAL, name);
      } catch (err) {
        const status = (err as { statusCode?: number }).statusCode;
        if (status !== 404) throw err;
      }
    },
  };
}
