-- Hide legacy demo experiences that were seeded before real marketplace data.
-- Keep rows instead of deleting so any historical foreign keys remain intact.
UPDATE experiences
SET is_active = 0,
    status = 'draft',
    updated_at = CURRENT_TIMESTAMP
WHERE creator_id IS NULL
  AND slug IN (
    'ranch-day-at-tierra-colorada',
    'rock-climbing-in-villa-guerrero',
    'coastal-dining-in-acapulco',
    'private-transport'
  );
