#!/bin/bash
#
# Bootstrap for the Inferno inference endpoint on Amazon Linux 2023 (x86_64).
#
# Mirrors how oc-router runs: no inbound web ports at all, Cloudflare reached
# OUTBOUND through a cloudflared tunnel, Postgres and Redis on the instance.
#
# Run as EC2 user-data, or by hand on a fresh box. Required environment:
#
#   TUNNEL_TOKEN   cloudflared connector token for this instance's tunnel
#   PG_PASSWORD    Postgres password (generate per instance, never reuse)
#   JWT_SECRET     app JWT signing secret (generate per instance)
#   DEFAULT_API_KEY  bootstrap admin key (generate per instance)
#   PUBLIC_URL     e.g. https://inference.tryopencomputer.com
#   IMAGE          ECR image ref to run
#
set -euo pipefail
exec > >(tee -a /var/log/inferno-bootstrap.log) 2>&1

for v in TUNNEL_TOKEN PG_PASSWORD JWT_SECRET DEFAULT_API_KEY PUBLIC_URL IMAGE; do
  [ -n "${!v:-}" ] || { echo "FATAL: $v is not set"; exit 2; }
done

REGION="${AWS_REGION:-us-east-1}"
REGISTRY="${IMAGE%%/*}"

# ---------------------------------------------------------------- packages
dnf -y update
dnf -y install docker
systemctl enable --now docker
usermod -aG docker ec2-user

mkdir -p /usr/local/lib/docker/cli-plugins
curl -fsSL https://github.com/docker/compose/releases/download/v2.32.4/docker-compose-linux-x86_64 \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

# The RPM asset is x86_64, NOT amd64 -- amd64 names the .deb and the raw
# binary. And -f matters more than the filename: without it curl saves a 404
# page as the .rpm and dnf fails on a file full of HTML, which is how the
# first run of this script died at this exact line with everything below it
# never executing.
curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-x86_64.rpm \
  -o /tmp/cloudflared.rpm
rpm -qp /tmp/cloudflared.rpm >/dev/null   # refuse to continue on a bad download
dnf -y install /tmp/cloudflared.rpm
rm -f /tmp/cloudflared.rpm

# ---------------------------------------------------------------- layout
mkdir -p /opt/inferno/pgdata /opt/inferno/data
cd /opt/inferno

cat > docker-compose.yml <<COMPOSE
services:
  postgres:
    image: postgres:18-alpine
    container_name: inferno-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: sub2api
      POSTGRES_PASSWORD: ${PG_PASSWORD}
      POSTGRES_DB: sub2api
      # Postgres 18 refuses a bare /var/lib/postgresql/data mount without
      # this; it wants the mount one level up unless PGDATA says otherwise.
      PGDATA: /var/lib/postgresql/data
    volumes:
      - /opt/inferno/pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sub2api"]
      interval: 5s
      timeout: 5s
      retries: 30
  redis:
    image: redis:8-alpine
    container_name: inferno-redis
    restart: unless-stopped
  inferno:
    image: ${IMAGE}
    container_name: inferno
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      - SERVER_HOST=0.0.0.0
      - SERVER_PORT=8080
      - SERVER_MODE=release
      # Becomes the OAuth \`iss\` claim. Unset, token exchange 500s for every
      # client while every health check stays green.
      - SERVER_FRONTEND_URL=${PUBLIC_URL}
      - DATABASE_HOST=postgres
      - DATABASE_PORT=5432
      - DATABASE_USER=sub2api
      - DATABASE_PASSWORD=${PG_PASSWORD}
      # DBNAME, not NAME. With the wrong key the app cannot see a database
      # and boots into the setup wizard instead.
      - DATABASE_DBNAME=sub2api
      - DATABASE_SSLMODE=disable
      - REDIS_HOST=redis
      - REDIS_PORT=6379
    volumes:
      - /opt/inferno/data:/app/data
    ports:
      - "127.0.0.1:8080:8080"
COMPOSE
chmod 600 docker-compose.yml

# The app decides "first run" from these two files, not from env. Without
# them it starts the setup wizard and ignores a perfectly good database.
cat > /opt/inferno/data/config.yaml <<CFG
server:
    host: 0.0.0.0
    port: 8080
    mode: release
    frontend_url: ${PUBLIC_URL}
database:
    host: postgres
    port: 5432
    user: sub2api
    password: ${PG_PASSWORD}
    dbname: sub2api
    sslmode: disable
redis:
    host: redis
    port: 6379
    username: ""
    password: ""
    db: 0
    enable_tls: false
jwt:
    secret: ${JWT_SECRET}
    expire_hour: 24
default:
    user_concurrency: 5
    user_balance: 0
    api_key: ${DEFAULT_API_KEY}
    rate_multiplier: 1
rate_limit:
    requests_per_minute: 60
    burst_size: 10
timezone: Asia/Kolkata
CFG
printf 'installed_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > /opt/inferno/data/.installed
chmod 600 /opt/inferno/data/config.yaml /opt/inferno/data/.installed
chown -R 1000:1000 /opt/inferno/data

# ---------------------------------------------------------------- run
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "$REGISTRY"

docker compose up -d postgres redis
for i in $(seq 1 40); do
  [ "$(docker inspect inferno-postgres --format '{{.State.Health.Status}}' 2>/dev/null)" = healthy ] && break
  sleep 5
done

# Restore a seed dump if one was staged. Do this BEFORE the app starts so its
# migrations run against the real data rather than an empty schema.
if [ -f /opt/inferno/seed.dump ]; then
  docker exec -i inferno-postgres pg_restore -U sub2api -d sub2api --no-owner --no-acl < /opt/inferno/seed.dump
  shred -u /opt/inferno/seed.dump 2>/dev/null || rm -f /opt/inferno/seed.dump
fi

docker compose up -d inferno

# Tunnel LAST, so nothing is publicly reachable before the app is serving.
cloudflared service install "$TUNNEL_TOKEN"
systemctl enable --now cloudflared

touch /opt/inferno/BOOTSTRAP_DONE
echo "BOOTSTRAP_STATUS=ok"
