# Destiny Migration Job

## Logging

This job requires Go 1.26.4 or newer. Each execution emits a canonical `migration-success` or `migration-failure` JSON event with Cloud Run task details under `jobMetadata` and the migration table and version metadata when available. When `CLOUD_RUN_JOB` is set, logs use compact Cloud Logging JSON; local runs use pretty-printed JSON with the same fields, including Cloud Logging `message` and `severity`.

Deploying it 
```shell
    gcloud run jobs deploy migration \
        --source . \
        --tasks 12 \
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