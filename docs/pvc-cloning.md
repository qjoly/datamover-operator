# PVC Cloning

This document explains how PVC (PersistentVolumeClaim) cloning works in the DataMover Operator and the requirements for successful cloning operations.

## Overview

PVC cloning is the foundation of the DataMover Operator. It creates a copy of an existing PVC without requiring volume snapshots, allowing data to be safely backed up while the original application continues running.

## How PVC Cloning Works

### Clone Creation Process

1. **Source PVC Verification**: The operator checks that the source PVC exists and is bound
2. **Clone PVC Creation**: Creates a new PVC with `dataSource` pointing to the source PVC
3. **Volume Provisioning**: The CSI driver creates a new volume as a clone of the source
4. **Binding**: The cloned PVC becomes bound and ready for use

### Clone Naming Convention

Cloned PVCs follow this naming pattern:
```
<source-pvc-name>-cloned-<timestamp>
```

Example:
- Source PVC: `web-app-data`
- Cloned PVC: `web-app-data-cloned-20240806143052`


### Compatible CSI Drivers

| CSI Driver | Clone Support | Notes |
|------------|---------------|-------|
| AWS EBS CSI | ✅ Yes | Supports cross-AZ cloning |
| GCE PD CSI | ✅ Yes | Regional and zonal disks |
| Azure Disk CSI | ✅ Yes | Premium and Standard SSDs |
| vSphere CSI | ✅ Yes | vSAN and VMFS datastores |
| Ceph CSI | ✅ Yes | RBD/CephFS clone feature |
| OpenEBS | ⚠️ Partial | Depends on storage engine |

## Clone Configuration

### Basic Clone Specification

The DataMover operator automatically handles clone creation. You only need to specify the source PVC:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: backup-web-data
spec:
  sourcePvc: "web-app-data"  # Source PVC to clone
  secretName: "storage-credentials"
```

### Clone Behavior

- **Storage Class**: Clones inherit the storage class of the source PVC
- **Access Modes**: Clones inherit access modes from the source PVC
- **Size**: Clones have the same size as the source PVC
- **Labels**: Clones get operator-specific labels for tracking

## Clone Lifecycle Management

### Automatic Cleanup

When `deletePvcAfterBackup` is enabled, clones are automatically deleted after successful backup:

```yaml
spec:
  sourcePvc: "web-app-data"
  secretName: "storage-credentials"
  deletePvcAfterBackup: true  # Auto-delete clone after backup
```

### Manual Cleanup

If automatic cleanup is disabled, you can manually clean up clones:

```bash
# List clones created by the operator
kubectl get pvc -l app.kubernetes.io/created-by=datamover-operator

# Delete specific clone
kubectl delete pvc restored-web-app-data-20240806143052

# Delete all operator-created clones
kubectl delete pvc -l app.kubernetes.io/created-by=datamover-operator
```

## Performance Considerations

### Clone Creation Time

Clone creation time depends on:

- **Storage Backend**: NVMe SSDs are faster than HDDs
- **Volume Size**: Larger volumes take longer to clone
- **CSI Driver Implementation**: Some drivers use copy-on-write, others full copy
- **Network Latency**: For network-attached storage

### Resource Usage

During cloning:

- **Storage**: Temporary storage equal to source PVC size
- **I/O**: Minimal impact on source PVC performance
- **Network**: Bandwidth usage depends on storage backend

### Best Practices

1. **Schedule During Low Traffic**: Clone during maintenance windows when possible
2. **Monitor Storage Usage**: Ensure sufficient storage for clones
3. **Use Fast Storage Classes**: Clone to high-performance storage when available
4. **Enable Auto-Cleanup**: Reduce storage waste with automatic cleanup

## Troubleshooting Clone Issues

### Common Problems

#### 1. Clone Creation Fails

**Error**: `Failed to create cloned PVC`

**Possible Causes**:
- CSI driver doesn't support cloning
- Insufficient storage quota
- Source PVC is not bound
- Storage class restrictions

**Diagnosis**:
```bash
# Check source PVC status
kubectl get pvc <source-pvc-name>

# Check storage class
kubectl describe storageclass <storage-class-name>

# Check CSI driver capabilities
kubectl describe csidriver <csi-driver-name>
```

#### 2. Clone Stuck in Pending

**Error**: Clone PVC remains in `Pending` state

**Possible Causes**:
- No available storage
- Node affinity conflicts
- CSI driver issues

**Diagnosis**:
```bash
# Check clone PVC events
kubectl describe pvc <clone-pvc-name>

# Check storage capacity
kubectl get pv

# Check node availability
kubectl get nodes
```

#### 3. Clone Performance Issues

**Error**: Clone creation is very slow

**Solutions**:
- Use faster storage classes
- Check storage backend performance
- Verify network connectivity
- Consider off-peak scheduling

### Debug Commands

```bash
# Monitor clone creation
kubectl get pvc -w

# Check detailed clone events
kubectl describe pvc <clone-pvc-name>

# View operator logs for clone operations
kubectl logs -n datamover-operator-system deployment/datamover-operator-controller-manager | grep -i clone

# Check storage class details
kubectl describe storageclass <storage-class>
```

## Advanced Clone Scenarios

### Cross-Namespace Cloning

Clones are created in the same namespace as the DataMover resource:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: cross-ns-backup
  namespace: backup-namespace  # Clone created here
spec:
  sourcePvc: "app-data"  # Must exist in backup-namespace
  secretName: "storage-credentials"
```

### Multiple Clones from Same Source

You can create multiple DataMover resources from the same source PVC:

```yaml
# Development backup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: dev-backup
spec:
  sourcePvc: "shared-data"
  secretName: "dev-storage"
---
# Production backup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: prod-backup
spec:
  sourcePvc: "shared-data"
  secretName: "prod-storage"
```

## Security Considerations

### Access Control

- Clone creation requires PVC creation permissions
- Clones inherit security context from operator
- Ensure proper RBAC for clone management

### Data Security

- Clones contain exact copy of source data
- Apply same security policies as source PVC
- Consider encryption for sensitive data

### Cleanup Security

- Ensure proper deletion of clones
- Verify data scrubbing policies
- Monitor clone lifecycle for compliance

## Monitoring Clone Operations

### Metrics

The operator provides metrics for clone operations:

- `datamover_operations_total{phase="CreatingClonedPVC"}`
- `datamover_phase_duration_seconds{phase="CreatingClonedPVC"}`

### Logging

Monitor clone operations through operator logs:

```bash
kubectl logs -n datamover-operator-system deployment/datamover-operator-controller-manager | grep -i "clone\|pvc"
```

### Status Tracking

Check clone status through DataMover resource:

```bash
kubectl get datamover -o wide
kubectl describe datamover <datamover-name>
```
