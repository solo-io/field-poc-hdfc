import { useState, useEffect, useCallback } from 'react';
import styled from '@emotion/styled';
import { spacing, colors } from '../../styles';
import { Modal } from '../../components/common/Modal';
import { Button } from '../../components/common/Button';
import { Input, Textarea, FormField } from '../../components/common/Input';
import { Select } from '../../components/common/Select';
import { BudgetDefinition, CreateBudgetRequest, EntityType, BudgetPeriod } from '../../api/types';
import { budgetsApi } from '../../api/budgets';
import { useAuth } from '../../contexts/AuthContext';

const Form = styled.form`
  display: flex;
  flex-direction: column;
  gap: ${spacing[4]};
`;

const Row = styled.div`
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: ${spacing[4]};
`;

const Footer = styled.div`
  display: flex;
  gap: ${spacing[3]};
  justify-content: flex-end;
`;

const ErrorText = styled.div`
  color: ${colors.error};
  font-size: 12px;
  margin-top: ${spacing[1]};
`;

const InfoText = styled.div`
  color: ${colors.mutedForeground};
  font-size: 12px;
  font-style: italic;
  margin-top: ${spacing[1]};
`;

interface BudgetFormProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: CreateBudgetRequest) => Promise<void>;
  editingBudget?: BudgetDefinition | null;
  availableBudgets?: BudgetDefinition[];
  loading?: boolean;
}

