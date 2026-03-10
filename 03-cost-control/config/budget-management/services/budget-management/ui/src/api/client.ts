import { ApiError } from './types';

const API_BASE = '/api/v1';

export class ApiClientError extends Error {
  constructor(
    message: string,
    public status: number,
    public isConflict: boolean = false
  ) {
    super(message);
    this.name = 'ApiClientError';
  }
}

// Handle session expiry - CORS errors typically mean auth redirect
function handleSessionExpiry() {
  // Reload the page to trigger auth flow
  window.location.reload();
}

async function fetchWithAuthRetry(url: string, options: RequestInit): Promise<Response> {
  try {
    const response = await fetch(url, options);
    // 401/403 may indicate session expiry
    if (response.status === 401 || response.status === 403) {
      handleSessionExpiry();
      throw new ApiClientError('Session expired', response.status);
    }
    return response;
  } catch (error) {
    // Network errors (including CORS) when session expires
    if (error instanceof TypeError && error.message.includes('fetch')) {
      handleSessionExpiry();
    }
    throw error;
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let message = 'An error occurred';
    try {
      const errorData: ApiError = await response.json();
      message = errorData.error?.message || message;
    } catch {
      message = response.statusText || message;
    }

    throw new ApiClientError(message, response.status, response.status === 409);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}

export const apiClient = {
  async get<T>(path: string): Promise<T> {
    const response = await fetchWithAuthRetry(`${API_BASE}${path}`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
      },
    });
    return handleResponse<T>(response);
  },

  async post<T, D = unknown>(path: string, data?: D): Promise<T> {
    const response = await fetchWithAuthRetry(`${API_BASE}${path}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: data ? JSON.stringify(data) : undefined,
    });
    return handleResponse<T>(response);
  },

  async put<T, D = unknown>(path: string, data: D): Promise<T> {
    const response = await fetchWithAuthRetry(`${API_BASE}${path}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(data),
    });
    return handleResponse<T>(response);
  },

  async delete<T>(path: string): Promise<T> {
    const response = await fetchWithAuthRetry(`${API_BASE}${path}`, {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json',
      },
    });
    return handleResponse<T>(response);
  },
};
