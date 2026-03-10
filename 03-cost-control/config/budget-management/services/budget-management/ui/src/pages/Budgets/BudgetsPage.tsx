import { useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import styled from '@emotion/styled';
import toast from 'react-hot-toast';
import { spacing, colors, fontSize } from '../../styles';
import { PageHeader } from '../../components/layout/PageHeader';
import { Button } from '../../components/common/Button';
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
import { useApi, useMutation } from '../../hooks/useApi';
import { budgetsApi } from '../../api/budgets';
import { BudgetDefinition, CreateBudgetRequest, UpdateBudgetRequest } from '../../api/types';
import { ApiClientError } from '../../api/client';
import { BudgetForm } from './BudgetForm';
import { useAuth } from '../../contexts/AuthContext';

const Container = styled.div``;

const ActionButtons = styled.div`
  display: flex;
  gap: ${spacing[2]};
  justify-content: flex-end;
`;

const ActionButton = styled.button`
  padding: ${spacing[1]} ${spacing[2]};
  border-radius: 4px;
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  transition: all 0.15s ease;

  &:hover {
    background: ${colors.hoverBg};
    color: ${colors.foreground};
  }
`;

const DisabledRow = styled(TableRow)<{ disabled?: boolean }>`
  opacity: ${({ disabled }) => (disabled ? 0.5 : 1)};
`;

const EntityCell = styled.div`
  display: flex;
  align-items: center;
`;

const TreeIndent = styled.span<{ depth: number }>`
  display: inline-flex;
  align-items: center;
  width: ${({ depth }) => depth * 24}px;
  flex-shrink: 0;
`;

const TreeLine = styled.span<{ isLast: boolean }>`
  display: inline-flex;
  align-items: center;
  color: ${colors.border};
  font-family: monospace;
  margin-right: ${spacing[2]};
  flex-shrink: 0;
`;

const EntityInfo = styled.div``;

const EntityId = styled.div`
  font-weight: 500;
`;

const EntityType = styled.div`
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  text-transform: capitalize;
`;

const UsageCell = styled.div`
  min-width: 150px;
`;

const UsageText = styled.div`
  font-size: ${fontSize.xs};
  color: ${colors.mutedForeground};
  margin-top: ${spacing[1]};
`;

const HeaderWithTooltip = styled.span`
  display: inline-flex;
  align-items: center;
  gap: ${spacing[1]};
  cursor: help;
`;

const HelpIcon = styled.span`
  font-size: 10px;
  color: ${colors.border};
  border: 1px solid ${colors.border};
  border-radius: 50%;
  width: 14px;
  height: 14px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
`;

function formatCurrency(amount: number): string {
  if (amount === 0) return '$0.00';
  if (amount >= 1) return `$${amount.toFixed(2)}`;
  if (amount >= 0.01) return `$${amount.toFixed(4)}`;
  if (amount >= 0.001) return `$${amount.toFixed(5)}`;
  if (amount >= 0.0001) return `$${amount.toFixed(6)}`;
  return `$${amount.toPrecision(2)}`;
}

function getStatusVariant(budget: BudgetDefinition): 'success' | 'warning' | 'error' {
  const usagePercent = (budget.current_usage_usd / budget.budget_amount_usd) * 100;
  if (usagePercent >= 100) return 'error';
  if (usagePercent >= budget.warning_threshold_pct) return 'warning';
  return 'success';
}

function getStatusLabel(budget: BudgetDefinition): string {
  const usagePercent = (budget.current_usage_usd / budget.budget_amount_usd) * 100;
  if (usagePercent >= 100) return 'Exceeded';
  if (usagePercent >= budget.warning_threshold_pct) return 'Warning';
  return 'Active';
}

interface FlatTreeItem {
  budget: BudgetDefinition;
  depth: number;
  isLast: boolean;
  hasChildren: boolean;
}

function buildBudgetTree(budgets: BudgetDefinition[]): FlatTreeItem[] {
  const budgetMap = new Map<string, BudgetDefinition>();
  const childrenMap = new Map<string, BudgetDefinition[]>();

  budgets.forEach(b => {
    budgetMap.set(b.id, b);
    if (b.parent_id) {
      const siblings = childrenMap.get(b.parent_id) || [];
      siblings.push(b);
      childrenMap.set(b.parent_id, siblings);
    }
  });

  // Treat as root if no parent_id OR parent is not in the visible list (orphaned)
  const rootBudgets = budgets.filter(b => !b.parent_id || !budgetMap.has(b.parent_id));

  const result: FlatTreeItem[] = [];

  function traverse(budget: BudgetDefinition, depth: number, isLast: boolean) {
    const children = childrenMap.get(budget.id) || [];
    result.push({
      budget,
      depth,
      isLast,
      hasChildren: children.length > 0,
    });
    children.forEach((child, idx) => {
      traverse(child, depth + 1, idx === children.length - 1);
    });
  }

  rootBudgets.forEach((budget, idx) => {
    traverse(budget, 0, idx === rootBudgets.length - 1);
  });

  return result;
}

export function BudgetsPage() {
  const navigate = useNavigate();
  const { permissions } = useAuth();
  const [formOpen, setFormOpen] = useState(false);
  const [editingBudget, setEditingBudget] = useState<BudgetDefinition | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BudgetDefinition | null>(null);

  const { data: budgets, loading, refresh } = useApi(useCallback(() => budgetsApi.list(), []));

  // Filter budgets based on permissions
  const visibleBudgets = budgets?.filter(b => permissions.canViewBudget(b));

  const createMutation = useMutation(budgetsApi.create);
  const updateMutation = useMutation(
    useCallback((id: string, data: UpdateBudgetRequest) => budgetsApi.update(id, data), [])
  );
  const deleteMutation = useMutation(budgetsApi.delete);

  const handleCreate = () => {
    setEditingBudget(null);
    setFormOpen(true);
  };

  const handleEdit = (budget: BudgetDefinition, e: React.MouseEvent) => {
    e.stopPropagation();
    setEditingBudget(budget);
    setFormOpen(true);
  };

  const handleRowClick = (budget: BudgetDefinition) => {
    navigate(`/budgets/${budget.id}`);
  };

  const handleFormSubmit = async (data: CreateBudgetRequest) => {
    try {
      if (editingBudget) {
        await updateMutation.execute(editingBudget.id, {
          ...data,
          version: editingBudget.version,
        });
        toast.success('Budget updated');
      } else {
        await createMutation.execute(data);
        toast.success('Budget created');
      }
      setFormOpen(false);
      refresh();
    } catch (error) {
      if (error instanceof ApiClientError && error.isConflict) {
        toast.error('This record was modified. Please refresh and try again.');
        refresh();
      } else {
        const message = error instanceof Error ? error.message : 'An error occurred';
        toast.error(message);
      }
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteMutation.execute(deleteTarget.id);
      toast.success('Budget deleted');
      setDeleteTarget(null);
      refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'An error occurred';
      toast.error(message);
    }
  };

  const handleDeleteClick = (budget: BudgetDefinition, e: React.MouseEvent) => {
    e.stopPropagation();
    setDeleteTarget(budget);
  };

  const handleToggleEnabled = async (budget: BudgetDefinition, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await updateMutation.execute(budget.id, {
        enabled: !budget.enabled,
        version: budget.version,
      });
      toast.success(budget.enabled ? 'Budget disabled' : 'Budget enabled');
      refresh();
    } catch (error) {
      if (error instanceof ApiClientError && error.isConflict) {
        toast.error('This record was modified. Please refresh and try again.');
        refresh();
      } else {
        const message = error instanceof Error ? error.message : 'An error occurred';
        toast.error(message);
      }
    }
  };

  return (
    <Container>
      <PageHeader title="Budgets" description="Manage budget definitions and track usage">
        <Button variant="secondary" onClick={refresh}>
          Refresh
        </Button>
        {permissions.canCreateBudgets && <Button onClick={handleCreate}>Create Budget</Button>}
      </PageHeader>

      {loading ? (
        <Loading />
      ) : visibleBudgets && visibleBudgets.length > 0 ? (
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableHeader>
                  <HeaderWithTooltip title="Entity name and type (org, team, or user) that this budget applies to">
                    Entity <HelpIcon>?</HelpIcon>
                  </HeaderWithTooltip>
                </TableHeader>
                <TableHeader align="right">
                  <HeaderWithTooltip title="Maximum spend allowed per period">
                    Budget <HelpIcon>?</HelpIcon>
                  </HeaderWithTooltip>
                </TableHeader>
                <TableHeader>
                  <HeaderWithTooltip title="Reset frequency: daily, weekly, monthly, or custom">
                    Period <HelpIcon>?</HelpIcon>
                  </HeaderWithTooltip>
                </TableHeader>
                <TableHeader>
                  <HeaderWithTooltip title="Current spend vs budget limit. Yellow at warning threshold, red when exceeded">
                    Usage <HelpIcon>?</HelpIcon>
                  </HeaderWithTooltip>
                </TableHeader>
                <TableHeader>
                  <HeaderWithTooltip title="Active: under budget | Warning: approaching limit | Exceeded: over budget | Disabled: not enforced">
                    Status <HelpIcon>?</HelpIcon>
                  </HeaderWithTooltip>
                </TableHeader>
                <TableHeader>
                  <HeaderWithTooltip title="Organization or team that owns this budget">
                    Owner <HelpIcon>?</HelpIcon>
                  </HeaderWithTooltip>
                </TableHeader>
                <TableHeader align="right">Actions</TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              {buildBudgetTree(visibleBudgets).map(({ budget, depth, isLast }) => (
                <DisabledRow
                  key={budget.id}
                  clickable
                  onClick={() => handleRowClick(budget)}
                  disabled={!budget.enabled}
                >
                  <TableCell>
                    <EntityCell>
                      {depth > 0 && (
                        <>
                          <TreeIndent depth={depth - 1} />
                          <TreeLine isLast={isLast}>{isLast ? '└─' : '├─'}</TreeLine>
                        </>
                      )}
                      <EntityInfo>
                        <EntityId>{budget.name}</EntityId>
                        <EntityType>{budget.entity_type}</EntityType>
                      </EntityInfo>
                    </EntityCell>
                  </TableCell>
                  <TableCell align="right">{formatCurrency(budget.budget_amount_usd)}</TableCell>
                  <TableCell style={{ textTransform: 'capitalize' }}>{budget.period}</TableCell>
                  <TableCell>
                    <UsageCell>
                      <ProgressBar
                        value={budget.current_usage_usd}
                        max={budget.budget_amount_usd}
                        warningThreshold={budget.warning_threshold_pct}
                      />
                      <UsageText>
                        {formatCurrency(budget.current_usage_usd)} /{' '}
                        {formatCurrency(budget.budget_amount_usd)}
                      </UsageText>
                    </UsageCell>
                  </TableCell>
                  <TableCell>
                    {budget.enabled ? (
                      <Badge variant={getStatusVariant(budget)}>{getStatusLabel(budget)}</Badge>
                    ) : (
                      <Badge variant="default">Disabled</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {budget.owner_org_id ? (
                      <Badge variant="default">Org: {budget.owner_org_id}</Badge>
                    ) : budget.owner_team_id ? (
                      <Badge variant="default">Team: {budget.owner_team_id}</Badge>
                    ) : (
                      <span style={{ color: '#71717A', fontSize: '12px' }}>-</span>
                    )}
                  </TableCell>
                  <TableCell align="right">
                    <ActionButtons>
                      {permissions.canEditBudget(budget) && (
                        <ActionButton onClick={e => handleToggleEnabled(budget, e)}>
                          {budget.enabled ? 'Disable' : 'Enable'}
                        </ActionButton>
                      )}
                      {permissions.canEditBudget(budget) && (
                        <ActionButton onClick={e => handleEdit(budget, e)}>Edit</ActionButton>
                      )}
                      {permissions.canDeleteBudgets && (
                        <ActionButton onClick={e => handleDeleteClick(budget, e)}>
                          Delete
                        </ActionButton>
                      )}
                    </ActionButtons>
                  </TableCell>
                </DisabledRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      ) : (
        <EmptyState>
          <EmptyStateText>No budgets configured yet.</EmptyStateText>
          {permissions.canCreateBudgets && <Button onClick={handleCreate}>Create Budget</Button>}
        </EmptyState>
      )}

      <BudgetForm
        open={formOpen}
        onClose={() => setFormOpen(false)}
        onSubmit={handleFormSubmit}
        editingBudget={editingBudget}
        availableBudgets={visibleBudgets || []}
        loading={createMutation.loading || updateMutation.loading}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Budget"
        message={`Are you sure you want to delete the budget for "${deleteTarget?.name}"? This will also delete all associated usage records. This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleteMutation.loading}
      />
    </Container>
  );
}
