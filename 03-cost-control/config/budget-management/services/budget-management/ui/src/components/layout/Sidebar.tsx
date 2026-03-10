import { NavLink } from 'react-router-dom';
import styled from '@emotion/styled';
import { colors, radius, spacing, fontSize } from '../../styles';
import { useAuth } from '../../contexts/AuthContext';

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
  }
`;

const UserSection = styled.div`
  padding: ${spacing[4]};
  border-top: 1px solid ${colors.sidebarBorder};
`;

const UserInfo = styled.div`
  display: flex;
  align-items: center;
  gap: ${spacing[3]};
  padding: ${spacing[2]} ${spacing[3]};
  border-radius: ${radius.md};
  background: ${colors.sidebarItemHover};
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
  color: ${colors.foreground};
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

const LogoutButton = styled.button`
  display: flex;
  align-items: center;
  justify-content: center;
  gap: ${spacing[2]};
  width: 100%;
  margin-top: ${spacing[2]};
  padding: ${spacing[2]} ${spacing[3]};
  border-radius: ${radius.md};
  font-size: ${fontSize.sm};
  color: ${colors.mutedForeground};
  background: transparent;
  border: 1px solid ${colors.sidebarBorder};
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    background: ${colors.errorBg};
    border-color: ${colors.error};
    color: ${colors.error};
  }

  svg {
    width: 16px;
    height: 16px;
  }
`;

const NotAuthenticated = styled.div`
  padding: ${spacing[2]} ${spacing[3]};
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  text-align: center;
`;

export function Sidebar() {
  const { identity, loading } = useAuth();

  const handleLogout = () => {
    window.location.href = '/logout';
  };

  return (
    <SidebarContainer>
      <Logo>
        <LogoText>Budget Management</LogoText>
        <LogoSubtext>Management Console</LogoSubtext>
      </Logo>
      <Nav>
        <NavSection>
          <NavSectionTitle>Configuration</NavSectionTitle>
          <NavItem to="/model-costs">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 2L2 7l10 5 10-5-10-5z" />
              <path d="M2 17l10 5 10-5" />
              <path d="M2 12l10 5 10-5" />
            </svg>
            Model Costs
          </NavItem>
          <NavItem to="/budgets">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
              <path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
            </svg>
            Budgets
          </NavItem>
        </NavSection>
      </Nav>
      <UserSection>
        {loading ? (
          <NotAuthenticated>Loading...</NotAuthenticated>
        ) : identity?.authenticated ? (
          <>
            <UserInfo>
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
            <LogoutButton onClick={handleLogout}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                <polyline points="16 17 21 12 16 7" />
                <line x1="21" y1="12" x2="9" y2="12" />
              </svg>
              Logout
            </LogoutButton>
          </>
        ) : (
          <NotAuthenticated>Not authenticated</NotAuthenticated>
        )}
      </UserSection>
    </SidebarContainer>
  );
}
