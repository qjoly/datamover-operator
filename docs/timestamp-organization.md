# Timestamp Organization

This document explains the timestamp organization feature that allows for structured, time-based backup organization in remote storage.

## Overview

The timestamp organization feature creates timestamped folders in remote storage, enabling point-in-time backups, easier data management, and organized storage structures. When enabled, each backup operation creates a unique timestamp-prefixed directory.

## Feature Configuration

### Enabling Timestamp Organization

Enable timestamp prefixes in your DataMover specification:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: timestamped-backup
spec:
  sourcePvc: "app-data"
  secretName: "storage-credentials"
  addTimestampPrefix: true  # Enable timestamp organization
```

### Disabling Timestamp Organization

For direct synchronization to bucket root:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: direct-sync
spec:
  sourcePvc: "app-data"
  secretName: "storage-credentials"
  addTimestampPrefix: false  # Sync directly to bucket root
```

**Default Behavior**: `addTimestampPrefix: false`

## Timestamp Format

### Format Specification

Timestamps use the format: `YYYY-MM-DD-HHMMSS`

- **YYYY**: 4-digit year
- **MM**: 2-digit month (01-12)
- **DD**: 2-digit day (01-31)
- **HH**: 2-digit hour (00-23)
- **MM**: 2-digit minute (00-59)
- **SS**: 2-digit second (00-59)

### Examples

```text
2024-08-06-143052  # August 6, 2024, 14:30:52
2024-12-25-091500  # December 25, 2024, 09:15:00
2025-01-01-000000  # January 1, 2025, 00:00:00
```

### Timezone Considerations

Timestamps are generated in **UTC timezone** to ensure consistency across different operator deployments and geographical locations.

## Storage Structure

### With Timestamp Organization

When `addTimestampPrefix: true`:

```text
my-backup-bucket/
├── 2024-08-06-143052/     # First backup
│   ├── app/
│   │   ├── config.yaml
│   │   └── app.log
│   ├── data/
│   │   ├── database.db
│   │   └── cache/
│   └── logs/
│       └── application.log
├── 2024-08-06-151225/     # Second backup
│   ├── app/
│   │   ├── config.yaml
│   │   └── app.log
│   ├── data/
│   │   ├── database.db
│   │   └── cache/
│   └── logs/
│       └── application.log
└── 2024-08-06-163408/     # Third backup
    ├── app/
    ├── data/
    └── logs/
```

### Without Timestamp Organization

When `addTimestampPrefix: false`:

```text
my-backup-bucket/
├── app/
│   ├── config.yaml
│   └── app.log
├── data/
│   ├── database.db
│   └── cache/
└── logs/
    └── application.log
```

## Use Cases

### 1. Point-in-Time Recovery

Maintain multiple backup versions for recovery:

```yaml
# Daily backup with timestamps
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: daily-backup
spec:
  sourcePvc: "production-data"
  secretName: "s3-credentials"
  addTimestampPrefix: true
```

**Benefits**:
- Recovery to specific time points
- Compare data between different backups
- Rollback to previous known-good states

### 2. Compliance and Auditing

Maintain historical records for compliance:

```yaml
# Compliance backup with retention
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: compliance-backup
spec:
  sourcePvc: "financial-data"
  secretName: "secure-storage"
  addTimestampPrefix: true
```

**Benefits**:
- Immutable backup history
- Audit trail of data changes
- Regulatory compliance support

### 3. Development Workflows

Snapshot development environments:

```yaml
# Development snapshot
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: dev-snapshot
spec:
  sourcePvc: "dev-workspace"
  secretName: "dev-storage"
  addTimestampPrefix: true
```

**Benefits**:
- Environment versioning
- Feature branch data isolation
- Easy environment restoration

### 4. Continuous Synchronization

For live synchronization without versioning:

```yaml
# Live sync without timestamps
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: live-sync
spec:
  sourcePvc: "shared-data"
  secretName: "sync-storage"
  addTimestampPrefix: false
```

**Benefits**:
- Real-time data replication
- Mirror maintenance
- Disaster recovery preparation

## Implementation Details

### Environment Variable

The feature is controlled by the `ADD_TIMESTAMP_PREFIX` environment variable passed to the rclone container:

```bash
# When addTimestampPrefix: true
ADD_TIMESTAMP_PREFIX=true

# When addTimestampPrefix: false
ADD_TIMESTAMP_PREFIX=false
```

### Rclone Integration

The custom entrypoint script (`entrypoint.sh`) handles timestamp generation:

```bash
#!/bin/bash

if [ "$ADD_TIMESTAMP_PREFIX" = "true" ]; then
    # Generate timestamp in UTC
    TIMESTAMP=$(date -u +"%Y-%m-%d-%H%M%S")
    DESTINATION="s3:${BUCKET_NAME}/${TIMESTAMP}/"
else
    DESTINATION="s3:${BUCKET_NAME}/"
fi

# Execute rclone sync
rclone sync /data/ "$DESTINATION" --progress
```

### Timestamp Generation

Timestamps are generated at job execution time, ensuring:

- **Uniqueness**: Each backup gets a unique timestamp
- **Consistency**: UTC timezone for global consistency
- **Precision**: Second-level precision for frequent backups

## Management and Cleanup

### Listing Timestamped Backups

Use rclone or cloud provider tools to list backups:

```bash
# List all timestamped backups
rclone lsd s3:my-bucket/

# List backups for specific date
rclone lsd s3:my-bucket/ | grep "2024-08-06"
```

### Automated Cleanup

Implement cleanup policies using cloud provider lifecycle rules:

#### AWS S3 Lifecycle Policy

