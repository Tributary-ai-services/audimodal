// eAIIngest Dashboard JavaScript

class Dashboard {
    constructor() {
        this.apiBaseUrl = '/api/v1';
        this.currentSection = 'dashboard';
        this.refreshInterval = null;
        this.init();
    }

    init() {
        this.bindEvents();
        this.showSection('dashboard');
        this.loadDashboardData();
        this.setupAutoRefresh();
        this.setupUploadArea();
    }

    bindEvents() {
        // Navigation
        document.querySelectorAll('.nav-link').forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const section = link.dataset.section;
                this.showSection(section);
            });
        });

        // Modal events
        document.getElementById('modal-overlay').addEventListener('click', (e) => {
            if (e.target.id === 'modal-overlay') {
                this.closeModal();
            }
        });

        // Form submissions
        document.getElementById('create-tenant-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.createTenant();
        });

        // File upload
        document.getElementById('file-input').addEventListener('change', (e) => {
            this.handleFileSelection(e.target.files);
        });

        // Settings timeframe change
        document.getElementById('analytics-timeframe').addEventListener('change', () => {
            this.updateAnalytics();
        });
    }

    showSection(sectionName) {
        // Update navigation
        document.querySelectorAll('.nav-link').forEach(link => {
            link.classList.remove('active');
        });
        document.querySelector(`[data-section="${sectionName}"]`).classList.add('active');

        // Update content
        document.querySelectorAll('.content-section').forEach(section => {
            section.classList.remove('active');
        });
        document.getElementById(`${sectionName}-section`).classList.add('active');

        this.currentSection = sectionName;

        // Load section-specific data
        this.loadSectionData(sectionName);
    }

    async loadSectionData(section) {
        try {
            switch (section) {
                case 'dashboard':
                    await this.loadDashboardData();
                    break;
                case 'tenants':
                    await this.loadTenantsData();
                    break;
                case 'processing':
                    await this.loadProcessingData();
                    break;
                case 'storage':
                    await this.loadStorageData();
                    break;
                case 'analytics':
                    await this.loadAnalyticsData();
                    break;
                case 'settings':
                    await this.loadSettingsData();
                    break;
            }
        } catch (error) {
            console.error(`Failed to load ${section} data:`, error);
            this.showToast(`Failed to load ${section} data`, 'error');
        }
    }

    async loadDashboardData() {
        this.showLoading();
        
        try {
            // Load system health
            const healthResponse = await fetch('/health');
            const healthData = await healthResponse.json();
            this.updateSystemHealth(healthData);

            // Load metrics
            const metricsResponse = await fetch('/metrics');
            const metricsText = await metricsResponse.text();
            this.updateDashboardMetrics(metricsText);

            // Load activity feed
            await this.loadActivityFeed();

            // Update charts
            this.updateDashboardCharts();

        } catch (error) {
            console.error('Failed to load dashboard data:', error);
            this.showToast('Failed to load dashboard data', 'error');
        } finally {
            this.hideLoading();
        }
    }

    updateSystemHealth(healthData) {
        const healthElement = document.getElementById('system-health');
        const statusClass = healthData.status === 'healthy' ? 'status-healthy' : 'status-unhealthy';
        
        healthElement.textContent = healthData.status;
        healthElement.className = `status-value ${statusClass}`;
    }

    updateDashboardMetrics(metricsText) {
        // Parse Prometheus-style metrics
        const metrics = this.parseMetrics(metricsText);
        
        // Update document count
        const docsProcessed = metrics['documents_processed_total'] || 0;
        document.getElementById('docs-processed').textContent = docsProcessed.toLocaleString();

        // Update tenant count
        const activeTenants = metrics['tenants_active'] || 0;
        document.getElementById('active-tenants').textContent = activeTenants;

        // Update storage
        const storageUsed = metrics['storage_used_bytes'] || 0;
        const storageGB = (storageUsed / (1024 * 1024 * 1024)).toFixed(2);
        document.getElementById('storage-used').textContent = `${storageGB} GB`;
    }

    parseMetrics(metricsText) {
        const metrics = {};
        const lines = metricsText.split('\n');
        
        lines.forEach(line => {
            if (line.startsWith('#') || !line.trim()) return;
            
            const parts = line.split(' ');
            if (parts.length >= 2) {
                const metricName = parts[0].split('{')[0];
                const value = parseFloat(parts[parts.length - 1]);
                metrics[metricName] = value;
            }
        });
        
        return metrics;
    }

    async loadActivityFeed() {
        const activities = [
            {
                icon: '📄',
                title: 'Document processed successfully',
                time: '2 minutes ago',
                type: 'success'
            },
            {
                icon: '👤',
                title: 'New tenant created: Acme Corp',
                time: '15 minutes ago',
                type: 'info'
            },
            {
                icon: '⚠️',
                title: 'DLP violation detected in document',
                time: '1 hour ago',
                type: 'warning'
            },
            {
                icon: '🔄',
                title: 'Batch processing completed',
                time: '2 hours ago',
                type: 'success'
            }
        ];

        const activityFeed = document.getElementById('activity-feed');
        activityFeed.innerHTML = activities.map(activity => `
            <div class="activity-item">
                <div class="activity-icon ${activity.type}">
                    ${activity.icon}
                </div>
                <div class="activity-content">
                    <div class="activity-title">${activity.title}</div>
                    <div class="activity-time">${activity.time}</div>
                </div>
            </div>
        `).join('');
    }

    async loadTenantsData() {
        try {
            const response = await fetch(`${this.apiBaseUrl}/tenants`);
            const data = await response.json();
            this.renderTenantsTable(data.data || []);
        } catch (error) {
            console.error('Failed to load tenants:', error);
            this.renderTenantsTable([]);
        }
    }

    renderTenantsTable(tenants) {
        const tbody = document.querySelector('#tenants-table tbody');
        
        if (tenants.length === 0) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="6" style="text-align: center; padding: 2rem; color: var(--text-muted);">
                        No tenants found. <a href="#" onclick="dashboard.showCreateTenantModal()" style="color: var(--primary-color);">Create your first tenant</a>
                    </td>
                </tr>
            `;
            return;
        }

        tbody.innerHTML = tenants.map(tenant => `
            <tr>
                <td>${tenant.name || 'N/A'}</td>
                <td><code>${tenant.id || 'N/A'}</code></td>
                <td>
                    <span class="status-badge ${tenant.status === 'active' ? 'success' : 'warning'}">
                        ${tenant.status || 'unknown'}
                    </span>
                </td>
                <td>${tenant.document_count || 0}</td>
                <td>${tenant.created_at ? new Date(tenant.created_at).toLocaleDateString() : 'N/A'}</td>
                <td>
                    <button class="btn btn-sm btn-secondary" onclick="dashboard.editTenant('${tenant.id}')">Edit</button>
                    <button class="btn btn-sm btn-danger" onclick="dashboard.deleteTenant('${tenant.id}')">Delete</button>
                </td>
            </tr>
        `).join('');
    }

    async loadProcessingData() {
        try {
            const response = await fetch(`${this.apiBaseUrl}/sessions`);
            const data = await response.json();
            this.renderProcessingSessions(data.data || []);
            this.updateQueueStats();
        } catch (error) {
            console.error('Failed to load processing data:', error);
        }
    }

    renderProcessingSessions(sessions) {
        const container = document.getElementById('active-sessions');
        
        if (sessions.length === 0) {
            container.innerHTML = '<p style="color: var(--text-muted); text-align: center;">No active sessions</p>';
            return;
        }

        container.innerHTML = sessions.map(session => `
            <div class="session-item">
                <div class="session-header">
                    <strong>${session.name || session.id}</strong>
                    <span class="session-status ${session.status}">${session.status}</span>
                </div>
                <div class="session-details">
                    <span>Files: ${session.file_count || 0}</span>
                    <span>Progress: ${session.progress || 0}%</span>
                </div>
            </div>
        `).join('');
    }

    updateQueueStats() {
        // Simulate queue statistics
        document.getElementById('queue-pending').textContent = '12';
        document.getElementById('queue-processing').textContent = '3';
        document.getElementById('queue-completed').textContent = '156';
        document.getElementById('queue-failed').textContent = '2';
    }

    async loadStorageData() {
        try {
            // Load vector store stats
            document.getElementById('vector-datasets').textContent = '5';
            document.getElementById('vector-count').textContent = '12,547';
            document.getElementById('vector-size').textContent = '245 MB';

            // Load document store stats
            document.getElementById('doc-files').textContent = '1,234';
            document.getElementById('doc-chunks').textContent = '45,678';
            document.getElementById('doc-size').textContent = '12.5 GB';

            // Load cloud storage stats
            document.getElementById('s3-buckets').textContent = '3';
            document.getElementById('gcs-buckets').textContent = '1';
            document.getElementById('cloud-objects').textContent = '5,432';

        } catch (error) {
            console.error('Failed to load storage data:', error);
        }
    }

    async loadAnalyticsData() {
        this.updateAnalytics();
    }

    updateAnalytics() {
        const timeframe = document.getElementById('analytics-timeframe').value;
        
        // Update performance metrics
        document.getElementById('avg-processing-time').textContent = '2.3s';
        document.getElementById('success-rate').textContent = '98.5%';
        document.getElementById('throughput').textContent = '145 docs/min';

        // Update charts would go here
        this.updateAnalyticsCharts(timeframe);
    }

    async loadSettingsData() {
        try {
            // Load current configuration
            // This would typically come from an API endpoint
            const config = {
                log_level: 'info',
                max_request_size: 10,
                rate_limit_rps: 100,
                tls_enabled: true,
                auth_enabled: true,
                jwt_expiration: '24h',
                chunk_size: 1000,
                chunk_overlap: 200,
                dlp_enabled: true
            };

            this.populateSettings(config);
        } catch (error) {
            console.error('Failed to load settings:', error);
        }
    }

    populateSettings(config) {
        // Populate server config
        document.getElementById('log-level').value = config.log_level;
        document.getElementById('max-request-size').value = config.max_request_size;
        document.getElementById('rate-limit').value = config.rate_limit_rps;

        // Populate security config
        document.getElementById('tls-enabled').checked = config.tls_enabled;
        document.getElementById('auth-enabled').checked = config.auth_enabled;
        document.getElementById('jwt-expiration').value = config.jwt_expiration;

        // Populate processing config
        document.getElementById('chunk-size').value = config.chunk_size;
        document.getElementById('chunk-overlap').value = config.chunk_overlap;
        document.getElementById('dlp-enabled').checked = config.dlp_enabled;
    }

    // Modal management
    showCreateTenantModal() {
        document.getElementById('modal-overlay').style.display = 'flex';
        document.getElementById('create-tenant-modal').style.display = 'block';
    }

    showUploadModal() {
        document.getElementById('modal-overlay').style.display = 'flex';
        document.getElementById('upload-modal').style.display = 'block';
    }

    closeModal() {
        document.getElementById('modal-overlay').style.display = 'none';
        document.querySelectorAll('.modal').forEach(modal => {
            modal.style.display = 'none';
        });
    }

    // Tenant management
    async createTenant() {
        const form = document.getElementById('create-tenant-form');
        const formData = new FormData(form);
        const tenantData = Object.fromEntries(formData);

        try {
            const response = await fetch(`${this.apiBaseUrl}/tenants`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(tenantData)
            });

            if (response.ok) {
                this.showToast('Tenant created successfully', 'success');
                this.closeModal();
                form.reset();
                this.loadTenantsData();
            } else {
                const error = await response.json();
                this.showToast(error.message || 'Failed to create tenant', 'error');
            }
        } catch (error) {
            console.error('Failed to create tenant:', error);
            this.showToast('Failed to create tenant', 'error');
        }
    }

    async editTenant(tenantId) {
        this.showToast('Edit tenant functionality coming soon', 'info');
    }

    async deleteTenant(tenantId) {
        if (!confirm('Are you sure you want to delete this tenant?')) {
            return;
        }

        try {
            const response = await fetch(`${this.apiBaseUrl}/tenants/${tenantId}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                this.showToast('Tenant deleted successfully', 'success');
                this.loadTenantsData();
            } else {
                const error = await response.json();
                this.showToast(error.message || 'Failed to delete tenant', 'error');
            }
        } catch (error) {
            console.error('Failed to delete tenant:', error);
            this.showToast('Failed to delete tenant', 'error');
        }
    }

    // File upload
    setupUploadArea() {
        const uploadArea = document.getElementById('upload-area');
        
        uploadArea.addEventListener('dragover', (e) => {
            e.preventDefault();
            uploadArea.classList.add('dragover');
        });

        uploadArea.addEventListener('dragleave', () => {
            uploadArea.classList.remove('dragover');
        });

        uploadArea.addEventListener('drop', (e) => {
            e.preventDefault();
            uploadArea.classList.remove('dragover');
            this.handleFileSelection(e.dataTransfer.files);
        });
    }

    selectFiles() {
        document.getElementById('file-input').click();
    }

    handleFileSelection(files) {
        if (files.length === 0) return;

        this.selectedFiles = Array.from(files);
        document.getElementById('upload-btn').disabled = false;
        
        const fileList = this.selectedFiles.map(file => file.name).join(', ');
        document.querySelector('#upload-area p').textContent = `Selected: ${fileList}`;
    }

    async uploadFiles() {
        if (!this.selectedFiles || this.selectedFiles.length === 0) return;

        const formData = new FormData();
        this.selectedFiles.forEach(file => {
            formData.append('files', file);
        });

        const progressArea = document.getElementById('upload-progress');
        const progressFill = document.getElementById('progress-fill');
        const uploadStatus = document.getElementById('upload-status');

        progressArea.style.display = 'block';
        document.getElementById('upload-btn').disabled = true;

        try {
            const xhr = new XMLHttpRequest();
            
            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    const percentComplete = (e.loaded / e.total) * 100;
                    progressFill.style.width = `${percentComplete}%`;
                    uploadStatus.textContent = `Uploading... ${Math.round(percentComplete)}%`;
                }
            });

            xhr.addEventListener('load', () => {
                if (xhr.status === 200) {
                    this.showToast('Files uploaded successfully', 'success');
                    this.closeModal();
                    this.loadProcessingData();
                } else {
                    this.showToast('Upload failed', 'error');
                }
                progressArea.style.display = 'none';
                document.getElementById('upload-btn').disabled = false;
            });

            xhr.addEventListener('error', () => {
                this.showToast('Upload failed', 'error');
                progressArea.style.display = 'none';
                document.getElementById('upload-btn').disabled = false;
            });

            xhr.open('POST', `${this.apiBaseUrl}/files/upload`);
            xhr.send(formData);

        } catch (error) {
            console.error('Upload failed:', error);
            this.showToast('Upload failed', 'error');
            progressArea.style.display = 'none';
            document.getElementById('upload-btn').disabled = false;
        }
    }

    // Settings
    async saveSettings() {
        const serverConfig = new FormData(document.getElementById('server-config-form'));
        const securityConfig = new FormData(document.getElementById('security-config-form'));
        const processingConfig = new FormData(document.getElementById('processing-config-form'));

        const settings = {
            ...Object.fromEntries(serverConfig),
            ...Object.fromEntries(securityConfig),
            ...Object.fromEntries(processingConfig)
        };

        try {
            const response = await fetch(`${this.apiBaseUrl}/settings`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(settings)
            });

            if (response.ok) {
                this.showToast('Settings saved successfully', 'success');
            } else {
                this.showToast('Failed to save settings', 'error');
            }
        } catch (error) {
            console.error('Failed to save settings:', error);
            this.showToast('Failed to save settings', 'error');
        }
    }

    // Utility functions
    showLoading() {
        document.getElementById('loading-spinner').style.display = 'flex';
    }

    hideLoading() {
        document.getElementById('loading-spinner').style.display = 'none';
    }

    showToast(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
            <div style="display: flex; justify-content: space-between; align-items: center;">
                <span>${message}</span>
                <button onclick="this.parentElement.parentElement.remove()" style="background: none; border: none; font-size: 18px; cursor: pointer;">&times;</button>
            </div>
        `;

        document.getElementById('toast-container').appendChild(toast);

        // Auto-remove after 5 seconds
        setTimeout(() => {
            if (toast.parentElement) {
                toast.remove();
            }
        }, 5000);
    }

    setupAutoRefresh() {
        // Refresh dashboard data every 30 seconds
        this.refreshInterval = setInterval(() => {
            if (this.currentSection === 'dashboard') {
                this.loadDashboardData();
            }
        }, 30000);
    }

    refreshDashboard() {
        this.loadDashboardData();
    }

    analyzeStorage() {
        this.showToast('Storage analysis started', 'info');
        // Implement storage analysis functionality
    }

    updateDashboardCharts() {
        // Charts will be implemented in charts.js
        if (window.Charts) {
            window.Charts.updateDashboardCharts();
        }
    }

    updateAnalyticsCharts(timeframe) {
        // Charts will be implemented in charts.js
        if (window.Charts) {
            window.Charts.updateAnalyticsCharts(timeframe);
        }
    }

    logout() {
        if (confirm('Are you sure you want to logout?')) {
            // Clear any stored tokens
            localStorage.removeItem('authToken');
            sessionStorage.removeItem('authToken');
            
            // Redirect to login page
            window.location.href = '/login';
        }
    }
}

// Global functions for onclick handlers
window.dashboard = null;

function showCreateTenantModal() {
    dashboard.showCreateTenantModal();
}

function showUploadModal() {
    dashboard.showUploadModal();
}

function closeModal() {
    dashboard.closeModal();
}

function createTenant() {
    dashboard.createTenant();
}

function selectFiles() {
    dashboard.selectFiles();
}

function uploadFiles() {
    dashboard.uploadFiles();
}

function refreshDashboard() {
    dashboard.refreshDashboard();
}

function analyzeStorage() {
    dashboard.analyzeStorage();
}

function updateAnalytics() {
    dashboard.updateAnalytics();
}

function saveSettings() {
    dashboard.saveSettings();
}

function logout() {
    dashboard.logout();
}

// Initialize dashboard when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.dashboard = new Dashboard();
});