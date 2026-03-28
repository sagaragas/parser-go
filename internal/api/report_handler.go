package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"parsergo/internal/job"
	"parsergo/internal/summary"
)

// ReportHandler handles browser-facing report routes.
// These routes serve human-readable HTML reports at /reports/*.
type ReportHandler struct {
	analysisHandler *Handler
	logger          *slog.Logger
}

// ReportIndexItem represents a single report in the index.
type ReportIndexItem struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	State     string    `json:"state"`
}

// NewReportHandler creates a new report handler.
func NewReportHandler(analysisHandler *Handler, logger *slog.Logger) *ReportHandler {
	return &ReportHandler{
		analysisHandler: analysisHandler,
		logger:          logger,
	}
}

// RegisterRoutes registers report routes on the mux.
func (h *ReportHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/reports", h.handleReportsIndex)
	mux.HandleFunc("/reports/", h.handleReportDetail)
}

// handleReportsIndex serves the report list page (VAL-RPT-001, VAL-RPT-002).
func (h *ReportHandler) handleReportsIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.analysisHandler.expireEligibleJobs(h.analysisHandler.now())

	// Get list of succeeded jobs that have reports
	jobs := h.analysisHandler.jobStore.List()
	var reports []ReportIndexItem
	for _, j := range jobs {
		if j.State == job.StateSucceeded {
			h.analysisHandler.workspacesMu.RLock()
			ws, hasWorkspace := h.analysisHandler.workspaces[j.ID]
			h.analysisHandler.workspacesMu.RUnlock()

			if hasWorkspace && ws != nil && ws.Summary != nil {
				reports = append(reports, ReportIndexItem{
					ID:        j.ID,
					CreatedAt: j.CreatedAt,
					State:     string(j.State),
				})
			}
		}
	}

	// Sort by creation time, newest first
	sortReportIndexItems(reports)

	// Generate HTML page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(h.generateReportIndexHTML(reports)))
}

// handleReportDetail serves the individual report detail page (VAL-RPT-003).
func (h *ReportHandler) handleReportDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.analysisHandler.expireEligibleJobs(h.analysisHandler.now())

	// Extract job ID from path: /reports/{id}
	path := strings.TrimPrefix(r.URL.Path, "/reports/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		h.writeMissingReport(w)
		return
	}

	jobID := parts[0]

	// Validate job ID format
	if !isValidJobID(jobID) {
		h.writeMissingReport(w)
		return
	}

	// Get job
	j, exists := h.analysisHandler.jobStore.Get(jobID)
	if !exists {
		h.writeMissingReport(w)
		return
	}

	// Check job state
	if j.State != job.StateSucceeded {
		if j.State == job.StateQueued || j.State == job.StateRunning {
			h.writeNotReadyReport(w, jobID)
			return
		}
		if j.State == job.StateFailed {
			h.writeFailedReport(w, jobID, j.Error)
			return
		}
		if j.State == job.StateExpired {
			h.writeExpiredReport(w, jobID)
			return
		}
		h.writeMissingReport(w)
		return
	}

	// Get summary from workspace
	h.analysisHandler.workspacesMu.RLock()
	ws, exists := h.analysisHandler.workspaces[jobID]
	h.analysisHandler.workspacesMu.RUnlock()

	if !exists || ws == nil || ws.Summary == nil {
		h.writeMissingReport(w)
		return
	}

	// Generate HTML report
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(h.generateReportDetailHTML(jobID, ws.Summary)))
}

