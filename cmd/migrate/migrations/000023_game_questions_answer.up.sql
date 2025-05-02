
CREATE TABLE IF NOT EXISTS game_questions (
  id BIGSERIAL PRIMARY KEY,
  game_id INT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question VARCHAR(120) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS game_question_replies (
  id BIGSERIAL PRIMARY KEY,
  question_id INT NOT NULL REFERENCES game_questions(id) ON DELETE CASCADE,
  admin_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reply VARCHAR(120) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Function to auto-update "updated_at" column before updates
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for game_questions
CREATE TRIGGER update_game_questions_modtime 
BEFORE UPDATE ON game_questions 
FOR EACH ROW EXECUTE FUNCTION update_modified_column();

-- Trigger for game_question_replies
CREATE TRIGGER update_game_question_replies_modtime 
BEFORE UPDATE ON game_question_replies 
FOR EACH ROW EXECUTE FUNCTION update_modified_column();

-- Optimize JOINs on foreign keys
CREATE INDEX idx_game_questions_game_id ON game_questions(game_id);
CREATE INDEX idx_replies_question_id ON game_question_replies(question_id);