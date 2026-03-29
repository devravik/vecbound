# VecBound

VecBound is a Go-based CLI for local-first text vectorization and semantic search. It crawls directories, extracts content from multiple file formats, and generates a SQLite database with local ONNX embeddings.

No Python, Docker, or API keys required. It is a standalone static binary for local data search.

## Quickstart

```bash
# 1. Setup (downloads models and runtime)
make setup

# 2. Index your data
./vecbound index -s ./path/to/your/docs

# 3. Search semantically
./vecbound search -q "your question"
```

---

## Core Stack
- **Runtime:** Go 1.24+
- **Database:** `modernc.org/sqlite` (CGO-free) with `sqlite-vec` support.
- **Embeddings:** `onnxruntime-go` (`all-MiniLM-L6-v2`).
- **Extraction:** Modular pipeline for 20+ file formats.

---

## Supported File Formats
VecBound supports the following formats:
- **Documents:** .pdf, .docx, .xlsx, .odt, .ods, .odp, .rtf
- **Text & Web:** .txt, .md, .html, .htm, .xml
- **Data:** .json, .jsonl, .csv, .tsv, .yaml, .yml, .sql

## Usage Details

### Global Flags
These flags control the tool's resource footprint.

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--max-cpu` | | (NumCPU/2) | Limit concurrent CPU workers. |
| `--max-mem` | | 512 | Soft memory limit in MB. |
| `--config` | | ~/.vecbound/config.json | Path to custom config file. |
| `--verbose` | -v | false | Enable debug logging. |

### Indexing (index command)
Build or update the vector database.

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--source` | -s | (Required) | Directory to crawl and index. |
| `--out` | -o | vec.db | Output SQLite database file path. |
| `--workers` | -w | 4 | Number of concurrent workers. |
| `--clear` | | false | Clear existing index before starting. |
| `--chunk-size` | | 500 | Tokens per chunk. |
| `--chunk-overlap` | | 50 | Overlapping tokens between chunks. |
| `--model` | | model.onnx | Path to the ONNX model file. |
| `--vocab` | | vocab.txt | Path to the tokenizer vocabulary file. |

**Ignore List:** Skips `.git`, `node_modules`, `vendor`, `.idea`, `.vscode`, `__pycache__`, and `.DS_Store`.

### Searching (search command)
Query the database using natural language.

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--db` | -d | vec.db | Path to the vector database. |
| `--query` | -q | (Required) | Search query. |
| `--limit` | -l | 5 | Max number of results. |
| `--offset` | | 0 | Results to skip (pagination). |
| `--boost` | -b | 0.5 | Exact match score boost. |
| `--format` | -f | text | Output format (text, json, jsonl, csv, table, md). |
| `--context` | -c | 200 | Context around the match. |
| `--out` | -o | (Stdout) | Write results to a file. |

---

## Technical Overview

### Semantic vs. Keyword Search
Standard search looks for exact word matches. VecBound uses semantic search to understand meaning (e.g., "canine" will find notes on "dogs").

### Hybrid Search (Semantic + Exact)
VecBound combines vector embeddings with literal string matching. If a query appears exactly in a chunk, it receives a score boost (default +0.5). This ensures technical terms or specific IDs surface even if semantic alignment is not perfect.

### Local-First Design
- **Privacy:** Data stays on your machine.
- **Speed:** Sub-10ms similarity searches locally.
- **Portability:** Single binary and a SQLite file.

### Use Case
Search through scattered notes using natural questions instead of exact phrases. VecBound finds mathematically similar content based on context rather than just keyword overlap.

## Framework Integrations

### Laravel (PHP)
Integrate or call VecBound using the `Process` facade.

```php
use Illuminate\Support\Facades\Process;

$result = Process::run('./vecbound search --query "race conditions" --format json')
    ->throw()
    ->json();

foreach ($result as $match) {
    echo "Found match in {$match['path']} Scored: {$match['score']}\n";
}
```

## Project Structure
- `cmd/`: CLI command definitions (Cobra).
- `internal/embedder/`: ONNX runtime integration.
- `internal/processor/`: File walker and extractors.
- `internal/storage/`: SQLite schema and similarity logic.

## Maintainer
**Ravi K Gupta**
- [devravik.github.io](https://devravik.github.io/)
- [github.com/devravik](https://github.com/devravik)

## License
MIT.