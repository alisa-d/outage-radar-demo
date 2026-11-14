# Modular monolith with process roles; Postgres as database and job queue

The backend is one Go binary and one container image. A role setting picks what a process runs: `api`, `ingest`, `notify`, or `all`. Background work runs on a Postgres-backed job queue (`SELECT … FOR UPDATE SKIP LOCKED`) instead of a separate message broker. At our scale (production: around 3k Notices a day, 100k daily users), a broker would add an extra system to run, secure and back up without any real throughput benefit. Putting jobs in the same database also lets us enqueue a job in the same transaction as the data change that caused it. On a single VM the demo runs one container with `all`; on Kubernetes each role becomes its own Deployment from the same image, with no change to application code.

## Consequences

- All configuration comes from the environment, and processes keep no local state, so any role can be scaled out or restarted freely.
- If queue volume ever outgrows Postgres, the queue sits behind an interface and can be swapped for a broker. Nothing suggests that will be needed.
