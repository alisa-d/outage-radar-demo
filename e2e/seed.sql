-- Test-only seed (e2e/run.sh applies it after migrations; never part of a real deployment).
-- Both Providers healthy, and one current ВиК Варна emergency Outage (water, so Quiet Hours never hold its Alert)
-- on улица Фердинанд 10 only. The scan job that turns it into an Alert is enqueued by the test, after the Provider goes live.
INSERT INTO provider_health (provider_id, interval_s, first_seen_at, last_success_at, seen_notices)
SELECT id, 86400, now(), now(), true FROM providers;

INSERT INTO notices (provider_id, external_id, text, content_hash, published_at)
SELECT id, 'e2e-1', 'Аварийно спиране на водата на улица Фердинанд 10', 'e2e', now() FROM providers WHERE name = 'ВиК Варна';

INSERT INTO outages (notice_id, provider_id, kind, severity, start_at, end_at, settlement, places)
SELECT n.id, n.provider_id, 'emergency', 'cut', now() - interval '1 hour', now() + interval '5 hours', 'Варна', ARRAY['улица Фердинанд 10']
FROM notices n WHERE n.external_id = 'e2e-1';
