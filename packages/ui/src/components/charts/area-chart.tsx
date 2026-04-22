import { Area, AreaChart as ReAreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

export interface AreaChartProps {
  data: Array<Record<string, number | string>>;
  dataKey: string;
  xKey?: string;
  color?: string;
  height?: number;
  hideAxes?: boolean;
}

export function AreaChart({
  data,
  dataKey,
  xKey = 'x',
  color = 'var(--accent)',
  height = 120,
  hideAxes = true,
}: AreaChartProps) {
  return (
    <ResponsiveContainer width='100%' height={height}>
      <ReAreaChart data={data} margin={{ top: 4, right: 4, bottom: 4, left: 4 }}>
        <defs>
          <linearGradient id={`fill-${dataKey}`} x1='0' y1='0' x2='0' y2='1'>
            <stop offset='0%' stopColor={color} stopOpacity={0.32} />
            <stop offset='100%' stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        {!hideAxes && <XAxis dataKey={xKey} fontSize={10} stroke='var(--fg-3)' />}
        {!hideAxes && <YAxis fontSize={10} stroke='var(--fg-3)' />}
        <Tooltip
          contentStyle={{
            background: 'var(--bg-2)',
            border: '1px solid var(--line)',
            borderRadius: 8,
            fontSize: 12,
          }}
        />
        <Area
          type='monotone'
          dataKey={dataKey}
          stroke={color}
          strokeWidth={1.5}
          fill={`url(#fill-${dataKey})`}
        />
      </ReAreaChart>
    </ResponsiveContainer>
  );
}
