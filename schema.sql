\c taskape

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    handle VARCHAR(255),
    profile_picture VARCHAR(255),
    bio VARCHAR(255),
    phone VARCHAR(255) NOT NULL
);
