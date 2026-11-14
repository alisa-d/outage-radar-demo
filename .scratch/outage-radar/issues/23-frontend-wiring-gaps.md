# 23: Frontend wiring gaps

**What to build:** The main path works in a real browser, end to end, before the browser E2E ticket (22) is written. Fix the gaps earlier tickets left in the web app: the signup form sends a Turnstile token (ticket 12 made it mandatory); the `/subscribe` page exists, is linked from the address check's "Get alerts" link (`/subscribe?street_id=..&number=..`, ticket 07) and subscribes the checked Address; `/manage` can add a Watched Address using the street id (ticket 13: AddressPicker returns only the street name there). Then open every page in a real browser and fix what is broken: Region picker, address check, status strip, region map, `/subscribe`, `/verify`, `/manage`, `/unsubscribe`, at desktop and phone width.

**Blocked by:** None (tickets 07, 09, 12, 13, 14 are merged)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`. Origin: the `## Answer` sections of tickets 07, 09, 12, 13 and 14.

- [ ] The signup form sends a Turnstile token (Cloudflare's documented test site key in dev and compose; real key by env)
- [ ] `/subscribe` exists and the address check's "Get alerts" link lands on it with the Address prefilled
- [ ] `/manage` can add a Watched Address
- [ ] Every page listed above was opened in a real browser, at desktop and phone width, with what was seen reported (or the report says plainly that no browser was available)
- [ ] `docker compose up` serves the full path locally with Mailpit receiving the verification email
