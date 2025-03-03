#!/bin/bash
create_db() {
    local db_file="$1"
    cat <<EOF | sqlite3 "$db_file"
DROP TABLE IF EXISTS Books;
CREATE TABLE Books (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    author TEXT NOT NULL,
    publication_year INTEGER
);
INSERT INTO Books (title, author, publication_year)
VALUES
('The Hitchhiker''s Guide to the Galaxy', 'Douglas Adams', 1979),
('The Lord of the Rings', 'J.R.R. Tolkien', 1954),
('Harry Potter and the Sorcerer''s Stone', 'J.K. Rowling', 1997),
('The Catcher in the Rye', 'J.D. Salinger', 1951),
('To Kill a Mockingbird', 'Harper Lee', 1960),
('1984', 'George Orwell', 1949),
('The Great Gatsby', 'F. Scott Fitzgerald', 1925);
EOF
    echo "Created $db_file"
}

rm -rf /tmp/harmonylite-1-* /tmp/harmonylite-2-* /tmp/harmonylite-3-*
create_db /tmp/harmonylite-1.db
create_db /tmp/harmonylite-2.db
create_db /tmp/harmonylite-3.db


cleanup() {
    kill "$job1" "$job2" "$job3"
}

trap cleanup EXIT
rm -rf /tmp/nats
../harmonylite -config node-1-config.toml -cluster-addr localhost:4221 -cluster-peers 'nats://localhost:4222/,nats://localhost:4223/' &
job1=$!

sleep 1
../harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/,nats://localhost:4223/' &
job2=$!

sleep 1
../harmonylite -config node-3-config.toml -cluster-addr localhost:4223 -cluster-peers 'nats://localhost:4221/,nats://localhost:4222/' &
job3=$!

wait $job1 $job2 $job3