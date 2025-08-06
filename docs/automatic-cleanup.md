# Automatic Cleanup

This document explains the automatic PVC cleanup feature that removes cloned PVCs after successful backup operations, helping to manage storage resources efficiently.

## Overview

The automatic cleanup feature allows the DataMover Operator to automatically delete cloned PVCs after successful data synchronization. This helps prevent storage waste, reduces operational overhead, and maintains a clean Kubernetes environment.

## Feature Configuration

### Enabling Automatic Cleanup

Enable automatic cleanup in your DataMover specification:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: auto-cleanup-backup
spec:
  sourcePvc: "app-data"
  secretName: "storage-credentials"
  deletePvcAfterBackup: true  # Enable automatic cleanup
```

### Disabling Automatic Cleanup

Keep cloned PVCs for manual management:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: manual-cleanup-backup
spec:
  sourcePvc: "app-data"
  secretName: "storage-credentials"
  deletePvcAfterBackup: false  # Disable automatic cleanup
```

**Default Behavior**: `deletePvcAfterBackup: false`

## Cleanup Workflow

### Phase Progression

When automatic cleanup is enabled, the DataMover follows this workflow:

1. **CreatingClonedPVC**: Create clone of source PVC
2. **ClonedPVCReady**: Clone is bound and ready
3. **CreatingPod**: Execute rclone job for data sync
4. **CleaningUp**: Delete cloned PVC (if `deletePvcAfterBackup: true`)
5. **Completed**: Operation finished successfully

### Phase Progression Without Cleanup

When automatic cleanup is disabled:

1. **CreatingClonedPVC**: Create clone of source PVC
2. **ClonedPVCReady**: Clone is bound and ready
3. **CreatingPod**: Execute rclone job for data sync
4. **Completed**: Operation finished (clone PVC remains)

### Cleanup Trigger

Cleanup is triggered only after:

- ✅ **Successful job completion**: Rclone job completes successfully
- ✅ **Data sync verification**: Sync operation reports success
- ✅ **Status confirmation**: Job status shows completion

Cleanup is **NOT** triggered when:

- ❌ **Job fails**: Any failure prevents cleanup
- ❌ **Sync errors**: Data synchronization errors prevent cleanup
- ❌ **Operator errors**: Internal operator errors prevent cleanup

## Implementation Details

### Cleanup Logic

The cleanup process involves:

```go
// Simplified cleanup logic
func (r *DataMoverReconciler) cleanupClonedPVC(ctx context.Context, dm *datamoverv1alpha1.DataMover) {
    if dm.Status.RestoredPVCName == "" {
        // No PVC to cleanup
        return
    }

    // Delete the cloned PVC
    pvc := &corev1.PersistentVolumeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:      dm.Status.RestoredPVCName,
            Namespace: dm.Namespace,
        },
    }

    if err := r.Delete(ctx, pvc); err != nil {
        // Handle deletion error
        return err
    }

    // Record cleanup metrics
    metrics.RecordCleanupOperation("success", dm.Namespace)
}
```

### Error Handling

If cleanup fails:

- **Retry**: Cleanup is retried on subsequent reconciliation
- **Logging**: Failure is logged with detailed error information
- **Metrics**: Cleanup failure is recorded in metrics
- **Status**: DataMover phase remains "CleaningUp" until successful

### Safety Mechanisms

The cleanup process includes safety checks:

1. **PVC Ownership**: Only delete PVCs created by the operator
2. **Status Verification**: Confirm successful job completion before cleanup
3. **Error Recovery**: Handle partial cleanup scenarios gracefully

## Benefits

### Storage Management

Automatic cleanup provides:

- **Cost Reduction**: Eliminates storage costs for temporary clones
- **Resource Efficiency**: Prevents storage quota exhaustion
- **Clean Environment**: Maintains organized Kubernetes resources

### Operational Benefits

- **Reduced Manual Work**: No need for manual PVC cleanup
- **Consistent Behavior**: Predictable resource lifecycle
- **Automation**: Fits well into automated backup workflows

### Example Storage Savings

Consider a backup operation for a 100GB PVC:

