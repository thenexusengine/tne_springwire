// Package endpoints provides HTTP endpoint handlers
package endpoints

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// DashboardMetrics holds real-time metrics for the dashboard
type DashboardMetrics struct {
	mu                 sync.RWMutex
	TotalAuctions      int64
	SuccessfulAuctions int64
	FailedAuctions     int64
	TotalBids          int64
	TotalImpressions   int64
	RecentAuctions     []AuctionLog
	BidderStats        map[string]int // Count of wins per bidder
	AverageDuration    float64
	LastUpdate         time.Time
	StartTime          time.Time
}

// AuctionLog represents a single auction event
type AuctionLog struct {
	Timestamp      time.Time `json:"timestamp"`
	RequestID      string    `json:"request_id"`
	ImpCount       int       `json:"imp_count"`
	BidCount       int       `json:"bid_count"`
	WinningBidders []string  `json:"winning_bidders"`
	Duration       int64     `json:"duration_ms"`
	Success        bool      `json:"success"`
	Error          string    `json:"error,omitempty"`
}

var globalMetrics = &DashboardMetrics{
	BidderStats:    make(map[string]int),
	RecentAuctions: make([]AuctionLog, 0, 100),
	StartTime:      time.Now(),
	LastUpdate:     time.Now(),
}

// LogAuction records an auction for dashboard metrics
func LogAuction(requestID string, impCount, bidCount int, winningBidders []string, duration time.Duration, success bool, err error) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	globalMetrics.TotalAuctions++
	globalMetrics.TotalImpressions += int64(impCount)
	globalMetrics.TotalBids += int64(bidCount)

	if success {
		globalMetrics.SuccessfulAuctions++
	} else {
		globalMetrics.FailedAuctions++
	}

	// Update bidder stats
	for _, bidder := range winningBidders {
		globalMetrics.BidderStats[bidder]++
	}

	// Calculate rolling average duration
	totalDuration := globalMetrics.AverageDuration * float64(globalMetrics.TotalAuctions-1)
	globalMetrics.AverageDuration = (totalDuration + float64(duration.Milliseconds())) / float64(globalMetrics.TotalAuctions)

	// Add to recent auctions (keep last 100)
	auctionLog := AuctionLog{
		Timestamp:      time.Now(),
		RequestID:      requestID,
		ImpCount:       impCount,
		BidCount:       bidCount,
		WinningBidders: winningBidders,
		Duration:       duration.Milliseconds(),
		Success:        success,
	}
	if err != nil {
		auctionLog.Error = err.Error()
	}

	globalMetrics.RecentAuctions = append([]AuctionLog{auctionLog}, globalMetrics.RecentAuctions...)
	if len(globalMetrics.RecentAuctions) > 100 {
		globalMetrics.RecentAuctions = globalMetrics.RecentAuctions[:100]
	}

	globalMetrics.LastUpdate = time.Now()
}

// DashboardHandler serves the live dashboard HTML
type DashboardHandler struct{}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTemplate.Execute(w, nil); err != nil {
		logger.Log.Error().Err(err).Msg("failed to render dashboard template")
	}
}

// MetricsAPIHandler serves metrics as JSON for the dashboard
type MetricsAPIHandler struct{}

// NewMetricsAPIHandler creates a new metrics API handler
func NewMetricsAPIHandler() *MetricsAPIHandler {
	return &MetricsAPIHandler{}
}

