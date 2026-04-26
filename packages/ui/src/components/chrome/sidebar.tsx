import { useSystemHealth, useSystemInfo } from '@/api/system';
import { useAuth } from '@/hooks/use-auth';
import { cn } from '@/lib/cn';
import { formatBytes } from '@/lib/format';
import type { NavItem } from '@/types';
import { Trans } from '@lingui/react';
import { Link, useRouterState } from '@tanstack/react-router';
import {
  AppWindow,
  Database,
  HardDrive,
  LayoutDashboard,
  MonitorCog,
  Network,
  Server,
  Share2,
  Shield,
  Users,
} from 'lucide-react';
import type { ComponentType } from 'react';
import { ADMIN_NAV, USER_NAV } from './nav';

const ICONS: Record<string, ComponentType<{ size?: number; className?: string }>> = {
  dashboard: LayoutDashboard,
  storage: Database,
  share: Share2,
  protect: Shield,
  app: AppWindow,
  vm: Server,
  network: Network,
  identity: Users,
  system: MonitorCog,
  dataset: Database,
  snap: HardDrive,
};

export function Sidebar() {
  const { user, hasRole } = useAuth();
  // Role resolution: admin sees everything; user sees the narrowed list;
  // viewer sees the same nav tree as 'user' but with all mutation actions
  // hidden on the pages themselves (enforced per-screen via `useAuth().hasRole`).
  const isAdmin = user ? hasRole('admin') : true; // default to admin-view while scaffolding
  const nav = isAdmin ? ADMIN_NAV : USER_NAV;
  const { location } = useRouterState();
  const pathname = location.pathname;

  return (
    <aside className='bg-panel-alt border-r border-border py-2.5 px-2 overflow-y-auto flex flex-col gap-0.5'>
      <nav className='flex flex-col gap-0.5'>
        {nav.map((item) => (
          <NavGroup key={item.id} item={item} pathname={pathname} />
        ))}
      </nav>

      <SidebarFooter />
    </aside>
  );
}

function SidebarFooter() {
  const health = useSystemHealth();
  const info = useSystemInfo();
  const used = health.data?.capacity?.usedBytes;
  const total = health.data?.capacity?.totalBytes;
  const pct = used != null && total && total > 0 ? Math.min(100, (used / total) * 100) : 0;
  const capacityLabel =
    used != null && total != null ? `${formatBytes(used)} / ${formatBytes(total)}` : '—';
  const uptime = info.data?.uptimeSeconds != null ? formatUptime(info.data.uptimeSeconds) : '—';

  return (
    <div className='mt-auto pt-2.5 px-2.5 border-t border-border flex flex-col gap-1.5 text-xs text-foreground-subtle'>
      <div className='flex items-center justify-between'>
        <span>
          <Trans id='Capacity used' />
        </span>
        <span className='mono text-foreground'>{capacityLabel}</span>
      </div>
      <div className='h-1 bg-elevated rounded-full overflow-hidden'>
        <div className='h-full bg-accent' style={{ width: `${pct}%` }} />
      </div>
      <div className='flex items-center justify-between opacity-80'>
        <span>
          <Trans id='Uptime' />
        </span>
        <span className='mono'>{uptime}</span>
      </div>
    </div>
  );
}

function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function NavGroup({ item, pathname }: { item: NavItem; pathname: string }) {
  const Icon = item.icon ? ICONS[item.icon] : undefined;
  const isActive =
    pathname === item.to ||
    (item.to !== '/' && pathname.startsWith(`${item.to}/`)) ||
    !!item.children?.some((c) => pathname === c.to || pathname.startsWith(`${c.to}/`));
  const expanded = isActive && !!item.children;

  return (
    <>
      <Link
        to={item.to}
        className={cn(
          'relative flex items-center gap-2.5 px-2.5 py-1.5 rounded-sm border border-transparent cursor-pointer',
          'text-foreground-muted hover:bg-panel hover:text-foreground',
          pathname === item.to && 'bg-elevated text-foreground border-border',
          isActive && pathname !== item.to && 'text-foreground'
        )}
      >
        {isActive && (
          <span className='absolute -left-2 top-1/2 -translate-y-1/2 w-[3px] h-4 bg-accent rounded-r-sm' />
        )}
        {Icon && <Icon size={15} />}
        <span>
          <Trans id={item.label} />
        </span>
        {item.count != null && (
          <span className='ml-auto text-xs text-foreground-subtle mono'>{item.count}</span>
        )}
      </Link>
      {expanded && item.children && (
        <div className='ml-6 flex flex-col gap-px mb-1'>
          {item.children.map((c) => (
            <Link
              key={c.id}
              to={c.to}
              className={cn(
                'px-2.5 py-1 rounded-sm text-sm',
                'text-foreground-muted hover:bg-panel hover:text-foreground',
                pathname === c.to && 'bg-elevated text-foreground'
              )}
            >
              <Trans id={c.label} />
            </Link>
          ))}
        </div>
      )}
    </>
  );
}
