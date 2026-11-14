# outage-radar-demo
Demo project for a talk on spec-driven development with AI agents: an outage alert service for water and power, built end to end with Claude Code

## Deploy

`deploy/wizard.sh` walks the human steps (old demo, backup bucket, Brevo, DNS) and writes `.env`. The stack runs from `compose.prod.yaml` behind Caddy; `deploy/backup.sh` makes the nightly encrypted dump and `deploy/restore-test.sh` proves a restore locally.
