import { useState, useRef, useEffect } from 'react';
import { NavLink } from 'react-router-dom';
import styled from '@emotion/styled';
import { colors, radius, spacing, fontSize } from '../../styles';
import { useAuth } from '../../contexts/AuthContext';
import { useTheme } from '../../contexts/ThemeContext';
import { approvalsApi } from '../../api/approvals';
import { budgetsApi } from '../../api/budgets';
import { rateLimitsApi } from '../../api/rate-limits';
import { useSWRApi, CacheKeys } from '../../hooks/useSWR';
import { config } from '../../config';

const SidebarContainer = styled.aside`
  width: 240px;
  min-width: 240px;
  background: ${colors.sidebarBg};
  border-right: 1px solid ${colors.sidebarBorder};
  display: flex;
  flex-direction: column;
  height: 100vh;
  position: sticky;
  top: 0;
  /* For absolute positioning of theme toggle */
  position: relative;
`;

const Logo = styled.div`
  padding: ${spacing[5]} ${spacing[4]};
  border-bottom: 1px solid ${colors.sidebarBorder};
`;

const LogoText = styled.h1`
  font-size: ${fontSize.lg};
  font-weight: 600;
  color: ${colors.foreground};
`;

const LogoSubtext = styled.span`
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
`;

const SummaryStats = styled.div`
  padding: ${spacing[3]} ${spacing[4]};
  border-top: 1px solid ${colors.sidebarBorder};
  margin-top: auto;
`;

const SummaryTitle = styled.div`
  font-size: 10px;
  font-weight: 500;
  color: ${colors.mutedForeground};
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: ${spacing[2]};
`;

const StatsRow = styled.div`
  display: flex;
  gap: ${spacing[2]};
`;

const StatBox = styled.div`
  flex: 1;
  text-align: center;
  padding: ${spacing[2]};
  border-radius: ${radius.md};
  background: ${colors.sidebarItemHover};
`;

const StatValue = styled.div`
  font-size: ${fontSize.lg};
  font-weight: 600;
  color: ${colors.foreground};
`;

const StatLabel = styled.div`
  font-size: 10px;
  color: ${colors.mutedForeground};
  text-transform: uppercase;
  letter-spacing: 0.05em;
`;

const Nav = styled.nav`
  padding: ${spacing[4]};
  flex: 1;
`;

const NavSection = styled.div`
  margin-bottom: ${spacing[4]};
`;

const NavSectionTitle = styled.h3`
  font-size: ${fontSize.xs};
  font-weight: 500;
  color: ${colors.mutedForeground};
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: 0 ${spacing[3]};
  margin-bottom: ${spacing[2]};
`;

const NavItem = styled(NavLink)`
  display: flex;
  align-items: center;
  gap: ${spacing[3]};
  padding: ${spacing[2]} ${spacing[3]};
  border-radius: ${radius.md};
  font-size: ${fontSize.sm};
  color: ${colors.mutedForeground};
  transition: all 0.15s ease;

  &:hover {
    background: ${colors.sidebarItemHover};
    color: ${colors.foreground};
  }

  &.active {
    background: ${colors.sidebarItemActive};
    color: ${colors.foreground};
  }

  svg {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
  }
`;

const NavBadge = styled.span`
  background: ${colors.error};
  color: white;
  font-size: 11px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: ${radius.full};
  margin-left: auto;
  min-width: 18px;
  text-align: center;
`;

const UserSection = styled.div`
  padding: ${spacing[4]};
  border-top: 1px solid ${colors.sidebarBorder};
  position: relative;
`;

const UserInfo = styled.button`
  display: flex;
  align-items: center;
  gap: ${spacing[3]};
  padding: ${spacing[2]} ${spacing[3]};
  border-radius: ${radius.md};
  background: ${colors.sidebarItemHover};
  width: 100%;
  border: none;
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    background: ${colors.sidebarItemActive};
  }
`;

