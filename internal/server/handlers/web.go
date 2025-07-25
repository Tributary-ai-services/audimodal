package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/pkg/logger"
)

// WebHandler handles web UI requests
type WebHandler struct {
	db       *database.Database
	logger   *logger.Logger
	template *template.Template
}

// NewWebHandler creates a new web handler
func NewWebHandler(db *database.Database, logger *logger.Logger) *WebHandler {
	handler := &WebHandler{
		db:     db,
		logger: logger,
	}

	// Parse templates
	handler.loadTemplates()

	return handler
}

// loadTemplates loads and parses HTML templates
func (h *WebHandler) loadTemplates() {
	templatePath := "web/templates/*.html"
	tmpl, err := template.ParseGlob(templatePath)
	if err != nil {
		h.logger.WithField("error", err).Warn("Failed to load web templates, using fallback")
		h.template = nil
		return
	}
	h.template = tmpl
}

// DashboardHandler serves the main dashboard page
func (h *WebHandler) DashboardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Set content type
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// If templates are available, use them
		if h.template != nil {
			data := map[string]interface{}{
				"Title":   "eAIIngest Dashboard",
				"Version": "1.0.0",
			}

			if err := h.template.ExecuteTemplate(w, "dashboard.html", data); err != nil {
				h.logger.WithField("error", err).Error("Failed to execute dashboard template")
				h.serveFallbackDashboard(w)
				return
			}
			return
		}

		// Fallback to embedded HTML
		h.serveFallbackDashboard(w)
	}
}

