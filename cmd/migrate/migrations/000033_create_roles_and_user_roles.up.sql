-- roles table
CREATE TABLE roles (
    id BIGSERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL, -- e.g. "admin", "owner", "customer"
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- user_roles table (many-to-many)
CREATE TABLE user_roles (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

-- Seed some roles
INSERT INTO roles (name, description)
VALUES 
    ('customer', 'Regular customer'),
    ('owner', 'Venue owner'),
    ('merchant', 'Merchant or business account'),
    ('admin', 'System administrator')
ON CONFLICT (name) DO NOTHING;