```json
{
    "Rules": [
        {
            "ID": "DataMoverBackupRetention",
            "Status": "Enabled",
            "Filter": {
                "Prefix": "2024-"
            },
            "Expiration": {
                "Days": 30
            }
        }
    ]
}
```

#### Manual Cleanup Script

```bash
#!/bin/bash
# Cleanup backups older than 30 days

BUCKET="my-backup-bucket"
CUTOFF_DATE=$(date -d "30 days ago" +"%Y-%m-%d")

# List and delete old backups
rclone lsd s3:$BUCKET/ | while read line; do
    BACKUP_DATE=$(echo $line | awk '{print $5}' | cut -d'-' -f1-3)
    if [[ "$BACKUP_DATE" < "$CUTOFF_DATE" ]]; then
        BACKUP_PATH=$(echo $line | awk '{print $5}')
        echo "Deleting old backup: $BACKUP_PATH"
        rclone purge s3:$BUCKET/$BACKUP_PATH/
    fi
done
```

### Storage Cost Optimization

Optimize storage costs with timestamped backups:

1. **Lifecycle Policies**: Move old backups to cheaper storage tiers
2. **Compression**: Enable compression for text-heavy data
3. **Deduplication**: Use storage backends with built-in deduplication
4. **Retention Policies**: Implement appropriate retention periods

## Best Practices

### 1. Naming Conventions

Combine timestamps with descriptive prefixes:

```yaml
# Multiple applications in same bucket
metadata:
  name: web-app-backup-timestamp
spec:
  sourcePvc: "web-app-data"
  additionalEnv:
    - name: "BUCKET_PREFIX"
      value: "web-app"
```

### 2. Backup Scheduling

Schedule backups to avoid conflicts:

```yaml
# Use CronJob for scheduled backups
apiVersion: batch/v1
kind: CronJob
metadata:
  name: hourly-backup
spec:
  schedule: "0 * * * *"  # Every hour
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup-trigger
            # Create DataMover resource
```

### 3. Monitoring

Monitor timestamp organization:

```bash
# Check recent backups
rclone lsd s3:my-bucket/ | tail -10

# Verify backup timing
kubectl get datamover -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,AGE:.metadata.creationTimestamp
```

### 4. Testing

Test timestamp functionality:

```yaml
# Test backup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: test-timestamp
spec:
  sourcePvc: "test-data"
  secretName: "test-credentials"
  addTimestampPrefix: true
```

## Troubleshooting

### Common Issues

#### 1. Timestamp Conflicts

**Problem**: Multiple backups with same timestamp

**Cause**: Rapid successive backup creation

**Solution**: Add random suffix or wait between operations

#### 2. Storage Path Issues

**Problem**: Incorrect storage paths with timestamps

**Cause**: Misconfigured bucket names or paths

**Diagnosis**:
```bash
# Check rclone logs for destination paths
kubectl logs job/verify-<pvc-name> | grep "destination"
```

#### 3. Timezone Confusion

**Problem**: Timestamps don't match expected local time

**Cause**: UTC timestamp generation

**Solution**: Convert UTC to local time for human readability

### Debug Commands

```bash
# Check environment variable setting
kubectl describe job verify-<pvc-name> | grep ADD_TIMESTAMP_PREFIX

# View rclone destination path
kubectl logs job/verify-<pvc-name> | head -20

# List storage contents
rclone lsd s3:my-bucket/
```

## Performance Considerations

### Storage Performance

Timestamped organization impacts:

- **List Operations**: More directories to scan
- **Search Performance**: Hierarchical structure affects search
- **Cleanup Operations**: More complex deletion patterns

### Optimization Strategies

1. **Batch Operations**: Group multiple operations when possible
2. **Parallel Processing**: Use parallel rclone operations for large datasets
3. **Indexing**: Maintain external indexes for fast backup location

## Integration Examples

### Backup Rotation

Implement backup rotation with timestamps:

```bash
#!/bin/bash
# Keep last 7 daily backups

BUCKET="my-backup-bucket"
KEEP_DAYS=7

# Find backups older than retention period
OLD_BACKUPS=$(rclone lsd s3:$BUCKET/ | awk '{print $5}' | sort -r | tail -n +$((KEEP_DAYS + 1)))

# Delete old backups
for backup in $OLD_BACKUPS; do
    echo "Deleting backup: $backup"
    rclone purge s3:$BUCKET/$backup/
done
```

### Backup Verification

Verify backup integrity:

```bash
#!/bin/bash
# Verify latest backup

BUCKET="my-backup-bucket"
LATEST_BACKUP=$(rclone lsd s3:$BUCKET/ | awk '{print $5}' | sort -r | head -1)

echo "Verifying backup: $LATEST_BACKUP"
rclone check s3:$BUCKET/$LATEST_BACKUP/ /local/reference/path/
```

### Multi-Environment Backups

Organize backups by environment and timestamp:

```yaml
# Production backup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: prod-backup
spec:
  sourcePvc: "prod-data"
  secretName: "prod-storage"
  addTimestampPrefix: true
  additionalEnv:
    - name: "BUCKET_PREFIX"
      value: "production"
---
# Staging backup
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: staging-backup
spec:
  sourcePvc: "staging-data"
  secretName: "staging-storage"
  addTimestampPrefix: true
  additionalEnv:
    - name: "BUCKET_PREFIX"
      value: "staging"
```

This creates structure like:
```text
my-bucket/
├── production/
│   ├── 2024-08-06-143052/
│   └── 2024-08-06-151225/
└── staging/
    ├── 2024-08-06-143100/
    └── 2024-08-06-151230/
```
