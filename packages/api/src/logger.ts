import { pino, type Logger, type LoggerOptions } from 'pino';

export interface CreateLoggerOptions {
  level?: string;
  pretty?: boolean;
  component?: string;
}

export function createLogger(opts: CreateLoggerOptions = {}): Logger {
  const { level = 'info', pretty = false, component = 'api' } = opts;

  const options: LoggerOptions = {
    level,
    base: { component },
    timestamp: pino.stdTimeFunctions.isoTime,
    redact: {
      paths: [
        'req.headers.authorization',
        'req.headers.cookie',
        '*.password',
        '*.secret',
        '*.token',
      ],
      remove: true,
    },
  };

  if (pretty) {
    return pino({
      ...options,
      transport: {
        target: 'pino-pretty',
        options: { colorize: true, translateTime: 'SYS:HH:MM:ss.l' },
      },
    });
  }

  return pino(options);
}
