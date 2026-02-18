-- Sample PostgreSQL seed data for Faucet demo

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    age INTEGER,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE posts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    title VARCHAR(255) NOT NULL,
    body TEXT,
    published BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE post_tags (
    post_id INTEGER REFERENCES posts(id) ON DELETE CASCADE,
    tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, tag_id)
);

-- Seed data
INSERT INTO users (name, email, age, status) VALUES
    ('Alice', 'alice@example.com', 30, 'active'),
    ('Bob', 'bob@example.com', 25, 'active'),
    ('Charlie', 'charlie@example.com', 35, 'inactive');

INSERT INTO posts (user_id, title, body, published) VALUES
    (1, 'Getting Started with Faucet', 'Faucet turns any database into a REST API...', true),
    (1, 'Advanced Filtering', 'Use DreamFactory-compatible filter syntax...', true),
    (2, 'Draft Post', 'This is still a draft.', false);

INSERT INTO tags (name) VALUES ('tutorial'), ('api'), ('database');

INSERT INTO post_tags (post_id, tag_id) VALUES (1, 1), (1, 2), (2, 2), (2, 3);