export function BudgetForm({
  open,
  onClose,
  onSubmit,
  editingBudget,
  availableBudgets = [],
  loading = false,
}: BudgetFormProps) {
  const { identity, permissions } = useAuth();
  const isOrgAdmin = permissions.isOrgAdmin;
  const isTeamMember = permissions.isTeamMember;

  const [formData, setFormData] = useState<CreateBudgetRequest>({
    entity_type: 'provider',
    name: '',
    match_expression: 'true',
    budget_amount_usd: 100,
    period: 'monthly',
    warning_threshold_pct: 80,
    isolated: true,
    allow_fallback: false,
    enabled: true,
    owner_org_id: identity?.org_id,
    owner_team_id: identity?.team_id,
  });

  const [celError, setCelError] = useState<string | null>(null);
  const [celValidating, setCelValidating] = useState(false);
  const [budgetAmountStr, setBudgetAmountStr] = useState('100');
  const [warningThresholdStr, setWarningThresholdStr] = useState('80');
  const [customPeriodStr, setCustomPeriodStr] = useState('');

  const validateCEL = useCallback(async (expression: string) => {
    if (!expression.trim()) {
      setCelError('Match expression is required');
      return;
    }

    setCelValidating(true);
    try {
      const result = await budgetsApi.validateCEL(expression);
      if (result.valid) {
        setCelError(null);
      } else {
        setCelError(result.error || 'Invalid CEL expression');
      }
    } catch {
      setCelError('Failed to validate expression');
    } finally {
      setCelValidating(false);
    }
  }, []);

  useEffect(() => {
    if (editingBudget) {
      setFormData({
        entity_type: editingBudget.entity_type,
        name: editingBudget.name,
        match_expression: editingBudget.match_expression,
        budget_amount_usd: editingBudget.budget_amount_usd,
        period: editingBudget.period,
        custom_period_seconds: editingBudget.custom_period_seconds,
        warning_threshold_pct: editingBudget.warning_threshold_pct,
        parent_id: editingBudget.parent_id,
        isolated: editingBudget.isolated,
        allow_fallback: editingBudget.allow_fallback,
        enabled: editingBudget.enabled,
        description: editingBudget.description,
        owner_org_id: editingBudget.owner_org_id,
        owner_team_id: editingBudget.owner_team_id,
      });
      setBudgetAmountStr(editingBudget.budget_amount_usd.toString());
      setWarningThresholdStr(editingBudget.warning_threshold_pct.toString());
      setCustomPeriodStr(editingBudget.custom_period_seconds?.toString() || '');
      setCelError(null);
    } else {
      setFormData({
        entity_type: 'provider',
        name: '',
        match_expression: 'true',
        budget_amount_usd: 100,
        period: 'monthly',
        warning_threshold_pct: 80,
        isolated: true,
        allow_fallback: false,
        enabled: true,
        owner_org_id: identity?.org_id,
        owner_team_id: identity?.team_id,
      });
      setBudgetAmountStr('100');
      setWarningThresholdStr('80');
      setCustomPeriodStr('');
      setCelError(null);
    }
  }, [editingBudget, open, identity]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onSubmit(formData);
  };

  const handleChange = <K extends keyof CreateBudgetRequest>(
    field: K,
    value: CreateBudgetRequest[K]
  ) => {
    setFormData(prev => ({ ...prev, [field]: value }));
    if (field === 'match_expression') {
      setCelError(null);
    }
    if (field === 'parent_id' && !value) {
      setFormData(prev => ({ ...prev, allow_fallback: false }));
    }
    if (field === 'entity_type' && (value === 'provider' || value === 'org')) {
      setFormData(prev => ({ ...prev, parent_id: undefined, allow_fallback: false }));
    }
  };

  const handleCELBlur = () => {
    if (formData.match_expression.trim()) {
      validateCEL(formData.match_expression);
    }
  };

  const isEditing = !!editingBudget;
  const title = isEditing ? 'Edit Budget' : 'Create Budget';

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      width="650px"
      footer={
        <Footer>
          <Button variant="secondary" onClick={onClose} disabled={loading}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading ? 'Saving...' : isEditing ? 'Save Changes' : 'Create'}
          </Button>
        </Footer>
      }
    >
      <Form onSubmit={handleSubmit}>
        <Row>
          <FormField
            label="Entity Type"
            tooltip="The type of entity this budget applies to. Provider budgets limit spend on a specific LLM provider. Team budgets limit spend for a group identified by request headers."
            fullWidth
          >
            <Select
              value={formData.entity_type}
              onChange={e => handleChange('entity_type', e.target.value as EntityType)}
              disabled={isEditing}
            >
              <option value="provider">Provider</option>
              {isOrgAdmin && <option value="org">Organization</option>}
              <option value="team">Team</option>
            </Select>
          </FormField>
          <FormField
            label="Name"
            tooltip="A unique identifier for this budget. Used for display and reference purposes."
            fullWidth
          >
            <Input
              value={formData.name}
              onChange={e => handleChange('name', e.target.value)}
              placeholder="e.g., openai, acme-corp, ml-platform"
              disabled={isEditing}
              required
            />
          </FormField>
        </Row>
        <Row>
          <FormField label="Budget Amount (USD)" fullWidth>
            <Input
              type="text"
              inputMode="decimal"
              value={budgetAmountStr}
              onChange={e => {
                setBudgetAmountStr(e.target.value);
                const num = parseFloat(e.target.value);
                if (!isNaN(num) && num >= 0) {
                  handleChange('budget_amount_usd', num);
                }
              }}
              onBlur={() => {
                const num = parseFloat(budgetAmountStr);
                if (isNaN(num) || num < 0) {
                  setBudgetAmountStr(formData.budget_amount_usd.toString());
                } else {
                  setBudgetAmountStr(num.toString());
                }
              }}
              placeholder="0.00"
              required
            />
          </FormField>
          <FormField
            label="Period"
            tooltip="How often the budget resets. Hourly, daily, weekly, and monthly periods align to calendar boundaries (start of hour, midnight UTC, Monday, 1st of month). Custom allows specifying an exact duration in seconds."
            fullWidth
          >
            <Select
              value={formData.period}
              onChange={e => handleChange('period', e.target.value as BudgetPeriod)}
            >
              <option value="hourly">Hourly</option>
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
              <option value="custom">Custom</option>
            </Select>
          </FormField>
        </Row>
        {formData.period === 'custom' && (
          <FormField label="Custom Period (seconds)" fullWidth>
            <Input
              type="text"
              inputMode="numeric"
              value={customPeriodStr}
              onChange={e => {
                setCustomPeriodStr(e.target.value);
                const num = parseInt(e.target.value);
                if (!isNaN(num) && num >= 1) {
                  handleChange('custom_period_seconds', num);
                } else if (e.target.value === '') {
                  handleChange('custom_period_seconds', undefined);
                }
              }}
              onBlur={() => {
                const num = parseInt(customPeriodStr);
                if (customPeriodStr !== '' && (isNaN(num) || num < 1)) {
                  setCustomPeriodStr(formData.custom_period_seconds?.toString() || '');
                }
              }}
              placeholder="e.g., 3600 for 1 hour"
            />
          </FormField>
        )}
        <FormField
          label="Match Expression (CEL)"
          tooltip='CEL expression that determines which requests this budget applies to. Use "true" to match all requests, or filter by headers (request.headers["x-team"]), path (request.path.startsWith("/openai")), or other request attributes.'
          fullWidth
        >
          <Textarea
            value={formData.match_expression}
            onChange={e => handleChange('match_expression', e.target.value)}
            onBlur={handleCELBlur}
            placeholder='e.g., "x-team" in request.headers && request.headers["x-team"] == "ml-platform"'
            required
          />
          {celValidating && (
            <ErrorText style={{ color: colors.mutedForeground }}>Validating...</ErrorText>
          )}
          {celError && !celValidating && <ErrorText>{celError}</ErrorText>}
        </FormField>
        <Row>
          <FormField
            label="Warning Threshold (%)"
            tooltip="Percentage of budget usage that triggers a warning. When usage exceeds this threshold, alerts are generated but requests are still allowed until 100% is reached."
            fullWidth
          >
            <Input
              type="text"
              inputMode="numeric"
              value={warningThresholdStr}
              onChange={e => {
                setWarningThresholdStr(e.target.value);
                const num = parseInt(e.target.value);
                if (!isNaN(num) && num >= 0 && num <= 100) {
                  handleChange('warning_threshold_pct', num);
                }
              }}
              onBlur={() => {
                const num = parseInt(warningThresholdStr);
                if (isNaN(num) || num < 0 || num > 100) {
                  setWarningThresholdStr((formData.warning_threshold_pct ?? 80).toString());
                } else {
                  setWarningThresholdStr(num.toString());
                }
              }}
              placeholder="0-100"
            />
          </FormField>
          <FormField
            label="Isolated"
            tooltip="When enabled, this budget is evaluated independently. When disabled, usage counts against both this budget AND any parent budgets in the hierarchy (e.g., team budget also counts against org budget)."
            fullWidth
          >
            <Select
              value={formData.isolated ? 'true' : 'false'}
              onChange={e => handleChange('isolated', e.target.value === 'true')}
            >
              <option value="true">Yes</option>
              <option value="false">No</option>
            </Select>
          </FormField>
        </Row>
        {formData.entity_type === 'team' && (
          <Row>
            <FormField
              label="Parent Budget (Organization)"
              tooltip="Optional parent organization budget. When set and Isolated is disabled, usage counts against both the team budget and the parent org budget."
              fullWidth
            >
              <Select
                value={formData.parent_id || ''}
                onChange={e => handleChange('parent_id', e.target.value || undefined)}
              >
                <option value="">None (standalone team budget)</option>
                {availableBudgets
                  .filter(b => b.id !== editingBudget?.id && b.entity_type === 'org')
                  .map(budget => (
                    <option key={budget.id} value={budget.id}>
                      {budget.name} - ${budget.budget_amount_usd}/{budget.period}
                    </option>
                  ))}
              </Select>
            </FormField>
            <FormField
              label="Allow Fallback"
              tooltip="When enabled and this team budget is exhausted, requests fall back to the parent org budget if it has remaining balance."
              fullWidth
            >
              <Select
                value={formData.allow_fallback ? 'true' : 'false'}
                onChange={e => handleChange('allow_fallback', e.target.value === 'true')}
                disabled={!formData.parent_id}
              >
                <option value="false">No (block when exhausted)</option>
                <option value="true">Yes (use org budget)</option>
              </Select>
            </FormField>
          </Row>
        )}
        <Row>
          <FormField
            label="Enabled"
            tooltip="When disabled, this budget is not enforced and requests bypass this budget's limits. All other settings are preserved and take effect when the budget is re-enabled."
            fullWidth
          >
            <Select
              value={formData.enabled ? 'true' : 'false'}
              onChange={e => handleChange('enabled', e.target.value === 'true')}
            >
              <option value="true">Yes (enforce budget)</option>
              <option value="false">No (bypass limits)</option>
            </Select>
            {!formData.enabled && (
              <InfoText>
                Budget is disabled. Isolated and Allow Fallback settings will take effect when
                enabled.
              </InfoText>
            )}
          </FormField>
          <FormField label="Description" fullWidth>
            <Input
              value={formData.description || ''}
              onChange={e => handleChange('description', e.target.value || undefined)}
              placeholder="Optional description"
            />
          </FormField>
        </Row>
        {(isOrgAdmin || isTeamMember) && (
          <Row>
            <FormField
              label="Owner Organization"
              tooltip="The organization that owns this budget. Budgets are automatically assigned to your organization."
              fullWidth
            >
              <Input value={formData.owner_org_id || ''} disabled placeholder="Your organization" />
              <InfoText>Automatically set to your organization</InfoText>
            </FormField>
            {isOrgAdmin ? (
              <FormField
                label="Assign to Team (Optional)"
                tooltip="Optionally assign this budget to a specific team within your organization. Leave empty for an org-level budget."
                fullWidth
              >
                <Input
                  value={formData.owner_team_id || ''}
                  onChange={e => handleChange('owner_team_id', e.target.value || undefined)}
                  placeholder="e.g., ml-platform, data-science"
                />
                <InfoText>Leave empty for organization-level budget</InfoText>
              </FormField>
            ) : (
              <FormField
                label="Team"
                tooltip="The team this budget belongs to. Defaults to your team."
                fullWidth
              >
                <Input
                  value={formData.owner_team_id || ''}
                  onChange={e => handleChange('owner_team_id', e.target.value || undefined)}
                  placeholder={identity?.team_id || 'e.g., ml-platform'}
                />
                <InfoText>
                  {formData.entity_type === 'team'
                    ? 'Create a budget for your team or another team in your org'
                    : 'Leave empty for provider-level budget'}
                </InfoText>
              </FormField>
            )}
          </Row>
        )}
      </Form>
    </Modal>
  );
}
