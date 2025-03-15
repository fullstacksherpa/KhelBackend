-- Up migration: Create the function and trigger

-- Create the function to update the updated_at column
CREATE OR REPLACE FUNCTION update_bookings_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create the trigger to execute the function on each update
CREATE TRIGGER update_bookings_modtime
BEFORE UPDATE ON bookings
FOR EACH ROW EXECUTE FUNCTION update_bookings_modified_column();