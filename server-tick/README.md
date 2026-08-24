# One Trick Server Tick Job

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `D2_API_KEY` | Yes | Bungie API key for Destiny 2 API access |
| `CLOUD_RUN_TASK_INDEX` | No | Cloud Run task index (defaults to 0) |
| `CLOUD_RUN_TASK_ATTEMPT` | No | Cloud Run task attempt identifier |
| `SKIP_SAVE` | No | Set to `1` to skip saving loadout snapshots |
| `DRY_RUN` | No | Set to `1` to run without writing to the database. All reads (Firestore queries, Bungie API calls) still execute so you get the full processing log, but all writes are replaced with `[DRY-RUN]` log messages |

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
