# Metrics and Monitoring

This document covers the comprehensive Prometheus metrics and monitoring capabilities provided by the DataMover Operator.

## Overview

The DataMover Operator provides detailed Prometheus metrics for monitoring backup operations, performance tracking, and operational observability. These metrics enable effective monitoring, alerting, and troubleshooting of data movement operations.

## Available Metrics

### Operation Metrics

#### `datamover_operations_total`

Counter tracking total operations by phase and status.

**Labels**:
- `phase`: Operation phase (CreatingClonedPVC, CreatingPod, CleaningUp)
- `status`: Operation status (started, success, failure)
- `namespace`: Kubernetes namespace

**Examples**:
```prometheus
datamover_operations_total{phase="CreatingClonedPVC", status="started", namespace="default"} 10
datamover_operations_total{phase="CreatingClonedPVC", status="success", namespace="default"} 9
datamover_operations_total{phase="CreatingClonedPVC", status="failure", namespace="default"} 1
```

#### `datamover_phase_duration_seconds`

Histogram tracking phase execution duration.

**Labels**:
- `phase`: Operation phase
- `namespace`: Kubernetes namespace

**Buckets**: 0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, +Inf seconds

**Examples**:
```prometheus
datamover_phase_duration_seconds_bucket{phase="CreatingPod", namespace="default", le="300"} 45
datamover_phase_duration_seconds_sum{phase="CreatingPod", namespace="default"} 12847.3
datamover_phase_duration_seconds_count{phase="CreatingPod", namespace="default"} 50
```

### Cleanup Metrics

#### `datamover_cleanup_operations_total`

Counter tracking PVC cleanup operations.

**Labels**:
- `status`: Cleanup status (success, failure)
- `namespace`: Kubernetes namespace

**Examples**:
```prometheus
datamover_cleanup_operations_total{status="success", namespace="default"} 25
datamover_cleanup_operations_total{status="failure", namespace="default"} 2
```

### Job Metrics

#### `datamover_pod_creation_operations_total`

Counter tracking job/pod creation operations.

**Labels**:
- `status`: Operation status (started, success, failure)
- `namespace`: Kubernetes namespace

**Examples**:
```prometheus
datamover_pod_creation_operations_total{status="started", namespace="default"} 30
datamover_pod_creation_operations_total{status="success", namespace="default"} 28
datamover_pod_creation_operations_total{status="failure", namespace="default"} 2
```

#### `datamover_data_sync_operations_total`

Counter tracking data synchronization operations.

**Labels**:
- `status`: Sync status (success, failure)
- `namespace`: Kubernetes namespace

**Examples**:
```prometheus
datamover_data_sync_operations_total{status="success", namespace="default"} 25
datamover_data_sync_operations_total{status="failure", namespace="default"} 3
```

## Metric Collection

### Prometheus Configuration

Configure Prometheus to scrape DataMover metrics:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'datamover-operator'
    static_configs:
      - targets: ['datamover-operator-controller-manager-metrics-service:8080']
    scrape_interval: 30s
    metrics_path: /metrics
```

### ServiceMonitor (Prometheus Operator)

For Prometheus Operator deployments:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: datamover-operator
  namespace: datamover-operator-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: https
    scheme: https
    path: /metrics
    tlsConfig:
      insecureSkipVerify: true
```

### Manual Metric Access

Access metrics directly:

```bash
# Port-forward to metrics endpoint
kubectl port-forward -n datamover-operator-system service/datamover-operator-controller-manager-metrics-service 8080:8080

# Query metrics
curl http://localhost:8080/metrics | grep datamover
```

## Monitoring Dashboards

### Grafana Dashboard Example

Create comprehensive Grafana dashboard:

```json
{
  "dashboard": {
    "title": "DataMover Operator",
    "panels": [
      {
        "title": "Operation Success Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(datamover_operations_total{status=\"success\"}[5m]) / rate(datamover_operations_total[5m]) * 100"
          }
        ]
      },
      {
        "title": "Phase Duration",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(datamover_phase_duration_seconds_bucket[5m]))"
          }
        ]
      },
      {
        "title": "Operations by Phase",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(datamover_operations_total[5m])"
          }
        ]
      }
    ]
  }
}
```

### Key Performance Indicators (KPIs)

