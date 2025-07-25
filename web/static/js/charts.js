// Charts functionality for eAIIngest Dashboard

class Charts {
    constructor() {
        this.charts = {};
        this.init();
    }

    init() {
        // Initialize charts after a short delay to ensure DOM is ready
        setTimeout(() => {
            this.initDashboardCharts();
            this.initAnalyticsCharts();
        }, 100);
    }

    initDashboardCharts() {
        this.createQueueChart();
        this.createTypesChart();
        this.createPerformanceChart();
    }

    initAnalyticsCharts() {
        this.createTrendsChart();
        this.createClassificationChart();
        this.createDLPChart();
    }

    createQueueChart() {
        const canvas = document.getElementById('queue-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        
        // Simple queue status visualization
        const data = [12, 3, 156, 2]; // pending, processing, completed, failed
        const labels = ['Pending', 'Processing', 'Completed', 'Failed'];
        const colors = ['#f59e0b', '#06b6d4', '#10b981', '#ef4444'];

        this.drawBarChart(ctx, data, labels, colors, 'Processing Queue Status');
        this.charts.queue = { ctx, data, labels, colors };
    }

    createTypesChart() {
        const canvas = document.getElementById('types-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        
        // Document types distribution
        const data = [45, 30, 15, 10]; // PDF, DOCX, TXT, Other
        const labels = ['PDF', 'DOCX', 'TXT', 'Other'];
        const colors = ['#2563eb', '#10b981', '#f59e0b', '#64748b'];

        this.drawPieChart(ctx, data, labels, colors, 'Document Types');
        this.charts.types = { ctx, data, labels, colors };
    }

    createPerformanceChart() {
        const canvas = document.getElementById('performance-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        
        // System performance over time
        const timeData = this.generateTimeSeriesData(24, 50, 100); // 24 hours, range 50-100
        const labels = Array.from({length: 24}, (_, i) => `${i}:00`);

        this.drawLineChart(ctx, timeData, labels, '#2563eb', 'CPU Usage (%)');
        this.charts.performance = { ctx, data: timeData, labels };
    }

    createTrendsChart() {
        const canvas = document.getElementById('trends-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        
        // Processing trends over the last 7 days
        const documentsData = this.generateTimeSeriesData(7, 100, 500);
        const chunksData = this.generateTimeSeriesData(7, 500, 2000);
        const labels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

        this.drawMultiLineChart(ctx, [documentsData, chunksData], labels, 
            ['#2563eb', '#10b981'], ['Documents', 'Chunks'], 'Processing Trends');
        this.charts.trends = { ctx, documentsData, chunksData, labels };
    }

    createClassificationChart() {
        const canvas = document.getElementById('classification-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        
        // Content classification distribution
        const data = [35, 25, 20, 15, 5]; // Financial, Legal, Technical, Marketing, Other
        const labels = ['Financial', 'Legal', 'Technical', 'Marketing', 'Other'];
        const colors = ['#2563eb', '#10b981', '#f59e0b', '#ef4444', '#64748b'];

        this.drawPieChart(ctx, data, labels, colors, 'Content Classification');
        this.charts.classification = { ctx, data, labels, colors };
    }

    createDLPChart() {
        const canvas = document.getElementById('dlp-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        
        // DLP violations by type
        const data = [5, 3, 2, 1]; // PII, Credit Card, SSN, Other
        const labels = ['PII', 'Credit Card', 'SSN', 'Other'];
        const colors = ['#ef4444', '#f59e0b', '#10b981', '#64748b'];

        this.drawBarChart(ctx, data, labels, colors, 'DLP Violations');
        this.charts.dlp = { ctx, data, labels, colors };
    }

    // Chart drawing utilities
    drawBarChart(ctx, data, labels, colors, title) {
        const canvas = ctx.canvas;
        const width = canvas.width;
        const height = canvas.height;
        const padding = 40;
        const chartWidth = width - 2 * padding;
        const chartHeight = height - 2 * padding - 40; // Extra space for title

        // Clear canvas
        ctx.clearRect(0, 0, width, height);
        
        // Set font
        ctx.font = '14px Inter, sans-serif';
        ctx.textAlign = 'center';

        // Draw title
        ctx.fillStyle = '#1e293b';
        ctx.fillText(title, width / 2, 20);

        // Calculate bar dimensions
        const maxValue = Math.max(...data);
        const barWidth = chartWidth / data.length * 0.8;
        const barSpacing = chartWidth / data.length * 0.2;

        // Draw bars
        data.forEach((value, index) => {
            const barHeight = (value / maxValue) * chartHeight;
            const x = padding + index * (barWidth + barSpacing) + barSpacing / 2;
            const y = padding + 30 + chartHeight - barHeight;

            // Draw bar
            ctx.fillStyle = colors[index];
            ctx.fillRect(x, y, barWidth, barHeight);

            // Draw value on top of bar
            ctx.fillStyle = '#1e293b';
            ctx.font = '12px Inter, sans-serif';
            ctx.fillText(value.toString(), x + barWidth / 2, y - 5);

            // Draw label
            ctx.fillText(labels[index], x + barWidth / 2, height - 10);
        });
    }

    drawPieChart(ctx, data, labels, colors, title) {
        const canvas = ctx.canvas;
        const width = canvas.width;
        const height = canvas.height;
        const centerX = width / 2;
        const centerY = height / 2 + 10; // Offset for title
        const radius = Math.min(width, height) / 3;

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Draw title
        ctx.font = '14px Inter, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#1e293b';
        ctx.fillText(title, centerX, 20);

        // Calculate total and angles
        const total = data.reduce((sum, value) => sum + value, 0);
        let currentAngle = -Math.PI / 2; // Start from top

        // Draw pie slices
        data.forEach((value, index) => {
            const sliceAngle = (value / total) * 2 * Math.PI;
            
            // Draw slice
            ctx.beginPath();
            ctx.moveTo(centerX, centerY);
            ctx.arc(centerX, centerY, radius, currentAngle, currentAngle + sliceAngle);
            ctx.closePath();
            ctx.fillStyle = colors[index];
            ctx.fill();

            // Draw label
            const labelAngle = currentAngle + sliceAngle / 2;
            const labelX = centerX + Math.cos(labelAngle) * (radius + 20);
            const labelY = centerY + Math.sin(labelAngle) * (radius + 20);
            
            ctx.fillStyle = '#1e293b';
            ctx.font = '12px Inter, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText(`${labels[index]} (${value})`, labelX, labelY);

            currentAngle += sliceAngle;
        });
    }

    drawLineChart(ctx, data, labels, color, title) {
        const canvas = ctx.canvas;
        const width = canvas.width;
        const height = canvas.height;
        const padding = 40;
        const chartWidth = width - 2 * padding;
        const chartHeight = height - 2 * padding - 40;

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Draw title
        ctx.font = '14px Inter, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#1e293b';
        ctx.fillText(title, width / 2, 20);

        // Calculate scaling
        const maxValue = Math.max(...data);
        const minValue = Math.min(...data);
        const valueRange = maxValue - minValue || 1;

        // Draw axes
        ctx.strokeStyle = '#e2e8f0';
        ctx.lineWidth = 1;
        
        // Y-axis
        ctx.beginPath();
        ctx.moveTo(padding, padding + 30);
        ctx.lineTo(padding, padding + 30 + chartHeight);
        ctx.stroke();

        // X-axis
        ctx.beginPath();
        ctx.moveTo(padding, padding + 30 + chartHeight);
        ctx.lineTo(padding + chartWidth, padding + 30 + chartHeight);
        ctx.stroke();

        // Draw line
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.beginPath();

        data.forEach((value, index) => {
            const x = padding + (index / (data.length - 1)) * chartWidth;
            const y = padding + 30 + chartHeight - ((value - minValue) / valueRange) * chartHeight;
            
            if (index === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        });

        ctx.stroke();

        // Draw points
        ctx.fillStyle = color;
        data.forEach((value, index) => {
            const x = padding + (index / (data.length - 1)) * chartWidth;
            const y = padding + 30 + chartHeight - ((value - minValue) / valueRange) * chartHeight;
            
            ctx.beginPath();
            ctx.arc(x, y, 3, 0, 2 * Math.PI);
            ctx.fill();
        });

        // Draw labels
        ctx.fillStyle = '#64748b';
        ctx.font = '10px Inter, sans-serif';
        ctx.textAlign = 'center';
        
        labels.forEach((label, index) => {
            const x = padding + (index / (data.length - 1)) * chartWidth;
            ctx.fillText(label, x, height - 10);
        });
    }

    drawMultiLineChart(ctx, datasets, labels, colors, lineLabels, title) {
        const canvas = ctx.canvas;
        const width = canvas.width;
        const height = canvas.height;
        const padding = 40;
        const chartWidth = width - 2 * padding;
        const chartHeight = height - 2 * padding - 60; // Extra space for legend

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Draw title
        ctx.font = '14px Inter, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#1e293b';
        ctx.fillText(title, width / 2, 20);

        // Calculate scaling
        const allValues = datasets.flat();
        const maxValue = Math.max(...allValues);
        const minValue = Math.min(...allValues);
        const valueRange = maxValue - minValue || 1;

        // Draw axes
        ctx.strokeStyle = '#e2e8f0';
        ctx.lineWidth = 1;
        
        // Y-axis
        ctx.beginPath();
        ctx.moveTo(padding, padding + 30);
        ctx.lineTo(padding, padding + 30 + chartHeight);
        ctx.stroke();

        // X-axis
        ctx.beginPath();
        ctx.moveTo(padding, padding + 30 + chartHeight);
        ctx.lineTo(padding + chartWidth, padding + 30 + chartHeight);
        ctx.stroke();

        // Draw lines
        datasets.forEach((data, datasetIndex) => {
            ctx.strokeStyle = colors[datasetIndex];
            ctx.lineWidth = 2;
            ctx.beginPath();

            data.forEach((value, index) => {
                const x = padding + (index / (data.length - 1)) * chartWidth;
                const y = padding + 30 + chartHeight - ((value - minValue) / valueRange) * chartHeight;
                
                if (index === 0) {
                    ctx.moveTo(x, y);
                } else {
                    ctx.lineTo(x, y);
                }
            });

            ctx.stroke();

            // Draw points
            ctx.fillStyle = colors[datasetIndex];
            data.forEach((value, index) => {
                const x = padding + (index / (data.length - 1)) * chartWidth;
                const y = padding + 30 + chartHeight - ((value - minValue) / valueRange) * chartHeight;
                
                ctx.beginPath();
                ctx.arc(x, y, 3, 0, 2 * Math.PI);
                ctx.fill();
            });
        });

        // Draw legend
        ctx.font = '12px Inter, sans-serif';
        ctx.textAlign = 'left';
        lineLabels.forEach((label, index) => {
            const x = padding + index * 100;
            const y = height - 20;
            
            // Draw color indicator
            ctx.fillStyle = colors[index];
            ctx.fillRect(x, y - 8, 12, 12);
            
            // Draw label
            ctx.fillStyle = '#1e293b';
            ctx.fillText(label, x + 20, y);
        });

        // Draw x-axis labels
        ctx.fillStyle = '#64748b';
        ctx.font = '10px Inter, sans-serif';
        ctx.textAlign = 'center';
        
        labels.forEach((label, index) => {
            const x = padding + (index / (labels.length - 1)) * chartWidth;
            ctx.fillText(label, x, padding + 30 + chartHeight + 15);
        });
    }

    generateTimeSeriesData(points, min, max) {
        const data = [];
        let current = min + (max - min) * Math.random();
        
        for (let i = 0; i < points; i++) {
            // Add some trend and noise
            const trend = (Math.random() - 0.5) * 10;
            const noise = (Math.random() - 0.5) * 20;
            current = Math.max(min, Math.min(max, current + trend + noise));
            data.push(Math.round(current));
        }
        
        return data;
    }

    updateDashboardCharts() {
        // Regenerate data and redraw charts
        if (this.charts.queue) {
            const newData = [
                Math.floor(Math.random() * 20) + 5,
                Math.floor(Math.random() * 10) + 1,
                Math.floor(Math.random() * 200) + 100,
                Math.floor(Math.random() * 5)
            ];
            this.drawBarChart(this.charts.queue.ctx, newData, 
                this.charts.queue.labels, this.charts.queue.colors, 'Processing Queue Status');
        }

        if (this.charts.performance) {
            const newData = this.generateTimeSeriesData(24, 50, 100);
            this.drawLineChart(this.charts.performance.ctx, newData, 
                this.charts.performance.labels, '#2563eb', 'CPU Usage (%)');
        }
    }

    updateAnalyticsCharts(timeframe) {
        let points;
        let labels;
        
        switch (timeframe) {
            case '24h':
                points = 24;
                labels = Array.from({length: 24}, (_, i) => `${i}:00`);
                break;
            case '7d':
                points = 7;
                labels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
                break;
            case '30d':
                points = 30;
                labels = Array.from({length: 30}, (_, i) => `${i+1}`);
                break;
            case '90d':
                points = 90;
                labels = Array.from({length: 90}, (_, i) => `${i+1}`);
                break;
            default:
                points = 7;
                labels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
        }

        if (this.charts.trends) {
            const documentsData = this.generateTimeSeriesData(points, 100, 500);
            const chunksData = this.generateTimeSeriesData(points, 500, 2000);
            
            this.drawMultiLineChart(this.charts.trends.ctx, [documentsData, chunksData], 
                labels, ['#2563eb', '#10b981'], ['Documents', 'Chunks'], 'Processing Trends');
        }
    }
}

// Initialize charts when the page loads
window.Charts = null;
document.addEventListener('DOMContentLoaded', () => {
    window.Charts = new Charts();
});