func (h *MetricsAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	uptime := time.Since(globalMetrics.StartTime)

	response := map[string]interface{}{
		"total_auctions":      globalMetrics.TotalAuctions,
		"successful_auctions": globalMetrics.SuccessfulAuctions,
		"failed_auctions":     globalMetrics.FailedAuctions,
		"total_bids":          globalMetrics.TotalBids,
		"total_impressions":   globalMetrics.TotalImpressions,
		"recent_auctions":     globalMetrics.RecentAuctions[:min(20, len(globalMetrics.RecentAuctions))],
		"bidder_stats":        globalMetrics.BidderStats,
		"average_duration":    globalMetrics.AverageDuration,
		"last_update":         globalMetrics.LastUpdate.Format(time.RFC3339),
		"uptime_seconds":      int64(uptime.Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Log.Error().Err(err).Msg("failed to encode metrics response")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Nexus Exchange - Live Dashboard</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0f172a;
            color: #e2e8f0;
            padding: 2rem;
        }
        .header {
            text-align: center;
            margin-bottom: 2rem;
        }
        .header h1 {
            font-size: 2rem;
            font-weight: 700;
            background: linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 0.5rem;
        }
        .header .subtitle {
            color: #94a3b8;
            font-size: 0.875rem;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: #1e293b;
            border-radius: 0.75rem;
            padding: 1.5rem;
            border: 1px solid #334155;
        }
        .stat-card .label {
            color: #94a3b8;
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.5rem;
        }
        .stat-card .value {
            font-size: 2rem;
            font-weight: 700;
            color: #3b82f6;
        }
        .stat-card.success .value { color: #10b981; }
        .stat-card.error .value { color: #ef4444; }
        .stat-card.warning .value { color: #f59e0b; }
        .section {
            background: #1e293b;
            border-radius: 0.75rem;
            padding: 1.5rem;
            margin-bottom: 1.5rem;
            border: 1px solid #334155;
        }
        .section h2 {
            font-size: 1.25rem;
            margin-bottom: 1rem;
            color: #f1f5f9;
        }
        .auctions-list {
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
            max-height: 600px;
            overflow-y: auto;
        }
        .auction-item {
            background: #0f172a;
            border-radius: 0.5rem;
            padding: 1rem;
            border: 1px solid #334155;
            display: grid;
            grid-template-columns: auto 1fr auto;
            gap: 1rem;
            align-items: center;
        }
        .auction-item.success { border-left: 3px solid #10b981; }
        .auction-item.error { border-left: 3px solid #ef4444; }
        .auction-time {
            color: #64748b;
            font-size: 0.75rem;
            font-family: 'Courier New', monospace;
        }
        .auction-info {
            display: flex;
            flex-direction: column;
            gap: 0.25rem;
        }
        .auction-id {
            font-family: 'Courier New', monospace;
            font-size: 0.875rem;
            color: #94a3b8;
        }
        .auction-metrics {
            display: flex;
            gap: 1rem;
            font-size: 0.75rem;
            color: #64748b;
        }
        .auction-bidders {
            display: flex;
            flex-wrap: wrap;
            gap: 0.25rem;
        }
        .bidder-tag {
            background: #1e293b;
            color: #3b82f6;
            padding: 0.25rem 0.5rem;
            border-radius: 0.25rem;
            font-size: 0.75rem;
            border: 1px solid #334155;
        }
        .auction-duration {
            font-weight: 600;
            color: #94a3b8;
        }
        .auction-duration.fast { color: #10b981; }
        .auction-duration.slow { color: #f59e0b; }
        .bidder-stats {
            display: flex;
            flex-wrap: wrap;
            gap: 0.5rem;
        }
        .bidder-stat {
            background: #0f172a;
            padding: 0.75rem 1rem;
            border-radius: 0.5rem;
            border: 1px solid #334155;
        }
        .bidder-stat .name {
            color: #94a3b8;
            font-size: 0.875rem;
            margin-bottom: 0.25rem;
        }
        .bidder-stat .count {
            color: #3b82f6;
            font-size: 1.5rem;
            font-weight: 700;
        }
        .pulse {
            display: inline-block;
            width: 8px;
            height: 8px;
            background: #10b981;
            border-radius: 50%;
            margin-right: 0.5rem;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .error-msg {
            color: #ef4444;
            font-size: 0.75rem;
            margin-top: 0.25rem;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>The Nexus Engine</h1>
        <p class="subtitle"><span class="pulse"></span>Live Auction Dashboard</p>
    </div>

    <div class="stats-grid">
        <div class="stat-card">
            <div class="label">Total Auctions</div>
            <div class="value" id="total-auctions">0</div>
        </div>
        <div class="stat-card success">
            <div class="label">Successful</div>
            <div class="value" id="successful-auctions">0</div>
        </div>
        <div class="stat-card error">
            <div class="label">Failed</div>
            <div class="value" id="failed-auctions">0</div>
        </div>
        <div class="stat-card">
            <div class="label">Total Bids</div>
            <div class="value" id="total-bids">0</div>
        </div>
        <div class="stat-card warning">
            <div class="label">Avg Duration</div>
            <div class="value" id="avg-duration">0ms</div>
        </div>
        <div class="stat-card">
            <div class="label">Uptime</div>
            <div class="value" id="uptime">0s</div>
        </div>
    </div>

    <div class="section">
        <h2>Top Bidders</h2>
        <div class="bidder-stats" id="bidder-stats">
            <div style="color: #64748b; font-size: 0.875rem;">Waiting for auction data...</div>
        </div>
    </div>

    <div class="section">
        <h2>Recent Auctions</h2>
        <div class="auctions-list" id="auctions-list">
            <div style="color: #64748b; font-size: 0.875rem; text-align: center; padding: 2rem;">
                Waiting for auction data...
            </div>
        </div>
    </div>

    <script>
        function formatUptime(seconds) {
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            const secs = seconds % 60;
            if (hours > 0) return hours + 'h ' + minutes + 'm';
            if (minutes > 0) return minutes + 'm ' + secs + 's';
            return secs + 's';
        }

        function formatTime(timestamp) {
            const date = new Date(timestamp);
            return date.toLocaleTimeString('en-US', { hour12: false });
        }

        async function updateDashboard() {
            try {
                const response = await fetch('/admin/metrics');
                const data = await response.json();

                document.getElementById('total-auctions').textContent = data.total_auctions;
                document.getElementById('successful-auctions').textContent = data.successful_auctions;
                document.getElementById('failed-auctions').textContent = data.failed_auctions;
                document.getElementById('total-bids').textContent = data.total_bids;
                document.getElementById('avg-duration').textContent = Math.round(data.average_duration) + 'ms';
                document.getElementById('uptime').textContent = formatUptime(data.uptime_seconds);

                // Update bidder stats
                const bidderStatsEl = document.getElementById('bidder-stats');
                const bidderEntries = Object.entries(data.bidder_stats || {}).sort((a, b) => b[1] - a[1]);
                if (bidderEntries.length > 0) {
                    bidderStatsEl.innerHTML = bidderEntries.map(([name, count]) =>
                        '<div class="bidder-stat"><div class="name">' + name + '</div><div class="count">' + count + '</div></div>'
                    ).join('');
                } else {
                    bidderStatsEl.innerHTML = '<div style="color: #64748b; font-size: 0.875rem;">No bids yet</div>';
                }

                // Update recent auctions
                const auctionsEl = document.getElementById('auctions-list');
                if (data.recent_auctions && data.recent_auctions.length > 0) {
                    auctionsEl.innerHTML = data.recent_auctions.map(auction => {
                        const statusClass = auction.success ? 'success' : 'error';
                        const durationClass = auction.duration_ms < 50 ? 'fast' : (auction.duration_ms > 100 ? 'slow' : '');
                        const bidders = auction.winning_bidders || [];

                        return '<div class="auction-item ' + statusClass + '">' +
                            '<div class="auction-time">' + formatTime(auction.timestamp) + '</div>' +
                            '<div class="auction-info">' +
                                '<div class="auction-id">' + auction.request_id + '</div>' +
                                '<div class="auction-metrics">' +
                                    '<span>' + auction.imp_count + ' imp</span>' +
                                    '<span>' + auction.bid_count + ' bids</span>' +
                                '</div>' +
                                (auction.error ? '<div class="error-msg">' + auction.error + '</div>' : '') +
                                (bidders.length > 0 ? '<div class="auction-bidders">' +
                                    bidders.map(b => '<span class="bidder-tag">' + b + '</span>').join('') +
                                '</div>' : '') +
                            '</div>' +
                            '<div class="auction-duration ' + durationClass + '">' + auction.duration_ms + 'ms</div>' +
                        '</div>';
                    }).join('');
                } else {
                    auctionsEl.innerHTML = '<div style="color: #64748b; font-size: 0.875rem; text-align: center; padding: 2rem;">No auctions yet</div>';
                }
            } catch (error) {
                console.error('Failed to update dashboard:', error);
            }
        }

        // Update every 2 seconds
        updateDashboard();
        setInterval(updateDashboard, 2000);
    </script>
</body>
</html>
`))
