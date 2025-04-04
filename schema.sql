CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    phone VARCHAR(255) NOT NULL UNIQUE,
    handle VARCHAR(255),
    profile_picture VARCHAR(255),
    bio VARCHAR(255),
    color VARCHAR(255)
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deadline TIMESTAMP WITH TIME ZONE,
    author VARCHAR(255) NOT NULL,
    "group" VARCHAR(255),
    group_id VARCHAR(255),
    assigned_to TEXT[],
    task_difficulty VARCHAR(50) DEFAULT 'medium',
    custom_hours INTEGER,
    mentioned_in_event BOOLEAN DEFAULT FALSE,
    is_completed BOOLEAN DEFAULT FALSE,
    proof_url TEXT,
    privacy_level VARCHAR(50) DEFAULT 'everyone',
    privacy_except_ids TEXT[]
);

CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_group_id ON tasks(group_id);
CREATE INDEX idx_tasks_author ON tasks(author);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS flag_status BOOLEAN DEFAULT FALSE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS flag_color VARCHAR(50) DEFAULT NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS flag_name VARCHAR(50) DEFAULT NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS display_order INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_tasks_display_order ON tasks(user_id, display_order);


CREATE TABLE IF NOT EXISTS user_friends (
    user_id INTEGER NOT NULL REFERENCES users(id),
    friend_id INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, friend_id)
);

CREATE TABLE IF NOT EXISTS friend_requests (
    id SERIAL PRIMARY KEY,
    sender_id INTEGER NOT NULL REFERENCES users(id),
    receiver_id INTEGER NOT NULL REFERENCES users(id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(sender_id, receiver_id)
);

CREATE INDEX IF NOT EXISTS idx_user_friends_user_id ON user_friends(user_id);
CREATE INDEX IF NOT EXISTS idx_user_friends_friend_id ON user_friends(friend_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_sender_id ON friend_requests(sender_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_receiver_id ON friend_requests(receiver_id);
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS proof_needed BOOLEAN DEFAULT FALSE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS proof_description TEXT;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS needs_confirmation BOOLEAN DEFAULT FALSE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS is_confirmed BOOLEAN DEFAULT FALSE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS confirmation_user_id VARCHAR(255);
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS confirmed_at TIMESTAMP WITH TIME ZONE;

CREATE TABLE IF NOT EXISTS events (
  id UUID PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  target_user_id INTEGER NOT NULL REFERENCES users(id),
  event_type VARCHAR(50) NOT NULL,
  event_size VARCHAR(10) NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  expires_at TIMESTAMP WITH TIME ZONE,
  task_ids TEXT[],
  streak_days INTEGER DEFAULT 0,
  likes_count INTEGER DEFAULT 0,
  comments_count INTEGER DEFAULT 0,
  CONSTRAINT valid_event_type CHECK (event_type IN ('new_tasks_added', 'newly_received', 'newly_completed', 'requires_confirmation', 'n_day_streak', 'deadline_coming_up')),
  CONSTRAINT valid_event_size CHECK (event_size IN ('small', 'medium', 'large'))
);

CREATE TABLE IF NOT EXISTS event_likes (
  id SERIAL PRIMARY KEY,
  event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE (event_id, user_id)
);

CREATE TABLE IF NOT EXISTS event_comments (
  id UUID PRIMARY KEY,
  event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id),
  content TEXT NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  is_edited BOOLEAN DEFAULT FALSE,
  edited_at TIMESTAMP WITH TIME ZONE,
  deleted BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS user_streaks (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    current_streak INTEGER NOT NULL DEFAULT 0,
    longest_streak INTEGER NOT NULL DEFAULT 0,
    last_completed_date DATE,
    last_streak_event_date DATE
);

CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);
CREATE INDEX IF NOT EXISTS idx_events_target_user_id ON events(target_user_id);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
CREATE INDEX IF NOT EXISTS idx_tasks_proof_needed ON tasks(proof_needed) WHERE proof_needed = TRUE;
CREATE INDEX IF NOT EXISTS idx_tasks_needs_confirmation ON tasks(needs_confirmation) WHERE needs_confirmation = TRUE;
CREATE INDEX IF NOT EXISTS idx_event_likes_event_id ON event_likes(event_id);
CREATE INDEX IF NOT EXISTS idx_event_likes_user_id ON event_likes(user_id);
CREATE INDEX IF NOT EXISTS idx_event_comments_event_id ON event_comments(event_id);
CREATE INDEX IF NOT EXISTS idx_event_comments_user_id ON event_comments(user_id);
CREATE INDEX IF NOT EXISTS idx_event_comments_created_at ON event_comments(created_at);

CREATE OR REPLACE FUNCTION update_event_likes_count()
RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE events SET likes_count = likes_count + 1 WHERE id = NEW.event_id;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE events SET likes_count = likes_count - 1 WHERE id = OLD.event_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS event_likes_count_trigger ON event_likes;
CREATE TRIGGER event_likes_count_trigger
AFTER INSERT OR DELETE ON event_likes
FOR EACH ROW
EXECUTE FUNCTION update_event_likes_count();

CREATE OR REPLACE FUNCTION update_event_comments_count()
RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'INSERT' AND NOT NEW.deleted THEN
    UPDATE events SET comments_count = comments_count + 1 WHERE id = NEW.event_id;
  ELSIF TG_OP = 'UPDATE' AND NEW.deleted = TRUE AND OLD.deleted = FALSE THEN
    UPDATE events SET comments_count = comments_count - 1 WHERE id = NEW.event_id;
  ELSIF TG_OP = 'UPDATE' AND NEW.deleted = FALSE AND OLD.deleted = TRUE THEN
    UPDATE events SET comments_count = comments_count + 1 WHERE id = NEW.event_id;
  ELSIF TG_OP = 'DELETE' AND NOT OLD.deleted THEN
    UPDATE events SET comments_count = comments_count - 1 WHERE id = OLD.event_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS event_comments_count_trigger ON event_comments;
CREATE TRIGGER event_comments_count_trigger
AFTER INSERT OR UPDATE OR DELETE ON event_comments
FOR EACH ROW
EXECUTE FUNCTION update_event_comments_count();

CREATE TABLE IF NOT EXISTS user_streaks (
    user_id INT PRIMARY KEY REFERENCES users(id),
    current_streak INT NOT NULL DEFAULT 0,
    longest_streak INT NOT NULL DEFAULT 0,
    last_completed_date TIMESTAMP,
    last_streak_event_date TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_user_streaks_user_id ON user_streaks(user_id);

CREATE OR REPLACE FUNCTION update_longest_streak()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.current_streak > NEW.longest_streak THEN
        NEW.longest_streak := NEW.current_streak;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_longest_streak_trigger ON user_streaks;
CREATE TRIGGER update_longest_streak_trigger
BEFORE UPDATE ON user_streaks
FOR EACH ROW
EXECUTE FUNCTION update_longest_streak();
