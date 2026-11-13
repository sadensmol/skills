# Kubernetes workloads, config, storage & JSONPath

Read this when you inspect deployments, rollouts, statefulsets or daemonsets, read configmaps or secrets, extract fields with JSONPath / custom columns, or check persistent storage.

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
