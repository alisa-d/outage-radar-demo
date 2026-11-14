CREATE TABLE streets (
    id            serial PRIMARY KEY,
    settlement_id int NOT NULL REFERENCES settlements,
    name          text NOT NULL,
    search        text NOT NULL, -- normalised Latin form for autocomplete
    geom          jsonb NOT NULL, -- [[lon, lat], ...]
    UNIQUE (settlement_id, name)
);

CREATE TABLE house_numbers (
    street_id int NOT NULL REFERENCES streets,
    number    text NOT NULL,
    lat       double precision NOT NULL,
    lon       double precision NOT NULL,
    PRIMARY KEY (street_id, number)
);
