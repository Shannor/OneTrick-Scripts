# One Trick Server Tick Job

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `D2_API_KEY` | Yes | Bungie API key for Destiny 2 API access |
| `CLOUD_RUN_TASK_INDEX` | No | Cloud Run task index (defaults to 0) |
| `CLOUD_RUN_TASK_ATTEMPT` | No | Cloud Run task attempt identifier |
| `SKIP_SAVE` | No | Set to `1` to skip saving loadout snapshots |
| `DRY_RUN` | No | Set to `1` to run without writing to the database. All reads still execute, and results are summarized in the single canonical event |

## Logging

This job requires Go 1.26.4 or newer. The job uses `clog.Run` as its sole event boundary and `clog.WithGroupFields` to add metadata; each execution emits one canonical `server-tick-success` or `server-tick-failure` JSON event to stdout. Cloud Run ingests that structured stdout JSON into Cloud Logging; no separate logging service client is used. When `CLOUD_RUN_JOB` is set, the event is compact JSON; local runs use pretty-printed JSON with the same fields. The `slog` API is used only to configure Clog output handlers, not for job or helper logging. Task details and processing counters are grouped under `jobMetadata`; `issueCount` and `manifestWarningCount` retain totals while `lastIssue` and `lastManifestWarning` keep only the latest structured detail so repeated issues cannot grow the event without bound.

The custom metrics select canonical job events and read counters from `jobMetadata`. Deploy the metric definitions after upgrading the job; the metrics now count one event per execution rather than separate operational records. The run-count metric includes successful events only.

## Session Inactivity Cutoff

Sessions are automatically ended after **1 hour** of inactivity. A session is considered stale when no new activities have been seen within that window.

## Running Locally

Build and run in dry-run mode against production data without making any changes:

```shell
cd server-tick
go build -o Server_Tick .
DRY_RUN=1 D2_API_KEY=<your-api-key> ./Server_Tick
```

To pull the API key from GCP secrets:

```shell
D2_API_KEY=$(gcloud secrets versions access latest --secret=D2_API_KEY --project=gruntt-destiny)
DRY_RUN=1 D2_API_KEY=$D2_API_KEY ./Server_Tick
```

To run locally against the Firestore emulator without database writes (the emulator must be running):

```shell
cd server-tick
go run . -use-emulator -emulator-host 127.0.0.1:8081 -dry-run
```

The Makefile targets default to `127.0.0.1:8081`. If Docker publishes the emulator container port on a different host port, find the mapped port with `docker ps` and pass it explicitly, for example `make run-server-tick-local-dry EMULATOR_HOST=127.0.0.1:<host-port>`. You can also set `FIRESTORE_EMULATOR_HOST=127.0.0.1:<host-port>`; an explicit `EMULATOR_HOST` Make variable takes precedence. The initial emulator session query has a three-second deadline and records the endpoint and query error in the canonical failure event if it times out.

## Deploying

```shell
cd server-tick
gcloud run jobs deploy server-tick \
    --source . \
    --tasks 1 \
    --task-timeout 5m \
    --memory 1Gi \
    --cpu 1 \
    --max-retries 2 \
    --set-secrets D2_API_KEY=D2_API_KEY:latest \
    --region us-central1 \
    --project=gruntt-destiny
```

This command is equivalent to running:

```shell
gcloud builds submit --pack image=[IMAGE] .
# OR
gcloud run jobs deploy migration --image [IMAGE]
```

To execute this job:

```shell
gcloud run jobs execute server-tick \
    --project=gruntt-destiny \
    --region us-central1
```
