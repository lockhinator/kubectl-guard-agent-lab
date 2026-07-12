#!/usr/bin/env bash
# Prove the guard -> exporter -> Prometheus -> Grafana pipeline WITHOUT an LLM:
# run a spread of kubectl commands THROUGH kubectl-guard (which forwards to your
# real kubectl) and watch the "Agent Activity" dashboard fill in. Blocked/denied
# commands never reach the cluster; allowed reads run against your current context.
#
#   KG=/path/to/kubectl-guard ./driver/run-demo.sh      # or rely on PATH / brew
set -uo pipefail
KG="${KG:-kubectl-guard}"
HERE="$(cd "$(dirname "$0")" && pwd)"
export KUBECTL_GUARD_CONFIG="$(cd "$HERE/../config" && pwd)/kubectl-guard.yaml"
export KUBECTL_GUARD_ACTOR="${KUBECTL_GUARD_ACTOR:-demo-agent}"
export KUBECTL_GUARD_NO_PROMPT=1   # gated commands abort (relayed) instead of prompting

label() { case "$1" in 0) echo "ALLOWED";; 2) echo "BLOCKED";; 3) echo "DENIED";; 4) echo "NEEDS-APPROVAL";; *) echo "exit $1";; esac; }
run() { printf '  kubectl %-45s -> ' "$*"; "$KG" "$@" >/dev/null 2>&1; echo "$(label $?)"; }

echo "Config: $KUBECTL_GUARD_CONFIG   (audit -> http://localhost:9099/audit)"
echo "Actor:  $KUBECTL_GUARD_ACTOR"
echo
echo "== investigate (reads pass) =="
run get pods
run get pods -A
run get events -n default
echo
echo "== try to exfiltrate a secret (blocked everywhere) =="
run get secret db-credentials -o yaml
run get secrets -A
echo
echo "== try to mutate prod (blocked) =="
run delete pod payments-abc -n prod
run delete deployment web -n prod
run scale deployment web --replicas=0 -n prod
echo
echo "== touch a node (gated -> relayed for human approval) =="
run drain worker-1
run delete node worker-1
echo
echo "Open Grafana at http://localhost:3000  ->  'kubectl-guard — Agent Activity'."
