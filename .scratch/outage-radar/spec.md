# Spec: Outage Radar (Varna demo, production-grade design)

Status: ready-for-agent

Vocabulary follows `GLOSSARY.md`. Decisions respect `docs/adr/0001` (account-less Subscribers), `0002` (modular monolith, Postgres as database and queue) and `0003` (Possibly affected and Unlocated).

## Problem Statement

Residents of Bulgarian cities have no single place to learn whether their water or power will be cut. Each Settlement is served by different Providers, and every Provider publishes Notices its own way. ВиК Варна posts loose prose on its website ("в района на ЖП гара", "част от кв. Аспарухово"). ЕРП Север lists streets and house numbers, but only for planned work. To find out whether their own building is affected, residents must check several sites every day and interpret vague area descriptions themselves. The notification services that Providers do offer are tied to the account holder (often a landlord), not to the person who actually lives at the address. As a result, people learn about outages when the tap runs dry.

## Solution

Outage Radar collects Notices from every Provider in a Region, turns them into structured Outages with an Affected Area, and shows them in one place: a Region page with today's and the coming week's Outages on a list and a map. Anyone can check an Address and get an honest per-Utility answer: Affected, Possibly affected, Not affected, or No data when we couldn't reach a Provider. Residents can subscribe one or more Watched Addresses with verified Contact Channels (email in the demo) and receive Alerts at a sensible time of day when an Outage newly affects them or changes materially. There is no account to create, every Alert has a one-click unsubscribe, and the resident's address and contact details are encrypted and invisible even to Operators. The demo covers the Varna Region with ВиК Варна (water) and ЕРП Север (power). Adding a Region or Provider means writing one new adapter and changing nothing else.

## User Stories

### Resident: viewing outages

1. As a resident, I want to pick my Region on the main page, so that I only see outages relevant to where I live.
2. As a resident, I want to see at a glance whether there are water or power Outages in my Region today, so that I can plan my day.
3. As a resident, I want to see Outages for today and the next 7 days grouped by day and Settlement, so that I can prepare for planned work in advance.
4. As a resident, I want each Outage to show its Utility, time window, Severity (Cut or Disturbance) and kind (planned or emergency), so that I understand how serious it is.
5. As a resident, I want approximate end times shown as approximate (e.g. "~19:00"), so that I am not misled by false precision.
6. As a resident, I want to read the Provider's original Notice text next to each Outage, so that I can verify the interpretation myself.
7. As a resident, I want to see Affected Areas on a map, with Possibly affected areas drawn differently from Affected ones, so that I can grasp the extent quickly.
8. As a resident, I want Outages that could not be placed on the map listed separately with their original text, so that I am not left unaware of them.
9. As a resident, I want a status strip that tells me per Utility whether the data is current or "unavailable since <time>", so that I know when the absence of Outages means nothing.
10. As a resident, I want to see each Provider's Coverage (e.g. "power: planned outages only"), so that I don't mistake missing emergency information for "all clear".
11. As a resident, I want the whole interface in Bulgarian or English, so that I can use it in my language.
12. As an English-speaking resident, I want structured Outage details (Utility, times, Severity, street names with Latin transliteration) in English while the original Notice stays in Bulgarian, so that I understand it without relying on machine translation.
13. As a resident on a phone, I want the main page to load fast and work at phone width, so that I can check it on the go.

### Resident: checking an address

14. As a resident, I want to choose my Settlement, then my street via autocomplete, then my building number, so that I enter an exact Address without typos.
15. As a resident, I want to see a pin for my building and confirm or drag it, so that the Address position is correct even when the registry only has an approximate position.
16. As a resident, I want a per-Utility result for my Address (Affected, Possibly affected, Not affected, No data), so that I know exactly what to expect.
17. As a resident, I want a Possibly affected result to explain why and link to the original Notice, so that I can judge for myself.
18. As a resident, I want to be told when there is an Unlocated Outage in my city, so that I know to read that Notice.
19. As a resident in a village, I want a village-wide Outage to show as affecting my Address, so that I'm informed even though the village has no street-level matching.
20. As a resident, I want my anonymous address check not to be stored, so that checking doesn't leave a trace of where I live.
21. As a resident, I want a clear "Get alerts for this address" action after checking, so that subscribing is one step away.

### Resident: subscribing and managing alerts

