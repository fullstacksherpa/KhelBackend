-- Begin transaction (optional, depending on your migration tool)
BEGIN;

-- Drop the trigger from the "users" table if it exists
DROP TRIGGER IF EXISTS update_users_modtime ON users;

-- Drop the function if it exists
DROP FUNCTION IF EXISTS update_modified_column();

COMMIT;
