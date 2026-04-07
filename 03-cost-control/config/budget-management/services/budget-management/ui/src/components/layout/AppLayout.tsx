import styled from '@emotion/styled';
import { colors, spacing } from '../../styles';
import { Sidebar } from './Sidebar';
import { Footer } from './Footer';

const LayoutContainer = styled.div`
  display: flex;
  min-height: 100vh;
  width: 100%;
`;

const MainContent = styled.main`
  flex: 1;
  display: flex;
  flex-direction: column;
  background: ${colors.background};
  overflow-x: hidden;
  min-height: 100vh;
`;

const ContentArea = styled.div`
  flex: 1;
  padding: ${spacing[8]};
`;

interface AppLayoutProps {
  children: React.ReactNode;
}

export function AppLayout({ children }: AppLayoutProps) {
  return (
    <LayoutContainer>
      <Sidebar />
      <MainContent>
        <ContentArea>{children}</ContentArea>
        <Footer />
      </MainContent>
    </LayoutContainer>
  );
}
