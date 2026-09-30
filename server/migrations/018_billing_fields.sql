-- M1 completion (INH-339): complete billing fields.
ALTER TABLE billing_events ADD COLUMN invoice_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_events ADD COLUMN plan_code TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_events ADD COLUMN period_start TIMESTAMPTZ;
ALTER TABLE billing_events ADD COLUMN period_end TIMESTAMPTZ;
