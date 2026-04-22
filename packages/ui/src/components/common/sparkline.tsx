import { useId } from 'react';

export interface SparklineProps {
  data: number[];
  width?: number;
  height?: number;
  color?: string;
  filled?: boolean;
  min?: number;
  max?: number;
}

export function Sparkline({
  data,
  width = 200,
  height = 40,
  color = 'currentColor',
  filled = true,
  min,
  max,
}: SparklineProps) {
  const uid = useId().replace(/:/g, '');
  if (!data.length) return null;
  const lo = min ?? Math.min(...data);
  const hi = max ?? Math.max(...data);
  const range = hi - lo || 1;
  const pad = 1;
  const w = width;
  const h = height - pad * 2;
  const step = w / Math.max(1, data.length - 1);
  const pts = data.map((v, i) => {
    const x = i * step;
    const y = pad + h - ((v - lo) / range) * h;
    return [x, y] as const;
  });
  const d = pts.map((p, i) => (i === 0 ? `M${p[0]},${p[1]}` : `L${p[0]},${p[1]}`)).join(' ');
  const dFill = `${d} L${w},${height} L0,${height} Z`;

  return (
    <svg
      viewBox={`0 0 ${w} ${height}`}
      preserveAspectRatio='none'
      style={{ width: '100%', height, display: 'block' }}
    >
      {filled && (
        <>
          <defs>
            <linearGradient id={`g-${uid}`} x1='0' y1='0' x2='0' y2='1'>
              <stop offset='0%' stopColor={color} stopOpacity='0.32' />
              <stop offset='100%' stopColor={color} stopOpacity='0' />
            </linearGradient>
          </defs>
          <path d={dFill} fill={`url(#g-${uid})`} stroke='none' />
        </>
      )}
      <path d={d} fill='none' stroke={color} strokeWidth='1.5' vectorEffect='non-scaling-stroke' />
    </svg>
  );
}
