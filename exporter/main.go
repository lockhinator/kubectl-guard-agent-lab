// audit-exporter turns kubectl-guard's audit webhook stream into Prometheus
// metrics (for dashboards) and structured stdout logs (for Loki / drill-down).
//
// kubectl-guard POSTs one JSON audit entry per decision to its `audit_webhook_url`.
// This service accepts those POSTs at /audit, increments a labeled counter, and
// echoes the raw event to stdout as JSON so a log pipeline (Loki, `docker logs`)
// can surface the exact commands an agent tried. /metrics is the Prometheus
// endpoint.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// auditEvent mirrors kubectl-guard's AuditEntry webhook payload. Only the fields
// used for metrics/labels are decoded; unknown fields (prev/hash/...) are ignored.
type auditEvent struct {
	Time    string `json:"time"`
	Actor   string `json:"actor"`
	Context string `json:"context"`
	Command string `json:"command"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

var (
	// decisions is the workhorse counter. Labels are all BOUNDED cardinality:
	// outcome (~10 values), verb (kubectl verbs), plus actor/context/reason which
	// come from a small configured set. The full command is deliberately NOT a
	// label (unbounded) — it goes to stdout/Loki instead.
	decisions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubectlguard_decisions_total",
			Help: "kubectl-guard decisions, by outcome and target, as reported via the audit webhook.",
		},
		[]string{"outcome", "verb", "actor", "context", "reason"},
	)
	eventsReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "kubectlguard_events_received_total",
		Help: "Total audit webhook events received (including any that failed to parse).",
	})
	eventsInvalid = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "kubectlguard_events_invalid_total",
		Help: "Audit webhook events that could not be parsed as JSON.",
	})
)

func init() {
	prometheus.MustRegister(decisions, eventsReceived, eventsInvalid)
}

// globalValueFlags are kubectl persistent flags that take a SPACE-separated value
// and can appear BEFORE the verb (`kubectl -n prod get pods`). Their value must be
// skipped when locating the verb, or the value (e.g. "prod") is mistaken for it.
// A small, sufficient set for a demo — not the guard's authoritative parser.
var globalValueFlags = map[string]bool{
	"-n": true, "--namespace": true, "--context": true, "--kubeconfig": true,
	"-s": true, "--server": true, "--token": true, "--as": true, "--as-group": true,
	"--as-uid": true, "--user": true, "-v": true, "--v": true, "--request-timeout": true,
	"--cluster": true, "--cache-dir": true, "--tls-server-name": true,
	"--certificate-authority": true, "--client-certificate": true, "--client-key": true,
}

// commandVerb extracts the kubectl verb (first positional token) from the redacted
// command string, so metrics can be grouped by verb (get/delete/exec/...) without
// exploding cardinality on the full command. It skips leading flags and the values
// of space-form global value flags. Returns "unknown" when none is found.
func commandVerb(command string) string {
	toks := strings.Fields(command)
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "-") {
			if strings.ContainsRune(t, '=') {
				continue // --flag=value: self-contained, nothing extra to skip
			}
			if globalValueFlags[t] {
				i++ // skip the flag's space-separated value
			}
			continue
		}
		return strings.ToLower(t)
	}
	return "unknown"
}

// orNone maps an empty label value to "none" so the label set is stable (empty
// label values are legal in Prometheus but read poorly in Grafana).
func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return s
}

func handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	eventsReceived.Inc()

	var ev auditEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		eventsInvalid.Inc()
		// Still 200: a malformed event must not make the guard's best-effort
		// webhook retry or block the command it was auditing.
		w.WriteHeader(http.StatusOK)
		return
	}

	decisions.WithLabelValues(
		orNone(ev.Outcome),
		commandVerb(ev.Command),
		orNone(ev.Actor),
		orNone(ev.Context),
		orNone(ev.Reason),
	).Inc()

	// Echo the raw event to stdout as one JSON line for the log pipeline (Loki),
	// so the exact command an agent attempted is auditable in Grafana's Explore.
	if out, mErr := json.Marshal(ev); mErr == nil {
		log.Println(string(out))
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":9099"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/audit", handleAudit)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	log.SetFlags(0) // structured JSON lines only; no log prefix
	log.Printf(`{"msg":"audit-exporter listening","addr":%q,"audit_path":"/audit","metrics_path":"/metrics"}`, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf(`{"msg":"server error","error":%q}`, err.Error())
	}
}
