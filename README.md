# goschedule-listener

A lightweight, long-running ECS Fargate service that listens to Supabase Realtime
for schedule changes and publishes MQTT messages to connected devices (e.g. Arduino Opta PLCs).

## Architecture

```
Supabase DB (schedules table)
│
│  INSERT / UPDATE (duration_min=0 to cancel)
▼
Supabase Realtime (WebSocket)
│
▼
goschedule-listener (ECS Fargate)
│  fetches full future schedule from DB after each change
▼
MQTT Broker (e.g. Mosquitto)
│
▼
Device (Arduino Opta) — topic: machine/schedule/<device_name>

```

## Features

- Persistent WebSocket connection to Supabase Realtime — no polling
- Publishes only future active schedules (end time = start + duration)
- Per-device MQTT topics — each device only receives its own schedule
- Automatic reconnect on connection loss
- mTLS support for MQTT broker
- Graceful shutdown on SIGTERM (ECS task stop)
- Soft delete pattern — set `duration_min = 0` to cancel a schedule

## Soft Delete Pattern

This service does **not** handle SQL `DELETE` events. To cancel a schedule, set
`duration_min = 0` instead. This triggers an `UPDATE` event which Realtime handles
reliably, and the device receives an empty schedule list.

```sql
-- Cancel a schedule
UPDATE schedules SET duration_min = 0 WHERE id = 123;

-- The device will receive:
-- { "schedules": [] }
```

## MQTT Payload

Published to topic `machine/schedule/<device_name>` on every change:

```json
{
  "schedules": [
    { "start": 1774831440, "duration": 30 },
    { "start": 1774918800, "duration": 60 }
  ]
}
```

- `start` — Unix timestamp (seconds)
- `duration` — minutes
- Empty array `[]` means no active or future schedules

## Database Setup

```sql
CREATE TABLE schedules (
    id           SERIAL PRIMARY KEY,
    start_time   TIMESTAMPTZ NOT NULL,
    duration_min INT NOT NULL,
    device_name  TEXT
);

-- Required for Realtime to fire events
ALTER PUBLICATION supabase_realtime ADD TABLE schedules;

-- Required for DELETE old_record (if using hard delete)
ALTER TABLE schedules REPLICA IDENTITY FULL;

-- Recommended: soft delete comment
COMMENT ON COLUMN schedules.duration_min IS 'Set to 0 to cancel — do not DELETE rows';
```

## Environment Variables

| Variable                      | Required | Default             | Description                               |
| ----------------------------- | -------- | ------------------- | ----------------------------------------- |
| `SUPABASE_URL`                | ✅       |                     | e.g. `https://xxxx.supabase.co`           |
| `SUPABASE_KEY`                | ✅       |                     | Supabase anon key                         |
| `SUPABASE_DB_HOST`            | ✅       |                     | e.g. `db.xxxx.supabase.co`                |
| `SUPABASE_DB_PASSWORD`        | ✅       |                     | Postgres password                         |
| `SUPABASE_DB_USER`            | ❌       | `postgres`          | Postgres user                             |
| `SUPABASE_DB_NAME`            | ❌       | `postgres`          | Postgres database name                    |
| `SUPABASE_DB_PORT`            | ❌       | `5432`              | Postgres port                             |
| `SUPABASE_CA_CERT`            | ❌       |                     | Path to Supabase CA cert (if verify-full) |
| `SUPABASE_REALTIME_SCHEMA`    | ❌       | `public`            | Schema to monitor                         |
| `SUPABASE_REALTIME_TABLE`     | ✅       |                     | Table to monitor e.g. `schedules`         |
| `MQTT_BROKER`                 | ✅       |                     | e.g. `ssl://192.168.0.5:8883`             |
| `MQTT_CLIENT_ID`              | ❌       | `schedule-listener` | MQTT client ID                            |
| `ECS_MQTT_CA_CERTIFICATE`     | ❌       |                     | Raw PEM string — CA certificate           |
| `ECS_MQTT_CLIENT_CERTIFICATE` | ❌       |                     | Raw PEM string — client certificate       |
| `ECS_MQTT_PRIVATE_KEY`        | ❌       |                     | Raw PEM string — client private key       |

> TLS is enabled automatically when all three `ECS_MQTT_*` vars are set.
> If only some are set, the app will refuse to start.

## Local Development

```bash
# 1. Clone
git clone https://github.com/yourorg/goschedule-listener
cd goschedule-listener

# 2. Copy and fill in env
cp .env.example .env

# 3. Run
go run cmd/main.go
```

### .env.example

```env
SUPABASE_URL=https://xxxxxxxxxxxx.supabase.co
SUPABASE_KEY=eyJ...
SUPABASE_DB_HOST=db.xxxxxxxxxxxx.supabase.co
SUPABASE_DB_PASSWORD=your-db-password
SUPABASE_DB_USER=postgres
SUPABASE_DB_NAME=postgres
SUPABASE_DB_PORT=5432
SUPABASE_REALTIME_SCHEMA=public
SUPABASE_REALTIME_TABLE=schedules

MQTT_BROKER=tcp://192.168.0.5:1883
MQTT_CLIENT_ID=schedule-listener

# Optional mTLS — all three required together if used
# ECS_MQTT_CA_CERTIFICATE="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
# ECS_MQTT_CLIENT_CERTIFICATE="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
# ECS_MQTT_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
```

## Docker

```bash
# Build
docker build -t goschedule-listener .

# Run
docker run --env-file .env goschedule-listener
```

## Deploy to ECS

```bash
# Build and push to ECR
chmod +x script/deploy-ecr.sh
./script/deploy-ecr.sh
```

### ECS Task Definition (key settings)

| Setting       | Value                   |
| ------------- | ----------------------- |
| Launch type   | Fargate                 |
| CPU           | 256 (.25 vCPU)          |
| Memory        | 512 MB                  |
| Network mode  | awsvpc                  |
| Desired count | 1                       |
| Health check  | none (not a web server) |

Store sensitive env vars (`SUPABASE_DB_PASSWORD`, `SUPABASE_KEY`, MQTT certs)
in **AWS Secrets Manager** and reference them as secrets in the task definition.

## Project Structure

```
goschedule-listener/
├── cmd/
│   └── main.go                   # Entry point
├── internal/
│   ├── config/
│   │   └── config.go             # Env loading and validation
│   ├── db/
│   │   └── db.go                 # Postgres client, schedule queries
│   ├── listener/
│   │   └── listener.go           # Realtime event handler
│   ├── mqtt/
│   │   └── mqtt.go               # MQTT client with mTLS support
│   └── supabase-realtime/        # Supabase Realtime WebSocket client
├── script/
│   ├── deploy-ecr.sh             # Build and push to ECR
│   └── bump_version.go           # Version bumper
├── .env.example
├── Dockerfile
└── go.mod
```

## License

MIT
