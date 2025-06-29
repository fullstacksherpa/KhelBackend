ALTER TABLE bookings
ADD COLUMN customer_name VARCHAR(100),
ADD COLUMN note TEXT CHECK (char_length(note) <= 255),
ADD COLUMN customer_phone VARCHAR(15)
  CHECK (
    customer_phone ~ '^0[1-9][0-9]{7}$' OR
    customer_phone ~ '^98[4-9][0-9]{7}$'
  );
