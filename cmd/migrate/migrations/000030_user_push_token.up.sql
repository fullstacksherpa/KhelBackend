CREATE TABLE user_push_tokens (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGSERIAL NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expo_push_token TEXT,
  device_info JSONB,
  last_updated TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW(),
  UNIQUE(user_id, expo_push_token) -- NULLs are considered distinct in UNIQUE constraints
);

CREATE INDEX idx_user_push_tokens_user_id ON user_push_tokens(user_id);
CREATE INDEX idx_user_push_tokens_token ON user_push_tokens(expo_push_token) 
  WHERE expo_push_token IS NOT NULL; -- Partial index for better performance
