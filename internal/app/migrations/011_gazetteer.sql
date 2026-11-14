-- Villages match as a whole Settlement; cities need a resolvable place (ADR 0003).
ALTER TABLE settlements ADD COLUMN kind text NOT NULL DEFAULT 'city' CHECK (kind IN ('city', 'village'));

-- Named areas (kind 'area': geom is a closed ring) and landmarks (kind 'point': geom has one point), [[lon, lat], ...].
CREATE TABLE gazetteer (
    id            serial PRIMARY KEY,
    settlement_id int NOT NULL REFERENCES settlements,
    name          text NOT NULL,
    search        text NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('area', 'point')),
    geom          jsonb NOT NULL,
    UNIQUE (settlement_id, name)
);
