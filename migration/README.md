# Destiny Migration Job

## Logging

This job requires Go 1.26.4 or newer. The job uses `clog.Run` as its sole event boundary, `clog.WithFields` for root-level execution metrics, and `clog.WithGroupFields` / `clog.WithGroupField` across the request, migration service, Bungie API client, and database (`db.*`) layers; each execution emits one canonical `migration-success` or `migration-failure` JSON event to stdout. When `CLOUD_RUN_JOB` is set, logs use compact Cloud Logging JSON; local runs use pretty-printed JSON with the same fields, including Cloud Logging `message` and `severity`.

Deploying it 
```shell
    gcloud run jobs deploy migration \
        --source . \
        --tasks 15 \
        --task-timeout 30m \
        --memory 2Gi \
        --cpu 1 \
        --max-retries 3 \
        --region us-central1 \
        --project=gruntt-destiny
```

This command is equivalent to running:
```shell
  gcloud builds submit --pack image=[IMAGE] .
  # OR
  gcloud run jobs deploy migration --image [IMAGE]
```


To execute this job, use:
```shell
  gcloud run jobs execute migration \
   --project=gruntt-destiny \
   --region us-central1
```