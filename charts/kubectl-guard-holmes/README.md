# kubectl-guard-holmes (Helm chart)

Runs **[HolmesGPT](https://github.com/robusta-dev/holmesgpt)** with
**[kubectl-guard](https://github.com/lockhinator/kubectl-guard)** PATH-shadowing
its `kubectl`, so the investigation agent can read freely but **cannot read
Secrets**, has **production mutations gated for human approval**, and has **every
kubectl decision audited** to a Prometheus exporter.

Cluster-agnostic: nothing here is tied to any specific cluster. In-cluster HolmesGPT
uses its pod ServiceAccount (no kubeconfig). The LLM key is referenced from a Secret
**you** create — this chart never contains credentials.

## How it works

An init container installs the kubectl-guard binary + a `kubectl` shim into a shared
volume (checksum-verified from the GitHub release). The HolmesGPT container gets
`/opt/shim` first on `PATH`, so its `kubectl` resolves to the guard, which forwards
to the image's real kubectl. The policy is a ConfigMap; audit events go to the
in-cluster exporter Service.

## Install

```bash
# 1. your LLM key as a Secret (NOT in this chart)
kubectl create secret generic holmes-llm --from-literal=OPENAI_API_KEY=sk-...

# 2. install
helm install holmes ./charts/kubectl-guard-holmes \
  --set llm.existingSecret=holmes-llm
```

## Key values

| Value | Default | Notes |
|---|---|---|
| `llm.existingSecret` | `""` | Name of a Secret with your LLM key (you create it) |
| `kubectlGuard.version` / `.arch` | `1.0.0` / `amd64` | kubectl-guard release + node arch |
| `kubectlGuard.policy` | block secrets + gate prod (relay) + audit all | the guard policy |
| `rbac.allowMutations` | `false` | read-only (no secrets). `true` lets the agent attempt writes so the guard is the gate |
| `exporter.serviceMonitor.enabled` | `false` | set `true` for the Prometheus Operator |

The `audit_webhook_url` is injected automatically to the in-cluster exporter Service.

See `values.yaml` for everything. The Grafana dashboard is
`deploy/grafana/dashboards/agent-activity.json` in this repo.
