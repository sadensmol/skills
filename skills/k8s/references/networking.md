# Kubernetes networking & service discovery

Read this when you inspect services and endpoints, debug DNS, test pod-to-pod connectivity, or check ingress, network policies or the Istio mesh.

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
