\c taskape

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
