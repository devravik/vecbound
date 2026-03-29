-- Database Dump for VecBound Demo
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
INSERT INTO users (id, name, email) VALUES (1, 'Alice Johnson', 'alice@example.com');
-- Remember to add a new index for the vector search table
INSERT INTO users (id, name, email) VALUES (2, 'Bob Smith', 'bob@example.com');
