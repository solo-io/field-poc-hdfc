import styled from '@emotion/styled';
import { colors, radius, spacing } from '../../styles';

interface CardProps {
  padding?: keyof typeof spacing;
}

export const Card = styled.div<CardProps>`
  background: ${colors.cardBg};
  border: 1px solid ${colors.border};
  border-radius: ${radius.lg};
  padding: ${({ padding = 6 }) => spacing[padding as keyof typeof spacing]};
`;

export const CardHeader = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: ${spacing[4]};
`;

export const CardTitle = styled.h2`
  font-size: 18px;
  font-weight: 500;
  color: ${colors.foreground};
`;

export const CardDescription = styled.p`
  font-size: 14px;
  color: ${colors.mutedForeground};
  margin-top: ${spacing[1]};
`;

export const CardContent = styled.div``;

export const CardFooter = styled.div`
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: ${spacing[3]};
  margin-top: ${spacing[4]};
  padding-top: ${spacing[4]};
  border-top: 1px solid ${colors.border};
`;
