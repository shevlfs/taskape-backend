\c taskape

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    phone VARCHAR(255) NOT NULL UNIQUE,
    handle VARCHAR(255),
    profile_picture VARCHAR(255),
    bio VARCHAR(255),
    color VARCHAR(255)
);
