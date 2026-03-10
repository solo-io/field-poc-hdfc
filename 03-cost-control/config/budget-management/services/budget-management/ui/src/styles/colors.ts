export const colors = {
  // Page backgrounds
  background: '#0D0E15',
  cardBg: '#11101C',
  surfaceBg: '#12101C',

  // Text colors
  foreground: '#FAFAFA',
  mutedForeground: '#A1A1AA',
  dimForeground: '#575961',

  // Borders
  border: '#27242E',
  borderLight: '#3B3C46',
  borderDark: '#1E1E25',

  // Primary action (purple)
  primary: '#6844FF',
  primaryHover: '#7A5AFF',
  primaryActive: '#5A3AE6',

  // Secondary (gray)
  secondary: '#27272A',
  secondaryHover: '#34343B',
  secondaryActive: '#3F3F46',

  // Semantic colors
  error: '#EF4444',
  errorBg: '#450A0A',
  errorBorder: '#7F1D1D',

  success: '#22C55E',
  successBg: '#052E16',
  successBorder: '#14532D',

  warning: '#F97316',
  warningBg: '#431407',
  warningBorder: '#7C2D12',

  info: '#3B82F6',
  infoBg: '#172554',
  infoBorder: '#1E3A8A',

  // Interactive states
  hoverBg: '#1C1C26',
  activeBg: '#262736',

  // Table
  tableRowHover: '#1C1C26',

  // Sidebar
  sidebarBg: '#11131B',
  sidebarBorder: '#27242E',
  sidebarItemHover: '#1C1C26',
  sidebarItemActive: '#27242E',
} as const;

export type ColorKey = keyof typeof colors;
