import { DiagConsoleLogger, DiagLogLevel, diag } from '@opentelemetry/api';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';
import { NodeSDK } from '@opentelemetry/sdk-node';

let sdk: NodeSDK | undefined;

/**
 * Initialise OpenTelemetry. Must be called before Fastify/Node modules that
 * we want to auto-instrument are imported in production builds. For the
 * scaffold we just wire the SDK with defaults — exporters can be configured
 * via `OTEL_*` environment variables.
 */
export function initTelemetry(serviceName = 'novanas-api'): void {
  if (sdk) return;
  if (process.env.OTEL_DIAG_LOG_LEVEL === 'debug') {
    diag.setLogger(new DiagConsoleLogger(), DiagLogLevel.DEBUG);
  }

  sdk = new NodeSDK({
    serviceName,
    instrumentations: [getNodeAutoInstrumentations()],
  });

  try {
    sdk.start();
  } catch (err) {
    // Telemetry failures must never crash the API.
    // eslint-disable-next-line no-console
    console.error('OpenTelemetry failed to start:', err);
  }
}

export async function shutdownTelemetry(): Promise<void> {
  if (!sdk) return;
  await sdk.shutdown().catch(() => undefined);
  sdk = undefined;
}
