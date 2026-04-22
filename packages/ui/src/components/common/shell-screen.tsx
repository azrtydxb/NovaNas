import type { ReactNode } from 'react';
import { EmptyState } from './empty-state';
import { PageHeader } from './page-header';

export interface ShellScreenProps {
  title: string;
  subtitle?: ReactNode;
  actions?: ReactNode;
  upcoming: string[];
  icon?: ReactNode;
}

/**
 * Route-shell placeholder. Renders a consistent page header plus an EmptyState
 * that lists the concrete surfaces that will appear on this screen. Used for
 * every route that has not been built-out yet.
 */
export function ShellScreen({ title, subtitle, actions, upcoming, icon }: ShellScreenProps) {
  return (
    <>
      <PageHeader title={title} subtitle={subtitle} actions={actions} />
      <EmptyState
        icon={icon}
        title={`${title} — coming soon`}
        description={
          <div className='flex flex-col gap-2 items-center'>
            <div>This surface will include:</div>
            <ul className='text-sm text-foreground-muted list-disc pl-5 text-left'>
              {upcoming.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          </div>
        }
      />
    </>
  );
}