const UserDropdown = styled.div<{ $open: boolean }>`
  position: absolute;
  top: 50%;
  left: 100%;
  transform: translateY(-50%) translateX(${({ $open }) => ($open ? '0' : '-8px')});
  margin-left: ${spacing[1]};
  background: ${colors.cardBg};
  border: 1px solid ${colors.border};
  border-radius: ${radius.md};
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  opacity: ${({ $open }) => ($open ? 1 : 0)};
  visibility: ${({ $open }) => ($open ? 'visible' : 'hidden')};
  transition: all 0.15s ease;
  overflow: hidden;
  white-space: nowrap;
`;

const DropdownItem = styled.button`
  display: flex;
  align-items: center;
  gap: ${spacing[2]};
  width: 100%;
  padding: ${spacing[3]} ${spacing[4]};
  background: transparent;
  border: none;
  font-size: ${fontSize.sm};
  color: ${colors.mutedForeground};
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    background: ${colors.errorBg};
    color: ${colors.error};
  }

  svg {
    width: 16px;
    height: 16px;
  }
`;

const UserAvatar = styled.div`
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: ${colors.primary};
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: ${fontSize.sm};
  font-weight: 600;
  color: #ffffff;
`;

const UserDetails = styled.div`
  flex: 1;
  min-width: 0;
`;

const UserEmail = styled.div`
  font-size: ${fontSize.sm};
  color: ${colors.foreground};
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
`;

const UserRole = styled.div`
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
`;

const NotAuthenticated = styled.div`
  padding: ${spacing[2]} ${spacing[3]};
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  text-align: center;
`;

const ThemeToggleButton = styled.button`
  position: absolute;
  top: ${spacing[2]};
  right: ${spacing[2]};
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: ${radius.md};
  color: ${colors.mutedForeground};
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all 0.15s ease;
  opacity: 0.6;

  &:hover {
    opacity: 1;
    color: ${colors.foreground};
  }

  svg {
    width: 14px;
    height: 14px;
  }
`;

function formatCompact(num: number | null): string {
  if (num === null) return '-';
  if (num < 1000) return num.toString();
  if (num < 10000) return (num / 1000).toFixed(1).replace(/\.0$/, '') + 'K';
  if (num < 1000000) return Math.round(num / 1000) + 'K';
  return (num / 1000000).toFixed(1).replace(/\.0$/, '') + 'M';
}

// Fetcher for sidebar stats - combines all counts in one request
async function fetchSidebarStats() {
  const promises: Promise<any>[] = [
    approvalsApi.count(),
    budgetsApi.list(1, 1, { enabledOnly: true }),
  ];
  if (config.enableRateLimits) {
    promises.push(rateLimitsApi.count());
    promises.push(rateLimitsApi.list(1, 1, { enabledOnly: true }));
  }
  const [budgetApprovals, budgets, rateLimitApprovals, rateLimits] = await Promise.all(promises);
  return {
    pendingCount:
      (budgetApprovals.count ?? 0) +
      (config.enableRateLimits ? (rateLimitApprovals?.count ?? 0) : 0),
    budgetCount: budgets.pagination?.total_count ?? 0,
    rateLimitCount: config.enableRateLimits ? (rateLimits?.pagination?.total_count ?? 0) : null,
  };
}

