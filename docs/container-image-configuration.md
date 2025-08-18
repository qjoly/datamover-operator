# Container Image Configuration

Both `DataMover` and `DataMoverSchedule` support custom container image configuration through the `image` field. This allows you to use custom rclone images or different versions.

## Image Specification

The `image` field has three sub-fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `repository` | string | `ghcr.io/qjoly/datamover-rclone` | Full container image repository including registry |
| `tag` | string | `latest` | Image tag or version |
| `pullPolicy` | string | `Always` | Kubernetes image pull policy |

## Pull Policies

Valid values for `pullPolicy`:

- **`Always`**: Always pull the image, even if it exists locally
- **`IfNotPresent`**: Only pull if the image doesn't exist locally
- **`Never`**: Never pull the image, use local copy only

## Usage Examples

### Default Configuration

When no `image` field is specified, the operator uses defaults:

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: default-image-example
spec:
  sourcePvc: "my-data"
  secretName: "storage-secret"
  # No image field = uses defaults:
  # image:
  #   repository: "ghcr.io/qjoly/datamover-rclone"
  #   tag: "latest"
  #   pullPolicy: "Always"
```

### Custom Image with Specific Tag

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: custom-image-example
spec:
  sourcePvc: "my-data"
  secretName: "storage-secret"
  image:
    repository: "ghcr.io/qjoly/datamover-rclone"
    tag: "v1.65.0"
    pullPolicy: "IfNotPresent"
```

### Private Registry Image

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: private-registry-example
spec:
  sourcePvc: "my-data"
  secretName: "storage-secret"
  image:
    repository: "my-company.com/tools/rclone"
    tag: "enterprise-v2.1.0"
    pullPolicy: "Always"
```

## DataMoverSchedule Examples

### Production Schedule with Stable Image

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMoverSchedule
metadata:
  name: production-backup
spec:
  schedule: "0 2 * * *"
  sourcePvc: "production-data"
  secretName: "backup-credentials"
  image:
    repository: "ghcr.io/qjoly/datamover-rclone"
    tag: "v1.65.0"           # Pinned version for stability
    pullPolicy: "IfNotPresent"  # Avoid unnecessary pulls
  successfulJobsHistoryLimit: 7
```

### Testing Schedule with Latest Features

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMoverSchedule
metadata:
  name: testing-backup
spec:
  schedule: "0 */6 * * *"  # Every 6 hours
  sourcePvc: "test-data"
  secretName: "test-credentials"
  image:
    repository: "ghcr.io/qjoly/datamover-rclone"
    tag: "latest"
    pullPolicy: "Always"    # Always get latest features
  successfulJobsHistoryLimit: 3
```

## Custom Images for Alternative Software

While DataMover was designed with rclone in mind, it's possible to use custom container images that implement other backup or data synchronization tools. This flexibility allows you to integrate your preferred data management software while leveraging DataMover's PVC cloning and job orchestration capabilities.

### Requirements for Custom Images

Any custom image used with DataMover must comply with the following requirements:

1. **Data Location**: The image must expect source data to be mounted at `/data/`
2. **Configuration via Environment Variables**: All configuration must be provided through environment variables (no config files)
3. **Exit Codes**: Must use standard exit codes (0 for success, non-zero for failure)
4. **Signal Handling**: Should handle SIGTERM gracefully for clean shutdown

### Environment Variables

Your custom image will receive all environment variables from the secret specified in `secretName`, plus any additional variables defined in `additionalEnv`. Common patterns include:

- Storage credentials (AWS keys, API tokens, etc.)
- Destination configuration (bucket names, endpoints, etc.)
- Behavioral flags and options

### Feature Compatibility Limitations

⚠️ **Important**: Some DataMover features may not work with custom images, depending on the software implementation:

- **`addTimestampPrefix`**: Only works if your image supports creating timestamped directory structures
- **Error handling**: Different tools may handle errors differently

### Example: Custom Image with rsync

```dockerfile
FROM alpine:3.18

# Install rsync and required tools
RUN apk add --no-cache rsync openssh-client

# Custom entrypoint script (that handles rsync)
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh

USER backup
WORKDIR /data

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

### Example: Custom Image Configuration

```yaml
apiVersion: datamover.a-cup-of.coffee/v1alpha1
kind: DataMover
metadata:
  name: custom-backup-tool
spec:
  sourcePvc: "my-data"
  secretName: "backup-credentials"
  image:
    repository: "my-registry.com/custom-backup"
    tag: "v2.1.0"
    pullPolicy: "IfNotPresent"
  # Note: addTimestampPrefix may not work with custom tools
  addTimestampPrefix: false
  additionalEnv:
    - name: "BACKUP_MODE"
      value: "incremental"
    - name: "COMPRESSION"
      value: "gzip"
```