**Without Cleanup**:
```text
Original PVC:  100GB (permanent)
Clone PVC:     100GB (remains after backup)
Total Usage:   200GB
```

**With Cleanup**:
```text
Original PVC:  100GB (permanent)
Clone PVC:     100GB (deleted after backup)
Total Usage:   100GB after completion
```

**Savings**: 50% storage reduction per backup operation

## Monitoring Cleanup Operations

### Metrics

The operator provides Prometheus metrics for cleanup operations:

```prometheus
# Cleanup operation counters
datamover_cleanup_operations_total{status="success", namespace="default"}
datamover_cleanup_operations_total{status="failure", namespace="default"}

# Phase duration including cleanup
datamover_phase_duration_seconds{phase="CleaningUp", namespace="default"}
```

### Status Tracking

Monitor cleanup through DataMover status:

```bash
# Watch cleanup progress
kubectl get datamover my-backup -w

# Check detailed status
kubectl describe datamover my-backup
```

Expected output during cleanup:
```yaml
status:
  phase: "CleaningUp"
  restoredPvcName: "restored-app-data-20240806143052"
```

### Logging

Monitor cleanup operations through operator logs:

```bash
# View cleanup logs
kubectl logs -n datamover-operator-system deployment/datamover-operator-controller-manager | grep -i cleanup

# Example log entries
# INFO Cleaning up cloned PVC {"pvc": "restored-app-data-20240806143052"}
# INFO Successfully deleted cloned PVC {"pvc": "restored-app-data-20240806143052"}
```

## Use Cases

### 1. Automated Backup Workflows

Perfect for scheduled backups where clones are temporary:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: nightly-backup
spec:
  sourcePvc: "production-data"
  secretName: "backup-credentials"
  deletePvcAfterBackup: true
  addTimestampPrefix: true
```

**Workflow**:
1. Clone production PVC
2. Sync to timestamped backup location
3. Automatically delete clone
4. Preserve only original PVC

### 2. Development Environment Snapshots

For development workflows where clones are not needed after sync:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: dev-snapshot
spec:
  sourcePvc: "dev-workspace"
  secretName: "dev-storage"
  deletePvcAfterBackup: true
```

### 3. Compliance Backups

For compliance where only the backup copy matters:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: compliance-backup
spec:
  sourcePvc: "financial-records"
  secretName: "compliance-storage"
  deletePvcAfterBackup: true
  addTimestampPrefix: true
```

## When NOT to Use Cleanup

### 1. Clone Analysis Workflows

When you need to analyze or compare cloned data:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: data-analysis
spec:
  sourcePvc: "production-data"
  secretName: "storage-credentials"
  deletePvcAfterBackup: false  # Keep clone for analysis
```

### 2. Multi-Stage Backups

When clones are used in multiple backup stages:

```yaml
# First stage: Create clone and initial backup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: stage1-backup
spec:
  sourcePvc: "app-data"
  secretName: "primary-storage"
  deletePvcAfterBackup: false  # Keep for stage 2

# Second stage: Use same clone for secondary backup
# (would reference the same cloned PVC)
```

### 3. Debugging Scenarios

When troubleshooting backup issues:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: debug-backup
spec:
  sourcePvc: "problematic-data"
  secretName: "storage-credentials"
  deletePvcAfterBackup: false  # Keep clone for debugging
```

## Troubleshooting

### Common Issues

#### 1. Cleanup Stuck in Progress

**Symptoms**: DataMover phase remains "CleaningUp"

**Possible Causes**:
- PVC has active pod attachments
- PVC finalizers preventing deletion
- RBAC permission issues

**Diagnosis**:
```bash
# Check PVC status
kubectl get pvc <cloned-pvc-name>

# Check for attached pods
kubectl get pods --all-namespaces -o wide | grep <cloned-pvc-name>

# Check PVC finalizers
kubectl get pvc <cloned-pvc-name> -o yaml | grep finalizers

# Check operator permissions
kubectl auth can-i delete persistentvolumeclaims --as=system:serviceaccount:datamover-operator-system:datamover-operator-controller-manager
```

#### 2. Cleanup Fails After Successful Sync

**Symptoms**: Job succeeds but cleanup fails

**Possible Causes**:
- PVC in use by other processes
- Storage class deletion policies
- Volume attachment issues

**Solutions**:
```bash
# Force PVC deletion (if safe)
kubectl patch pvc <cloned-pvc-name> -p '{"metadata":{"finalizers":[]}}' --type=merge

