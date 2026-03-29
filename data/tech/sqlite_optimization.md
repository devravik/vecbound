# SQLite Optimization Guide

SQLite is a powerful, embedded database engine. To get the best performance for vector storage and retrieval, consider the following:

## 1. Using Write-Ahead Log (WAL)
WAL mode allows multiple readers and one writer to work concurrently. It improves write performance and avoids blocking readers during writes.

```sql
PRAGMA journal_mode=WAL;
```

## 2. Synchronous Settings
For maximum speed, you can set synchronous to `NORMAL`. This is a tradeoff between speed and durability. In `NORMAL` mode, the database might become corrupt if the OS crashes, but not if just the application crashes.

```sql
PRAGMA synchronous=NORMAL;
```

## 3. Page Size
Matching the database page size to the OS block size (usually 4096) can improve I/O efficiency. Larger page sizes (like 16384) can be better for large BLOB storage.

## 4. Vector Storage
When storing embeddings, use `BLOB` fields for compact storage. Indexed vector search typically requires a virtual table like `sqlite-vec`.

```sql
CREATE VIRTUAL TABLE vec_index USING vec0(
  chunk_id INTEGER PRIMARY KEY,
  embedding FLOAT[384]
);
```

## 5. Memory-Mapped I/O (mmap)
SQLite can use `mmap` to access database files directly. This can significantly speed up read operations by avoiding copies between kernel and user space.

```sql
PRAGMA mmap_size=268435456; -- 256MB
```

## 6. Temporary Files
Storing temporary tables and indices in memory can speed up complex queries.

```sql
PRAGMA temp_store=MEMORY;
```

## 7. Vacuum and Auto-Vacuum
Over time, SQLite databases can become fragmented. Regular `VACUUM` calls reclaim unused space and defragment the database file. Auto-vacuum can automate this, but handles it less efficiently than a full vacuum.
