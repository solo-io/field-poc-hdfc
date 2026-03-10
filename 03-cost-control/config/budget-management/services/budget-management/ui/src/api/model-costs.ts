import { apiClient } from './client';
import {
  ModelCost,
  ListModelCostsResponse,
  CreateModelCostRequest,
  UpdateModelCostRequest,
} from './types';

export const modelCostsApi = {
  async list(): Promise<ModelCost[]> {
    const response = await apiClient.get<ListModelCostsResponse>('/model-costs');
    return response.model_costs || [];
  },

  async get(modelId: string): Promise<ModelCost> {
    return apiClient.get<ModelCost>(`/model-costs/${encodeURIComponent(modelId)}`);
  },

  async create(data: CreateModelCostRequest): Promise<ModelCost> {
    return apiClient.post<ModelCost, CreateModelCostRequest>('/model-costs', data);
  },

  async update(modelId: string, data: UpdateModelCostRequest): Promise<ModelCost> {
    return apiClient.put<ModelCost, UpdateModelCostRequest>(
      `/model-costs/${encodeURIComponent(modelId)}`,
      data
    );
  },

  async delete(modelId: string): Promise<void> {
    return apiClient.delete(`/model-costs/${encodeURIComponent(modelId)}`);
  },
};
