import { useState, useCallback } from 'react';
import { Link, useNavigate } from 'react-router-dom';
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
import { Tooltip } from '../../components/common/Tooltip';
import { ConfirmDialog } from '../../components/common/ConfirmDialog';
import { Loading } from '../../components/common/Spinner';
import { Pagination } from '../../components/common/Pagination';
import { useMutation } from '../../hooks/useApi';
import { useSWRApi, CacheKeys, invalidateKey } from '../../hooks/useSWR';
import { budgetsApi } from '../../api/budgets';
import { approvalsApi } from '../../api/approvals';
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

const CollapseIcon = styled.span<{ $collapsed: boolean }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  font-size: 10px;
  color: ${colors.mutedForeground};
  cursor: pointer;
  margin-right: ${spacing[1]};
  transform: ${({ $collapsed }) => ($collapsed ? 'rotate(-90deg)' : 'rotate(0deg)')};
  transition: transform 0.15s ease;

  &:hover {
    color: ${colors.foreground};
  }
`;

const PaginationWrapper = styled.div`
  display: flex;
  justify-content: flex-end;
  padding: ${spacing[4]} 0;
`;

const ApprovalBanner = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: ${colors.infoBg};
  border: 1px solid ${colors.infoBorder};
  border-radius: 6px;
  padding: ${spacing[3]} ${spacing[4]};
  margin-bottom: ${spacing[4]};
  font-size: ${fontSize.sm};
  color: ${colors.foreground};
`;

const BannerLink = styled(Link)`
  color: ${colors.info};
  text-decoration: underline;
  margin-left: ${spacing[2]};
  &:hover {
    opacity: 0.8;
  }
`;

const BannerDismiss = styled.button`
  color: ${colors.mutedForeground};
  font-size: ${fontSize.xs};
  padding: ${spacing[1]} ${spacing[2]};
  border-radius: 4px;
  &:hover {
    background: ${colors.hoverBg};
    color: ${colors.foreground};
  }
`;

