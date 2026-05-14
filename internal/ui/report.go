// Package ui renders local review surfaces for scan results.
package ui

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"digital-exhaust-cleaner/internal/analyzer"
)

// WriteReport renders an analysis result to a standalone HTML file.
func WriteReport(path string, result analyzer.Result) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return fmt.Errorf("create report directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	defer file.Close()

	if err := reportTemplate.Execute(file, viewModel{Result: result, Interactive: false}); err != nil {
		return fmt.Errorf("render report: %w", err)
	}
	return nil
}

type viewModel struct {
	Result      analyzer.Result
	Interactive bool
}

func (v viewModel) TotalRecoverable() int64 {
	var total int64
	for _, group := range v.Result.DuplicateGroups {
		total += group.WastedBytes
	}
	return total
}

var reportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"bytes": formatBytes,
	"pct":   formatPercent,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Digital Exhaust Cleaner</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f7f8fb;
      --panel: #ffffff;
      --ink: #17202a;
      --muted: #687385;
      --line: #dfe5ee;
      --blue: #2456d6;
      --teal: #087f7a;
      --amber: #9a6400;
      --red: #b42318;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--ink);
      font: 14px/1.5 "Segoe UI", system-ui, sans-serif;
    }
    header {
      padding: 24px 32px 16px;
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    h1, h2 { margin: 0; letter-spacing: 0; }
    h1 { font-size: 24px; font-weight: 700; }
    h2 { font-size: 16px; margin-bottom: 12px; }
    main {
      width: min(1180px, calc(100vw - 32px));
      margin: 24px auto 48px;
    }
    .path { color: var(--muted); margin-top: 6px; word-break: break-all; }
    .metrics {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
      margin-bottom: 20px;
    }
    .metric, section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
    }
    .metric { padding: 16px; }
    .metric span { color: var(--muted); display: block; font-size: 12px; text-transform: uppercase; }
    .metric strong { display: block; font-size: 24px; margin-top: 4px; }
    section { padding: 18px; margin-bottom: 16px; }
    table { width: 100%; border-collapse: collapse; }
    th, td {
      padding: 10px 8px;
      border-top: 1px solid var(--line);
      text-align: left;
      vertical-align: top;
    }
    th { color: var(--muted); font-weight: 600; font-size: 12px; text-transform: uppercase; }
    .score {
      display: inline-block;
      min-width: 52px;
      padding: 2px 8px;
      border-radius: 999px;
      color: white;
      background: var(--blue);
      text-align: center;
      font-weight: 600;
    }
    .category { color: var(--teal); font-weight: 650; white-space: nowrap; }
    .rules { color: var(--muted); font-size: 12px; }
    button {
      border: 1px solid var(--line);
      border-radius: 6px;
      background: var(--panel);
      color: var(--ink);
      cursor: pointer;
      font: inherit;
      padding: 6px 10px;
    }
    button:hover { border-color: var(--blue); color: var(--blue); }
    button.danger { border-color: #f2c7c3; color: var(--red); }
    button.danger:hover { background: #fff4f2; }
    button:disabled { color: var(--muted); cursor: not-allowed; opacity: .7; }
    .empty { color: var(--muted); padding: 16px 0 4px; }
    @media (max-width: 760px) {
      header { padding: 18px; }
      .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      th:nth-child(4), td:nth-child(4) { display: none; }
    }
  </style>
</head>
<body>
  <header>
    <h1>Digital Exhaust Cleaner</h1>
    <div class="path">{{.Result.Root}}</div>
  </header>
  <main>
    <div class="metrics">
      <div class="metric"><span>Files scanned</span><strong>{{.Result.FilesScanned}}</strong></div>
      <div class="metric"><span>Recommendations</span><strong>{{len .Result.Recommendations}}</strong></div>
      <div class="metric"><span>Duplicate groups</span><strong>{{len .Result.DuplicateGroups}}</strong></div>
      <div class="metric"><span>Recoverable</span><strong>{{bytes .TotalRecoverable}}</strong></div>
    </div>

    <section>
      <h2>Cleanup Recommendations</h2>
      {{if .Result.Recommendations}}
      <table>
        <thead><tr><th>Score</th><th>Category</th><th>Explanation</th><th>Path</th>{{if .Interactive}}<th>Action</th>{{end}}</tr></thead>
        <tbody>
          {{range .Result.Recommendations}}
          <tr>
            <td><span class="score">{{printf "%.2f" .Score}}</span></td>
            <td class="category">{{.Category}}<div class="rules">{{range .Rules}}{{.}} {{end}}</div></td>
            <td>{{.Explanation}}</td>
            <td>{{.Path}}</td>
            {{if $.Interactive}}<td><button class="danger" data-path="{{.Path}}" onclick="quarantine(this)">Delete</button></td>{{end}}
          </tr>
          {{end}}
        </tbody>
      </table>
      {{else}}<div class="empty">No cleanup recommendations were generated.</div>{{end}}
    </section>

    <section>
      <h2>Behavior and Semantic Signals</h2>
      {{if or .Result.Classifications .Result.Findings}}
      <table>
        <thead><tr><th>Type</th><th>Confidence</th><th>Explanation</th><th>Path</th></tr></thead>
        <tbody>
          {{range .Result.Classifications}}
          <tr><td class="category">{{.Label}}</td><td>{{pct .Confidence}}</td><td>{{.Explanation}}</td><td>{{.Path}}</td></tr>
          {{end}}
          {{range .Result.Findings}}
          <tr><td class="category">{{.Pattern}}</td><td>{{pct .Confidence}}</td><td>{{.Explanation}}</td><td>{{.Path}}</td></tr>
          {{end}}
        </tbody>
      </table>
      {{else}}<div class="empty">No behavioral or semantic signals were found.</div>{{end}}
    </section>
  </main>
  {{if .Interactive}}
  <script>
    async function quarantine(button) {
      const path = button.dataset.path;
      if (!confirm("Move this file to quarantine?\n\n" + path)) return;
      button.disabled = true;
      button.textContent = "Moving...";
      const response = await fetch("/api/quarantine", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({path})
      });
      if (!response.ok) {
        const body = await response.text();
        button.disabled = false;
        button.textContent = "Delete";
        alert(body || "Unable to quarantine file.");
        return;
      }
      button.textContent = "Quarantined";
      button.closest("tr").style.opacity = "0.55";
    }
  </script>
  {{end}}
</body>
</html>`))

func formatBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.0f%%", value*100)
}
