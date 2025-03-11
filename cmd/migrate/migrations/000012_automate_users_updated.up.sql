-- Begin transaction (optional, depending on your migration tool)
BEGIN;

-- Create or replace the function to auto-update the "updated_at" column
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create a trigger on the "users" table to invoke the function before each update
CREATE TRIGGER update_users_modtime
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION update_modified_column();

COMMIT;
