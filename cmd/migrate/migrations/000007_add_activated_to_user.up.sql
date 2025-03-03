ALTER TABLE users DROP COLUMN email_verification_token;
ALTER TABLE users DROP COLUMN email_verification_expires;
ALTER TABLE users RENAME COLUMN is_email_verified TO is_active;
