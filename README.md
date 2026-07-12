# kubectl-guard agent lab

**See how [kubectl-guard](https://github.com/lockhinator/kubectl-guard) contains an AI agent operating a Kubernetes cluster — live, in Grafana.**

A runnable demo of using kubectl-guard as a **guardrail + audit layer around an agentic Kubernetes workflow**. An LLM agent ([HolmesGPT](https://github.com/robusta-dev/holmesgpt)) investigates and tries to act on a cluster; kubectl-guard sits under its `kubectl`, lets the safe reads through, **blocks secret reads and production mutations**, routes gated actions for **human approval through the agent**, and **ships every decision to Prometheus + Grafana** — so it's visible exactly what the agent did, what it tried, and what was stopped.

> ⚠️ **MVP / scaffold.** The observability pipeline + scripted driver are runnable now; the HolmesGPT container is wired but not yet tested end-to-end (needs a cluster + an LLM key). See [Status](#status).

## The story

| The agent tries to… | kubectl-guard | The result |
|---|---|---|
| Investigate (`get`/`describe`/`logs`) | **allows** | reads flow; the guard is invisible for safe work |
| Read a Secret (`get secret -o yaml`) | **blocks** (`protected_resources`) | the secret never enters the model's context window |
| Mutate production (`delete deploy -n prod`) | **blocks** (`protected_namespaces`, block mode) | the attempt is recorded |
| Drain a node | **gates** → agent-relay | a JSON approval request the agent relays to a human |
| everything | **audits** → webhook | Prometheus counters + a live Grafana dashboard |

The point vs. RBAC: RBAC is allow/deny with no memory. kubectl-guard adds **a per-command audit of what the agent *tried***, gate-with-justification, output redaction, and human-in-the-loop — the observability and control RBAC can't provide. Use both.

## Architecture

```
                 shimmed kubectl                audit_webhook_url (JSON per decision)
  HolmesGPT ───────────────────► kubectl-guard ───────────────────────────► audit-exporter
  (or the driver script)          (policy: config/kubectl-guard.yaml)          │  │
                                        │ forwards allowed cmds                 │  └─ stdout JSON ─► Loki (drill-down)
                                        ▼                                       ▼ /metrics
                                   real kubectl ─► cluster            Prometheus ─► Grafana ("Agent Activity")
```

The one custom piece is [`exporter/`](exporter/) (~150 lines of Go): it receives kubectl-guard's audit webhook POSTs and exposes `kubectlguard_decisions_total{outcome, verb, actor, context, reason}`.

## Quickstart

Works against any Kubernetes cluster the active kubeconfig points at. Blocked/denied commands never reach the cluster; allowed reads do.

### Part 1 — the pipeline, no LLM required (fastest "aha")

```bash
# 1. exporter + Prometheus + Grafana
cd deploy && docker compose up --build -d && cd ..

# 2. drive a spread of guarded commands against the active kube context
KG=/path/to/kubectl-guard ./driver/run-demo.sh      # or rely on PATH / a package install

# 3. open Grafana
open http://localhost:3000     # anonymous access; admin/admin for editing
#    -> dashboard "kubectl-guard — Agent Activity"
```

The dashboard fills in with the allowed reads, the blocked secret reads and production mutations, and the node commands routed for approval — counted by outcome, verb, and reason.

### Part 2 — drive it with the HolmesGPT agent

Give the agent a task and let it generate the kubectl for real. Its `kubectl` is the guard shim (see [`agent/Dockerfile`](agent/Dockerfile)).

```bash
docker build -t holmesgpt-lab ./agent
docker run --rm \
  -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  -e KUBECTL_GUARD_CONFIG=/etc/kubectl-guard/kubectl-guard.yaml \
  -v "$HOME/.kube:/root/.kube:ro" \
  --add-host host.docker.internal:host-gateway \
  holmesgpt-lab ask "$(cat scenarios/03-read-secret.txt)"
```

The agent will try to read the secret, the guard blocks it, and it fails safe in Grafana. Try `scenarios/02-remediate-prod.txt` (blocked mutation) and `01-investigate.txt` (allowed reads).

> When the agent runs in a container, point `audit_webhook_url` at the host exporter — set it to `http://host.docker.internal:9099/audit` in the mounted config (the baked-in default is `localhost:9099` for the Part-1 local path).

## Repo layout

```
exporter/    audit-webhook → Prometheus/Loki exporter (Go, unit-tested)
config/      example kubectl-guard.yaml (the demo policy)
driver/      run-demo.sh — prove the pipeline with scripted kubectl (no LLM)
agent/       HolmesGPT + kubectl + kubectl-guard shim (Dockerfile)
scenarios/   natural-language tasks for the agent
deploy/      docker-compose (exporter + Prometheus + Grafana) + provisioned dashboard
```

## Status

Runnable now: the exporter (built + unit-tested), the docker-compose observability stack, the Grafana dashboard, and the scripted driver.

Next: finish the HolmesGPT end-to-end run against a real cluster + LLM; add Loki + a "recent blocked commands" log panel; add in-cluster manifests (kube-prometheus-stack + the exporter + the agent as a Job) for a fully in-cluster deploy; wire `audit_hmac_key_file` to show the tamper-evident audit chain; add the agent-relay → chat approval loop.

## Honest limitations

kubectl-guard guards **`kubectl` subprocess calls** — an agent that talks to the Kubernetes API directly (client-go) bypasses it. So the demo agent uses `kubectl` as its tool (the common LLM pattern), and for a real deployment you pair the guard with RBAC on the agent's ServiceAccount. The guard is defense-in-depth and an audit/usability layer, **not** an admission controller.