// generateReportIndexHTML creates the reports list page with empty state (VAL-RPT-001, VAL-RPT-002).
func (h *ReportHandler) generateReportIndexHTML(reports []ReportIndexItem) string {
	title := "Analysis Reports"
	header := "Generated Reports"
	emptyState := len(reports) == 0

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + escapeHTML(title) + `</title>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 900px;
            margin: 0 auto;
            padding: 20px;
            background: #f8f9fa;
        }
        h1 {
            color: #2c3e50;
            border-bottom: 2px solid #3498db;
            padding-bottom: 10px;
            margin-bottom: 30px;
        }
        .report-count {
            color: #666;
            font-size: 0.9em;
            margin-bottom: 20px;
        }
        .report-list {
            list-style: none;
            padding: 0;
        }
        .report-item {
            background: white;
            border: 1px solid #ddd;
            border-radius: 8px;
            padding: 15px 20px;
            margin-bottom: 15px;
            transition: box-shadow 0.2s, transform 0.1s;
        }
        .report-item:hover {
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            transform: translateY(-1px);
        }
        .report-link {
            text-decoration: none;
            color: inherit;
            display: block;
        }
        .report-id {
            font-family: "SF Mono", Monaco, Inconsolata, monospace;
            font-size: 0.85em;
            color: #3498db;
            margin-bottom: 5px;
        }
        .report-meta {
            font-size: 0.85em;
            color: #666;
        }
        .empty-state {
            background: white;
            border: 2px dashed #ddd;
            border-radius: 8px;
            padding: 60px 20px;
            text-align: center;
            color: #666;
        }
        .empty-state-icon {
            font-size: 48px;
            margin-bottom: 20px;
            color: #bbb;
        }
        .empty-state-title {
            font-size: 1.3em;
            margin-bottom: 10px;
            color: #555;
        }
        .empty-state-help {
            font-size: 0.9em;
            max-width: 400px;
            margin: 0 auto;
            line-height: 1.5;
        }
        .back-link {
            display: inline-block;
            margin-top: 30px;
            color: #3498db;
            text-decoration: none;
        }
        .back-link:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <h1>` + escapeHTML(header) + `</h1>
`

	if emptyState {
		html += `    <div class="empty-state">
        <div class="empty-state-icon">&#128196;</div>
        <div class="empty-state-title">No Reports Yet</div>
        <div class="empty-state-help">
            No analysis reports have been generated. Submit a log file for analysis 
            using the API at <code>POST /v1/analyses</code> to create your first report.
        </div>
    </div>
`
	} else {
		html += fmt.Sprintf(`    <div class="report-count">%d report(s) available</div>
    <ul class="report-list">
`, len(reports))
		for _, r := range reports {
			created := r.CreatedAt.Format("Jan 2, 2006 at 3:04 PM")
			html += fmt.Sprintf(`        <li class="report-item">
            <a href="/reports/%s" class="report-link">
                <div class="report-id">%s</div>
                <div class="report-meta">Created: %s</div>
            </a>
        </li>
`, escapeHTML(r.ID), escapeHTML(r.ID), escapeHTML(created))
		}
		html += `    </ul>
`
	}

	html += `</body>
</html>`

	return html
}

