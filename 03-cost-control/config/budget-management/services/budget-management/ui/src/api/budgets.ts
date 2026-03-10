import { apiClient } from './client';
import {
  BudgetDefinition,
  ListBudgetsResponse,
  CreateBudgetRequest,
  UpdateBudgetRequest,
  UsageRecord,
  UsageHistoryResponse,
  ValidateCELResponse,
} from './types';

export const budgetsApi = {
  async list(): Promise<BudgetDefinition[]> {
    const response = await apiClient.get<ListBudgetsResponse>('/budgets');
    return response.budgets || [];
  },

  async get(id: string): Promise<BudgetDefinition> {
    return apiClient.get<BudgetDefinition>(`/budgets/${id}`);
  },

  async create(data: CreateBudgetRequest): Promise<BudgetDefinition> {
    return apiClient.post<BudgetDefinition, CreateBudgetRequest>('/budgets', data);
  },

  async update(id: string, data: UpdateBudgetRequest): Promise<BudgetDefinition> {
    return apiClient.put<BudgetDefinition, UpdateBudgetRequest>(`/budgets/${id}`, data);
  },

  async delete(id: string): Promise<void> {
    return apiClient.delete(`/budgets/${id}`);
  },

  async getUsage(id: string, since?: Date, limit?: number): Promise<UsageRecord[]> {
    const params = new URLSearchParams();
    if (since) {
      params.set('since', since.toISOString());
    }
    if (limit) {
      params.set('limit', limit.toString());
    }
    const query = params.toString();
    const path = `/budgets/${id}/usage${query ? `?${query}` : ''}`;
    const response = await apiClient.get<UsageHistoryResponse>(path);
    return response.usage_records || [];
  },

  async reset(id: string): Promise<void> {
    await apiClient.post(`/budgets/${id}/reset`);
  },

  async validateCEL(expression: string): Promise<ValidateCELResponse> {
    return apiClient.post<ValidateCELResponse, { expression: string }>('/validate-cel', {
      expression,
    });
  },
};
