# Data Synchronization

This document covers the data synchronization capabilities of the DataMover Operator, including rclone integration and supported storage backends.

## Overview

The DataMover Operator uses [rclone](https://rclone.org/) to synchronize data from cloned PVCs to remote storage backends. This provides a robust, well-tested solution for data transfer with support for numerous cloud and on-premises storage systems.

## Rclone Integration

### Container Image

The operator uses a custom rclone container image: `ttl.sh/rclone_op:latest`

This image includes:
- Latest rclone binary
- Custom entrypoint script for operator integration
- Support for timestamp prefix functionality
- Optimized for Kubernetes environments

### Synchronization Process

1. **PVC Mounting**: Cloned PVC is mounted at `/data/` in the rclone container
2. **Configuration**: Storage credentials loaded from Kubernetes secrets
3. **Sync Execution**: Rclone syncs `/data/` to configured remote destination
4. **Completion**: Job completes with success/failure status

## Supported Storage Backends

### MinIO

**Configuration**:
```yaml
AWS_ACCESS_KEY_ID: minio-access-key
AWS_SECRET_ACCESS_KEY: minio-secret-key
AWS_REGION: us-east-1
BUCKET_HOST: minio.example.com
BUCKET_NAME: backups
BUCKET_PORT: "9000"
TLS_HOST: "false"
```

## Configuration Management

### Secret Structure

Storage credentials are provided via Kubernetes secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storage-credentials
type: Opaque
data:
  # Base64-encoded values
  AWS_ACCESS_KEY_ID: <encoded-access-key>
  AWS_SECRET_ACCESS_KEY: <encoded-secret-key>
  AWS_REGION: <encoded-region>
  BUCKET_HOST: <encoded-host>
  BUCKET_NAME: <encoded-bucket>
  BUCKET_PORT: <encoded-port>
  TLS_HOST: <encoded-true-or-false>
```

### Environment Variables

The rclone container receives configuration through environment variables:

```bash
# Storage backend configuration
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
AWS_REGION=us-west-2
BUCKET_HOST=s3.amazonaws.com
BUCKET_NAME=my-backup-bucket
BUCKET_PORT=443
TLS_HOST=true

# Operator-specific configuration
ADD_TIMESTAMP_PREFIX=true  # Enable timestamp organization
```

### Additional Environment Variables

You can provide additional environment variables through the DataMover spec:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: custom-sync
spec:
  sourcePvc: "app-data"
  secretName: "storage-credentials"
  additionalEnv:
    - name: "RCLONE_TRANSFERS"
      value: "8"
    - name: "RCLONE_CHECKERS"  
      value: "16"
    - name: "CUSTOM_PATH_PREFIX"
      value: "production"
```

## Synchronization Features

### Timestamp Organization

When `addTimestampPrefix: true` is set, data is organized with timestamps:

```
bucket/
├── 2024-08-06-143052/    # Timestamp folder
│   ├── app/
│   ├── data/
│   └── logs/
└── 2024-08-06-151225/    # Another backup
    ├── app/
    ├── data/
    └── logs/
```

**Format**: `YYYY-MM-DD-HHMMSS`

**Benefits**:
- Point-in-time recovery
- Historical backup tracking
- Organized storage structure
- Easy cleanup of old backups

### Incremental Synchronization

Rclone performs incremental synchronization by default:

- **Changed files**: Only modified files are transferred
- **New files**: New files are uploaded
- **Deleted files**: Files deleted from source are removed from destination
- **Checksums**: File integrity verified through checksums

### Metrics Collection

The operator tracks synchronization metrics:

```prometheus
# Sync operation counters
datamover_data_sync_operations_total{status="success"}
datamover_data_sync_operations_total{status="failure"}

# Phase duration tracking
datamover_phase_duration_seconds{phase="CreatingPod"}
```

## Performance Tuning

### Transfer Optimization

Optimize transfer performance based on your environment:

#### High Bandwidth Networks
```yaml
additionalEnv:
  - name: "RCLONE_TRANSFERS"
    value: "8"          # More parallel transfers
  - name: "RCLONE_CHECKERS"
    value: "16"         # More parallel checks
```

#### Limited Bandwidth Networks
```yaml
additionalEnv:
  - name: "RCLONE_TRANSFERS"
    value: "2"          # Fewer parallel transfers
  - name: "RCLONE_BW_LIMIT"
    value: "10M"        # Bandwidth limit
```

#### Large Files
```yaml
additionalEnv:
  - name: "RCLONE_MULTI_THREAD_CUTOFF"
    value: "50M"        # Multi-thread for files > 50MB
  - name: "RCLONE_MULTI_THREAD_STREAMS"
    value: "4"          # 4 streams per large file
```

### Resource Allocation

Configure appropriate resources for sync jobs:

```yaml
# In job template (operator configuration)
resources:
  requests:
    memory: "512Mi"     # Base memory for rclone
    cpu: "200m"         # Base CPU for operations
  limits:
    memory: "2Gi"       # Maximum memory (adjust for large files)
    cpu: "1000m"        # Maximum CPU for parallel operations
```

## Error Handling

### Common Sync Errors

#### 1. Authentication Failures

**Error**: `Failed to configure s3 backend: NoCredentialsErr`

**Solutions**:
- Verify secret credentials are correct
- Check credential encoding (base64)
- Validate IAM permissions for storage access

#### 2. Network Connectivity Issues

**Error**: `Failed to copy: connection timeout`

**Solutions**:
- Check network policies and firewall rules
- Verify storage endpoint accessibility
- Consider bandwidth limitations

#### 3. Storage Permission Issues

**Error**: `AccessDenied: Access Denied`

**Solutions**:
- Verify bucket/container permissions
- Check IAM roles and policies
- Validate storage account access keys

#### 4. Storage Space Issues

**Error**: `No space left on device`

**Solutions**:
- Check storage quota limits
- Verify available space in destination
- Consider data compression options

### Retry Strategy

Rclone has built-in retry mechanisms:

- **File-level retries**: Individual file transfer failures
- **Operation retries**: Overall operation failures  
- **Exponential backoff**: Increasing delays between retries

Combined with Kubernetes Job retries, this provides robust error recovery.

## Security Considerations

### Credential Management

- Store credentials only in Kubernetes secrets
- Use least-privilege access policies
- Rotate credentials regularly
- Monitor credential usage

### Data Encryption

#### In Transit
- Enable TLS for all connections (`TLS_HOST: "true"`)
- Use encrypted storage endpoints
- Verify certificate validation

#### At Rest
- Configure server-side encryption
- Use customer-managed encryption keys when available
- Enable storage backend encryption features

### Network Security

- Use private endpoints when possible
- Implement network policies for job pods
- Restrict egress traffic to required destinations
- Monitor network access patterns

## Troubleshooting Synchronization

### Diagnosis Commands

```bash
# Check rclone job status
kubectl get jobs -l app.kubernetes.io/created-by=datamover-operator

# View sync logs
kubectl logs job/verify-<pvc-name>

# Check secret configuration
kubectl get secret <secret-name> -o yaml

# Test storage connectivity
kubectl run rclone-test --rm -it --image=rclone/rclone -- rclone lsd remote:
```

### Debug Configuration

For debugging sync issues, add debug environment variables:

```yaml
additionalEnv:
  - name: "RCLONE_VERBOSE"
    value: "2"          # Increase verbosity
  - name: "RCLONE_LOG_LEVEL"
    value: "DEBUG"      # Debug logging
  - name: "RCLONE_DUMP"
    value: "headers"    # Dump HTTP headers
```

### Performance Analysis

Monitor sync performance:

```bash
# Check transfer statistics
kubectl logs job/verify-<pvc-name> | grep "Transferred:"

# Monitor resource usage
kubectl top pod <rclone-pod-name>

# Check storage backend performance
# (depends on storage backend monitoring tools)
```

## Best Practices

### 1. Configuration Management

- Use separate secrets for different environments
- Validate configuration before creating DataMover resources
- Document storage backend requirements

### 2. Performance Optimization

- Tune rclone settings for your environment
- Monitor transfer performance and adjust accordingly
- Consider storage backend limitations

### 3. Security

- Follow principle of least privilege
- Enable encryption in transit and at rest
- Regular security audits of configurations

### 4. Monitoring

- Set up alerts for sync failures
- Monitor sync duration trends
- Track storage usage patterns

### 5. Testing

- Test sync operations with sample data
- Validate backup integrity
- Test restore procedures regularly
