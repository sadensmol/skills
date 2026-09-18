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

## Deployments, ReplicaSets, StatefulSets

```bash
# deployment status and strategy
kubectl get deployments -n <namespace> -o wide
kubectl describe deployment <name> -n <namespace>

# rollout status and history
kubectl rollout status deployment/<name> -n <namespace>
kubectl rollout history deployment/<name> -n <namespace>

# replicasets (useful for debugging failed rollouts)
kubectl get rs -n <namespace>

# statefulsets
kubectl get statefulsets -n <namespace>
kubectl describe statefulset <name> -n <namespace>

# daemonsets
kubectl get daemonsets -A
```

## Network Architecture & Service Discovery

### Services

```bash
# all services with type, cluster IP, external IP, ports
kubectl get svc -n <namespace> -o wide
kubectl get svc -A -o wide

# service details (endpoints, selectors, ports)
kubectl describe svc <service-name> -n <namespace>

# service endpoints (which pods back a service)
kubectl get endpoints <service-name> -n <namespace>
kubectl get endpointslices -n <namespace>

# verify selector matches running pods
kubectl get pods -l <key>=<value> -n <namespace>
```

### DNS & Internal Resolution

```bash
# check DNS from inside a pod
kubectl exec -it <pod-name> -n <namespace> -- nslookup <service-name>
kubectl exec -it <pod-name> -n <namespace> -- nslookup <service-name>.<namespace>.svc.cluster.local

# check CoreDNS pods
kubectl get pods -n kube-system -l k8s-app=kube-dns
kubectl logs -n kube-system -l k8s-app=kube-dns

# DNS config inside pod
kubectl exec <pod-name> -n <namespace> -- cat /etc/resolv.conf
```

### Connectivity Testing

```bash
# test TCP connectivity between pods
kubectl exec -it <source-pod> -n <namespace> -- nc -zv <target-service> <port>
kubectl exec -it <source-pod> -n <namespace> -- wget -qO- --timeout=5 http://<target-service>:<port>/health

# curl from inside a pod
kubectl exec -it <pod-name> -n <namespace> -- curl -s http://<service-name>:<port>/endpoint

# if curl/wget not available, use a debug pod
kubectl run debug --rm -it --image=nicolaka/netshoot -- /bin/bash
```

### Ingress & Egress

```bash
# ingress resources
kubectl get ingress -A
kubectl describe ingress <name> -n <namespace>

# ingress controller pods and logs
kubectl get pods -A | grep ingress
kubectl logs -n <ingress-namespace> <ingress-controller-pod>

# network policies (firewall rules between pods)
kubectl get networkpolicies -n <namespace>
kubectl describe networkpolicy <name> -n <namespace>

# check if network policies are blocking traffic
kubectl get networkpolicies -A -o yaml
```

### Service Mesh (Istio)

```bash
# istio sidecar status
kubectl get pods -n <namespace> -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{","}{end}{"\n"}{end}'

# virtual services and destination rules
kubectl get virtualservices -n <namespace>
kubectl get destinationrules -n <namespace>
kubectl describe virtualservice <name> -n <namespace>

# istio proxy config for a pod
istioctl proxy-config routes <pod-name> -n <namespace>
istioctl proxy-config clusters <pod-name> -n <namespace>
istioctl proxy-config listeners <pod-name> -n <namespace>

# check envoy sidecar logs
kubectl logs <pod-name> -c istio-proxy -n <namespace>
```

## ConfigMaps & Secrets

```bash
# list
kubectl get configmaps -n <namespace>
kubectl get secrets -n <namespace>

# view configmap data
kubectl describe configmap <name> -n <namespace>
kubectl get configmap <name> -n <namespace> -o yaml

# view secret (base64 encoded)
kubectl get secret <name> -n <namespace> -o yaml

# decode a secret value
kubectl get secret <name> -n <namespace> -o jsonpath='{.data.<key>}' | base64 -d
```

## Resource Inspection with JSONPath & Custom Columns

```bash
# pod IPs
kubectl get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.podIP}{"\n"}{end}' -n <namespace>

# container images per pod
kubectl get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.image}{","}{end}{"\n"}{end}' -n <namespace>

# custom columns
kubectl get pods -o custom-columns='NAME:.metadata.name,STATUS:.status.phase,IP:.status.podIP,NODE:.spec.nodeName' -n <namespace>

# all environment variables for a pod
kubectl get pod <pod-name> -n <namespace> -o jsonpath='{range .spec.containers[*].env[*]}{.name}={.value}{"\n"}{end}'
```

## Persistent Storage

```bash
# persistent volumes and claims
kubectl get pv
kubectl get pvc -n <namespace>
kubectl describe pvc <name> -n <namespace>

# storage classes
kubectl get storageclasses
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
