CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email citext UNIQUE NOT NULL,
    phone VARCHAR(20) UNIQUE NOT NULL CHECK (phone ~ '^[0-9]{10}$'),
    password bytea NOT NULL,
    first_name varchar(255) NOT NULL,
    last_name varchar(255) NOT NULL,
    profile_picture_url TEXT CHECK (profile_picture_url ~* '^https?://'),
    skill_level VARCHAR(20) CHECK (skill_level IN ('beginner', 'intermediate', 'advanced')),
    no_of_games INTEGER DEFAULT 0,
    refresh_token TEXT,
    is_email_verified BOOLEAN DEFAULT FALSE,
    email_verification_token TEXT,
    email_verification_expires TIMESTAMP,
    reset_password_token TEXT,
    reset_password_expires TIMESTAMP,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    updated_at timestamp(0) with time zone NOT NULL DEFAULT NOW()
);