// generateReportDetailHTML creates the report detail page (VAL-RPT-003, VAL-RPT-004, VAL-RPT-005).
func (h *ReportHandler) generateReportDetailHTML(jobID string, sum *summary.Summary) string {
	duration := time.Duration(0)
	if sum.RequestsPerSec > 0 {
		duration = time.Duration(float64(time.Second) * float64(sum.RequestsTotal) / sum.RequestsPerSec)
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Analysis Report - ` + escapeHTML(jobID) + `</title>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f8f9fa;
        }
        h1 {
            color: #2c3e50;
            border-bottom: 2px solid #3498db;
            padding-bottom: 10px;
            margin-bottom: 20px;
        }
        .report-id {
            font-family: "SF Mono", Monaco, Inconsolata, monospace;
            font-size: 0.9em;
            color: #666;
            background: #f0f0f0;
            padding: 4px 8px;
            border-radius: 4px;
            display: inline-block;
            margin-bottom: 20px;
        }
        .summary-section {
            background: white;
            border: 1px solid #ddd;
            border-radius: 8px;
            padding: 25px;
            margin-bottom: 30px;
        }
        .summary-title {
            font-size: 1.2em;
            color: #2c3e50;
            margin-top: 0;
            margin-bottom: 20px;
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .metric-card {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            border-radius: 8px;
            text-align: center;
        }
        .metric-value {
            font-size: 2em;
            font-weight: bold;
            margin-bottom: 5px;
        }
        .metric-label {
            font-size: 0.85em;
            opacity: 0.9;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .metric-card.alt {
            background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
        }
        .metric-card.alt2 {
            background: linear-gradient(135deg, #fc4a1a 0%, #f7b733 100%);
        }
        .metric-card.alt3 {
            background: linear-gradient(135deg, #2193b0 0%, #6dd5ed 100%);
        }
        .workload-stats {
            background: #f8f9fa;
            border-left: 4px solid #3498db;
            padding: 15px 20px;
            margin-top: 20px;
            border-radius: 0 4px 4px 0;
        }
        .workload-stats h3 {
            margin-top: 0;
            color: #2c3e50;
            font-size: 1em;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
        }
        .stat-item {
            display: flex;
            justify-content: space-between;
        }
        .stat-label {
            color: #666;
        }
        .stat-value {
            font-weight: 600;
            color: #2c3e50;
        }
        .top-requests-section {
            background: white;
            border: 1px solid #ddd;
            border-radius: 8px;
            padding: 25px;
            margin-bottom: 30px;
        }
        .charts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .chart-card {
            background: white;
            border: 1px solid #ddd;
            border-radius: 8px;
            padding: 25px;
        }
        .chart-title {
            font-size: 1.1em;
            color: #2c3e50;
            margin-top: 0;
            margin-bottom: 8px;
        }
        .chart-help {
            color: #5f6b7a;
            font-size: 0.9em;
            margin-top: 0;
            margin-bottom: 20px;
        }
        .chart-svg {
            width: 100%;
            height: auto;
            display: block;
        }
        .chart-svg text {
            fill: #243447;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            font-size: 13px;
        }
        .chart-svg .svg-axis-label {
            fill: #64748b;
            font-size: 12px;
        }
        .chart-svg .svg-value {
            font-weight: 600;
        }
        .chart-legend {
            list-style: none;
            padding: 0;
            margin: 16px 0 0;
        }
        .chart-legend li {
            display: flex;
            align-items: center;
            gap: 10px;
            color: #4a5568;
            margin-top: 8px;
        }
        .legend-swatch {
            width: 12px;
            height: 12px;
            border-radius: 999px;
            display: inline-block;
            flex: 0 0 auto;
        }
        .legend-matched { background: #2563eb; }
        .legend-filtered { background: #f59e0b; }
        .legend-unmatched { background: #94a3b8; }
        .chart-note {
            margin-top: 14px;
            color: #64748b;
            font-size: 0.85em;
        }
        .top-requests-title {
            font-size: 1.2em;
            color: #2c3e50;
            margin-top: 0;
            margin-bottom: 20px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            font-size: 0.95em;
        }
        th, td {
            text-align: left;
            padding: 12px 15px;
            border-bottom: 1px solid #eee;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
            color: #555;
            text-transform: uppercase;
            font-size: 0.8em;
            letter-spacing: 0.5px;
        }
        tr:hover {
            background: #f8f9fa;
        }
        .rank {
            width: 60px;
            text-align: center;
        }
        .count {
            width: 100px;
            text-align: right;
        }
        .percentage {
            width: 100px;
            text-align: right;
        }
        .method {
            width: 80px;
        }
        .method-badge {
            display: inline-block;
            padding: 2px 8px;
            border-radius: 3px;
            font-size: 0.8em;
            font-weight: 600;
            text-transform: uppercase;
        }
        .method-get { background: #e8f5e9; color: #2e7d32; }
        .method-post { background: #e3f2fd; color: #1565c0; }
        .method-put { background: #fff3e0; color: #ef6c00; }
        .method-delete { background: #ffebee; color: #c62828; }
        .method-other { background: #f5f5f5; color: #616161; }
        .path {
            font-family: "SF Mono", Monaco, Inconsolata, monospace;
            font-size: 0.9em;
            color: #444;
            word-break: break-all;
        }
        .no-data {
            color: #999;
            font-style: italic;
            padding: 40px;
            text-align: center;
        }
        .back-link {
            display: inline-block;
            margin-bottom: 20px;
            color: #3498db;
            text-decoration: none;
        }
        .back-link:hover {
            text-decoration: underline;
        }
        .badge {
            display: inline-block;
            padding: 4px 10px;
            border-radius: 4px;
            font-size: 0.75em;
            font-weight: 600;
            text-transform: uppercase;
        }
        .badge-success {
            background: #d4edda;
            color: #155724;
        }
        @media (max-width: 768px) {
            .metrics-grid {
                grid-template-columns: 1fr;
            }
            .stats-grid {
                grid-template-columns: 1fr;
            }
            th, td {
                padding: 8px 10px;
            }
            .method, .percentage {
                display: none;
            }
        }
    </style>
</head>
<body>
    <a href="/reports" class="back-link">&larr; Back to Reports</a>
    <h1>Analysis Report</h1>
    <div class="report-id">` + escapeHTML(jobID) + ` <span class="badge badge-success">completed</span></div>

    <div class="summary-section">
        <h2 class="summary-title">Core Metrics</h2>
        <div class="metrics-grid">
            <div class="metric-card">
                <div class="metric-value">` + formatNumber(sum.RequestsTotal) + `</div>
                <div class="metric-label">Total Requests</div>
            </div>
            <div class="metric-card alt">
                <div class="metric-value">` + formatFloat(sum.RequestsPerSec) + `</div>
                <div class="metric-label">Requests/sec</div>
            </div>
            <div class="metric-card alt2">
                <div class="metric-value">` + formatNumber(int64(sum.TotalLines)) + `</div>
                <div class="metric-label">Total Lines</div>
            </div>
            <div class="metric-card alt3">
                <div class="metric-value">` + formatNumber(int64(sum.MatchedLines)) + `</div>
                <div class="metric-label">Matched Lines</div>
            </div>
        </div>
        <div class="workload-stats">
            <h3>Workload Accounting</h3>
            <div class="stats-grid">
                <div class="stat-item">
                    <span class="stat-label">Input Size:</span>
                    <span class="stat-value">` + formatBytes(sum.InputBytes) + `</span>
                </div>
                <div class="stat-item">
                    <span class="stat-label">Filtered Lines:</span>
                    <span class="stat-value">` + formatNumber(int64(sum.FilteredLines)) + `</span>
                </div>
                <div class="stat-item">
                    <span class="stat-label">Unique Requests:</span>
                    <span class="stat-value">` + formatNumber(int64(sum.RowCount)) + `</span>
                </div>
                <div class="stat-item">
                    <span class="stat-label">Duration:</span>
                    <span class="stat-value">` + formatDuration(duration) + `</span>
                </div>
            </div>
        </div>
    </div>

    ` + generateChartsHTML(sum) + `

    <div class="top-requests-section">
        <h2 class="top-requests-title">Top Requests by Frequency</h2>
`

	if len(sum.RankedRequests) == 0 {
		html += `        <p class="no-data">No request data available for this analysis.</p>
`
	} else {
		html += `        <table>
            <thead>
                <tr>
                    <th class="rank">Rank</th>
                    <th class="method">Method</th>
                    <th>Path</th>
                    <th class="count">Count</th>
                    <th class="percentage">%</th>
                </tr>
            </thead>
            <tbody>
`
		for i, req := range sum.RankedRequests {
			methodClass := "method-other"
			switch req.Method {
			case "GET":
				methodClass = "method-get"
			case "POST":
				methodClass = "method-post"
			case "PUT":
				methodClass = "method-put"
			case "DELETE":
				methodClass = "method-delete"
			}
			html += fmt.Sprintf(`                <tr>
                    <td class="rank">%d</td>
                    <td class="method"><span class="method-badge %s">%s</span></td>
                    <td class="path">%s</td>
                    <td class="count">%s</td>
                    <td class="percentage">%.1f%%</td>
                </tr>
`, i+1, methodClass, escapeHTML(req.Method), escapeHTML(req.Path),
				formatNumber(req.Count), req.Percentage)
		}
		html += `            </tbody>
        </table>
`
	}

	html += `    </div>
</body>
</html>`

	return html
}

// writeMissingReport writes a readable error for missing reports (VAL-RPT-009).
func (h *ReportHandler) writeMissingReport(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Report Not Found</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            max-width: 600px;
            margin: 100px auto;
            text-align: center;
            padding: 20px;
            color: #333;
        }
        .error-code {
            font-size: 72px;
            color: #e74c3c;
            margin-bottom: 20px;
        }
        h1 {
            color: #2c3e50;
            margin-bottom: 15px;
        }
        p {
            color: #666;
            line-height: 1.6;
        }
        .back-link {
            display: inline-block;
            margin-top: 30px;
            padding: 10px 20px;
            background: #3498db;
            color: white;
            text-decoration: none;
            border-radius: 4px;
        }
        .back-link:hover {
            background: #2980b9;
        }
    </style>
</head>
<body>
    <div class="error-code">404</div>
    <h1>Report Not Found</h1>
    <p>The requested analysis report does not exist or has been removed.<br>
    Please check the report ID and try again.</p>
    <a href="/reports" class="back-link">View All Reports</a>
</body>
</html>`))
}

func sortReportIndexItems(reports []ReportIndexItem) {
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].CreatedAt.Equal(reports[j].CreatedAt) {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})
}

func generateChartsHTML(sum *summary.Summary) string {
	return `    <div class="charts-grid">
        <section class="chart-card">
            <h2 class="chart-title">Top Request Share</h2>
            <p class="chart-help">Self-contained inline SVG rendered from the report summary with no external scripts or data fetches.</p>
            ` + renderTopRequestsChart(sum) + `
        </section>
        <section class="chart-card">
            <h2 class="chart-title">Line Processing Breakdown</h2>
            <p class="chart-help">Matched, filtered, and remaining lines shown directly from workload accounting data embedded in the report.</p>
            ` + renderLineBreakdownChart(sum) + `
        </section>
    </div>
`
}

func renderTopRequestsChart(sum *summary.Summary) string {
	if len(sum.RankedRequests) == 0 {
		return `<p class="no-data">No ranked request data is available for chart rendering.</p>`
	}

	ranked := sum.RankedRequests
	if len(ranked) > 6 {
		ranked = ranked[:6]
	}

	maxCount := ranked[0].Count
	if maxCount <= 0 {
		maxCount = 1
	}

	const (
		chartWidth   = 760
		labelX       = 20
		barStartX    = 280
		barWidth     = 330
		rowHeight    = 48
		topPadding   = 44
		bottomMargin = 26
	)

	chartHeight := topPadding + len(ranked)*rowHeight + bottomMargin
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(`<svg class="chart-svg" viewBox="0 0 %d %d" role="img" aria-labelledby="request-share-chart-title request-share-chart-desc">`, chartWidth, chartHeight))
	builder.WriteString(`<title id="request-share-chart-title">Top request share</title>`)
	builder.WriteString(fmt.Sprintf(`<desc id="request-share-chart-desc">Bar chart showing the top %d ranked requests by frequency.</desc>`, len(ranked)))

	for index, req := range ranked {
		y := topPadding + index*rowHeight
		width := int(float64(req.Count) / float64(maxCount) * float64(barWidth))
		if req.Count > 0 && width < 6 {
			width = 6
		}

		label := truncateLabel(req.Method+" "+req.Path, 32)
		builder.WriteString(fmt.Sprintf(`<text x="%d" y="%d">%s</text>`, labelX, y+17, escapeHTML(label)))
		builder.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="22" rx="6" fill="#e2e8f0"></rect>`, barStartX, y, barWidth))
		builder.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="22" rx="6" fill="#2563eb"></rect>`, barStartX, y, width))
		builder.WriteString(fmt.Sprintf(`<text class="svg-value" x="%d" y="%d">%s • %.1f%%</text>`, barStartX+width+12, y+17, formatNumber(req.Count), req.Percentage))
	}

	builder.WriteString(`</svg>`)
	builder.WriteString(fmt.Sprintf(`<p class="chart-note">Showing the top %d ranked request rows from %d total.</p>`, len(ranked), len(sum.RankedRequests)))
	return builder.String()
}

func renderLineBreakdownChart(sum *summary.Summary) string {
	matched := sum.MatchedLines
	if matched < 0 {
		matched = 0
	}
	filtered := sum.FilteredLines
	if filtered < 0 {
		filtered = 0
	}
	unmatched := sum.TotalLines - matched - filtered
	if unmatched < 0 {
		unmatched = 0
	}

	total := matched + filtered + unmatched
	if total <= 0 {
		return `<p class="no-data">No workload accounting data is available for chart rendering.</p>`
	}

	const (
		chartWidth = 760
		barX       = 20
		barY       = 44
		barWidth   = 700
		barHeight  = 28
	)

	type segment struct {
		label string
		value int
		fill  string
	}

	segments := []segment{
		{label: "Matched", value: matched, fill: "#2563eb"},
		{label: "Filtered", value: filtered, fill: "#f59e0b"},
		{label: "Unmatched", value: unmatched, fill: "#94a3b8"},
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(`<svg class="chart-svg" viewBox="0 0 %d 120" role="img" aria-labelledby="line-breakdown-chart-title line-breakdown-chart-desc">`, chartWidth))
	builder.WriteString(`<title id="line-breakdown-chart-title">Line processing breakdown</title>`)
	builder.WriteString(`<desc id="line-breakdown-chart-desc">Stacked bar chart showing matched, filtered, and unmatched lines from the report summary.</desc>`)
	builder.WriteString(fmt.Sprintf(`<text class="svg-axis-label" x="%d" y="24">0</text>`, barX))
	builder.WriteString(fmt.Sprintf(`<text class="svg-axis-label" x="%d" y="24">%s total lines</text>`, barX+barWidth-90, escapeHTML(formatNumber(int64(sum.TotalLines)))))

	x := barX
	for _, seg := range segments {
		if seg.value <= 0 {
			continue
		}

		width := int(float64(seg.value) / float64(total) * float64(barWidth))
		if width < 4 {
			width = 4
		}
		if x+width > barX+barWidth {
			width = barX + barWidth - x
		}

		builder.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="8" fill="%s"></rect>`, x, barY, width, barHeight, seg.fill))
		if width > 78 {
			builder.WriteString(fmt.Sprintf(`<text class="svg-value" x="%d" y="%d">%s</text>`, x+10, barY+18, escapeHTML(seg.label)))
		}
		x += width
	}

	builder.WriteString(`</svg>`)
	builder.WriteString(`<ul class="chart-legend">`)
	for _, seg := range segments {
		builder.WriteString(fmt.Sprintf(`<li><span class="legend-swatch legend-%s"></span>%s lines: %s</li>`,
			strings.ToLower(seg.label),
			escapeHTML(seg.label),
			formatNumber(int64(seg.value)),
		))
	}
	builder.WriteString(`</ul>`)
	return builder.String()
}

func truncateLabel(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes <= 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

// writeNotReadyReport writes a readable error for reports not yet ready.
func (h *ReportHandler) writeNotReadyReport(w http.ResponseWriter, jobID string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Report Not Ready</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            max-width: 600px;
            margin: 100px auto;
            text-align: center;
            padding: 20px;
            color: #333;
        }
        .status-icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            color: #2c3e50;
            margin-bottom: 15px;
        }
        p {
            color: #666;
            line-height: 1.6;
        }
        .job-id {
            font-family: "SF Mono", monospace;
            background: #f0f0f0;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.9em;
        }
        .refresh-link {
            display: inline-block;
            margin-top: 20px;
            padding: 10px 20px;
            background: #3498db;
            color: white;
            text-decoration: none;
            border-radius: 4px;
        }
        .refresh-link:hover {
            background: #2980b9;
        }
        .back-link {
            display: inline-block;
            margin-top: 10px;
            color: #666;
            text-decoration: none;
        }
    </style>
</head>
<body>
    <div class="status-icon">&#9203;</div>
    <h1>Analysis In Progress</h1>
    <p>The analysis <span class="job-id">%s</span> is still running.<br>
    Please wait a moment and refresh this page, or check the job status via the API.</p>
    <a href="/reports/%s" class="refresh-link">Refresh Page</a>
    <br>
    <a href="/reports" class="back-link">View All Reports</a>
</body>
</html>`, escapeHTML(jobID), escapeHTML(jobID))))
}

// writeFailedReport writes a readable error for failed analyses.
func (h *ReportHandler) writeFailedReport(w http.ResponseWriter, jobID string, jobErr *job.Error) {
	errMsg := "Analysis failed"
	if jobErr != nil && jobErr.Message != "" {
		errMsg = escapeHTML(jobErr.Message)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Analysis Failed</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            max-width: 600px;
            margin: 100px auto;
            text-align: center;
            padding: 20px;
            color: #333;
        }
        .status-icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            color: #c0392b;
            margin-bottom: 15px;
        }
        p {
            color: #666;
            line-height: 1.6;
        }
        .error-box {
            background: #ffebee;
            border: 1px solid #ef9a9a;
            border-radius: 4px;
            padding: 15px;
            margin: 20px 0;
            color: #c62828;
            font-family: "SF Mono", monospace;
            font-size: 0.9em;
        }
        .job-id {
            font-family: "SF Mono", monospace;
            background: #f0f0f0;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.9em;
        }
        .back-link {
            display: inline-block;
            margin-top: 20px;
            padding: 10px 20px;
            background: #3498db;
            color: white;
            text-decoration: none;
            border-radius: 4px;
        }
        .back-link:hover {
            background: #2980b9;
        }
    </style>
</head>
<body>
    <div class="status-icon">&#10060;</div>
    <h1>Analysis Failed</h1>
    <p>The analysis <span class="job-id">%s</span> could not be completed.</p>
    <div class="error-box">%s</div>
    <p>Please check your input data and try again with a valid log file.</p>
    <a href="/reports" class="back-link">View All Reports</a>
</body>
</html>`, escapeHTML(jobID), errMsg)))
}

// writeExpiredReport writes a readable error for expired analyses.
func (h *ReportHandler) writeExpiredReport(w http.ResponseWriter, jobID string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusGone)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Report Expired</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            max-width: 600px;
            margin: 100px auto;
            text-align: center;
            padding: 20px;
            color: #333;
        }
        .status-icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            color: #7f8c8d;
            margin-bottom: 15px;
        }
        p {
            color: #666;
            line-height: 1.6;
        }
        .job-id {
            font-family: "SF Mono", monospace;
            background: #f0f0f0;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.9em;
        }
        .back-link {
            display: inline-block;
            margin-top: 30px;
            padding: 10px 20px;
            background: #3498db;
            color: white;
            text-decoration: none;
            border-radius: 4px;
        }
        .back-link:hover {
            background: #2980b9;
        }
    </style>
</head>
<body>
    <div class="status-icon">&#9209;</div>
    <h1>Report Expired</h1>
    <p>The analysis <span class="job-id">%s</span> has expired and is no longer available.<br>
    Please submit a new analysis if you need an updated report.</p>
    <a href="/reports" class="back-link">View All Reports</a>
</body>
</html>`, escapeHTML(jobID))))
}

// writeError writes a simple error response.
func (h *ReportHandler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Error</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            max-width: 600px;
            margin: 100px auto;
            text-align: center;
            padding: 20px;
            color: #333;
        }
        h1 { color: #c0392b; }
        p { color: #666; }
    </style>
</head>
<body>
    <h1>Error</h1>
    <p>%s</p>
</body>
</html>`, escapeHTML(message))))
}

// formatBytes formats a byte count for display.
func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.1f ms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}
