import styled from '@emotion/styled';
import { colors, spacing } from '../../styles';
import { Sidebar } from './Sidebar';

const LayoutContainer = styled.div`
  display: flex;
  min-height: 100vh;
  width: 100%;
`;

const MainContent = styled.main`
  flex: 1;
  padding: ${spacing[8]};
  background: ${colors.background};
  overflow-x: hidden;
`;

interface AppLayoutProps {
  children: React.ReactNode;
}

export function AppLayout({ children }: AppLayoutProps) {
  return (
    <LayoutContainer>
      <Sidebar />
      <MainContent>{children}</MainContent>
    </LayoutContainer>
  );
}