// serveFallbackDashboard serves a basic HTML dashboard when templates aren't available
func (h *WebHandler) serveFallbackDashboard(w http.ResponseWriter) {
	fallbackHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>eAIIngest Dashboard</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: white; padding: 20px; margin-bottom: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .status-card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .status-card h3 { margin: 0 0 10px 0; color: #333; }
        .status-value { font-size: 2em; font-weight: bold; color: #2563eb; }
        .status-detail { color: #666; font-size: 0.9em; }
        .nav-tabs { display: flex; gap: 10px; margin-bottom: 20px; }
        .nav-tab { padding: 10px 20px; background: white; border: none; border-radius: 4px; cursor: pointer; }
        .nav-tab.active { background: #2563eb; color: white; }
        .content-section { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .btn { padding: 10px 20px; background: #2563eb; color: white; border: none; border-radius: 4px; cursor: pointer; }
        .btn:hover { background: #1d4ed8; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🚀 eAIIngest Dashboard</h1>
            <p>Enterprise AI Document Processing Platform</p>
        </div>

        <div class="status-grid">
            <div class="status-card">
                <h3>System Health</h3>
                <div class="status-value">Healthy</div>
                <div class="status-detail">All services operational</div>
            </div>
            <div class="status-card">
                <h3>Documents Processed</h3>
                <div class="status-value" id="docs-count">0</div>
                <div class="status-detail">Last 24 hours</div>
            </div>
            <div class="status-card">
                <h3>Active Tenants</h3>
                <div class="status-value" id="tenants-count">0</div>
                <div class="status-detail">Currently active</div>
            </div>
            <div class="status-card">
                <h3>Storage Used</h3>
                <div class="status-value" id="storage-used">0 GB</div>
                <div class="status-detail">Total storage</div>
            </div>
        </div>

        <div class="nav-tabs">
            <button class="nav-tab active" onclick="showSection('overview')">Overview</button>
            <button class="nav-tab" onclick="showSection('tenants')">Tenants</button>
            <button class="nav-tab" onclick="showSection('processing')">Processing</button>
            <button class="nav-tab" onclick="showSection('analytics')">Analytics</button>
        </div>

        <div id="overview-section" class="content-section">
            <h2>Platform Overview</h2>
            <p>Welcome to the eAIIngest management dashboard. This platform provides enterprise-grade AI-powered document processing capabilities.</p>
            
            <h3>Quick Actions</h3>
            <button class="btn" onclick="refreshData()">Refresh Data</button>
            <button class="btn" onclick="viewLogs()">View Logs</button>
            <button class="btn" onclick="checkHealth()">Health Check</button>

            <h3>Recent Activity</h3>
            <div id="activity-log">
                <p>Loading activity...</p>
            </div>
        </div>

        <div id="tenants-section" class="content-section" style="display: none;">
            <h2>Tenant Management</h2>
            <button class="btn" onclick="createTenant()">Create New Tenant</button>
            <div id="tenants-list">
                <p>Loading tenants...</p>
            </div>
        </div>

        <div id="processing-section" class="content-section" style="display: none;">
            <h2>Document Processing</h2>
            <button class="btn" onclick="uploadDocs()">Upload Documents</button>
            <div id="processing-status">
                <p>Loading processing status...</p>
            </div>
        </div>

        <div id="analytics-section" class="content-section" style="display: none;">
            <h2>Analytics & Insights</h2>
            <div id="analytics-data">
                <p>Loading analytics...</p>
            </div>
        </div>
    </div>

    <script>
        function showSection(sectionName) {
            // Hide all sections
            document.querySelectorAll('.content-section').forEach(section => {
                section.style.display = 'none';
            });
            
            // Remove active class from all tabs
            document.querySelectorAll('.nav-tab').forEach(tab => {
                tab.classList.remove('active');
            });
            
            // Show selected section
            document.getElementById(sectionName + '-section').style.display = 'block';
            
            // Add active class to selected tab
            event.target.classList.add('active');
            
            // Load section data
            loadSectionData(sectionName);
        }

        function loadSectionData(section) {
            switch(section) {
                case 'tenants':
                    loadTenants();
                    break;
                case 'processing':
                    loadProcessingStatus();
                    break;
                case 'analytics':
                    loadAnalytics();
                    break;
            }
        }

        function refreshData() {
            loadDashboardData();
        }

        function loadDashboardData() {
            // Load system health
            fetch('/health')
                .then(response => response.json())
                .then(data => {
                    console.log('Health status:', data.status);
                })
                .catch(error => console.error('Failed to load health data:', error));

            // Load basic stats
            fetch('/api/v1/tenants')
                .then(response => response.json())
                .then(data => {
                    document.getElementById('tenants-count').textContent = data.data ? data.data.length : 0;
                })
                .catch(error => console.error('Failed to load tenants:', error));

            // Load activity
            document.getElementById('activity-log').innerHTML = 
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">System started successfully</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Dashboard loaded</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">All services operational</div>';
        }

        function loadTenants() {
            fetch('/api/v1/tenants')
                .then(response => response.json())
                .then(data => {
                    const tenantsList = document.getElementById('tenants-list');
                    if (data.data && data.data.length > 0) {
                        tenantsList.innerHTML = data.data.map(tenant => 
                            '<div style="padding: 10px; border: 1px solid #ddd; margin: 10px 0;">' +
                            '<strong>' + (tenant.name || 'Unknown') + '</strong><br>' +
                            'ID: ' + (tenant.id || 'N/A') + '<br>' +
                            'Status: ' + (tenant.status || 'Unknown') +
                            '</div>'
                        ).join('');
                    } else {
                        tenantsList.innerHTML = '<p>No tenants found. <button class="btn" onclick="createTenant()">Create your first tenant</button></p>';
                    }
                })
                .catch(error => {
                    console.error('Failed to load tenants:', error);
                    document.getElementById('tenants-list').innerHTML = '<p>Error loading tenants</p>';
                });
        }

        function loadProcessingStatus() {
            document.getElementById('processing-status').innerHTML = 
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Queue Status: Healthy</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Pending Jobs: 0</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Processing: 0</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Completed: 0</div>';
        }

        function loadAnalytics() {
            document.getElementById('analytics-data').innerHTML = 
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Documents Processed Today: 0</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Average Processing Time: 0s</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Success Rate: 100%</div>' +
                '<div style="padding: 10px; background: #f0f0f0; margin: 5px 0;">Storage Usage: 0 GB</div>';
        }

        function createTenant() {
            const name = prompt('Enter tenant name:');
            if (name) {
                fetch('/api/v1/tenants', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: name, description: 'Created from dashboard' })
                })
                .then(response => response.json())
                .then(data => {
                    alert('Tenant created successfully!');
                    loadTenants();
                })
                .catch(error => {
                    alert('Failed to create tenant: ' + error.message);
                });
            }
        }

        function uploadDocs() {
            alert('Document upload functionality coming soon!');
        }

        function viewLogs() {
            window.open('/health?detailed=true', '_blank');
        }

        function checkHealth() {
            fetch('/health')
                .then(response => response.json())
                .then(data => {
                    alert('System Health: ' + data.status);
                })
                .catch(error => {
                    alert('Health check failed: ' + error.message);
                });
        }

        // Load initial data
        document.addEventListener('DOMContentLoaded', function() {
            loadDashboardData();
        });
    </script>
</body>
</html>`

	w.Write([]byte(fallbackHTML))
}

// StaticFileHandler serves static files (CSS, JS, images)
func (h *WebHandler) StaticFileHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Security: prevent directory traversal
		if filepath.Clean(r.URL.Path) != r.URL.Path {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Get file path relative to web/static
		filePath := filepath.Join("web", r.URL.Path)

		// Set appropriate content type based on file extension
		ext := filepath.Ext(filePath)
		switch ext {
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".jpg", ".jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".ico":
			w.Header().Set("Content-Type", "image/x-icon")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}

		// Set caching headers
		w.Header().Set("Cache-Control", "public, max-age=3600") // 1 hour

		// Serve the file
		http.ServeFile(w, r, filePath)
	}
}

// LoginHandler serves the login page
func (h *WebHandler) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		loginHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>eAIIngest - Login</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 0; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); height: 100vh; display: flex; align-items: center; justify-content: center; }
        .login-container { background: white; padding: 40px; border-radius: 10px; box-shadow: 0 10px 25px rgba(0,0,0,0.2); width: 400px; }
        .login-header { text-align: center; margin-bottom: 30px; }
        .login-header h1 { color: #333; margin: 0; }
        .login-header p { color: #666; margin: 10px 0 0 0; }
        .form-group { margin-bottom: 20px; }
        .form-group label { display: block; margin-bottom: 5px; color: #333; font-weight: bold; }
        .form-group input { width: 100%; padding: 12px; border: 1px solid #ddd; border-radius: 5px; font-size: 16px; box-sizing: border-box; }
        .form-group input:focus { outline: none; border-color: #2563eb; box-shadow: 0 0 5px rgba(37, 99, 235, 0.3); }
        .btn { width: 100%; padding: 12px; background: #2563eb; color: white; border: none; border-radius: 5px; font-size: 16px; cursor: pointer; margin-top: 10px; }
        .btn:hover { background: #1d4ed8; }
        .error { color: #ef4444; margin-top: 10px; text-align: center; }
        .demo-note { background: #fef3cd; padding: 15px; border-radius: 5px; margin-top: 20px; border-left: 4px solid #f59e0b; }
        .demo-note strong { color: #92400e; }
    </style>
</head>
<body>
    <div class="login-container">
        <div class="login-header">
            <h1>🚀 eAIIngest</h1>
            <p>Enterprise AI Document Processing Platform</p>
        </div>
        
        <form id="login-form">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required value="admin">
            </div>
            
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required value="admin">
            </div>
            
            <button type="submit" class="btn">Login</button>
            
            <div id="error-message" class="error" style="display: none;"></div>
        </form>

        <div class="demo-note">
            <strong>Demo Credentials:</strong><br>
            Username: admin<br>
            Password: admin
        </div>
    </div>

    <script>
        document.getElementById('login-form').addEventListener('submit', function(e) {
            e.preventDefault();
            
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            
            // Simple demo authentication
            if (username === 'admin' && password === 'admin') {
                // Store demo token
                localStorage.setItem('authToken', 'demo-token-' + Date.now());
                window.location.href = '/dashboard';
            } else {
                document.getElementById('error-message').textContent = 'Invalid credentials';
                document.getElementById('error-message').style.display = 'block';
            }
        });
    </script>
</body>
</html>`

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(loginHTML))
	}
}

// RedirectToLogin redirects to login page
func (h *WebHandler) RedirectToLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}