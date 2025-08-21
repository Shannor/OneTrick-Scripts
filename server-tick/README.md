# One Trick Server Tick Job

Deploying it
```shell
    gcloud run jobs deploy server-tick \
        --source . \
        --tasks 1 \
        --task-timeout 5m \
        --memory 1Gi \
        --cpu 1 \
        --max-retries 2 \
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
  gcloud run jobs execute server-tick \
   --project=gruntt-destiny \
   --region us-central1
```