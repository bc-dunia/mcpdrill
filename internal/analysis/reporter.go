// Package analysis provides telemetry aggregation and metrics computation.
package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"time"
)

// Report contains all data for generating a run report.
type Report struct {
	RunID      string             `json:"run_id"`
	ScenarioID string             `json:"scenario_id"`
	StartTime  int64              `json:"start_time"`  // unix timestamp ms
	EndTime    int64              `json:"end_time"`    // unix timestamp ms
	Duration   int64              `json:"duration_ms"` // duration in ms
	Metrics    *AggregatedMetrics `json:"metrics"`
	StopReason string             `json:"stop_reason"`
}

// Reporter generates HTML and JSON reports from aggregated metrics.
type Reporter struct{}

// NewReporter creates a new Reporter instance.
func NewReporter() *Reporter {
	return &Reporter{}
}

// GenerateJSON generates a pretty-printed JSON report.
func (r *Reporter) GenerateJSON(report *Report) ([]byte, error) {
	if report == nil {
		return nil, fmt.Errorf("report cannot be nil")
	}

	// Ensure metrics is not nil for clean JSON output
	if report.Metrics == nil {
		report.Metrics = &AggregatedMetrics{
			ByOperation: make(map[string]*OperationMetrics),
			ByTool:      make(map[string]*OperationMetrics),
		}
	}

	return json.MarshalIndent(report, "", "  ")
}

