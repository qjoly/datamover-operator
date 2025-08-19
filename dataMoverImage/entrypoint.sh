#!/bin/bash

echo "🚀 Starting rclone configuration..."
rclone_version=$(rclone --version | head -n 1 | awk '{print $2}')
echo "📦 Rclone version: $rclone_version"

# Set HOME to /config for rclone configuration (otherwise, it can try to write in /.rclone, which is not desirable)
export HOME="/config/"

# Configure rclone s3 generic
if [ -z "$AWS_ACCESS_KEY_ID" ] || [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
  echo "❌ AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set."
  exit 1
fi

if [ -z "$AWS_REGION" ]; then
  AWS_REGION="us-east-1"  # Default region if not set
  echo "🌍 AWS_REGION not set, using default: $AWS_REGION"
fi

if [ -z "$BUCKET_HOST" ]; then
  echo "❌ BUCKET_HOST must be set."
  exit 1
fi

if [ -z "$BUCKET_NAME" ]; then
  echo "❌ BUCKET_NAME must be set."
  exit 1
fi

if [ -z "$BUCKET_PORT" ]; then
    BUCKET_PORT="443"  # Default port if not set
    echo "🔌 BUCKET_PORT not set, using default: $BUCKET_PORT"
fi

if [ -z "$TLS_HOST" ]; then
    TLS_HOST="true"  # Default to true if not set
    echo "🔒 TLS_HOST not set, using default: $TLS_HOST"
fi
if [ "$TLS_HOST" != "true" ] && [ "$TLS_HOST" != "false" ]; then
    echo "❌ TLS_HOST must be 'true' or 'false'."
    exit 1
fi

if [ "$TLS_HOST" == "true" ]; then
    endpoint="https://$BUCKET_HOST:$BUCKET_PORT"
else
    endpoint="http://$BUCKET_HOST:$BUCKET_PORT"
fi

echo "⚙️ Configuring rclone with endpoint: $endpoint"

rclone config create s3generic s3 \
  env_auth true \
  access_key_id "$AWS_ACCESS_KEY_ID" \
  secret_access_key "$AWS_SECRET_ACCESS_KEY" \
  region "$AWS_REGION" \
  endpoint "$endpoint" \
  bucket "$BUCKET_NAME" \
  v2_auth false > /dev/null || { echo "❌ Rclone configuration failed."; exit 1; }

echo "✅ Rclone configuration completed successfully."

# Check if this is population mode (restore from remote to local)
if [ "$POPULATION_MODE" == "true" ]; then
    set -x
    echo "🔄 Population mode enabled - restoring from remote storage"
    
    if [ -z "$SOURCE_PATH" ]; then
        echo "❌ SOURCE_PATH must be set in population mode."
        exit 1
    fi
    
    echo "🔍 Testing rclone connection..."
    rclone lsd s3generic:$BUCKET_NAME -vv || { echo "❌ Rclone connection test failed."; exit 1; }
    echo "✅ Rclone connection test succeeded."
    
    echo "📥 Starting rclone population (restore) process..."
    echo "📂 Source: $SOURCE_PATH"
    echo "🎯 Destination: /data/"
    
    # For population, we copy from the source path to /data/
    rclone sync "s3generic:$BUCKET_NAME/$SOURCE_PATH" /data/ -v || { echo "❌ Rclone population failed."; exit 1; }
    echo "🎉 Rclone population completed successfully."
    
else
    # Original backup mode (local to remote)
    echo "💾 Backup mode enabled - syncing to remote storage"
    
    # Determine destination path based on ADD_TIMESTAMP_PREFIX
    if [ "$ADD_TIMESTAMP_PREFIX" == "true" ]; then
        timestamp=$(date "+%Y-%m-%d-%H%M%S")
        destination_path="s3generic:$BUCKET_NAME/$timestamp/"
        echo "📅 Timestamp prefix enabled. Destination: $destination_path"
    else
        destination_path="s3generic:$BUCKET_NAME/"
        echo "🔄 Using root destination: $destination_path"
    fi

    echo "🔍 Testing rclone connection..."
    rclone lsd s3generic:$BUCKET_NAME -vv || { echo "❌ Rclone connection test failed."; exit 1; }
    echo "✅ Rclone connection test succeeded."
    echo "🔄 Starting rclone sync process..."
    echo "📂 Source: /data/"
    echo "🎯 Destination: $destination_path"
    rclone sync /data/ "$destination_path" -v || { echo "❌ Rclone sync failed."; exit 1; }
    echo "🎉 Rclone sync completed successfully."
fi
