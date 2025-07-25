// API client for eAIIngest Dashboard

class APIClient {
    constructor() {
        this.baseURL = '/api/v1';
        this.token = this.getAuthToken();
        this.requestInterceptors = [];
        this.responseInterceptors = [];
    }

    getAuthToken() {
        return localStorage.getItem('authToken') || sessionStorage.getItem('authToken');
    }

    setAuthToken(token, persistent = false) {
        if (persistent) {
            localStorage.setItem('authToken', token);
        } else {
            sessionStorage.setItem('authToken', token);
        }
        this.token = token;
    }

    clearAuthToken() {
        localStorage.removeItem('authToken');
        sessionStorage.removeItem('authToken');
        this.token = null;
    }

    async request(endpoint, options = {}) {
        const url = `${this.baseURL}${endpoint}`;
        const config = {
            headers: {
                'Content-Type': 'application/json',
                ...options.headers,
            },
            ...options,
        };

        // Add authentication header if token exists
        if (this.token) {
            config.headers['Authorization'] = `Bearer ${this.token}`;
        }

        // Apply request interceptors
        for (const interceptor of this.requestInterceptors) {
            await interceptor(config);
        }

        try {
            const response = await fetch(url, config);
            
            // Apply response interceptors
            for (const interceptor of this.responseInterceptors) {
                await interceptor(response);
            }

            // Handle authentication errors
            if (response.status === 401) {
                this.clearAuthToken();
                window.location.href = '/login';
                throw new Error('Authentication required');
            }

            // Handle other HTTP errors
            if (!response.ok) {
                const errorData = await response.json().catch(() => ({}));
                throw new APIError(errorData.message || 'Request failed', response.status, errorData);
            }

            // Return JSON response
            const contentType = response.headers.get('content-type');
            if (contentType && contentType.includes('application/json')) {
                return await response.json();
            }

            return response;

        } catch (error) {
            if (error instanceof APIError) {
                throw error;
            }
            throw new APIError(error.message || 'Network error', 0, { originalError: error });
        }
    }

    // HTTP method helpers
    get(endpoint, params = {}) {
        const searchParams = new URLSearchParams(params);
        const url = searchParams.toString() ? `${endpoint}?${searchParams}` : endpoint;
        return this.request(url, { method: 'GET' });
    }

    post(endpoint, data = {}) {
        return this.request(endpoint, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    }

    put(endpoint, data = {}) {
        return this.request(endpoint, {
            method: 'PUT',
            body: JSON.stringify(data),
        });
    }

    patch(endpoint, data = {}) {
        return this.request(endpoint, {
            method: 'PATCH',
            body: JSON.stringify(data),
        });
    }

    delete(endpoint) {
        return this.request(endpoint, { method: 'DELETE' });
    }

    // File upload helper
    async upload(endpoint, files, onProgress = null) {
        const formData = new FormData();
        
        if (Array.isArray(files)) {
            files.forEach(file => formData.append('files', file));
        } else {
            formData.append('file', files);
        }

        return new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            
            // Add authentication header
            if (this.token) {
                xhr.setRequestHeader('Authorization', `Bearer ${this.token}`);
            }

            // Track upload progress
            if (onProgress) {
                xhr.upload.addEventListener('progress', (e) => {
                    if (e.lengthComputable) {
                        const percentComplete = (e.loaded / e.total) * 100;
                        onProgress(percentComplete, e.loaded, e.total);
                    }
                });
            }

            xhr.addEventListener('load', () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        const response = JSON.parse(xhr.responseText);
                        resolve(response);
                    } catch (e) {
                        resolve(xhr.responseText);
                    }
                } else {
                    reject(new APIError(`Upload failed with status ${xhr.status}`, xhr.status));
                }
            });

            xhr.addEventListener('error', () => {
                reject(new APIError('Upload failed', 0));
            });

            xhr.open('POST', `${this.baseURL}${endpoint}`);
            xhr.send(formData);
        });
    }

    // Interceptor management
    addRequestInterceptor(interceptor) {
        this.requestInterceptors.push(interceptor);
    }

    addResponseInterceptor(interceptor) {
        this.responseInterceptors.push(interceptor);
    }
}

// Custom error class for API errors
class APIError extends Error {
    constructor(message, status, data = {}) {
        super(message);
        this.name = 'APIError';
        this.status = status;
        this.data = data;
    }
}

// API service classes
class TenantService {
    constructor(client) {
        this.client = client;
    }

    async list(params = {}) {
        return this.client.get('/tenants', params);
    }

    async get(id) {
        return this.client.get(`/tenants/${id}`);
    }

    async create(data) {
        return this.client.post('/tenants', data);
    }

    async update(id, data) {
        return this.client.put(`/tenants/${id}`, data);
    }

    async delete(id) {
        return this.client.delete(`/tenants/${id}`);
    }

    async getStats(id) {
        return this.client.get(`/tenants/${id}/stats`);
    }
}

class ProcessingService {
    constructor(client) {
        this.client = client;
    }

    async getSessions(params = {}) {
        return this.client.get('/sessions', params);
    }

    async getSession(id) {
        return this.client.get(`/sessions/${id}`);
    }

    async createSession(data) {
        return this.client.post('/sessions', data);
    }

    async cancelSession(id) {
        return this.client.post(`/sessions/${id}/cancel`);
    }

    async getQueueStatus() {
        return this.client.get('/queue/status');
    }

    async uploadFiles(files, onProgress) {
        return this.client.upload('/files/upload', files, onProgress);
    }
}

class StorageService {
    constructor(client) {
        this.client = client;
    }

    async getStats() {
        return this.client.get('/storage/stats');
    }

    async getDatasets() {
        return this.client.get('/storage/datasets');
    }

