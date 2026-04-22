import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { type ThemePreference, useUiStore } from '@/stores/ui';
import { Check, Monitor, Moon, Sun } from 'lucide-react';

const OPTIONS: Array<{ value: ThemePreference; label: string; icon: typeof Sun }> = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
];

export function ThemeToggle() {
  const theme = useUiStore((s) => s.theme);
  const setTheme = useUiStore((s) => s.setTheme);
  const current = OPTIONS.find((o) => o.value === theme) ?? OPTIONS[1]!;
  const Icon = current.icon;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type='button'
          className='w-[30px] h-[30px] grid place-items-center rounded-md text-foreground-muted hover:bg-elevated hover:text-foreground'
          aria-label={`Theme: ${current.label}`}
        >
          <Icon size={15} />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <DropdownMenuLabel>Theme</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {OPTIONS.map((opt) => {
          const Ico = opt.icon;
          const active = theme === opt.value;
          return (
            <DropdownMenuItem key={opt.value} onSelect={() => setTheme(opt.value)}>
              <Ico size={14} />
              <span className='flex-1'>{opt.label}</span>
              {active && <Check size={12} />}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
