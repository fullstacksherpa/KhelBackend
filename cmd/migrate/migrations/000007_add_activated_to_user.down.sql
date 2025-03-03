ALTER TABLE users ADD COLUMN email_verification_token TEXT;
ALTER TABLE users ADD COLUMN email_verification_expires TIMESTAMP;
ALTER TABLE users RENAME COLUMN is_active TO is_email_verified;
