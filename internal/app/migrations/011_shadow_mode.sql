-- Shadow Mode: a shadow Provider's Alerts are recorded (shadow = true) and never delivered. Both demo Providers start in it.
ALTER TABLE providers ADD COLUMN shadow boolean NOT NULL DEFAULT true;
ALTER TABLE alerts ADD COLUMN shadow boolean NOT NULL DEFAULT false;