export function Sidebar() {
  const { identity, loading, permissions } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const userMenuRef = useRef<HTMLDivElement>(null);

  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (userMenuRef.current && !userMenuRef.current.contains(event.target as Node)) {
        setUserMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Use SWR for sidebar stats with 30 second refresh interval
  const { data: stats } = useSWRApi(CacheKeys.sidebarStats, fetchSidebarStats, {
    refreshInterval: 30000,
    revalidateOnFocus: true,
  });

  const pendingCount = stats?.pendingCount ?? 0;
  const budgetCount = stats?.budgetCount ?? null;
  const rateLimitCount = stats?.rateLimitCount ?? null;

  const handleLogout = () => {
    window.location.href = '/logout';
  };

  return (
    <SidebarContainer>
      <ThemeToggleButton
        onClick={toggleTheme}
        title={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
      >
        {theme === 'dark' ? (
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
          </svg>
        ) : (
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="12" cy="12" r="5" />
            <line x1="12" y1="1" x2="12" y2="3" />
            <line x1="12" y1="21" x2="12" y2="23" />
            <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
            <line x1="1" y1="12" x2="3" y2="12" />
            <line x1="21" y1="12" x2="23" y2="12" />
            <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
            <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
          </svg>
        )}
      </ThemeToggleButton>
      <Logo>
        <LogoText>Quota Management</LogoText>
        <LogoSubtext>Management Console</LogoSubtext>
      </Logo>
      <Nav>
        <NavSection>
          <NavSectionTitle>Quota Management</NavSectionTitle>
          <NavItem to="/budgets">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
              <path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
            </svg>
            Budgets
          </NavItem>
          {config.enableRateLimits && (
            <NavItem to="/rate-limits">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
              </svg>
              Rate Limits
            </NavItem>
          )}
          {permissions.isOrgAdmin && (
            <NavItem to="/approvals">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                <polyline points="22 4 12 14.01 9 11.01" />
              </svg>
              Approvals
              {pendingCount > 0 && <NavBadge>{pendingCount}</NavBadge>}
            </NavItem>
          )}
          <NavItem to="/audit">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
              <rect x="8" y="2" width="8" height="4" rx="1" ry="1" />
              <line x1="8" y1="12" x2="16" y2="12" />
              <line x1="8" y1="16" x2="12" y2="16" />
            </svg>
            Audit
          </NavItem>
        </NavSection>
        <NavSection>
          <NavSectionTitle>Settings</NavSectionTitle>
          <NavItem to="/model-costs">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 2L2 7l10 5 10-5-10-5z" />
              <path d="M2 17l10 5 10-5" />
              <path d="M2 12l10 5 10-5" />
            </svg>
            Model Costs
          </NavItem>
        </NavSection>
      </Nav>
      <SummaryStats>
        <SummaryTitle>At a Glance</SummaryTitle>
        <StatsRow>
          <StatBox>
            <StatValue>{formatCompact(budgetCount)}</StatValue>
            <StatLabel>Budgets</StatLabel>
          </StatBox>
          {config.enableRateLimits && (
            <StatBox>
              <StatValue>{formatCompact(rateLimitCount)}</StatValue>
              <StatLabel>Limits</StatLabel>
            </StatBox>
          )}
          {permissions.isOrgAdmin && (
            <StatBox>
              <StatValue>{formatCompact(pendingCount)}</StatValue>
              <StatLabel>Pending</StatLabel>
            </StatBox>
          )}
        </StatsRow>
      </SummaryStats>
      <UserSection ref={userMenuRef}>
        {loading ? (
          <NotAuthenticated>Loading...</NotAuthenticated>
        ) : identity?.authenticated ? (
          <>
            <UserDropdown $open={userMenuOpen}>
              <DropdownItem onClick={handleLogout}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                  <polyline points="16 17 21 12 16 7" />
                  <line x1="21" y1="12" x2="9" y2="12" />
                </svg>
                Logout
              </DropdownItem>
            </UserDropdown>
            <UserInfo onClick={() => setUserMenuOpen(!userMenuOpen)}>
              <UserAvatar>{identity.email ? identity.email[0].toUpperCase() : 'U'}</UserAvatar>
              <UserDetails>
                <UserEmail>{identity.email || identity.subject || 'User'}</UserEmail>
                <UserRole>
                  {identity.is_org
                    ? `Admin • ${identity.org_id}`
                    : identity.team_id
                      ? `Member • ${identity.team_id}`
                      : 'Guest'}
                </UserRole>
              </UserDetails>
            </UserInfo>
          </>
        ) : (
          <NotAuthenticated>Not authenticated</NotAuthenticated>
        )}
      </UserSection>
    </SidebarContainer>
  );
}
