export function Brand({ hostname = 'nas-01' }: { hostname?: string }) {
  return (
    <div className='flex items-center gap-2 pr-2.5 mr-1 h-full border-r border-border min-w-[204px]'>
      <div
        className='w-[22px] h-[22px] grid place-items-center rounded-md text-[var(--accent-fg)] font-semibold'
        style={{
          background:
            'linear-gradient(140deg, var(--accent) 0%, oklch(0.55 0.14 calc(var(--accent-h) + 30)) 100%)',
        }}
      >
        <svg
          width='14'
          height='14'
          viewBox='0 0 14 14'
          fill='none'
          stroke='currentColor'
          strokeWidth='1.6'
          strokeLinejoin='round'
        >
          <path d='M2 4.5L7 2l5 2.5L7 7z' fill='currentColor' fillOpacity='0.9' stroke='none' />
          <path d='M2 7L7 9.5 12 7M2 9.5L7 12l5-2.5' />
        </svg>
      </div>
      <div className='font-semibold text-foreground tracking-tight'>NovaNas</div>
      <span className='ml-auto mono text-xs text-foreground-subtle'>{hostname}</span>
    </div>
  );
}
