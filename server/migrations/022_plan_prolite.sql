-- Pro Lite plan tier observed from the Codex app-server (rateLimits.planType).
DO $$
DECLARE c TEXT;
BEGIN
  SELECT conname INTO c FROM pg_constraint WHERE conrelid='codex_plan'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%plan_type%';
  IF c IS NOT NULL THEN EXECUTE 'ALTER TABLE codex_plan DROP CONSTRAINT '||c; END IF;
END $$;
ALTER TABLE codex_plan ADD CONSTRAINT codex_plan_plan_type_check CHECK (plan_type IN ('plus','pro','prolite','unknown'));