22. As a resident, I want to subscribe by giving my email address without creating an account or password, so that signing up takes seconds.
23. As a resident, I want to confirm my email via a verification message before anything else is sent, so that nobody can subscribe me without my consent.
24. As a resident, I want to give explicit consent to the privacy notice when I verify, so that I know how my data is used.
25. As a resident, I want to watch up to 5 Addresses (e.g. home and my parents' flat), so that I'm alerted about all the places I care about.
26. As a resident, I want to choose my Alert language (bg or en), so that Alerts arrive in the language I read.
27. As a resident, I want to receive an Alert when a new Outage Affects or Possibly affects one of my Watched Addresses, so that I can prepare.
28. As a resident, I want a Possibly affected Alert worded as such, with the original Notice, so that I'm not alarmed unnecessarily.
29. As a resident, I want Alerts for Disturbances worded as "possible disturbances", not "no power", so that the message matches reality.
30. As a resident, I want another Alert only when an Outage changes materially (the Match Result for my Address changes, or the time window moves by 30 minutes or more), so that I'm not spammed by trivial updates.
31. As a resident, I want never to receive the same Alert twice, so that I trust every message.
32. As a resident, I want non-urgent Alerts produced between 22:00 and 08:00 to arrive at 08:00, so that I'm not woken up.
33. As a resident, I want an Alert for an Outage starting before 09:00 sent immediately even at night, so that I learn about an early cut in time.
34. As a resident, I want emergency Outage Alerts sent immediately regardless of the hour, so that I can react to an outage that is already happening.
35. As a resident, I want to request a magic link to my verified email to manage my Watched Addresses and Contact Channels, so that I can change things without a password.
36. As a resident, I want magic links to expire quickly and work only once, so that a forwarded or leaked email can't be used against me.
37. As a resident, I want a one-click unsubscribe in every email (including the mail client's built-in unsubscribe button), so that I can stop Alerts instantly.
38. As a resident, I want removing my last Contact Channel to delete everything you hold about me, so that leaving is complete.
39. As a resident, I want to export all data you hold about me via a magic link, so that I can exercise my GDPR rights without writing to anyone.
40. As a resident, I want to delete all my data via a magic link, so that I can exercise my right to erasure myself.
41. As a resident who started but never verified a signup, I want that data purged automatically within 24 hours, so that abandoned signups don't linger.

### Resident: trust and correctness

42. As a resident, I want never to be told "no outage" while a Provider's data is stale, so that I'm not given false assurance.
43. As a resident, I want Outages already known to remain visible and alerting even when a Provider later becomes unreachable, so that a scraping failure doesn't hide real Outages.
44. As a resident, I want Alerts from newly added Providers to start only after they have been validated, so that I don't receive false Alerts from an untested source.
45. As a resident, I want my address and contact details invisible to other users and to backoffice staff, so that my home location is not exposed.

### Operator (backoffice)

46. As an Operator, I want to sign in to the backoffice with single sign-on and mandatory 2FA, so that access is strongly protected.
47. As an Operator, I want a review queue of Unlocated Outages and unresolved place names, so that I can fix what automatic resolution missed.
48. As an Operator, I want to attach an Affected Area to an Unlocated Outage (by drawing it or choosing streets or Gazetteer entries), so that the Alerts held back for it are released.
49. As an Operator, I want to add and edit Gazetteer entries (quarters, местности, resorts, landmarks) with their geometry, so that future Notices resolve automatically.
50. As an Operator, I want to see Provider Health (Healthy, Stale, Suspicious) with last success time and recent errors, so that I can tell whether data is trustworthy.
51. As an Operator, I want to see aggregate statistics (Subscribers per Settlement, Alerts sent per day, delivery success) without personal data, so that I can monitor the service without exposure.
52. As an Operator, I want to put a Provider in Shadow Mode and see the Alerts it would have sent, so that I can validate it against reality before going live.
53. As an Operator, I want to switch a Provider from Shadow Mode to live, so that its Alerts start flowing once validated.
54. As an Operator, I want to see both the original Notice and the extracted Outages side by side, so that I can spot extraction mistakes.
55. As an Operator, I want to correct an extracted Outage (time window, Severity, Affected Area), so that a wrong extraction doesn't produce wrong Alerts.

### Privacy officer

56. As a privacy officer, I want break-glass access to a single Subscriber's data, requiring a stated reason and expiring after a time limit, so that legitimate requests can be handled without standing access.
57. As a privacy officer, I want every break-glass access recorded in an audit log, so that misuse is detectable.
58. As a privacy officer, I want to process data-subject requests (export, erasure) on someone's behalf, so that requests arriving outside the self-service path are handled.
59. As a privacy officer, I want consent records stored with the time and the version of the privacy notice, so that we can demonstrate lawful basis.

### Engineering

60. As an engineer, I want an alert when a Provider becomes Stale or Suspicious, so that I fix a broken adapter before residents notice.
61. As an engineer, I want metrics per adapter (fetch success and latency, Notices found, extraction failures, unresolved names), so that I can see degradation trends.
62. As an engineer, I want Alert delivery metrics per channel (sent, failed, bounced), so that I can catch delivery problems.
63. As an engineer, I want an alert when the SMS budget cut-off trips, so that a bot attack or runaway cost is noticed (relevant once SMS exists).
64. As an engineer, I want to add a new Provider by writing one adapter without touching other code, so that expanding to new Regions is cheap and safe.
65. As an engineer, I want each adapter to choose its own fetch strategy (plain HTTP now; headless browser or proxy later), so that providers with bot protection can be supported without changing the rest.
66. As an engineer, I want adapters tested against real captured Notices, so that a provider format change shows up as a failing test.
67. As an engineer, I want the same container image to run all roles on one VM or separate roles on Kubernetes, so that moving to k8s changes only deployment, not code.
68. As an engineer, I want logs free of personal data, so that logs can be shared and retained without privacy risk.
69. As an engineer, I want structured logs and OpenTelemetry metrics and traces, so that I can switch observability backends without code changes.
70. As an engineer, I want nightly encrypted backups stored off the VM, so that losing the VM doesn't lose the data.
71. As an engineer, I want database migrations runnable as a one-off command from the app image, so that schema changes are repeatable everywhere.
72. As an engineer, I want the extraction accuracy measured on a labelled set of real Notices, so that I trust (or reject) changes to extraction logic or models.

### Abuse protection

73. As the service owner, I want a CAPTCHA on signup, so that bots can't mass-subscribe.
74. As the service owner, I want rate limits per IP, per Contact Channel and per Address on signup and verification, so that the verification channel can't be abused to spam people or run up costs.
75. As the service owner, I want a daily cap on verification attempts per Contact Channel, so that one address or number can't be flooded.
76. As the service owner, I want nothing costly (no Alerts) sent to an unverified Contact Channel, so that bots gain nothing from fake signups.
77. As the service owner, I want SMS restricted to +359 numbers with a global daily SMS budget cut-off (when SMS is added), so that SMS costs can't explode.

## Implementation Decisions

### Shape and runtime

- One Go 1.27 binary and one container image. A role setting picks what a process runs: `api`, `ingest`, `notify`, or `all` (ADR 0002). Processes are stateless; all configuration comes from the environment.
- Postgres is the database and the job queue (jobs taken with `FOR UPDATE SKIP LOCKED`, e.g. via the `river` library). A job is enqueued in the same transaction as the data change that caused it.
- The backend is API-first: a public JSON API (for the web app now and mobile later) and a backoffice JSON API.
- The web frontend is a static SvelteKit build in its own small container, with the map drawn by Leaflet over OSM tiles. It uses the public API only. bg and en translations cover all UI text.
- An injectable clock (time source) is used everywhere "now" matters: Quiet Hours, staleness, the 7-day window, Outage expiry, magic-link expiry and purges. All domain times are Europe/Sofia.

### Modules (deep modules behind small interfaces)

1. **Provider adapters (ingestion)**: one adapter per Provider, each self-contained. The interface is roughly: identify the Provider (Utility, Service Territory, Coverage, polling interval), and fetch the current raw Notices, each with the Provider's own ID (if any), publication time, Settlement hint, verbatim text and any structured fields the source offers. An adapter owns its fetch strategy. A shared fetch policy (identifying User-Agent, per-Provider polling interval, caching, backoff) is a helper that adapters use, not something forced on them.
   - **ЕРП Север adapter**: uses the oblast JSON endpoint (Varna oblast) with the `for_next_48_hours` and `all_active` types. Power Coverage is planned outages only, because emergency power outages are not published.
   - **ВиК Варна adapter**: uses the HTML lists for emergency (`breakdown`) and planned (`repair`) Notices, filtered to the Varna region and paging only as far as needed. The ArcGIS polygons are an optional extra input for matching, never the source of truth.
2. **Notice store**: deduplicates Notices by Provider ID, or else by a hash of the normalised text. A Notice whose text changes is re-extracted. Notices and Outages contain no personal data and are retained as history.
3. **Extraction**: turns a Notice into Outages (time window, kind, Severity, place descriptions). It works in two stages:
   - A Provider may supply a deterministic parser for regular formats (ЕРП Север: "гр. Варна - ул. X 1, 3, 5").
   - Anything the parser can't fully handle goes to a shared LLM extractor (Claude Haiku 5.5, structured output schema). The extractor receives only Notice text, never personal data, and runs once per unique Notice text.

   The extraction interface is a single operation, Notice in and Outages out, so another model or a rules engine can replace the LLM. Dates missing from a Notice default to the publication date. "Около" end times are flagged as approximate.
4. **Geo resolution**: turns place descriptions into an Affected Area with a confidence level, using the address registry and the Gazetteer. The rules:
   - **Named street:** the street's geometry, or specific building numbers where listed.
   - **"Between streets A, B, C, D":** the polygon enclosed by those streets.
   - **Named area with a polygon:** that polygon. Qualifiers like "част от" or "в района на" cap the confidence so the result is at most Possibly affected.
   - **Point landmark:** a radius of about 300 m, giving Possibly affected.
   - **Whole village:** the Settlement's area.
   - **Nothing resolvable inside a city:** the Outage is Unlocated and goes to the review queue.
   - **Unresolved names** lower the confidence and are queued for an Operator.
5. **Address registry**: Settlements (with ЕКАТТЕ codes), streets and building numbers with coordinates. It is seeded from OpenStreetMap, and missing house numbers are interpolated along the street geometry (odd and even sides), with interpolated points flagged as approximate. It offers street autocomplete and number lookup. The demo builds street-level data for Varna city; other Settlements exist at Settlement level only.
6. **Gazetteer**: named areas and landmarks per Settlement with their geometry, imported from OSM and edited by Operators.
7. **Matching**: a pure function from an Outage (Affected Area, confidence) and an Address to a Match Result (Affected, Possibly affected, Not affected). An Address check combines the Match Results with Provider Health and Coverage to give a per-Utility answer that includes No data and mentions Unlocated Outages.
8. **Provider Health**: tracks the last successful fetch and parse and the parse outcomes for each Provider.
   - **Stale:** no success within 3× the polling interval.
   - **Suspicious:** zero Notices where some are normally present, or an unusual share of items that fail to parse.

   Outages already known stay valid while a Provider is unhealthy; only the claim of completeness is withdrawn.
9. **Subscribers** (ADR 0001):
   - A Subscriber is known only through verified Contact Channels, and has up to 5 Watched Addresses and a language.
   - Verification and management use signed, short-lived, single-use magic links.
   - Consent records store the time and the version of the privacy notice.
   - Removing the last Contact Channel deletes the Subscriber.
   - Unverified signups are purged after 24 hours.
   - Data export and erasure are self-service.
10. **Alerting**: when Outages change, finds affected Watched Addresses and creates Alerts with a deduplication key (Outage, Watched Address, Match Result, time window).
    - A new Alert is created only for a new Outage, or for a material change: the Match Result changes, or the time window moves by 30 minutes or more.
    - Cancellations are not alerted.
    - Unlocated Outages create no Alerts until an Operator places them.
    - Providers in Shadow Mode create Alerts that are recorded but never delivered.
11. **Scheduling and delivery**: applies Quiet Hours (22:00–08:00 Europe/Sofia, fixed for now). Held Alerts are released at 08:00, except that Outages starting before 09:00 and all emergency Outages are sent immediately.
    - Delivery runs as one job per Contact Channel, through a channel adapter interface. Email is the only channel in this spec.
    - Retries use backoff.
    - Bounces and complaints arriving via the provider's webhook disable that Contact Channel.
    - Each delivery attempt is recorded, without the message body.
    - Templates are per channel and per language, and render structured fields in the Subscriber's language with the original Notice in Bulgarian.
12. **Email channel**: SMTP adapter with a bounce/complaint webhook. Mailpit is used locally and Brevo (EU, free tier) for the live demo. Mail is sent from `alerts@outage-radar.alisa-d.dev`. Every email carries `List-Unsubscribe` and `List-Unsubscribe-Post` headers (RFC 8058) and a footer link that removes that Contact Channel.
13. **Personal data protection**:
    - **Encryption at rest:** contact values and Watched Addresses (the chosen Address reference and confirmed coordinates) are encrypted field by field, using envelope encryption. Keys come from the environment or a file in the demo, behind a key-provider interface that a KMS or Vault can replace later.
    - **Blind index:** an HMAC of the normalised contact value supports lookup and uniqueness without decrypting.
    - **Logs, metrics and traces** never contain personal data.
    - **Backups** contain only ciphertext for these fields.
14. **Abuse protection**:
    - A Cloudflare Turnstile check runs on signup.
    - Rate limits apply per IP, per Contact Channel (by blind index) and per Address.
    - There is a daily cap on verification attempts per Contact Channel.
    - No Alerts go to unverified channels.
    - A global per-channel budget circuit breaker exists, set to its maximum for email and to apply to SMS later.
15. **Backoffice and access control**:
    - Operators sign in through OIDC with 2FA required. The demo uses Keycloak in compose, or any OIDC provider.
    - There are two roles:
      - **Operator:** the review queue, Outage correction, the Gazetteer, Provider Health, Shadow Mode switching and aggregate statistics.
      - **Privacy officer:** break-glass access to one Subscriber, with a reason and a time limit, every use written to the audit log; and data-subject requests.
    - No backoffice endpoint returns decrypted personal data outside break-glass access.
16. **Observability**: OpenTelemetry metrics and traces, and `slog` structured logs. The demo compose file adds Prometheus, Grafana and Alertmanager, with engineering alerts by email. The key metrics:
    - for each adapter: fetch success and latency, Notices found, extraction failures, unresolved names;
    - Provider Health state;
    - Alert delivery outcomes per channel;
    - circuit-breaker trips.

### Public API (contract outline)

- List Regions, with each Region's Settlements, Providers, Coverage and current Provider Health.
- Get the Outages for a Region over a date range (default: today plus 7 days). Each Outage includes its Utility, kind, Severity, time window (with the approximate flag), Affected Area geometry and confidence, whether it is Unlocated, its Settlement, and the original Notice text and link.
- Autocomplete streets in a Settlement, and look up building numbers on a street, returning coordinates and the approximate flag.
- Check an Address (or confirmed coordinates): returns a per-Utility answer (a Match Result or No data) with the contributing Outages, any Unlocated Outages in the Settlement, and Coverage notes. Nothing is stored.
- Start a subscription: Turnstile token, contact value, Address and confirmed position, language. Returns the same neutral response whether or not the contact value is already known, so the endpoint can't be used to discover who is subscribed.
- Verify (consumes a magic link and records consent), request a management magic link, manage Watched Addresses, Contact Channels and language through a management session, export data, delete everything.
- One-click unsubscribe endpoint (RFC 8058 POST), plus a provider webhook for email bounces and complaints.

### Data model outline (no personal data unless noted)

- Region, Settlement (ЕКАТТЕ code), Provider (Utility, Service Territory, Coverage, Shadow Mode flag, health state).
- Notice (Provider, Provider ID or text hash, publication time, verbatim text, version), and Outage (Notice, Utility, kind, Severity, window start and end, approximate flag, Affected Area geometry, confidence, Unlocated flag, Operator corrections).
- Street, Address (coordinates, approximate flag), Gazetteer entry (name, kind, geometry, Settlement).
- Subscriber, Contact Channel (encrypted value, blind index, verified-at, disabled-at), Watched Address (encrypted Address reference and coordinates), Consent record. These are the personal-data tables.
- Alert (deduplication key, Match Result, scheduled-for, shadow flag), Delivery attempt (channel, outcome, time; no body).
- Audit log (break-glass actions, Operator corrections, Shadow Mode switches).
- Job queue tables.

### Deployment

- Docker Compose on the existing VM (`176.126.85.17`), running: the app (role `all`), Postgres, the frontend container, Caddy, and the observability stack. Keycloak and Mailpit are added for local development.
- Caddy owns ports 80 and 443 and obtains and renews TLS certificates automatically for `outage-radar.alisa-d.dev`. The old demo project on the VM is shut down; its certificate expired on 2026-08-07.
- Migrations run as a one-off command from the app image (a Job on k8s later).
- Nightly encrypted `pg_dump` goes to the hoster's S3-compatible object storage.
- DNS: the existing wildcard A record already covers the subdomain. Add Brevo's DKIM records, an SPF record and DMARC (`p=quarantine`, reports to an address the owner reads) on `outage-radar.alisa-d.dev`.
- The app has no knowledge of the orchestrator, so moving to k8s means new manifests and CI/CD, not code changes.

### Rollout

- Both demo Providers start in Shadow Mode. An Operator compares the recorded Alerts with reality for a few days, then switches each Provider live.

## Testing Decisions

- **What makes a good test**: tests assert external behaviour (API responses, emails sent, Provider Health, Match Results), never internal structure. A refactor that keeps behaviour must not break tests. Use real Postgres, not mocks of our own code. Fake only things outside the system, at their natural boundary.
- **Seam 1, the main one: the backend as a black box.** The real binary with role `all` runs against real Postgres (testcontainers). Provider sites are a stub HTTP server replaying captured real Notices; adapters point at it through their base-URL configuration. The Claude API is stubbed at HTTP level. Email goes to an in-test SMTP sink. Time comes from the injectable clock. Tests drive the public and backoffice APIs. Most user stories are covered here: address check, subscribe, verify, Alert creation, deduplication, material change, Quiet Hours and urgency, Unlocated holding and release, Shadow Mode, Provider Health and No data, unsubscribe (including RFC 8058), purges, export and erasure, break-glass and audit, rate limits, and that no personal data appears in logs or in backoffice responses.
- **Seam 2: extraction.** Notice text in, Outages out. A labelled set of real Varna Notices (both Providers, including the examples gathered during design) with expected Outages. Deterministic parsers run against it in CI. The LLM path runs on demand (it costs money) and reports accuracy; it is the gate for changing models or prompts.
- **Seam 3: matching.** Affected Area plus Address in, Match Result out. Fixtures use real Varna geometry: an enclosed block (бул. Васил Левски / ул. Беласица / ул. Тодор Влайков / бул. Сливница), listed building numbers, "част от" a quarter, a landmark radius (ЖП гара, Фестивален и Конгресен център), a whole village, and Unlocated.
- **Adapter tests** (through Seam 1's stub server): golden files of captured real responses for each Provider, so that a provider format change becomes a failing fixture. Fixtures for Provider Health cover empty responses, errors and garbage.
- **Frontend**: a few browser end-to-end tests of the main path only (Region page → address check → subscribe → verification email in Mailpit → manage → unsubscribe). Everything else is covered at the API.
- **Prior art**: none. This is a greenfield repo, and these tests set the patterns.

## Out of Scope

- SMS, Viber and push channels, and the mobile apps (the channel adapter interface and budget circuit breaker are designed for them, but they are not built).
- Regions other than Varna, and Providers other than ВиК Варна and ЕРП Север.
- Street-level matching outside Varna city (other Settlements are matched at Settlement level only).
- Alerts for cancellations, and reminders before an Outage starts.
- Emergency power outages, which ЕРП Север doesn't publish. Scraping press releases or the per-customer check (which needs other people's PINs) is ruled out.
- Machine translation of Notice text.
- Per-Subscriber Quiet Hours and per-address channel choice.
- Kubernetes manifests and production CI/CD beyond what the single VM needs.
- Bot-protection evasion for Providers (the adapter design allows it, but none is needed for Varna).

## Further Notes

- **Source facts (checked 2026-11-14):**
  - ЕРП Север publishes 3–9 days ahead in working-day batches, through an unauthenticated JSON endpoint.
  - ВиК Варна's HTML has no Cloudflare and no robots.txt; planned work is announced about a day ahead, and emergency Notices appear ad hoc several times a day.
  - Neither source shows a cancellation mechanism.
  - OSM has house numbers on only about 13% of Varna buildings, which is why the confirmed pin is the normal path rather than an edge case.
  - Check each Provider's terms of use before production.
- **Extraction cost estimate:** about $1 a month for the demo and about $60 a month at production scale on Claude Haiku 5.5, because cost scales with Notices, not users.
- **Brevo's free tier** is 300 emails a day, and new accounts may need approval, so sign up early.
- **GDPR**: lawful basis is consent; EU hosting; a data processing agreement with the email provider; a record of processing and a privacy notice in bg and en.
- `.dev` is HSTS-preloaded, so the site only works once Caddy holds a valid certificate.
