# 01: Walking skeleton

**What to build:** A runnable end-to-end skeleton: the single Go binary starts with a role (`api`, `ingest`, `notify`, `all`) chosen from the environment, connects to Postgres, runs migrations through a one-off command, and runs a Postgres-backed job queue. All "now" comes from an injectable clock. The public API lists Regions (Varna seeded with its Settlements and the two Providers), and a static SvelteKit page in bg/en shows the Region picker. `docker compose up` brings up app, Postgres, frontend and Mailpit locally. The Seam 1 test harness exists: real Postgres via testcontainers, a stub HTTP server for provider sites, a stub HTTP server for the Claude API, an in-test SMTP sink, and a controllable clock (see spec, Testing Decisions).

**Blocked by:** None (can start immediately)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] The binary runs each role (`api`, `ingest`, `notify`, `all`) from the same image, and the roles are selected only by environment
- [ ] Migrations run as a one-off command from the app image
- [ ] A job enqueued inside a transaction is processed only if that transaction commits (integration test)
- [ ] The public API returns the Varna Region with its Settlements, Providers and their Coverage
- [ ] The web page shows the Region picker in Bulgarian and English and works at phone width
- [ ] A Seam 1 test starts the system against real Postgres with the stub servers and clock, and asserts on the Regions API
- [ ] `docker compose up` runs everything locally

## Answer

- Single Go binary with env-selected role (api/ingest/notify/all), Postgres migrations, Postgres-backed job queue and injectable clock.
- Public API lists Regions (Varna seeded); static SvelteKit bg/en page shows the Region picker; docker compose runs app, Postgres, frontend, Mailpit.
- Seam 1 test harness (testcontainers Postgres, stub provider/Claude servers, SMTP sink, controllable clock) is in place.

Merged in fe57556.