# Check for volume attachments
kubectl get volumeattachment | grep <pv-name>
```

#### 3. Metrics Not Recording Cleanup

**Symptoms**: Cleanup happens but metrics not updated

**Diagnosis**:
```bash
# Check operator logs for metric errors
kubectl logs -n datamover-operator-system deployment/datamover-operator-controller-manager | grep -i metric

# Verify Prometheus scraping
curl http://operator-metrics-service:8080/metrics | grep cleanup
```

### Debug Commands

```bash
# Monitor cleanup process
kubectl get datamover <name> -w

# Check cleanup logs
kubectl logs -n datamover-operator-system deployment/datamover-operator-controller-manager | grep -i "cleanup\|delete"

# List PVCs created by operator
kubectl get pvc -l app.kubernetes.io/created-by=datamover-operator

# Check PVC deletion events
kubectl get events --field-selector involvedObject.kind=PersistentVolumeClaim
```

## Best Practices

### 1. Resource Planning

Consider cleanup in resource planning:

- **Temporary Storage**: Plan for peak usage during clone creation
- **Cleanup Timing**: Consider cleanup duration in scheduling
- **Quota Management**: Account for temporary storage quota usage

### 2. Monitoring

Set up monitoring for cleanup operations:

```yaml
# Example Prometheus alert
groups:
- name: datamover.cleanup
  rules:
  - alert: DataMoverCleanupFailing
    expr: increase(datamover_cleanup_operations_total{status="failure"}[5m]) > 0
    for: 0m
    labels:
      severity: warning
    annotations:
      summary: "DataMover cleanup operations are failing"
      description: "DataMover cleanup failures in namespace {{ $labels.namespace }}"
```

### 3. Backup Verification

Always verify backup success before cleanup:

```bash
# Verify backup exists before cleanup completes
rclone lsd s3:my-bucket/ | grep $(date +%Y-%m-%d)
```

### 4. Testing

Test cleanup behavior:

```yaml
# Test cleanup with small PVC
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: cleanup-test
spec:
  sourcePvc: "test-data"
  secretName: "test-credentials"
  deletePvcAfterBackup: true
```

### 5. Documentation

Document cleanup policies in your backup procedures:

- When cleanup is enabled/disabled
- Storage impact of cleanup decisions
- Recovery procedures if cleanup fails

## Advanced Scenarios

### Conditional Cleanup

Implement conditional cleanup based on backup verification:

```yaml
# Example: Only cleanup after backup verification
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: verified-cleanup
spec:
  sourcePvc: "critical-data"
  secretName: "storage-credentials"
  deletePvcAfterBackup: true
  additionalEnv:
    - name: "VERIFY_BACKUP"
      value: "true"
```

### Multi-Destination Cleanup

When backing up to multiple destinations:

```yaml
# Primary backup with cleanup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: primary-backup
spec:
  sourcePvc: "important-data"
  secretName: "primary-storage"
  deletePvcAfterBackup: false  # Keep for secondary backup

# Secondary backup without cleanup  
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: secondary-backup
spec:
  sourcePvc: "important-data"  # Same source
  secretName: "secondary-storage"
  deletePvcAfterBackup: true   # Cleanup after both complete
```

### Cleanup with Lifecycle Management

Integrate with external lifecycle management:

```bash
#!/bin/bash
# External cleanup verification script

NAMESPACE="default"
DATAMOVER_NAME="my-backup"

# Wait for backup completion
kubectl wait --for=condition=Complete datamover/$DATAMOVER_NAME -n $NAMESPACE --timeout=3600s

# Verify backup in storage
if rclone check s3:my-bucket/latest/ /verify/path/; then
    echo "Backup verified, cleanup can proceed"
else
    echo "Backup verification failed, manual intervention required"
    exit 1
fi
```

This comprehensive documentation covers all aspects of the automatic cleanup feature, providing users with the knowledge needed to effectively use and troubleshoot this functionality.
