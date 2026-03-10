import { useState, useCallback } from 'react';
import styled from '@emotion/styled';
import toast from 'react-hot-toast';
import { spacing, colors, fontSize } from '../../styles';
import { PageHeader } from '../../components/layout/PageHeader';
import { Button } from '../../components/common/Button';
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
import { modelCostsApi } from '../../api/model-costs';
import { ModelCost, CreateModelCostRequest } from '../../api/types';
import { ModelCostForm } from './ModelCostForm';
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

function formatCost(cost: number): string {
  if (cost === 0) return '$0.00';
  if (cost >= 1) return `$${cost.toFixed(2)}`;
  if (cost >= 0.01) return `$${cost.toFixed(4)}`;
  if (cost >= 0.001) return `$${cost.toFixed(5)}`;
  if (cost >= 0.0001) return `$${cost.toFixed(6)}`;
  return `$${cost.toPrecision(2)}`;
}

export function ModelCostsPage() {
  const { permissions } = useAuth();
  const [formOpen, setFormOpen] = useState(false);
  const [editingCost, setEditingCost] = useState<ModelCost | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ModelCost | null>(null);

  const { data: costs, loading, refresh } = useApi(useCallback(() => modelCostsApi.list(), []));

  const createMutation = useMutation(modelCostsApi.create);
  const updateMutation = useMutation(
    useCallback(
      (modelId: string, data: CreateModelCostRequest) => modelCostsApi.update(modelId, data),
      []
    )
  );
  const deleteMutation = useMutation(modelCostsApi.delete);

  const handleCreate = () => {
    setEditingCost(null);
    setFormOpen(true);
  };

  const handleEdit = (cost: ModelCost) => {
    setEditingCost(cost);
    setFormOpen(true);
  };

  const handleFormSubmit = async (data: CreateModelCostRequest) => {
    try {
      if (editingCost) {
        await updateMutation.execute(editingCost.model_id, data);
        toast.success('Model cost updated');
      } else {
        await createMutation.execute(data);
        toast.success('Model cost created');
      }
      setFormOpen(false);
      refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'An error occurred';
      toast.error(message);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteMutation.execute(deleteTarget.model_id);
      toast.success('Model cost deleted');
      setDeleteTarget(null);
      refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'An error occurred';
      toast.error(message);
    }
  };

  return (
    <Container>
      <PageHeader title="Model Costs" description="Configure pricing for LLM models">
        <Button variant="secondary" onClick={refresh}>
          Refresh
        </Button>
        {permissions.canManageModelCosts && <Button onClick={handleCreate}>Add Model Cost</Button>}
      </PageHeader>

      {loading ? (
        <Loading />
      ) : costs && costs.length > 0 ? (
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableHeader>Model ID</TableHeader>
                <TableHeader>Provider</TableHeader>
                <TableHeader align="right">Input Cost</TableHeader>
                <TableHeader align="right">Output Cost</TableHeader>
                <TableHeader>Pattern</TableHeader>
                {permissions.canManageModelCosts && (
                  <TableHeader align="right">Actions</TableHeader>
                )}
              </TableRow>
            </TableHead>
            <TableBody>
              {costs.map(cost => (
                <TableRow key={cost.model_id}>
                  <TableCell>{cost.model_id}</TableCell>
                  <TableCell>{cost.provider}</TableCell>
                  <TableCell align="right">{formatCost(cost.input_cost_per_million)}</TableCell>
                  <TableCell align="right">{formatCost(cost.output_cost_per_million)}</TableCell>
                  <TableCell>{cost.model_pattern || '—'}</TableCell>
                  <TableCell align="right">
                    {permissions.canManageModelCosts && (
                      <ActionButtons>
                        <ActionButton onClick={() => handleEdit(cost)}>Edit</ActionButton>
                        <ActionButton onClick={() => setDeleteTarget(cost)}>Delete</ActionButton>
                      </ActionButtons>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      ) : (
        <EmptyState>
          <EmptyStateText>No model costs configured yet.</EmptyStateText>
          {permissions.canManageModelCosts && (
            <Button onClick={handleCreate}>Add Model Cost</Button>
          )}
        </EmptyState>
      )}

      <ModelCostForm
        open={formOpen}
        onClose={() => setFormOpen(false)}
        onSubmit={handleFormSubmit}
        editingCost={editingCost}
        loading={createMutation.loading || updateMutation.loading}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Model Cost"
        message={`Are you sure you want to delete the cost configuration for "${deleteTarget?.model_id}"? This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleteMutation.loading}
      />
    </Container>
  );
}