// GenerateHTML generates a self-contained HTML report with embedded CSS.
func (r *Reporter) GenerateHTML(report *Report) ([]byte, error) {
	if report == nil {
		return nil, fmt.Errorf("report cannot be nil")
	}

	// Ensure metrics is not nil
	if report.Metrics == nil {
		report.Metrics = &AggregatedMetrics{
			ByOperation: make(map[string]*OperationMetrics),
			ByTool:      make(map[string]*OperationMetrics),
		}
	}

	// Prepare template data
	data := htmlReportData{
		RunID:         report.RunID,
		ScenarioID:    report.ScenarioID,
		StartTime:     formatTimestamp(report.StartTime),
		EndTime:       formatTimestamp(report.EndTime),
		Duration:      formatDuration(report.Duration),
		StopReason:    report.StopReason,
		TotalOps:      report.Metrics.TotalOps,
		SuccessOps:    report.Metrics.SuccessOps,
		FailureOps:    report.Metrics.FailureOps,
		RPS:           fmt.Sprintf("%.2f", report.Metrics.RPS),
		ErrorRate:     fmt.Sprintf("%.2f%%", report.Metrics.ErrorRate),
		LatencyP50:    report.Metrics.LatencyP50,
		LatencyP95:    report.Metrics.LatencyP95,
		LatencyP99:    report.Metrics.LatencyP99,
		Operations:    buildOperationRows(report.Metrics.ByOperation),
		Tools:         buildOperationRows(report.Metrics.ByTool),
		HasOperations: len(report.Metrics.ByOperation) > 0,
		HasTools:      len(report.Metrics.ByTool) > 0,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	if report.Metrics.SessionMetrics != nil {
		data.HasSessionMetrics = true
		data.SessionMode = report.Metrics.SessionMetrics.SessionMode
		data.TotalSessions = report.Metrics.SessionMetrics.TotalSessions
		data.OpsPerSession = fmt.Sprintf("%.2f", report.Metrics.SessionMetrics.OpsPerSession)
		data.SessionReuseRate = fmt.Sprintf("%.1f%%", 100*report.Metrics.SessionMetrics.SessionReuseRate)
		data.SessionCreated = report.Metrics.SessionMetrics.TotalCreated
		data.SessionEvicted = report.Metrics.SessionMetrics.TotalEvicted
		data.SessionReconnects = report.Metrics.SessionMetrics.Reconnects
	}

	if report.Metrics.WorkerHealth != nil {
		data.HasWorkerHealth = true
		data.PeakCPUPercent = fmt.Sprintf("%.1f%%", report.Metrics.WorkerHealth.PeakCPUPercent)
		data.PeakMemoryMB = fmt.Sprintf("%.1f MB", report.Metrics.WorkerHealth.PeakMemoryMB)
		data.AvgActiveVUs = fmt.Sprintf("%.1f", report.Metrics.WorkerHealth.AvgActiveVUs)
		data.WorkerCount = report.Metrics.WorkerHealth.WorkerCount
		data.SaturationDetected = report.Metrics.WorkerHealth.SaturationDetected
		data.SaturationReason = report.Metrics.WorkerHealth.SaturationReason
	}

	if report.Metrics.ChurnMetrics != nil {
		data.HasChurnMetrics = true
		data.ChurnSessionsCreated = report.Metrics.ChurnMetrics.SessionsCreated
		data.ChurnSessionsDestroyed = report.Metrics.ChurnMetrics.SessionsDestroyed
		data.ChurnActiveSessions = report.Metrics.ChurnMetrics.ActiveSessions
		data.ChurnReconnectAttempts = report.Metrics.ChurnMetrics.ReconnectAttempts
		data.ChurnRate = fmt.Sprintf("%.2f", report.Metrics.ChurnMetrics.ChurnRate)
	}

	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// htmlReportData holds data for HTML template rendering.
type htmlReportData struct {
	RunID                  string
	ScenarioID             string
	StartTime              string
	EndTime                string
	Duration               string
	StopReason             string
	TotalOps               int
	SuccessOps             int
	FailureOps             int
	RPS                    string
	ErrorRate              string
	LatencyP50             int
	LatencyP95             int
	LatencyP99             int
	Operations             []operationRow
	Tools                  []operationRow
	HasOperations          bool
	HasTools               bool
	GeneratedAt            string
	HasSessionMetrics      bool
	SessionMode            string
	TotalSessions          int
	OpsPerSession          string
	SessionReuseRate       string
	SessionCreated         int64
	SessionEvicted         int64
	SessionReconnects      int64
	HasWorkerHealth        bool
	PeakCPUPercent         string
	PeakMemoryMB           string
	AvgActiveVUs           string
	WorkerCount            int
	SaturationDetected     bool
	SaturationReason       string
	HasChurnMetrics        bool
	ChurnSessionsCreated   int64
	ChurnSessionsDestroyed int64
	ChurnActiveSessions    int
	ChurnReconnectAttempts int64
	ChurnRate              string
}

// operationRow represents a row in the operations/tools table.
type operationRow struct {
	Name       string
	TotalOps   int
	SuccessOps int
	FailureOps int
	ErrorRate  string
	LatencyP50 int
	LatencyP95 int
	LatencyP99 int
}

// formatTimestamp formats a unix timestamp (ms) to RFC3339.
func formatTimestamp(ts int64) string {
	if ts == 0 {
		return "N/A"
	}
	return time.UnixMilli(ts).UTC().Format(time.RFC3339)
}

// formatDuration formats duration in ms to human-readable string.
func formatDuration(ms int64) string {
	if ms == 0 {
		return "0s"
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Second {
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// buildOperationRows converts a map of operation metrics to sorted rows.
func buildOperationRows(metrics map[string]*OperationMetrics) []operationRow {
	if len(metrics) == 0 {
		return nil
	}

	// Get sorted keys for deterministic output
	keys := make([]string, 0, len(metrics))
	for k := range metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]operationRow, 0, len(keys))
	for _, name := range keys {
		m := metrics[name]
		rows = append(rows, operationRow{
			Name:       name,
			TotalOps:   m.TotalOps,
			SuccessOps: m.SuccessOps,
			FailureOps: m.FailureOps,
			ErrorRate:  fmt.Sprintf("%.2f%%", m.ErrorRate),
			LatencyP50: m.LatencyP50,
			LatencyP95: m.LatencyP95,
			LatencyP99: m.LatencyP99,
		})
	}
	return rows
}

// htmlTemplate is the self-contained HTML template with embedded CSS.
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MCP Drill Report - {{.RunID}}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f5f5;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: #fff;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            padding: 30px;
        }
        h1 {
            color: #2c3e50;
            border-bottom: 3px solid #3498db;
            padding-bottom: 10px;
            margin-bottom: 20px;
        }
        h2 {
            color: #34495e;
            margin: 25px 0 15px 0;
            padding-bottom: 5px;
            border-bottom: 1px solid #eee;
        }
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 20px;
        }
        .summary-card {
            background: #f8f9fa;
            border-radius: 6px;
            padding: 15px;
            border-left: 4px solid #3498db;
        }
        .summary-card.success {
            border-left-color: #27ae60;
        }
        .summary-card.error {
            border-left-color: #e74c3c;
        }
        .summary-card.latency {
            border-left-color: #9b59b6;
        }
        .summary-card.session {
            border-left-color: #f39c12;
        }
        .summary-card label {
            display: block;
            font-size: 12px;
            color: #7f8c8d;
            text-transform: uppercase;
            margin-bottom: 5px;
        }
        .summary-card .value {
            font-size: 24px;
            font-weight: bold;
            color: #2c3e50;
        }
        .meta-info {
            background: #ecf0f1;
            border-radius: 6px;
            padding: 15px;
            margin-bottom: 20px;
        }
        .meta-info dl {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 10px;
        }
        .meta-info dt {
            font-weight: bold;
            color: #7f8c8d;
            font-size: 12px;
            text-transform: uppercase;
        }
        .meta-info dd {
            color: #2c3e50;
            margin-bottom: 10px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 10px;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #eee;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
            color: #34495e;
            font-size: 12px;
            text-transform: uppercase;
        }
        tr:hover {
            background: #f8f9fa;
        }
        .latency-section {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 15px;
            margin-bottom: 20px;
        }
        .latency-card {
            background: #f8f9fa;
            border-radius: 6px;
            padding: 20px;
            text-align: center;
        }
        .latency-card label {
            display: block;
            font-size: 14px;
            color: #7f8c8d;
            margin-bottom: 5px;
        }
        .latency-card .value {
            font-size: 28px;
            font-weight: bold;
            color: #9b59b6;
        }
        .latency-card .unit {
            font-size: 14px;
            color: #7f8c8d;
        }
        .no-data {
            color: #95a5a6;
            font-style: italic;
            padding: 20px;
            text-align: center;
            background: #f8f9fa;
            border-radius: 6px;
        }
        .warning-banner {
            background: #fff3cd;
            border: 1px solid #ffc107;
            border-left: 4px solid #ffc107;
            border-radius: 6px;
            padding: 15px;
            margin-bottom: 15px;
            color: #856404;
        }
        footer {
            margin-top: 30px;
            padding-top: 15px;
            border-top: 1px solid #eee;
            color: #95a5a6;
            font-size: 12px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>MCP Drill Report</h1>
        
        <div class="meta-info">
            <dl>
                <div>
                    <dt>Run ID</dt>
                    <dd>{{.RunID}}</dd>
                </div>
                <div>
                    <dt>Scenario ID</dt>
                    <dd>{{.ScenarioID}}</dd>
                </div>
                <div>
                    <dt>Start Time</dt>
                    <dd>{{.StartTime}}</dd>
                </div>
                <div>
                    <dt>End Time</dt>
                    <dd>{{.EndTime}}</dd>
                </div>
                <div>
                    <dt>Duration</dt>
                    <dd>{{.Duration}}</dd>
                </div>
                <div>
                    <dt>Stop Reason</dt>
                    <dd>{{.StopReason}}</dd>
                </div>
            </dl>
        </div>

        <h2>Summary</h2>
        <div class="summary-grid">
            <div class="summary-card">
                <label>Total Operations</label>
                <div class="value">{{.TotalOps}}</div>
            </div>
            <div class="summary-card success">
                <label>Successful</label>
                <div class="value">{{.SuccessOps}}</div>
            </div>
            <div class="summary-card error">
                <label>Failed</label>
                <div class="value">{{.FailureOps}}</div>
            </div>
            <div class="summary-card">
                <label>Requests/Second</label>
                <div class="value">{{.RPS}}</div>
            </div>
            <div class="summary-card error">
                <label>Error Rate</label>
                <div class="value">{{.ErrorRate}}</div>
            </div>
        </div>

        <h2>Latency Percentiles</h2>
        <div class="latency-section">
            <div class="latency-card">
                <label>P50 (Median)</label>
                <div class="value">{{.LatencyP50}}<span class="unit">ms</span></div>
            </div>
            <div class="latency-card">
                <label>P95</label>
                <div class="value">{{.LatencyP95}}<span class="unit">ms</span></div>
            </div>
            <div class="latency-card">
                <label>P99</label>
                <div class="value">{{.LatencyP99}}<span class="unit">ms</span></div>
            </div>
        </div>

        {{if .HasSessionMetrics}}
        <h2>Session Metrics</h2>
        <div class="summary-grid">
            <div class="summary-card session">
                <label>Session Mode</label>
                <div class="value">{{.SessionMode}}</div>
            </div>
            <div class="summary-card session">
                <label>Total Sessions</label>
                <div class="value">{{.TotalSessions}}</div>
            </div>
            <div class="summary-card session">
                <label>Ops/Session</label>
                <div class="value">{{.OpsPerSession}}</div>
            </div>
            <div class="summary-card session">
                <label>Reuse Rate</label>
                <div class="value">{{.SessionReuseRate}}</div>
            </div>
            {{if .SessionCreated}}
            <div class="summary-card">
                <label>Sessions Created</label>
                <div class="value">{{.SessionCreated}}</div>
            </div>
            {{end}}
            {{if .SessionEvicted}}
            <div class="summary-card">
                <label>Sessions Evicted</label>
                <div class="value">{{.SessionEvicted}}</div>
            </div>
            {{end}}
            {{if .SessionReconnects}}
            <div class="summary-card">
                <label>Reconnects</label>
                <div class="value">{{.SessionReconnects}}</div>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .HasWorkerHealth}}
        <h2>Runner Health</h2>
        {{if .SaturationDetected}}
        <div class="warning-banner">
            <strong>Warning:</strong> Load generator saturation detected - {{.SaturationReason}}
        </div>
        {{end}}
        <div class="summary-grid">
            <div class="summary-card{{if .SaturationDetected}} error{{end}}">
                <label>Peak CPU</label>
                <div class="value">{{.PeakCPUPercent}}</div>
            </div>
            <div class="summary-card">
                <label>Peak Memory</label>
                <div class="value">{{.PeakMemoryMB}}</div>
            </div>
            <div class="summary-card">
                <label>Avg Active VUs</label>
                <div class="value">{{.AvgActiveVUs}}</div>
            </div>
            <div class="summary-card">
                <label>Worker Count</label>
                <div class="value">{{.WorkerCount}}</div>
            </div>
        </div>
        {{end}}

        {{if .HasChurnMetrics}}
        <h2>Session Churn</h2>
        <div class="summary-grid">
            <div class="summary-card session">
                <label>Sessions Created</label>
                <div class="value">{{.ChurnSessionsCreated}}</div>
            </div>
            <div class="summary-card session">
                <label>Sessions Destroyed</label>
                <div class="value">{{.ChurnSessionsDestroyed}}</div>
            </div>
            <div class="summary-card session">
                <label>Active Sessions</label>
                <div class="value">{{.ChurnActiveSessions}}</div>
            </div>
            <div class="summary-card session">
                <label>Reconnect Attempts</label>
                <div class="value">{{.ChurnReconnectAttempts}}</div>
            </div>
            <div class="summary-card">
                <label>Churn Rate (sessions/sec)</label>
                <div class="value">{{.ChurnRate}}</div>
            </div>
        </div>
        {{end}}

        <h2>Operations Breakdown</h2>
        {{if .HasOperations}}
        <table>
            <thead>
                <tr>
                    <th>Operation</th>
                    <th>Total</th>
                    <th>Success</th>
                    <th>Failed</th>
                    <th>Error Rate</th>
                    <th>P50 (ms)</th>
                    <th>P95 (ms)</th>
                    <th>P99 (ms)</th>
                </tr>
            </thead>
            <tbody>
                {{range .Operations}}
                <tr>
                    <td>{{.Name}}</td>
                    <td>{{.TotalOps}}</td>
                    <td>{{.SuccessOps}}</td>
                    <td>{{.FailureOps}}</td>
                    <td>{{.ErrorRate}}</td>
                    <td>{{.LatencyP50}}</td>
                    <td>{{.LatencyP95}}</td>
                    <td>{{.LatencyP99}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <div class="no-data">No operation data available</div>
        {{end}}

        <h2>Tools Breakdown</h2>
        {{if .HasTools}}
        <table>
            <thead>
                <tr>
                    <th>Tool</th>
                    <th>Total</th>
                    <th>Success</th>
                    <th>Failed</th>
                    <th>Error Rate</th>
                    <th>P50 (ms)</th>
                    <th>P95 (ms)</th>
                    <th>P99 (ms)</th>
                </tr>
            </thead>
            <tbody>
                {{range .Tools}}
                <tr>
                    <td>{{.Name}}</td>
                    <td>{{.TotalOps}}</td>
                    <td>{{.SuccessOps}}</td>
                    <td>{{.FailureOps}}</td>
                    <td>{{.ErrorRate}}</td>
                    <td>{{.LatencyP50}}</td>
                    <td>{{.LatencyP95}}</td>
                    <td>{{.LatencyP99}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <div class="no-data">No tool data available</div>
        {{end}}

        <footer>
            Generated by MCP Drill at {{.GeneratedAt}}
        </footer>
    </div>
</body>
</html>`
