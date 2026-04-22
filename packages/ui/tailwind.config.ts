import type { Config } from 'tailwindcss';
import animate from 'tailwindcss-animate';

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: ['class', '.theme-dark'],
  theme: {
    extend: {
      fontFamily: {
        sans: [
          'Geist',
          'Inter',
          'system-ui',
          '-apple-system',
          'Segoe UI',
          'Roboto',
          'sans-serif',
        ],
        mono: [
          'Geist Mono',
          'JetBrains Mono',
          'ui-monospace',
          'Menlo',
          'monospace',
        ],
      },
      fontSize: {
        // Compact dashboard scale
        '2xs': ['10px', '14px'],
        xs: ['11px', '16px'],
        sm: ['12px', '18px'],
        base: ['13px', '20px'],
        md: ['14px', '22px'],
        lg: ['16px', '24px'],
        xl: ['18px', '26px'],
        '2xl': ['22px', '28px'],
        '3xl': ['28px', '34px'],
      },
      colors: {
        // Semantic tokens — bound to CSS vars in tokens.css
        surface: 'var(--bg-0)',
        panel: 'var(--bg-2)',
        'panel-alt': 'var(--bg-1)',
        elevated: 'var(--bg-3)',
        inset: 'var(--bg-inset)',
        border: 'var(--line)',
        'border-strong': 'var(--line-strong)',
        foreground: 'var(--fg-0)',
        'foreground-muted': 'var(--fg-2)',
        'foreground-subtle': 'var(--fg-3)',
        'foreground-faint': 'var(--fg-4)',

        accent: {
          DEFAULT: 'var(--accent)',
          soft: 'var(--accent-soft)',
          dim: 'var(--accent-dim)',
          fg: 'var(--accent-fg)',
        },
        muted: 'var(--bg-3)',
        danger: {
          DEFAULT: 'var(--err)',
          soft: 'var(--err-soft)',
        },
        warning: {
          DEFAULT: 'var(--warn)',
          soft: 'var(--warn-soft)',
        },
        success: {
          DEFAULT: 'var(--ok)',
          soft: 'var(--ok-soft)',
        },
        info: 'var(--info)',

        // novanas scale — coarse neutrals usable ad-hoc
        novanas: {
          50: 'oklch(0.98 0.004 250)',
          100: 'oklch(0.94 0.006 250)',
          200: 'oklch(0.86 0.008 250)',
          300: 'oklch(0.68 0.010 250)',
          400: 'oklch(0.52 0.010 250)',
          500: 'oklch(0.40 0.010 250)',
          600: 'oklch(0.30 0.010 250)',
          700: 'oklch(0.26 0.009 250)',
          800: 'oklch(0.22 0.008 250)',
          900: 'oklch(0.19 0.007 250)',
          950: 'oklch(0.16 0.006 250)',
        },
      },
      borderRadius: {
        xs: 'var(--r-xs)',
        sm: 'var(--r-sm)',
        md: 'var(--r-md)',
        lg: 'var(--r-lg)',
        xl: 'var(--r-xl)',
      },
      boxShadow: {
        'soft-sm': 'var(--shadow-sm)',
        'soft-md': 'var(--shadow-md)',
      },
      keyframes: {
        'accordion-down': {
          from: { height: '0' },
          to: { height: 'var(--radix-accordion-content-height)' },
        },
        'accordion-up': {
          from: { height: 'var(--radix-accordion-content-height)' },
          to: { height: '0' },
        },
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
      },
    },
  },
  plugins: [animate],
};

export default config;
