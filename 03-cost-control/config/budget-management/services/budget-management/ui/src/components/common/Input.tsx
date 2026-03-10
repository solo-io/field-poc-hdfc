import styled from '@emotion/styled';
import { colors, radius, spacing, fontSize } from '../../styles';

interface InputWrapperProps {
  fullWidth?: boolean;
}

export const InputWrapper = styled.div<InputWrapperProps>`
  display: flex;
  flex-direction: column;
  gap: ${spacing[1]};
  ${({ fullWidth }) => fullWidth && 'width: 100%;'}
`;

export const Label = styled.label`
  font-size: ${fontSize.sm};
  font-weight: 400;
  color: ${colors.mutedForeground};
`;

export const Input = styled.input`
  height: 40px;
  padding: 0 ${spacing[3]};
  background: ${colors.background};
  border: 1px solid ${colors.border};
  border-radius: ${radius.md};
  color: ${colors.foreground};
  font-size: ${fontSize.sm};
  transition: border-color 0.15s ease;

  &::placeholder {
    color: ${colors.dimForeground};
  }

  &:hover:not(:disabled) {
    border-color: ${colors.borderLight};
  }

  &:focus {
    outline: none;
    border-color: ${colors.primary};
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
`;

export const Textarea = styled.textarea`
  min-height: 100px;
  padding: ${spacing[3]};
  background: ${colors.background};
  border: 1px solid ${colors.border};
  border-radius: ${radius.md};
  color: ${colors.foreground};
  font-size: ${fontSize.sm};
  resize: vertical;
  transition: border-color 0.15s ease;

  &::placeholder {
    color: ${colors.dimForeground};
  }

  &:hover:not(:disabled) {
    border-color: ${colors.borderLight};
  }

  &:focus {
    outline: none;
    border-color: ${colors.primary};
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
`;

export const InputError = styled.span`
  font-size: ${fontSize.xs};
  color: ${colors.error};
`;

const LabelRow = styled.div`
  display: flex;
  align-items: center;
  gap: ${spacing[1]};
`;

const TooltipWrapper = styled.div`
  position: relative;
  display: inline-flex;
  align-items: center;
`;

const TooltipIcon = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: ${colors.border};
  color: ${colors.mutedForeground};
  font-size: 11px;
  font-weight: 600;
  cursor: help;

  &:hover + div {
    opacity: 1;
    visibility: visible;
  }
`;

const TooltipContent = styled.div`
  position: absolute;
  bottom: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
  padding: ${spacing[2]} ${spacing[3]};
  background: ${colors.foreground};
  color: ${colors.background};
  font-size: ${fontSize.xs};
  line-height: 1.4;
  border-radius: ${radius.md};
  white-space: normal;
  width: 250px;
  max-width: 300px;
  opacity: 0;
  visibility: hidden;
  transition:
    opacity 0.15s ease,
    visibility 0.15s ease;
  z-index: 1000;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);

  &::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border: 6px solid transparent;
    border-top-color: ${colors.foreground};
  }
`;

interface FormFieldProps {
  label: string;
  error?: string;
  tooltip?: string;
  children: React.ReactNode;
  fullWidth?: boolean;
}

export function FormField({ label, error, tooltip, children, fullWidth }: FormFieldProps) {
  return (
    <InputWrapper fullWidth={fullWidth}>
      <LabelRow>
        <Label>{label}</Label>
        {tooltip && (
          <TooltipWrapper>
            <TooltipIcon>?</TooltipIcon>
            <TooltipContent>{tooltip}</TooltipContent>
          </TooltipWrapper>
        )}
      </LabelRow>
      {children}
      {error && <InputError>{error}</InputError>}
    </InputWrapper>
  );
}