Monitor these critical metrics:

1. **Success Rate**: Overall operation success percentage
2. **Phase Duration**: Time taken for each operation phase
3. **Failure Rate**: Operations failing per time period
4. **Cleanup Success**: PVC cleanup operation success rate

### Dashboard Panels

#### Success Rate Panel
```promql
# Overall success rate
rate(datamover_operations_total{status="success"}[5m]) / 
rate(datamover_operations_total[5m]) * 100
```

#### Average Phase Duration Panel
```promql
# Average phase duration by phase
rate(datamover_phase_duration_seconds_sum[5m]) / 
rate(datamover_phase_duration_seconds_count[5m])
```

#### Operations Volume Panel
```promql
# Operations per minute by phase
rate(datamover_operations_total[1m]) * 60
```

#### Cleanup Success Rate Panel
```promql
# Cleanup success rate
rate(datamover_cleanup_operations_total{status="success"}[5m]) / 
rate(datamover_cleanup_operations_total[5m]) * 100
```

## Alerting

### Prometheus Alerting Rules

Configure alerts for operational issues:

```yaml
groups:
- name: datamover.rules
  rules:
  
  # High failure rate alert
  - alert: DataMoverHighFailureRate
    expr: |
      (
        rate(datamover_operations_total{status="failure"}[5m]) / 
        rate(datamover_operations_total[5m])
      ) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "DataMover high failure rate"
      description: "DataMover failure rate is above 10% in namespace {{ $labels.namespace }}"

  # Long-running operations alert
  - alert: DataMoverLongRunningOperation
    expr: |
      histogram_quantile(0.95, rate(datamover_phase_duration_seconds_bucket[5m])) > 1800
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "DataMover operations taking too long"
      description: "95th percentile of DataMover operations exceeds 30 minutes"

  # Cleanup failures alert
  - alert: DataMoverCleanupFailures
    expr: |
      rate(datamover_cleanup_operations_total{status="failure"}[5m]) > 0
    for: 1m
    labels:
      severity: warning
    annotations:
      summary: "DataMover cleanup operations failing"
      description: "PVC cleanup operations are failing in namespace {{ $labels.namespace }}"

  # No operations alert (for scheduled backups)
  - alert: DataMoverNoOperations
    expr: |
      rate(datamover_operations_total[1h]) == 0
    for: 2h
    labels:
      severity: info
    annotations:
      summary: "No DataMover operations detected"
      description: "No DataMover operations in the last hour"

  # Job retry exhaustion alert
  - alert: DataMoverJobRetriesExhausted
    expr: |
      rate(datamover_pod_creation_operations_total{status="failure"}[5m]) > 0
    for: 0m
    labels:
      severity: critical
    annotations:
      summary: "DataMover jobs exhausting retries"
      description: "DataMover jobs are failing after all retry attempts"
```

### Alert Severity Levels

**Critical**: Immediate attention required
- Job retries exhausted
- Complete operation failures
- Security violations

**Warning**: Investigation needed
- High failure rates
- Performance degradation
- Cleanup failures

**Info**: Awareness notifications
- No operations detected
- Configuration changes
- Routine maintenance

## Performance Monitoring

### Baseline Metrics

Establish baseline performance metrics:

| Metric | Typical Range | Alert Threshold |
|--------|---------------|-----------------|
| PVC Clone Creation | 30s - 5min | > 10min |
| Job Execution | 1min - 30min | > 60min |
| Cleanup Operation | 5s - 30s | > 2min |
| Overall Success Rate | > 95% | < 90% |

### Performance Queries

#### Average Operation Duration
```promql
# Average duration by phase over last hour
avg_over_time(
  rate(datamover_phase_duration_seconds_sum[5m])[1h:5m]
) / 
avg_over_time(
  rate(datamover_phase_duration_seconds_count[5m])[1h:5m]
)
```

#### Success Rate Trend
```promql
# Success rate trend over 24 hours
rate(datamover_operations_total{status="success"}[1h]) / 
rate(datamover_operations_total[1h])
```

#### Resource Utilization Correlation
```promql
# Correlate phase duration with resource usage
rate(datamover_phase_duration_seconds_sum[5m]) and on() 
rate(container_cpu_usage_seconds_total{pod=~"datamover-operator.*"}[5m])
```
