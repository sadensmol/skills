---
name: sadensmol-k8s
description: "Kubernetes cluster inspection, debugging, and diagnostics using kubectl. Use when: (1) checking k8s resource configuration (pods, services, deployments, configmaps, secrets), (2) reading pod/container logs, (3) diagnosing pod failures or CrashLoopBackOff, (4) investigating network architecture between pods and services, (5) tracing service-to-service connectivity, (6) inspecting ingress/egress rules, (7) debugging DNS resolution within the cluster, (8) checking resource quotas and limits, (9) any Kubernetes troubleshooting or cluster inspection task."
---

# Kubernetes Diagnostics & Inspection

## Context & Namespace

Always establish context before running commands:

```bash
# current context and cluster
kubectl config current-context
kubectl config get-contexts

# switch context
kubectl config use-context <context-name>

# list namespaces
kubectl get namespaces

# set default namespace for session
kubectl config set-context --current --namespace=<namespace>
```

All commands below assume correct context. Add `-n <namespace>` when targeting a specific namespace, or `--all-namespaces` / `-A` for cluster-wide view.

## Cluster Overview

```bash
# node status and resource capacity
kubectl get nodes -o wide
kubectl top nodes

# all resources in a namespace
kubectl get all -n <namespace>

# cluster-wide events sorted by time
kubectl get events -A --sort-by='.lastTimestamp'

# resource quotas and limits
kubectl get resourcequotas -A
kubectl get limitranges -A
```

## Pod Inspection & Debugging

### Pod Status

```bash
# pods with node placement and IPs
kubectl get pods -o wide -n <namespace>

# pod resource usage
kubectl top pods -n <namespace>

# filter by label
kubectl get pods -l app=<app-name> -n <namespace>

# detailed pod info (events, conditions, volumes, containers)
kubectl describe pod <pod-name> -n <namespace>
```

### Common Pod Issues

**CrashLoopBackOff / Error**:
```bash
# check exit code and restart count
kubectl get pod <pod-name> -o jsonpath='{.status.containerStatuses[*].lastState.terminated}' -n <namespace>

# previous container logs (crashed container)
kubectl logs <pod-name> --previous -n <namespace>

# init container logs
kubectl logs <pod-name> -c <init-container-name> -n <namespace>
```

**Pending pods**:
```bash
# check events for scheduling failures
kubectl describe pod <pod-name> -n <namespace> | grep -A 20 Events

# check node resources vs requests
kubectl describe nodes | grep -A 5 "Allocated resources"
```

**ImagePullBackOff**:
```bash
kubectl describe pod <pod-name> -n <namespace> | grep -A 5 "Events"
# verify image exists and credentials are correct
kubectl get secrets -n <namespace> | grep docker
```

### Logs

```bash
# current logs
kubectl logs <pod-name> -n <namespace>

# specific container in multi-container pod
kubectl logs <pod-name> -c <container-name> -n <namespace>

# stream logs
kubectl logs -f <pod-name> -n <namespace>

# last N lines
kubectl logs --tail=100 <pod-name> -n <namespace>

# logs since duration
kubectl logs --since=1h <pod-name> -n <namespace>

# all pods matching a label
kubectl logs -l app=<app-name> --all-containers -n <namespace>
```

### Exec Into Pod

```bash
# interactive shell
kubectl exec -it <pod-name> -n <namespace> -- /bin/sh

# run a single command
kubectl exec <pod-name> -n <namespace> -- cat /etc/resolv.conf
```

## Diagnostic Workflow

When debugging an issue, follow this sequence:

1. **Identify failing resource** — `kubectl get pods -n <ns>` / `kubectl get events -n <ns> --sort-by='.lastTimestamp'`
2. **Describe the resource** — `kubectl describe pod/svc/deploy <name> -n <ns>` (check Events section)
3. **Check logs** — `kubectl logs <pod> -n <ns>` and `--previous` for crashed containers
4. **Verify networking** — `kubectl get svc`, `kubectl get endpoints`, DNS resolution, connectivity test
5. **Check config** — configmaps, secrets, env vars injected into pod
6. **Check resources** — `kubectl top pod`, resource requests/limits vs node capacity
7. **Check network policies** — `kubectl get networkpolicies -n <ns>` for blocked traffic

For detailed reference on specific topics, see `references/advanced-diagnostics.md`.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You inspect services/endpoints, debug DNS, test connectivity, or check ingress/egress, network policies or Istio | `references/networking.md` |
| You inspect deployments, rollouts, ReplicaSets, StatefulSets or DaemonSets | `references/workloads-config.md` |
| You read ConfigMaps or Secrets, extract fields with JSONPath / custom columns, or check PV / PVC / storage classes | `references/workloads-config.md` |
| You debug nodes, port-forward, RBAC, Jobs/CronJobs, HPA, TLS certificates, control-plane health or config drift | `references/advanced-diagnostics.md` |
