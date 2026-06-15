import { useAuthStore } from '../store/authStore';

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

interface RequestOptions extends RequestInit {
  headers?: Record<string, string>;
}

// Custom API fetch wrapper
export async function apiFetch(endpoint: string, options: RequestOptions = {}): Promise<Response> {
  const url = `${API_URL}${endpoint}`;
  
  // 1. Get current access token
  const state = useAuthStore.getState();
  const token = state.accessToken;

  // Initialize headers
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  } as Record<string, string>;

  // Attach token if present
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const finalOptions = {
    ...options,
    headers,
  };

  // 2. Perform initial request
  let response = await fetch(url, finalOptions);

  // 3. Intercept 401 and attempt silent refresh
  if (response.status === 401 && state.refreshToken) {
    console.log('Access token expired, attempting silent refresh...');
    
    try {
      const refreshResponse = await fetch(`${API_URL}/api/auth/refresh`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ refresh_token: state.refreshToken }),
      });

      if (refreshResponse.ok) {
        const data = await refreshResponse.json();
        const newAccessToken = data.access_token;
        const user = state.user;

        if (user && newAccessToken && state.refreshToken) {
          // Update Zustand store
          state.setAuth(user, newAccessToken, state.refreshToken);

          // Retry the original request with the new access token
          headers['Authorization'] = `Bearer ${newAccessToken}`;
          response = await fetch(url, {
            ...options,
            headers,
          });
        }
      } else {
        // Refresh token is expired or invalid, force logout
        console.error('Refresh token invalid, logging out user.');
        state.clearAuth();
      }
    } catch (err) {
      console.error('Failed to perform silent token refresh', err);
      state.clearAuth();
    }
  }

  return response;
}
