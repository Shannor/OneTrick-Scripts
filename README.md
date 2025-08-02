# Destiny Migration Job


Deploying it 
```shell
    gcloud run jobs deploy migration \
        --source . \
        --tasks 14 \
        --task-timeout 30m \
        --memory 2Gi \
        --cpu 1 \
        --max-retries 5 \
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