import { Badge, type BadgeProps } from '@/components/ui/badge';
import type { StatusTone } from '@/types';

const TONE_MAP: Record<StatusTone, BadgeProps['tone']> = {
  ok: 'ok',
  warn: 'warn',
  err: 'err',
  info: 'accent',
  idle: 'default',
};

export function HealthPill({
  tone = 'ok',
  children,
}: {
  tone?: StatusTone;
  children: React.ReactNode;
}) {
  return (
    <Badge tone={TONE_MAP[tone]} dot>
      {children}
    </Badge>
  );
}
