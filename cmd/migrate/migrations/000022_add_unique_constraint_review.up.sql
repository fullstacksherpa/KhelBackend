ALTER TABLE reviews
ADD CONSTRAINT uq_reviews_venue_user
  UNIQUE (venue_id, user_id);