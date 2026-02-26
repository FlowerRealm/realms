import axios from 'axios';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '',
  withCredentials: true,
  headers: {
    'Cache-Control': 'no-store',
  },
});

api.interceptors.request.use((config) => {
  try {
    const isPersonalApp = import.meta.env.MODE === 'personal';
    if (isPersonalApp) {
      const personalKey = (localStorage.getItem('personal_mode_key') || '').trim();
      if (personalKey) {
        config.headers = config.headers ?? {};
        const headers = config.headers as Record<string, string>;
        if (!headers['Authorization'] && !headers['authorization'] && !headers['x-api-key'] && !headers['X-Api-Key']) {
          headers['Authorization'] = `Bearer ${personalKey}`;
        }
      }
    }

    const raw = localStorage.getItem('user');
    if (raw) {
      const parsed = JSON.parse(raw) as { id?: number };
      const id = parsed?.id;
      if (typeof id === 'number' && id > 0) {
        config.headers = config.headers ?? {};
        (config.headers as Record<string, string>)['Realms-User'] = String(id);
      }
    }
  } catch {
    // ignore invalid localStorage content
  }
  return config;
});
