# 23: Frontend wiring gaps

**What to build:** The main path works in a real browser, end to end, before the browser E2E ticket (22) is written. Fix the gaps earlier tickets left in the web app: the signup form sends a Turnstile token (ticket 12 made it mandatory); the `/subscribe` page exists, is linked from the address check's "Get alerts" link (`/subscribe?street_id=..&number=..`, ticket 07) and subscribes the checked Address; `/manage` can add a Watched Address using the street id (ticket 13: AddressPicker returns only the street name there). Then open every page in a real browser and fix what is broken: Region picker, address check, status strip, region map, `/subscribe`, `/verify`, `/manage`, `/unsubscribe`, at desktop and phone width.

**Blocked by:** None (tickets 07, 09, 12, 13, 14 are merged)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`. Origin: the `## Answer` sections of tickets 07, 09, 12, 13 and 14.

- [x] The signup form sends a Turnstile token (Cloudflare's documented test site key in dev and compose; real key by env)
- [x] `/subscribe` exists and the address check's "Get alerts" link lands on it with the Address prefilled
- [x] `/manage` can add a Watched Address
- [x] Every page listed above was opened in a real browser, at desktop and phone width, with what was seen reported (or the report says plainly that no browser was available)
- [x] `docker compose up` serves the full path locally with Mailpit receiving the verification email

## Answer

- The signup form now sends a Turnstile token (test site key in dev/compose), `/subscribe` exists and is linked from "Get alerts" with the Address prefilled, and `/manage` adds a Watched Address by street id.
- Every page was driven in headless Chromium via Playwright at desktop and phone width, and `docker compose up` served the full path with Mailpit receiving the verification email.
- Merged in commit 5ac292f (`git merge --no-ff worktree-agent-a77337c3e50fd16e7`).

Known gaps:
- `/unsubscribe` was rendered but its button never clicked with a real token (no Alert was sent in that run).
- The map popup and legend were not clicked.
- The street, numbers and outage used were hand-seeded with SQL and are not committed; compose has no seed.
- No frontend tests; the Playwright script is not in the repo and screenshots live only in a session scratchpad (ticket 22 should make this repeatable).
- `compose.prod.yaml` is untouched, so prod needs the real `TURNSTILE_SECRET` and `VITE_TURNSTILE_SITE_KEY` via env.
