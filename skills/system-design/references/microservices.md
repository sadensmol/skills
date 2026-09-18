# Microservices Architecture

## Service Design Principles

- Single responsibility per service
- Domain-driven boundaries (bounded contexts)
- Database per service
- API-first development
- Stateless service design
- Configuration externalization
- Graceful degradation

## Service Decomposition

### Domain Analysis
1. Bounded context mapping
2. Aggregate identification
3. Event storming for event flows
4. Service dependency analysis
5. Transaction boundary definition
6. Team topology alignment (Conway's law)

### Migration from Monolith
1. Identify seams in existing code
2. Extract services by domain
3. Decouple data gradually
4. Define migration pathway with rollback plan
5. Measure success metrics at each step

## Communication Patterns

### Synchronous
- **REST**: resource-oriented HTTP APIs
- **gRPC**: binary protocol for internal service calls (lower latency)

### Asynchronous
- **Event-driven**: services react to domain events
- **Pub/sub**: broadcast events to multiple consumers
- **Message queues**: point-to-point reliable delivery
- **Fire-and-forget**: no response expected

### Data Patterns
- **Event sourcing**: store events not state
- **CQRS**: separate read/write models
- **Saga**: coordinate distributed transactions
- **Outbox**: reliable event publishing with DB transactions

## Resilience Strategies

| Pattern | Purpose |
|---------|---------|
| Circuit breaker | fail fast when downstream unhealthy |
| Retry with backoff | handle transient failures |
| Timeout | prevent resource exhaustion |
| Bulkhead | isolate failures to prevent cascade |
| Rate limiting | protect from overload |
| Fallback | degrade gracefully |
| Health checks | enable orchestrator to route traffic |

## Data Management

- **Database per service**: each service owns its data
- **Eventual consistency**: accept temporary inconsistency for availability
- **Schema evolution**: backward-compatible changes
- **Data synchronization**: events for cross-service data needs

## Container Orchestration (Kubernetes)

```yaml
# essential resource configuration
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "256Mi"
    cpu: "500m"
```

Key configurations:
- Deployments with rolling updates
- Services for internal discovery
- Ingress for external traffic
- ConfigMaps for configuration
- Secrets for sensitive data
- HPA for autoscaling
- Network policies for security

## Service Mesh (Istio/Linkerd)

Capabilities:
- Traffic management (routing, load balancing)
- Canary and blue/green deployments
- Mutual TLS between services
- Authorization policies
- Observability (metrics, traces)
- Fault injection for testing

## Observability

### Three Pillars
1. **Metrics**: Prometheus + Grafana dashboards
2. **Logs**: centralized logging (ELK, Loki)
3. **Traces**: distributed tracing (Jaeger, Zipkin)

### Operational Excellence
- Define SLIs and SLOs
- Set up alerting on SLO breaches
- Create runbooks for common incidents
- Practice chaos engineering

## Deployment Strategies

| Strategy | Use Case |
|----------|----------|
| Rolling | standard zero-downtime deploys |
| Blue/green | instant rollback capability |
| Canary | gradual rollout with metrics validation |
| Feature flags | decouple deploy from release |

## Security

- Zero-trust networking (verify every request)
- mTLS for service-to-service communication
- API gateway for external traffic (auth, rate limiting)
- Secret rotation automation
- Vulnerability scanning in CI/CD
- Audit logging for compliance

## Production Checklist

- [ ] Load testing completed
- [ ] Failure scenarios tested (chaos engineering)
- [ ] Monitoring dashboards configured
- [ ] Alerting rules defined
- [ ] Runbooks documented
- [ ] Disaster recovery tested
- [ ] Security scanning passed
- [ ] Team on-call rotation established

## Cost Optimization

- Right-size resource requests/limits
- Use spot/preemptible instances for non-critical workloads
- Consider serverless for variable load
- Optimize data transfer between regions
- Eliminate idle resources
- Multi-tenant where appropriate
