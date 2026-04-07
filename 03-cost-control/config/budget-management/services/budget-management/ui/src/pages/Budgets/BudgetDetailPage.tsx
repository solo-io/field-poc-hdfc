import { useCallback, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import styled from '@emotion/styled';
import toast from 'react-hot-toast';
import { spacing, colors, fontSize, radius } from '../../styles';
import { PageHeader } from '../../components/layout/PageHeader';
import { Button } from '../../components/common/Button';
import { Card, CardTitle } from '../../components/common/Card';
import { Badge } from '../../components/common/Badge';
import { ProgressBar } from '../../components/common/ProgressBar';
import {
  TableContainer,
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableHeader,
  TableCell,
  EmptyState,
  EmptyStateText,
} from '../../components/common/Table';
import { ConfirmDialog } from '../../components/common/ConfirmDialog';
import { Loading } from '../../components/common/Spinner';
import { useMutation } from '../../hooks/useApi';
import { useSWRApi, CacheKeys, invalidateKey } from '../../hooks/useSWR';
import { budgetsApi } from '../../api/budgets';
import { ApiClientError } from '../../api/client';
import { useAuth } from '../../contexts/AuthContext';

const Container = styled.div``;

const BackButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: ${spacing[2]};
  color: ${colors.mutedForeground};
  font-size: ${fontSize.sm};
  margin-bottom: ${spacing[4]};
  transition: color 0.15s ease;

  &:hover {
    color: ${colors.foreground};
  }

  svg {
    width: 16px;
    height: 16px;
  }
`;

const Grid = styled.div`
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: ${spacing[4]};
  margin-bottom: ${spacing[6]};
`;

const StatCard = styled.div`
  background: ${colors.cardBg};
  border: 1px solid ${colors.border};
  border-radius: ${radius.lg};
  padding: ${spacing[4]};
`;

const StatLabel = styled.div`
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  margin-bottom: ${spacing[1]};
`;

const StatValue = styled.div`
  font-size: ${fontSize.xl};
  font-weight: 600;
  color: ${colors.foreground};
`;

const SummaryCard = styled(Card)`
  margin-bottom: ${spacing[6]};
`;

const SummaryGrid = styled.div`
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: ${spacing[4]};
`;

const SummaryItem = styled.div``;

const SummaryLabel = styled.div`
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  margin-bottom: ${spacing[1]};
`;

const SummaryValue = styled.div`
  font-size: ${fontSize.sm};
  color: ${colors.foreground};
`;

const ProgressSection = styled.div`
  margin-top: ${spacing[4]};
  padding-top: ${spacing[4]};
  border-top: 1px solid ${colors.border};
`;

const ProgressLabel = styled.div`
  display: flex;
  justify-content: space-between;
  margin-bottom: ${spacing[2]};
`;

const UsageSection = styled.div`
  margin-top: ${spacing[6]};
`;

const SectionHeader = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: ${spacing[4]};
`;

const SectionTitle = styled.h2`
  font-size: ${fontSize.lg};
  font-weight: 500;
  color: ${colors.foreground};
`;

function formatCurrency(amount: number): string {
  return `$${amount.toFixed(6)}`;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function formatNumber(num: number): string {
  return num.toLocaleString();
}

export function BudgetDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { permissions, identity } = useAuth();
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [showCascadeConfirm, setShowCascadeConfirm] = useState(false);
  const [cascadeCount, setCascadeCount] = useState(0);

  // Use SWR for budget detail with background refresh
  const {
    data: budget,
    loading: budgetLoading,
    refresh: refreshBudget,
  } = useSWRApi(id ? CacheKeys.budgetDetail(id) : null, () => budgetsApi.get(id!), {
    refreshInterval: 30000,
    skip: !id,
  });

  // Use SWR for usage records with background refresh
  const {
    data: usageRecords,
    loading: usageLoading,
    refresh: refreshUsage,
  } = useSWRApi(
    id ? CacheKeys.budgetUsage(id) : null,
    () => budgetsApi.getUsage(id!, undefined, 100),
    { refreshInterval: 30000, skip: !id }
  );

  const resetMutation = useMutation(budgetsApi.reset);
  const updateMutation = useMutation(
    useCallback(
      (budgetId: string, data: { enabled: boolean; version?: number }) =>
        budgetsApi.update(budgetId, data),
      []
    )
  );

  const handleRefresh = useCallback(() => {
    refreshBudget();
    refreshUsage();
    invalidateKey(CacheKeys.sidebarStats);
  }, [refreshBudget, refreshUsage]);

  const handleReset = async () => {
    if (!id) return;
    try {
      await resetMutation.execute(id);
      toast.success('Budget reset successfully');
      setResetDialogOpen(false);
      handleRefresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'An error occurred';
      toast.error(message);
    }
  };

  const handleToggleEnabled = async (enabled: boolean) => {
    if (!id || !budget) return;
    try {
      await updateMutation.execute(id, {
        enabled,
        version: budget.version,
      });
      toast.success(enabled ? 'Budget enabled' : 'Budget disabled');
      handleRefresh();
    } catch (error) {
      if (error instanceof ApiClientError && error.isConflict) {
        toast.error('This record was modified. Please refresh and try again.');
        handleRefresh();
      } else {
        const message = error instanceof Error ? error.message : 'An error occurred';
        toast.error(message);
      }
    }
  };

  const handleDisableClick = async () => {
    // Only check cascade for org entity types when current user is org-admin
    if (budget && budget.entity_type === 'org' && identity?.is_org) {
      try {
        const children = await budgetsApi.getChildren(budget.id);
        const nonIsolatedEnabled = children.filter(c => !c.isolated && c.enabled);
        if (nonIsolatedEnabled.length > 0) {
          setCascadeCount(nonIsolatedEnabled.length);
          setShowCascadeConfirm(true);
          return;
        }
      } catch (err) {
        console.error('Failed to check children:', err);
      }
    }
    // No cascade needed, disable directly
    await handleToggleEnabled(false);
  };

  if (budgetLoading) {
    return (
      <Container>
        <Loading />
      </Container>
    );
  }

  if (!budget) {
    return (
      <Container>
        <EmptyState>
          <EmptyStateText>Budget not found</EmptyStateText>
          <Button onClick={() => navigate('/budgets')}>Back to Budgets</Button>
        </EmptyState>
      </Container>
    );
  }

  const usagePercent =
    budget.budget_amount_usd > 0 ? (budget.current_usage_usd / budget.budget_amount_usd) * 100 : 0;

  return (
    <Container>
      <BackButton onClick={() => navigate('/budgets')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M19 12H5M12 19l-7-7 7-7" />
        </svg>
        Back to Budgets
      </BackButton>

      <PageHeader
        title={budget.name}
        description={`${budget.entity_type} budget - ${budget.period} period${!budget.enabled ? ' (Disabled)' : ''}`}
      >
        <Button variant="secondary" onClick={handleRefresh}>
          Refresh
        </Button>
        {permissions.canToggleBudgetEnabled && budget.can_enable !== false && (
          <Button
            variant={budget.enabled ? 'secondary' : 'primary'}
            onClick={budget.enabled ? handleDisableClick : () => handleToggleEnabled(true)}
            disabled={updateMutation.loading}
          >
            {budget.enabled ? 'Disable' : 'Enable'}
          </Button>
        )}
        {permissions.canEditBudget(budget) && (
          <Button variant="danger" onClick={() => setResetDialogOpen(true)}>
            Reset Budget
          </Button>
        )}
      </PageHeader>

      <Grid>
        <StatCard>
          <StatLabel>Budget Amount</StatLabel>
          <StatValue>{formatCurrency(budget.budget_amount_usd)}</StatValue>
        </StatCard>
        <StatCard>
          <StatLabel>Current Usage</StatLabel>
          <StatValue>{formatCurrency(budget.current_usage_usd)}</StatValue>
        </StatCard>
        <StatCard>
          <StatLabel>Remaining</StatLabel>
          <StatValue>{formatCurrency(budget.remaining_usd)}</StatValue>
        </StatCard>
        <StatCard>
          <StatLabel>Status</StatLabel>
          <StatValue>
            {budget.enabled ? (
              <Badge
                variant={
                  usagePercent >= 100
                    ? 'error'
                    : usagePercent >= budget.warning_threshold_pct
                      ? 'warning'
                      : 'success'
                }
              >
                {usagePercent >= 100
                  ? 'Exceeded'
                  : usagePercent >= budget.warning_threshold_pct
                    ? 'Warning'
                    : 'Active'}
              </Badge>
            ) : (
              <Badge variant="default">Disabled</Badge>
            )}
          </StatValue>
        </StatCard>
      </Grid>

      <SummaryCard>
        <CardTitle>Budget Details</CardTitle>
        <SummaryGrid>
          <SummaryItem>
            <SummaryLabel>Match Expression</SummaryLabel>
            <SummaryValue style={{ fontFamily: 'monospace' }}>
              {budget.match_expression}
            </SummaryValue>
          </SummaryItem>
          <SummaryItem>
            <SummaryLabel>Warning Threshold</SummaryLabel>
            <SummaryValue>{budget.warning_threshold_pct}%</SummaryValue>
          </SummaryItem>
          <SummaryItem>
            <SummaryLabel>Period Start</SummaryLabel>
            <SummaryValue>{formatDate(budget.current_period_start)}</SummaryValue>
          </SummaryItem>
          <SummaryItem>
            <SummaryLabel>Next Reset</SummaryLabel>
            <SummaryValue>
              {budget.next_period_start ? formatDate(budget.next_period_start) : '—'}
            </SummaryValue>
          </SummaryItem>
          {/* Only show Isolated for org budgets or team budgets with parent */}
          {(budget.entity_type === 'org' || budget.parent_id) && (
            <SummaryItem>
              <SummaryLabel>Isolated</SummaryLabel>
              <SummaryValue>{budget.isolated ? 'Yes' : 'No'}</SummaryValue>
            </SummaryItem>
          )}
          <SummaryItem>
            <SummaryLabel>Enabled</SummaryLabel>
            <SummaryValue>{budget.enabled ? 'Yes' : 'No'}</SummaryValue>
          </SummaryItem>
          {!budget.enabled && (
            <SummaryItem>
              <SummaryLabel>Disabled by</SummaryLabel>
              <SummaryValue>
                {budget.disabled_by_email || 'Unknown'}
                {budget.disabled_by_is_org && ' (org admin)'}
                {budget.disabled_at && ` on ${formatDate(budget.disabled_at)}`}
              </SummaryValue>
            </SummaryItem>
          )}
          <SummaryItem>
            <SummaryLabel>Description</SummaryLabel>
            <SummaryValue>{budget.description || '—'}</SummaryValue>
          </SummaryItem>
          <SummaryItem>
            <SummaryLabel>Owner Organization</SummaryLabel>
            <SummaryValue>{budget.owner_org_id || '—'}</SummaryValue>
          </SummaryItem>
          <SummaryItem>
            <SummaryLabel>Owner Team</SummaryLabel>
            <SummaryValue>{budget.owner_team_id || '—'}</SummaryValue>
          </SummaryItem>
        </SummaryGrid>
        <ProgressSection>
          <ProgressLabel>
            <span>Usage Progress</span>
            <span>{usagePercent.toFixed(1)}%</span>
          </ProgressLabel>
          <ProgressBar
            value={budget.current_usage_usd}
            max={budget.budget_amount_usd}
            warningThreshold={budget.warning_threshold_pct}
          />
        </ProgressSection>
      </SummaryCard>

      <UsageSection>
        <SectionHeader>
          <SectionTitle>Usage History</SectionTitle>
        </SectionHeader>

        {usageLoading ? (
          <Loading />
        ) : usageRecords && usageRecords.length > 0 ? (
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Time</TableHeader>
                  <TableHeader>Model</TableHeader>
                  <TableHeader align="right">Input Tokens</TableHeader>
                  <TableHeader align="right">Output Tokens</TableHeader>
                  <TableHeader align="right">Cost</TableHeader>
                  <TableHeader>Request ID</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {usageRecords.map(record => (
                  <TableRow key={record.id}>
                    <TableCell>{formatDate(record.created_at)}</TableCell>
                    <TableCell>{record.model_id}</TableCell>
                    <TableCell align="right">{formatNumber(record.input_tokens)}</TableCell>
                    <TableCell align="right">{formatNumber(record.output_tokens)}</TableCell>
                    <TableCell align="right">{formatCurrency(record.cost_usd)}</TableCell>
                    <TableCell style={{ fontFamily: 'monospace', fontSize: '12px' }}>
                      {record.request_id.slice(0, 8)}...
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        ) : (
          <EmptyState>
            <EmptyStateText>No usage records found for this budget.</EmptyStateText>
          </EmptyState>
        )}
      </UsageSection>

      <ConfirmDialog
        open={resetDialogOpen}
        onClose={() => setResetDialogOpen(false)}
        onConfirm={handleReset}
        title="Reset Budget"
        message="Are you sure you want to reset this budget? This will clear the current usage counter and start a new period."
        confirmLabel="Reset"
        variant="danger"
        loading={resetMutation.loading}
      />

      <ConfirmDialog
        open={showCascadeConfirm}
        onClose={() => setShowCascadeConfirm(false)}
        onConfirm={async () => {
          setShowCascadeConfirm(false);
          await handleToggleEnabled(false);
        }}
        title="Disable Budget"
        message={`This will also disable ${cascadeCount} linked team budget${cascadeCount > 1 ? 's' : ''}. Continue?`}
        confirmLabel="Disable All"
        variant="danger"
      />
    </Container>
  );
}
