# HolmesGPT + kubectl, with kubectl-guard PATH-shadowing kubectl so EVERY kubectl
# HolmesGPT runs is gated and audited. The shim dir is FIRST on PATH, so the plain
# `kubectl` HolmesGPT invokes resolves to the guard, which forwards to the real
# kubectl (skipping itself). See the README for how to run this.
FROM python:3.12-slim

ARG KUBECTL_VERSION=v1.33.0
ARG KUBECTL_GUARD_VERSION=1.0.0
ARG TARGETARCH=amd64

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates tar && rm -rf /var/lib/apt/lists/*

# Real kubectl (kept OUT of the shim dir so the guard can forward to it).
RUN curl -fsSLo /usr/local/bin/kubectl \
      "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
 && chmod +x /usr/local/bin/kubectl

# kubectl-guard from the GitHub release, install the shim.
# (You can instead COPY a locally built binary if you prefer.)
RUN set -eux; \
    base="https://github.com/lockhinator/kubectl-guard/releases/download/v${KUBECTL_GUARD_VERSION}"; \
    curl -fsSLo /tmp/kg.tgz "${base}/kubectl-guard_${KUBECTL_GUARD_VERSION}_linux_${TARGETARCH}.tar.gz"; \
    tar -xzf /tmp/kg.tgz -C /usr/local/bin kubectl-guard; \
    mkdir -p /opt/shim; \
    cp /usr/local/bin/kubectl-guard /opt/shim/kubectl-guard; \
    ln -s kubectl-guard /opt/shim/kubectl

# Shim dir FIRST on PATH -> HolmesGPT's `kubectl` is the guard.
ENV PATH="/opt/shim:${PATH}"
# Point the guard at the demo policy + the audit exporter (override at run time).
ENV KUBECTL_GUARD_CONFIG=/etc/kubectl-guard/kubectl-guard.yaml
ENV KUBECTL_GUARD_ACTOR=holmesgpt
ENV KUBECTL_GUARD_NO_PROMPT=1
COPY --chmod=0644 ../config/kubectl-guard.yaml /etc/kubectl-guard/kubectl-guard.yaml

RUN pip install --no-cache-dir holmesgpt

# HolmesGPT reads $HOME/.kube/config and an LLM key (e.g. OPENAI_API_KEY /
# ANTHROPIC_API_KEY) from the environment. Pass a task as the CMD:
#   docker run ... holmesgpt-lab holmes ask "$(cat scenarios/03-read-secret.txt)"
ENTRYPOINT ["holmes"]
CMD ["ask", "list the pods in the default namespace"]
