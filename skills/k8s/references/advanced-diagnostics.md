# Advanced Kubernetes Diagnostics

## Table of Contents

- [Node-Level Diagnostics](#node-level-diagnostics)
- [Network Debugging Deep Dive](#network-debugging-deep-dive)
- [RBAC & Permissions](#rbac--permissions)
- [Jobs & CronJobs](#jobs--cronjobs)
- [HPA & Scaling](#hpa--scaling)
- [Certificate & TLS Issues](#certificate--tls-issues)
- [Cluster Component Health](#cluster-component-health)
- [Resource Diff & Drift Detection](#resource-diff--drift-detection)

## Node-Level Diagnostics

```bash
# node conditions (disk pressure, memory pressure, PID pressure)
kubectl describe node <node-name> | grep -A 10 Conditions

# pods on a specific node
kubectl get pods -A --field-selector spec.nodeName=<node-name>

# node taints and tolerations
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.taints}{"\n"}{end}'

# node labels (for scheduling decisions)
kubectl get nodes --show-labels

# cordon/drain status
kubectl get nodes -o custom-columns='NAME:.metadata.name,UNSCHEDULABLE:.spec.unschedulable'
```

## Network Debugging Deep Dive

### Port Forwarding for Local Testing

```bash
# forward pod port to localhost
kubectl port-forward pod/<pod-name> <local-port>:<pod-port> -n <namespace>

# forward service port
kubectl port-forward svc/<service-name> <local-port>:<service-port> -n <namespace>
```

### Network Policy Debugging

```bash
# list all network policies with their pod selectors
kubectl get networkpolicies -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,POD-SELECTOR:.spec.podSelector.matchLabels'

# check which pods are affected by a network policy
kubectl get pods -n <namespace> -l <selector-from-policy>

# full policy spec
kubectl get networkpolicy <name> -n <namespace> -o yaml
```

### Service Mesh Debugging (Istio)

```bash
# check mutual TLS status
istioctl authn tls-check <pod-name>.<namespace>

# analyze configuration for issues
istioctl analyze -n <namespace>

# proxy status
istioctl proxy-status

# envoy access logs
kubectl logs <pod-name> -c istio-proxy -n <namespace> --tail=50

# gateway resources
kubectl get gateways -A
kubectl describe gateway <name> -n <namespace>
```

### Tracing Service Communication

Map the full service graph:

```bash
# all services and their endpoints
kubectl get svc -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,TYPE:.spec.type,CLUSTER-IP:.spec.clusterIP,PORTS:.spec.ports[*].port,SELECTOR:.spec.selector'

# find which deployment/pod serves a service
kubectl get endpoints <svc-name> -n <ns> -o yaml
# then check pod labels to find the owning deployment

# trace a request path: ingress → service → pod
kubectl get ingress -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,HOST:.spec.rules[*].host,BACKEND:.spec.rules[*].http.paths[*].backend.service.name'
```

## RBAC & Permissions

```bash
# check if a service account can perform an action
kubectl auth can-i <verb> <resource> --as=system:serviceaccount:<namespace>:<sa-name> -n <namespace>

# list roles and bindings
kubectl get roles,rolebindings -n <namespace>
kubectl get clusterroles,clusterrolebindings

# describe a role to see its permissions
kubectl describe role <name> -n <namespace>
kubectl describe clusterrole <name>

# service accounts in a namespace
kubectl get serviceaccounts -n <namespace>

# which service account a pod uses
kubectl get pod <pod-name> -n <namespace> -o jsonpath='{.spec.serviceAccountName}'
```

## Jobs & CronJobs

```bash
# list jobs and status
kubectl get jobs -n <namespace>
kubectl describe job <name> -n <namespace>

# cronjob schedule and last run
kubectl get cronjobs -n <namespace>
kubectl describe cronjob <name> -n <namespace>

# pods from a job (check for failures)
kubectl get pods -l job-name=<job-name> -n <namespace>
kubectl logs job/<job-name> -n <namespace>
```

## HPA & Scaling

```bash
# horizontal pod autoscaler status
kubectl get hpa -n <namespace>
kubectl describe hpa <name> -n <namespace>

# current vs desired replicas
kubectl get hpa -n <namespace> -o custom-columns='NAME:.metadata.name,MIN:.spec.minReplicas,MAX:.spec.maxReplicas,CURRENT:.status.currentReplicas,DESIRED:.status.desiredReplicas'

# check metrics availability
kubectl top pods -n <namespace>
kubectl get --raw /apis/metrics.k8s.io/v1beta1/pods -n <namespace> | jq .
```

## Certificate & TLS Issues

```bash
# check TLS secrets
kubectl get secrets -n <namespace> -o custom-columns='NAME:.metadata.name,TYPE:.type' | grep tls

# inspect certificate dates
kubectl get secret <tls-secret> -n <namespace> -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates -subject

# cert-manager certificates (if installed)
kubectl get certificates -A
kubectl describe certificate <name> -n <namespace>
kubectl get certificaterequests -n <namespace>
```

## Cluster Component Health

```bash
# control plane component status
kubectl get componentstatuses  # deprecated but still works on some clusters
kubectl get pods -n kube-system

# etcd health
kubectl get pods -n kube-system -l component=etcd
kubectl logs -n kube-system -l component=etcd --tail=20

# API server health
kubectl get --raw /healthz
kubectl get --raw /readyz

# scheduler and controller manager
kubectl get pods -n kube-system -l component=kube-scheduler
kubectl get pods -n kube-system -l component=kube-controller-manager
```

## Resource Diff & Drift Detection

```bash
# compare running config with stored manifest
kubectl diff -f <manifest.yaml>

# export current state for comparison
kubectl get deployment <name> -n <namespace> -o yaml > current.yaml

# check annotation for last applied config
kubectl get deployment <name> -n <namespace> -o jsonpath='{.metadata.annotations.kubectl\.kubernetes\.io/last-applied-configuration}' | jq .
```
