# 02: Deploy the skeleton to the VM

**What to build:** The walking skeleton is live at `https://outage-radar.alisa-d.dev` on the existing VM (`176.126.85.17`). The old demo project on the VM is shut down; it currently serves a certificate that expired on 2026-08-07. Caddy owns ports 80 and 443 and obtains and renews TLS automatically. `.dev` is HSTS-preloaded, so a valid certificate is mandatory. A production compose file runs app (role `all`), Postgres, frontend and Caddy. Nightly encrypted `pg_dump` backups go to the hoster's S3-compatible object storage. The Brevo account is created and the sending subdomain is authenticated. The steps only a human can do (hoster console, Porkbun DNS, Brevo dashboard, secrets) are captured in an interactive wizard script.

**Blocked by:** 01 (Walking skeleton)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] The old demo is stopped and no longer bound to ports 80 or 443
- [ ] `https://outage-radar.alisa-d.dev` serves the skeleton with a valid Let's Encrypt certificate, and HTTP redirects to HTTPS
- [ ] The production compose file starts the full stack after a VM reboot
- [ ] A nightly encrypted backup lands in off-VM object storage, and a restore into a scratch database has been tested once
- [ ] The Brevo account is approved, and the DKIM, SPF and DMARC (`p=quarantine`) records for `outage-radar.alisa-d.dev` pass Brevo's verification
- [ ] A wizard script walks the human steps and writes the secrets it collects to `.env` without committing them
