# One Trick Scripts

In this repo is the folder of all the scripts/jobs that are used for one time jobs.

- [Migration Job](./migration)
- [Server Tick Job](./server-tick)

## Deploy
Currently, all deploys are done by hand as I don't need them deployed often 
nor do they update often. 

Check the README in each folder for the command to deploy with gcp cli.

## Jobs

### Migration Job

The migration job runs every day at 8 AM and will migrate certain Destiny 2
data base models from the JSON file to Firebase. 

All collections will be prefixed with "d2" i.e. "d2Places"

It will create a task per collection migration needed. It will also keep
track of the version for each collection needed and update them separately
once upgraded.

### Server Tick Job

The Server Tick job run every 5 minutes. Meaning every interval of 5 on the hour (00, 05, 10, 15, etc).
The purpose of the job is to act as the server tick for checking all "active" sessions and update them
per character. 

It will also complete stale or inactive sessions when a user forgets to turn off
a session. 