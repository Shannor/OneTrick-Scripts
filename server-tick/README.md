# One Trick Server Tick Job

## Environment Variables

| Variable                 | Required | Description                                                                                                                                                                                                    |
|--------------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `D2_API_KEY`             | Yes      | Bungie API key for Destiny 2 API access                                                                                                                                                                        |
| `CLOUD_RUN_TASK_INDEX`   | No       | Cloud Run task index (defaults to 0)                                                                                                                                                                           |
| `CLOUD_RUN_TASK_ATTEMPT` | No       | Cloud Run task attempt identifier                                                                                                                                                                              |
| `SKIP_SAVE`              | No       | Set to `1` to skip saving loadout snapshots                                                                                                                                                                    |
| `DRY_RUN`                | No       | Set to `1` to run without writing to the database. All reads still execute, and results are summarized in the single canonical event |

## Logging

This job requires Go 1.26.4 or newer. The job uses `clog.Run` as its sole event boundary and `clog.WithGroupFields` to add metadata; each execution emits one canonical `server-tick-success` or `server-tick-failure` JSON event to stdout. Cloud Run ingests that structured stdout JSON into Cloud Logging; no separate logging service client is used. When `CLOUD_RUN_JOB` is set, the event is compact JSON; local runs use pretty-printed JSON with the same fields. The `slog` API is used only to configure Clog's output handler, not for job or helper logging. Task details and processing counters are grouped under `jobMetadata`; `issueCount` and `manifestWarningCount` retain totals while `lastIssue` and `lastManifestWarning` keep only the latest structured detail so repeated issues cannot grow the event without bound.

The custom metrics select canonical job events and read counters from `jobMetadata`. Deploy the metric definitions after upgrading the job; the metrics now count one event per execution rather than separate operational records. The run-count metric includes successful events only.

## Session Inactivity Cutoff

Sessions are automatically ended after **1 hour** of inactivity. A session is considered stale when no new activities have been seen within that window.

## Running Locally

For convenient local runs, copy `.env.example` to `.env` and add your API key. `.env` is ignored by Git and excluded from Cloud source uploads; the Go program does not load it automatically. From the repository root, initialize the file once and run the Makefile target:

```shell
[ -f server-tick/.env ] || cp server-tick/.env.example server-tick/.env
${EDITOR:-vi} server-tick/.env
make run-server-tick-local-env
```

The target exports variables from `.env` in its command subshell, clears any inherited or file-configured `FIRESTORE_EMULATOR_HOST`, then runs `go run .` against production Firestore. The example defaults to `DRY_RUN=1`, which allows reads but prevents database writes. `SKIP_SAVE=0` only controls skipping loadout snapshot saves; it does not disable dry-run protection. To allow writes, explicitly set `DRY_RUN=0` and ensure you intend to run against production. Use the existing emulator targets instead when intentionally targeting a local emulator.

Alternatively, run directly with the key pulled from GCP secrets:

```shell
D2_API_KEY=$(gcloud secrets versions access latest --secret=D2_API_KEY --project=gruntt-destiny)
DRY_RUN=1 D2_API_KEY=$D2_API_KEY ./Server_Tick
```

To run locally against the Firestore emulator without database writes (the emulator must be running):

```shell
cd server-tick
go run . -use-emulator -emulator-host 127.0.0.1:8081 -dry-run
```

The Makefile targets default to `127.0.0.1:8081`. If Docker publishes the emulator's container port on a different host port, find the mapped port with `docker ps` and pass it explicitly, for example `make run-server-tick-local-dry EMULATOR_HOST=127.0.0.1:<host-port>`. You can also set `FIRESTORE_EMULATOR_HOST=127.0.0.1:<host-port>`; an explicit `EMULATOR_HOST` Make variable takes precedence. The initial emulator session query has a three-second deadline and records the endpoint and query error in the canonical failure event if it times out.

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
