CREATE TABLE regions (
    id   serial PRIMARY KEY,
    name text NOT NULL UNIQUE
);

CREATE TABLE settlements (
    id        serial PRIMARY KEY,
    region_id int NOT NULL REFERENCES regions,
    name      text NOT NULL,
    ekatte    text NOT NULL UNIQUE
);

CREATE TABLE providers (
    id        serial PRIMARY KEY,
    region_id int NOT NULL REFERENCES regions,
    name      text NOT NULL,
    utility   text NOT NULL CHECK (utility IN ('water', 'power')),
    coverage  text NOT NULL,
    UNIQUE (region_id, name)
);

-- Postgres-backed job queue (ADR 0002). run_at NULL means ready now.
CREATE TABLE jobs (
    id       bigserial PRIMARY KEY,
    kind     text NOT NULL,
    payload  bytea NOT NULL,
    attempts int NOT NULL DEFAULT 0,
    run_at   timestamptz,
    done_at  timestamptz
);
CREATE INDEX jobs_pending ON jobs (id) WHERE done_at IS NULL;

INSERT INTO regions (name) VALUES ('Варна');
INSERT INTO settlements (region_id, name, ekatte) VALUES (1, 'Варна', '10135');
INSERT INTO providers (region_id, name, utility, coverage) VALUES
    (1, 'ВиК Варна', 'water', 'planned and emergency outages'),
    (1, 'ЕРП Север', 'power', 'planned outages only');