    async getDataset(name) {
        return this.client.get(`/storage/datasets/${name}`);
    }

    async analyzeStorage() {
        return this.client.post('/storage/analyze');
    }

    async getFiles(params = {}) {
        return this.client.get('/files', params);
    }

    async getFile(id) {
        return this.client.get(`/files/${id}`);
    }

    async deleteFile(id) {
        return this.client.delete(`/files/${id}`);
    }
}

class AnalyticsService {
    constructor(client) {
        this.client = client;
    }

    async getMetrics(timeframe = '24h') {
        return this.client.get('/analytics/metrics', { timeframe });
    }

    async getProcessingTrends(timeframe = '7d') {
        return this.client.get('/analytics/trends', { timeframe });
    }

    async getContentClassification(timeframe = '30d') {
        return this.client.get('/analytics/classification', { timeframe });
    }

    async getDLPViolations(timeframe = '30d') {
        return this.client.get('/analytics/dlp', { timeframe });
    }

    async getPerformanceMetrics(timeframe = '24h') {
        return this.client.get('/analytics/performance', { timeframe });
    }

    async exportReport(type, timeframe = '30d') {
        return this.client.get(`/analytics/export/${type}`, { timeframe });
    }
}

class SystemService {
    constructor(client) {
        this.client = client;
    }

    async getHealth() {
        // Health endpoint doesn't require auth
        return fetch('/health').then(res => res.json());
    }

    async getHealthDetailed() {
        return fetch('/health?detailed=true').then(res => res.json());
    }

    async getMetrics() {
        // Metrics endpoint doesn't require auth
        return fetch('/metrics').then(res => res.text());
    }

    async getConfig() {
        return this.client.get('/system/config');
    }

    async updateConfig(config) {
        return this.client.put('/system/config', config);
    }

    async getLogs(params = {}) {
        return this.client.get('/system/logs', params);
    }

    async backup() {
        return this.client.post('/system/backup');
    }

    async restore(backupId) {
        return this.client.post(`/system/restore/${backupId}`);
    }
}

class AuthService {
    constructor(client) {
        this.client = client;
    }

    async login(username, password) {
        const response = await this.client.post('/auth/login', { username, password });
        if (response.token) {
            this.client.setAuthToken(response.token);
        }
        return response;
    }

    async logout() {
        try {
            await this.client.post('/auth/logout');
        } finally {
            this.client.clearAuthToken();
        }
    }

    async refreshToken() {
        const response = await this.client.post('/auth/refresh');
        if (response.token) {
            this.client.setAuthToken(response.token);
        }
        return response;
    }

    async getCurrentUser() {
        return this.client.get('/auth/me');
    }

    async updateProfile(data) {
        return this.client.put('/auth/profile', data);
    }

    async changePassword(currentPassword, newPassword) {
        return this.client.post('/auth/change-password', {
            current_password: currentPassword,
            new_password: newPassword
        });
    }
}

// Global API client instance
class API {
    constructor() {
        this.client = new APIClient();
        this.tenants = new TenantService(this.client);
        this.processing = new ProcessingService(this.client);
        this.storage = new StorageService(this.client);
        this.analytics = new AnalyticsService(this.client);
        this.system = new SystemService(this.client);
        this.auth = new AuthService(this.client);

        this.setupInterceptors();
    }

    setupInterceptors() {
        // Request interceptor to add common headers
        this.client.addRequestInterceptor(async (config) => {
            config.headers['X-Requested-With'] = 'XMLHttpRequest';
            config.headers['X-Client-Version'] = '1.0.0';
        });

        // Response interceptor for logging
        this.client.addResponseInterceptor(async (response) => {
            if (window.DEBUG) {
                console.log(`API ${response.url}:`, response.status);
            }
        });
    }

    // Convenience methods
    async healthCheck() {
        return this.system.getHealth();
    }

    async getSystemStatus() {
        const [health, metrics] = await Promise.allSettled([
            this.system.getHealth(),
            this.system.getMetrics()
        ]);

        return {
            health: health.status === 'fulfilled' ? health.value : null,
            metrics: metrics.status === 'fulfilled' ? metrics.value : null
        };
    }

    // Error handling helpers
    handleError(error) {
        if (error instanceof APIError) {
            switch (error.status) {
                case 400:
                    return 'Invalid request. Please check your input.';
                case 401:
                    return 'Authentication required. Please log in.';
                case 403:
                    return 'Access denied. You do not have permission for this action.';
                case 404:
                    return 'Resource not found.';
                case 409:
                    return 'Conflict. The resource already exists or is in use.';
                case 422:
                    return error.data.message || 'Validation failed.';
                case 429:
                    return 'Rate limit exceeded. Please try again later.';
                case 500:
                    return 'Internal server error. Please try again later.';
                case 503:
                    return 'Service unavailable. Please try again later.';
                default:
                    return error.message || 'An unexpected error occurred.';
            }
        }
        return 'Network error. Please check your connection.';
    }

    // Utility methods
    async retry(operation, maxRetries = 3, delay = 1000) {
        for (let i = 0; i < maxRetries; i++) {
            try {
                return await operation();
            } catch (error) {
                if (i === maxRetries - 1) throw error;
                await new Promise(resolve => setTimeout(resolve, delay * Math.pow(2, i)));
            }
        }
    }

    // Batch operations
    async batchRequest(requests) {
        const results = await Promise.allSettled(requests);
        return results.map((result, index) => ({
            index,
            success: result.status === 'fulfilled',
            data: result.status === 'fulfilled' ? result.value : null,
            error: result.status === 'rejected' ? result.reason : null
        }));
    }
}

// Initialize global API instance
window.API = new API();

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { API, APIClient, APIError };
}