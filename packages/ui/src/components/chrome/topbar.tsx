import { HealthPill } from '@/components/common/health-pill';
import {
  Avatar,
  AvatarFallback,
  Badge,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Input,
} from '@/components/ui';
import { useAuth } from '@/hooks/use-auth';
import { cn } from '@/lib/cn';
import { Bell, ChevronDown, LogOut, Search, Settings, User as UserIcon } from 'lucide-react';
import { Brand } from './brand';

export function Topbar({ currentPageTitle }: { currentPageTitle?: string }) {
  const { user, logout } = useAuth();
  const initials = user ? initialsFromName(user.name || user.username) : 'NN';

  return (
    <div className='row-start-1 col-span-full flex items-center gap-3 px-3 h-12 bg-panel-alt border-b border-border relative z-10'>
      <Brand />

      <div className='flex items-center gap-1.5 text-xs text-foreground-subtle'>
        <span>Console</span>
        <span className='opacity-50'>/</span>
        <span className='text-foreground'>{currentPageTitle ?? 'Dashboard'}</span>
      </div>

      <div className='flex-1' />

      <div
        className={cn(
          'hidden md:flex items-center gap-2 h-[30px] px-2.5 w-[320px]',
          'bg-surface border border-border rounded-md text-foreground-muted'
        )}
      >
        <Search size={13} />
        <Input
          className='flex-1 h-auto border-0 bg-transparent focus:ring-0 px-0 text-xs'
          placeholder='Search datasets, apps, disks…'
          aria-label='Search'
        />
        <kbd className='mono text-[10px] bg-panel px-1.5 py-0.5 border border-border rounded text-foreground-subtle'>
          ⌘K
        </kbd>
      </div>

      <HealthPill tone='ok'>26.07.3</HealthPill>

      <button
        type='button'
        className='relative w-[30px] h-[30px] grid place-items-center rounded-md text-foreground-muted hover:bg-elevated hover:text-foreground'
        aria-label='Alerts'
      >
        <Bell size={15} />
        <span className='absolute top-1 right-1 h-2 w-2 rounded-full bg-danger border-2 border-panel-alt' />
      </button>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type='button'
            className='inline-flex items-center gap-2 py-0.5 pl-0.5 pr-2.5 bg-surface border border-border rounded-full'
            aria-label='User menu'
          >
            <Avatar>
              <AvatarFallback>{initials}</AvatarFallback>
            </Avatar>
            <span className='text-sm text-foreground'>{user?.username ?? 'guest'}</span>
            <ChevronDown size={12} className='text-foreground-subtle' />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
          <DropdownMenuLabel>{user?.email ?? 'Signed out'}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem>
            <UserIcon size={14} /> Profile
          </DropdownMenuItem>
          <DropdownMenuItem>
            <Settings size={14} /> Preferences
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => void logout()}>
            <LogOut size={14} /> Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function initialsFromName(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 0 || !parts[0]) return 'NN';
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return (parts[0]![0]! + parts[parts.length - 1]![0]!).toUpperCase();
}