function formatCurrency(amount: number): string {
  if (amount === 0) return '$0.00';
  if (amount >= 1) return `$${amount.toFixed(2)}`;
  if (amount >= 0.01) return `$${amount.toFixed(4)}`;
  if (amount >= 0.001) return `$${amount.toFixed(5)}`;
  if (amount >= 0.0001) return `$${amount.toFixed(6)}`;
  return `$${amount.toPrecision(2)}`;
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '—';
  const d = new Date(dateStr);
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
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

interface BudgetWithDepth {
  budget: BudgetDefinition;
  depth: number;
  parentId: string | null; // Track parent for collapse filtering
  hasChildren: boolean; // True if this budget has children
}

// Build a flat list with parent-child hierarchy (children indented under parents)
function buildHierarchicalList(budgets: BudgetDefinition[]): BudgetWithDepth[] {
  const budgetMap = new Map(budgets.map(b => [b.id, b]));

  // Build parent → children map
  const childrenOf = new Map<string, BudgetDefinition[]>();
  budgets.forEach(b => {
    if (b.parent_id && budgetMap.has(b.parent_id)) {
      const list = childrenOf.get(b.parent_id) ?? [];
      list.push(b);
      childrenOf.set(b.parent_id, list);
    }
  });

  // Recursively build items with depth
  function buildItems(
    b: BudgetDefinition,
    depth: number,
    parentId: string | null
  ): BudgetWithDepth[] {
    const children = (childrenOf.get(b.id) ?? []).sort(
      (a, c) => new Date(c.created_at).getTime() - new Date(a.created_at).getTime()
    );
    const items: BudgetWithDepth[] = [
      { budget: b, depth, parentId, hasChildren: children.length > 0 },
    ];
    children.forEach(child => items.push(...buildItems(child, depth + 1, b.id)));
    return items;
  }

  // Find root budgets (no parent or parent not in visible list)
  const roots = budgets
    .filter(b => !b.parent_id || !budgetMap.has(b.parent_id))
    .sort((a, b) => {
      // Sort by entity_type: org first, then team, then provider
      const typeOrder = { org: 0, team: 1, provider: 2 };
      const aOrder = typeOrder[a.entity_type as keyof typeof typeOrder] ?? 3;
      const bOrder = typeOrder[b.entity_type as keyof typeof typeOrder] ?? 3;
      if (aOrder !== bOrder) return aOrder - bOrder;
      return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    });

  const result: BudgetWithDepth[] = [];
  roots.forEach(root => {
    result.push(...buildItems(root, 0, null));
  });

  return result;
}

export function BudgetsPage() {
  const navigate = useNavigate();
  const { permissions } = useAuth();
  const [formOpen, setFormOpen] = useState(false);
  const [editingBudget, setEditingBudget] = useState<BudgetDefinition | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BudgetDefinition | null>(null);
  const [hasChildrenBlocker, setHasChildrenBlocker] = useState<BudgetDefinition | null>(null);
  const [forceDeleteConfirm, setForceDeleteConfirm] = useState<BudgetDefinition | null>(null);
  const [page, setPage] = useState(1);
  const [bannerDismissed, setBannerDismissed] = useState(false);
  const [collapsedBudgets, setCollapsedBudgets] = useState<Set<string>>(new Set());

  // Use SWR for budgets list with background refresh
  const {
    data: pageData,
    loading,
    refresh,
  } = useSWRApi(`${CacheKeys.budgets}?page=${page}`, () => budgetsApi.list(page, 30), {
    refreshInterval: 30000,
  });

  // Fetch parent candidates for form dropdown (minimal data: id, name)
  const { data: parentCandidates } = useSWRApi(CacheKeys.parentCandidates, () =>
    budgetsApi.listParentCandidates()
  );

  // Fetch pending approvals count with SWR
  const { data: pendingData } = useSWRApi(CacheKeys.approvalCount, () => approvalsApi.count(), {
    refreshInterval: 30000,
  });
  const pendingCount = pendingData?.count ?? 0;

  const budgets = pageData?.data ?? null;
  const pagination = pageData?.pagination ?? null;
  const hierarchicalBudgets = budgets ? buildHierarchicalList(budgets) : [];

  // Filter out children of collapsed parents
  const visibleBudgets = hierarchicalBudgets.filter(item => {
    if (!item.parentId) return true; // Root items always visible
    return !collapsedBudgets.has(item.parentId);
  });

  const toggleCollapse = (budgetId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setCollapsedBudgets(prev => {
      const next = new Set(prev);
      if (next.has(budgetId)) {
        next.delete(budgetId);
      } else {
        next.add(budgetId);
      }
      return next;
    });
  };

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

  const refreshAll = useCallback(() => {
    refresh();
    invalidateKey(CacheKeys.sidebarStats);
    invalidateKey(CacheKeys.approvalCount);
  }, [refresh]);

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
      refreshAll();
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
      refreshAll();
    } catch (error) {
      if (error instanceof ApiClientError && error.isConflict) {
        toast.error('Cannot delete: budget has child budgets. Delete children first.');
        refresh();
      } else {
        const message = error instanceof Error ? error.message : 'An error occurred';
        toast.error(message);
      }
    }
  };

  const handleForceDeleteClick = () => {
    if (!hasChildrenBlocker) return;
    // Move to second confirmation
    setForceDeleteConfirm(hasChildrenBlocker);
    setHasChildrenBlocker(null);
  };

  const handleForceDeleteConfirm = async () => {
    if (!forceDeleteConfirm) return;
    try {
      // Use cascade delete API to delete parent and all descendants atomically
      await budgetsApi.delete(forceDeleteConfirm.id, { cascade: true });
      toast.success('Budget and all child budgets deleted');
      setForceDeleteConfirm(null);
      refreshAll();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'An error occurred';
      toast.error(message);
    }
  };

  const handleDeleteClick = (
    budget: BudgetDefinition,
    hasChildren: boolean,
    e: React.MouseEvent
  ) => {
    e.stopPropagation();
    if (hasChildren) {
      setHasChildrenBlocker(budget);
    } else {
      setDeleteTarget(budget);
    }
  };

  const handleToggleEnabled = async (budget: BudgetDefinition, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await updateMutation.execute(budget.id, {
        enabled: !budget.enabled,
        version: budget.version,
      });
      toast.success(budget.enabled ? 'Budget disabled' : 'Budget enabled');
      refreshAll();
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

  const handlePageChange = (newPage: number) => {
    setPage(newPage);
  };

  return (
    <Container>
      <PageHeader title="Budgets" description="Manage budget definitions and track usage">
        <Button variant="secondary" onClick={refresh}>
          Refresh
        </Button>
        {permissions.canCreateBudgets && <Button onClick={handleCreate}>Create Budget</Button>}
      </PageHeader>

      {permissions.isOrgAdmin && !bannerDismissed && pendingCount > 0 && (
        <ApprovalBanner>
          <span>
            You have {pendingCount} budget{pendingCount !== 1 ? 's' : ''} awaiting approval.
            <BannerLink to="/approvals">View approvals</BannerLink>
          </span>
          <BannerDismiss onClick={() => setBannerDismissed(true)}>Dismiss</BannerDismiss>
        </ApprovalBanner>
      )}

      {loading ? (
        <Loading />
      ) : budgets && budgets.length > 0 ? (
        <>
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>
                    <Tooltip text="Entity name and type (org, team, or user) that this budget applies to">
                      Entity
                    </Tooltip>
                  </TableHeader>
                  <TableHeader align="right">
                    <Tooltip text="Maximum spend allowed per period">Budget</Tooltip>
                  </TableHeader>
                  <TableHeader>
                    <Tooltip text="Reset frequency: daily, weekly, monthly, or custom">
                      Period
                    </Tooltip>
                  </TableHeader>
                  <TableHeader>
                    <Tooltip
                      text={`Current spend vs budget limit.\nYellow at warning threshold, red when exceeded.`}
                    >
                      Usage
                    </Tooltip>
                  </TableHeader>
                  <TableHeader>
                    <Tooltip text="Active: under budget. Warning: approaching limit. Exceeded: over budget. Disabled: not enforced">
                      Status
                    </Tooltip>
                  </TableHeader>
                  <TableHeader>
                    <Tooltip text="Organization or team that owns this budget">Owner</Tooltip>
                  </TableHeader>
                  <TableHeader>Created</TableHeader>
                  <TableHeader align="right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {visibleBudgets.map(({ budget, depth, hasChildren }) => (
                  <DisabledRow
                    key={budget.id}
                    clickable
                    onClick={() => handleRowClick(budget)}
                    disabled={!budget.enabled}
                  >
                    <TableCell>
                      <EntityCell>
                        <EntityInfo
                          style={{ paddingLeft: depth > 0 ? `${depth * 20}px` : undefined }}
                        >
                          {hasChildren && depth === 0 && (
                            <CollapseIcon
                              $collapsed={collapsedBudgets.has(budget.id)}
                              onClick={e => toggleCollapse(budget.id, e)}
                            >
                              ▼
                            </CollapseIcon>
                          )}
                          <EntityId>
                            {depth > 0 && '↳ '}
                            {budget.name}
                          </EntityId>
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
                      ) : budget.disabled_by_is_org ? (
                        <Badge variant="error">Disabled (by org)</Badge>
                      ) : (
                        <Badge variant="warning">Disabled</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      {budget.owner_team_id ? (
                        <Badge variant="default">Team: {budget.owner_team_id}</Badge>
                      ) : budget.owner_org_id ? (
                        <Badge variant="default">Org: {budget.owner_org_id}</Badge>
                      ) : (
                        <span style={{ color: '#71717A', fontSize: '12px' }}>-</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <span style={{ fontSize: fontSize.xs, color: colors.mutedForeground }}>
                        {formatDate(budget.created_at)}
                      </span>
                    </TableCell>
                    <TableCell align="right">
                      <ActionButtons>
                        {permissions.canToggleBudgetEnabled && budget.can_enable !== false && (
                          <ActionButton onClick={e => handleToggleEnabled(budget, e)}>
                            {budget.enabled ? 'Disable' : 'Enable'}
                          </ActionButton>
                        )}
                        {permissions.canEditBudget(budget) && (
                          <ActionButton onClick={e => handleEdit(budget, e)}>Edit</ActionButton>
                        )}
                        {permissions.canDeleteBudgets && (
                          <ActionButton onClick={e => handleDeleteClick(budget, hasChildren, e)}>
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
          {pagination && pagination.total_pages > 1 && (
            <PaginationWrapper>
              <Pagination
                page={pagination.page}
                totalPages={pagination.total_pages}
                onPageChange={handlePageChange}
              />
            </PaginationWrapper>
          )}
        </>
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
        parentCandidates={parentCandidates || []}
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

      <ConfirmDialog
        open={!!hasChildrenBlocker}
        onClose={() => setHasChildrenBlocker(null)}
        onConfirm={handleForceDeleteClick}
        title="Budget Has Children"
        message={`The budget "${hasChildrenBlocker?.name}" has child budgets. Deleting this budget will permanently delete ALL child budgets and their usage history.`}
        confirmLabel="Force Delete All"
        variant="danger"
      />

      <ConfirmDialog
        open={!!forceDeleteConfirm}
        onClose={() => setForceDeleteConfirm(null)}
        onConfirm={handleForceDeleteConfirm}
        title="Confirm Permanent Deletion"
        message={`Are you absolutely sure? This will permanently delete "${forceDeleteConfirm?.name}" and ALL its child budgets. This action cannot be undone.`}
        confirmLabel="Yes, Delete Everything"
        variant="danger"
      />
    </Container>
  );
}
