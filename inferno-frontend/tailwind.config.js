/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],

  // Dark is signalled two ways while the redesign lands.
  //
  //   .dark                  what the 8,955 existing dark: utilities read
  //   [data-theme="dark"]    what June's token layer reads
  //
  // June components never write a dark: utility -- [data-theme="dark"]
  // redefines the custom properties, so var(--card) is already correct in both
  // themes. This variant exists only so unconverted Tailwind screens keep
  // responding to the theme while they are still on screen.
  //
  // main.ts mirrors .dark onto [data-theme], so both are always present.
  // Delete the .dark half of this, the mirror, and the class itself once the
  // last Tailwind component is gone.
  darkMode: [
    'variant',
    ['&:where(.dark, .dark *)', '&:where([data-theme="dark"], [data-theme="dark"] *)']
  ],

  theme: {
    extend: {
      colors: {
        // 主色调 - Teal/Cyan 青色系
        primary: {
          50: '#f0fdfa',
          100: '#ccfbf1',
          200: '#99f6e4',
          300: '#5eead4',
          400: '#2dd4bf',
          500: '#14b8a6',
          600: '#0d9488',
          700: '#0f766e',
          800: '#115e59',
          900: '#134e4a',
          950: '#042f2e'
        },
        // 辅助色 - 深蓝灰
        accent: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617'
        },
        // 深色模式背景
        dark: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      // June ground rule 9: cards have no shadow, just a 1px --border-subtle.
      // Shadows in June come from --shadow-* tokens and are pure elevation
      // that never carries a ring. Nothing here is used by a June component.
      //
      // glow / glow-lg / inner-glow are deleted: zero @apply references, so
      // removal is safe. The rest survive only because style.css @applies them
      // (shadow-glass x2, shadow-card, shadow-card-hover) and an @apply of an
      // undefined utility is a build error. They die with their last consumer.
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)'
      },

      // backgroundImage removed entirely. June ground rule 7: no gradients, no
      // glass, no mesh. style.css only ever used core `bg-gradient-to-r`, which
      // Tailwind provides, so nothing here was load-bearing. Template
      // references to the deleted names become inert classes, not build errors.

      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out'
        // pulse-slow, shimmer and glow deleted. Zero references anywhere.
        // June: skeletons are flat static --surface-subtle with no sweep and no
        // pulse (ground rule: "No gradients, no glass, no glow, no mesh, no
        // shimmer-on-skeleton"), and durations are 100/160/240ms only.
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        }
        // shimmer and glow keyframes deleted with their animations.
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
