// CSS variable-based colors for theming
export const colors = {
  // Page backgrounds
  background: 'var(--color-background)',
  cardBg: 'var(--color-card-bg)',
  surfaceBg: 'var(--color-surface-bg)',

  // Text colors
  foreground: 'var(--color-foreground)',
  mutedForeground: 'var(--color-muted-foreground)',
  dimForeground: 'var(--color-dim-foreground)',

  // Borders
  border: 'var(--color-border)',
  borderLight: 'var(--color-border-light)',
  borderDark: 'var(--color-border-dark)',

  // Primary action (purple)
  primary: '#6844FF',
  primaryHover: '#7A5AFF',
  primaryActive: '#5A3AE6',

  // Secondary (gray)
  secondary: 'var(--color-secondary)',
  secondaryHover: 'var(--color-secondary-hover)',
  secondaryActive: 'var(--color-secondary-active)',

  // Semantic colors
  error: '#EF4444',
  errorBg: 'var(--color-error-bg)',
  errorBorder: 'var(--color-error-border)',

  success: '#22C55E',
  successBg: 'var(--color-success-bg)',
  successBorder: 'var(--color-success-border)',

  warning: '#F97316',
  warningBg: 'var(--color-warning-bg)',
  warningBorder: 'var(--color-warning-border)',

  info: '#3B82F6',
  infoBg: 'var(--color-info-bg)',
  infoBorder: 'var(--color-info-border)',

  // Interactive states
  hoverBg: 'var(--color-hover-bg)',
  activeBg: 'var(--color-active-bg)',

  // Table
  tableRowHover: 'var(--color-table-row-hover)',

  // Sidebar
  sidebarBg: 'var(--color-sidebar-bg)',
  sidebarBorder: 'var(--color-sidebar-border)',
  sidebarItemHover: 'var(--color-sidebar-item-hover)',
  sidebarItemActive: 'var(--color-sidebar-item-active)',
} as const;

export type ColorKey = keyof typeof colors;

// Dark theme values
export const darkTheme = {
  '--color-background': '#0D0E15',
  '--color-card-bg': '#11101C',
  '--color-surface-bg': '#12101C',
  '--color-foreground': '#FAFAFA',
  '--color-muted-foreground': '#A1A1AA',
  '--color-dim-foreground': '#575961',
  '--color-border': '#27242E',
  '--color-border-light': '#3B3C46',
  '--color-border-dark': '#1E1E25',
  '--color-secondary': '#27272A',
  '--color-secondary-hover': '#34343B',
  '--color-secondary-active': '#3F3F46',
  '--color-error-bg': '#450A0A',
  '--color-error-border': '#7F1D1D',
  '--color-success-bg': '#052E16',
  '--color-success-border': '#14532D',
  '--color-warning-bg': '#431407',
  '--color-warning-border': '#7C2D12',
  '--color-info-bg': '#172554',
  '--color-info-border': '#1E3A8A',
  '--color-hover-bg': '#1C1C26',
  '--color-active-bg': '#262736',
  '--color-table-row-hover': '#1C1C26',
  '--color-sidebar-bg': '#11131B',
  '--color-sidebar-border': '#27242E',
  '--color-sidebar-item-hover': '#1C1C26',
  '--color-sidebar-item-active': '#27242E',
  '--color-scheme': 'dark',
  '--color-tooltip-bg': '#d4d4d8',
  '--color-tooltip-text': '#18181b',
};

// Light theme values
export const lightTheme = {
  '--color-background': '#F8FAFC',
  '--color-card-bg': '#FFFFFF',
  '--color-surface-bg': '#F1F5F9',
  '--color-foreground': '#0F172A',
  '--color-muted-foreground': '#64748B',
  '--color-dim-foreground': '#94A3B8',
  '--color-border': '#E2E8F0',
  '--color-border-light': '#CBD5E1',
  '--color-border-dark': '#E2E8F0',
  '--color-secondary': '#E2E8F0',
  '--color-secondary-hover': '#CBD5E1',
  '--color-secondary-active': '#94A3B8',
  '--color-error-bg': '#FEF2F2',
  '--color-error-border': '#FECACA',
  '--color-success-bg': '#F0FDF4',
  '--color-success-border': '#BBF7D0',
  '--color-warning-bg': '#FFFBEB',
  '--color-warning-border': '#FED7AA',
  '--color-info-bg': '#EFF6FF',
  '--color-info-border': '#BFDBFE',
  '--color-hover-bg': '#F1F5F9',
  '--color-active-bg': '#E2E8F0',
  '--color-table-row-hover': '#F1F5F9',
  '--color-sidebar-bg': '#FFFFFF',
  '--color-sidebar-border': '#E2E8F0',
  '--color-sidebar-item-hover': '#F1F5F9',
  '--color-sidebar-item-active': '#E2E8F0',
  '--color-scheme': 'light',
  '--color-tooltip-bg': '#FFFFFF',
  '--color-tooltip-text': '#18181b',
};
