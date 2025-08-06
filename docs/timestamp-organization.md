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
                "Prefix": "2025-"
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
3. **Retention Policies**: Implement appropriate retention periods
