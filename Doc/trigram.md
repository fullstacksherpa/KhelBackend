# 🧠 PostgreSQL Trigram (`pg_trgm`) Deep Dive

## Complete Mental Model & End-to-End Flow

---

### 🧩 User Journey

**Flow:**  
`User Types "Barca" → Application → PostgreSQL Query → pg_trgm → GIN/GiST Index → Results`

---

## 1. What is pg_trgm?

`pg_trgm` is a PostgreSQL extension that enables **fuzzy string matching** using _trigrams_ (groups of 3 consecutive characters).

### 🔹 Core Concept: Trigrams

A **trigram** is a sequence of three consecutive characters extracted from a string.

**Example**

| String    | Trigrams                                      |
| --------- | --------------------------------------------- |
| `"hello"` | `" h"`, `" he"`, `hel`, `ell`, `llo`, `"lo "` |

> Padding with spaces at start/end is important for boundary matching.

```sql
-- Enable the extension
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- View trigrams for a string
SELECT show_trgm('hello');
-- Result: {" h"," he",ell,hel,llo,"lo "}

```

## 2. 🔢 Similarity Algorithms

### Key functions

```sql
-- Basic similarity (range: 0.0 - 1.0)
SELECT similarity('hello', 'hell');          -- 0.5714286
SELECT similarity('christopher', 'chris');   -- 0.46153846

-- Distance (inverse of similarity)
SELECT 'hello' <-> 'hell' AS distance;       -- 0.4285714

-- Word similarity (substring matching)
SELECT word_similarity('chris', 'christopher'); -- 0.8333333

```

## 3. ⚙️ Index Types: GIN vs GiST

🔸 GIN (Generalized Inverted Index)

- ✅ Faster for reads, slower for writes
- ✅ Better for multiple search terms
- ✅ Ideal for search-heavy applications
- ❌ Larger disk space usage

```sql
CREATE INDEX CONCURRENTLY users_name_gin_idx
ON users USING gin (name gin_trgm_ops);

```

🔸 GiST (Generalized Search Tree)

- ✅ Faster for writes, smaller disk footprint
- ✅ Better for mixed read/write workloads
- ❌ Slower for complex searches

```sql
CREATE INDEX CONCURRENTLY users_name_gist_idx
ON users USING gist (name gist_trgm_ops);

```

## 4. 🔁 Complete End-to-End Flow

- Step 1: User Input

- User searches: "michal" (intended: "michael")

- Step 2: Application Query

```sql
SELECT
name,
similarity(name, 'michal') AS score
FROM users
WHERE name % 'michal'
ORDER BY score DESC
LIMIT 10;
```

- Step 3: PostgreSQL Execution Flow

- Parse query with % operator

- Access GIN/GiST trigram index

- Compute trigrams for 'michal':
  → {" m"," mi","mic","ich","cha","hal","al "}

- Retrieve overlapping trigrams via index

- Calculate similarity scores

- Return ranked results

## 🧩 Building Optimal Queries

✅ Basic Similarity Search

```sql
-- Simple fuzzy match (uses index)
SELECT name FROM users WHERE name % 'michal';

-- With scoring and ordering
SELECT
  name,
  similarity(name, 'michal') AS match_score
FROM users
WHERE name % 'michal'
ORDER BY match_score DESC
LIMIT 10;


```

⚡ Advanced Multi-Strategy Search

```sql
SELECT
  name,
  similarity(name, 'michal') AS basic_score,
  word_similarity('michal', name) AS word_score,
  (similarity(name, 'michal') * 0.6 +
   word_similarity('michal', name) * 0.4) AS combined_score
FROM users
WHERE
  name % 'michal' OR
  name ILIKE '%michal%' OR
  'michal' % name
ORDER BY combined_score DESC
LIMIT 20;

```

## 9. 🛍 Real-World E-commerce Search Example

```sql
CREATE TABLE products (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  category TEXT,
  brand TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Trigram indexes
CREATE INDEX CONCURRENTLY products_name_trgm_idx
  ON products USING gin (name gin_trgm_ops);

CREATE INDEX CONCURRENTLY products_description_trgm_idx
  ON products USING gin (description gin_trgm_ops);

CREATE INDEX CONCURRENTLY products_brand_trgm_idx
  ON products USING gin (brand gin_trgm_ops);

-- Composite index
CREATE INDEX CONCURRENTLY products_search_composite_idx
  ON products USING gin (
    name gin_trgm_ops,
    description gin_trgm_ops,
    brand gin_trgm_ops
  );

```

## Advanced Product Search Function

```sql

CREATE OR REPLACE FUNCTION search_products(
  search_query TEXT,
  category_filter TEXT DEFAULT NULL,
  min_similarity FLOAT DEFAULT 0.2,
  result_limit INT DEFAULT 50
)
RETURNS TABLE (
  product_id BIGINT,
  product_name TEXT,
  product_category TEXT,
  product_brand TEXT,
  relevance_score FLOAT,
  match_source TEXT
)
LANGUAGE plpgsql
STABLE
AS $$
BEGIN
  RETURN QUERY
  SELECT
    p.id,
    p.name,
    p.category,
    p.brand,
    GREATEST(
      similarity(p.name, search_query),
      word_similarity(search_query, p.name),
      similarity(p.description, search_query) * 0.7,
      similarity(p.brand, search_query) * 0.9
    ) AS score,
    CASE
      WHEN p.name ILIKE '%' || search_query || '%' THEN 'name_exact'
      WHEN p.description ILIKE '%' || search_query || '%' THEN 'desc_exact'
      WHEN p.brand ILIKE '%' || search_query || '%' THEN 'brand_exact'
      ELSE 'fuzzy_match'
    END AS source
  FROM products p
  WHERE
    (p.name % search_query OR
     p.description % search_query OR
     p.brand % search_query OR
     search_query % p.name OR
     p.name ILIKE '%' || search_query || '%' OR
     p.description ILIKE '%' || search_query || '%' OR
     p.brand ILIKE '%' || search_query || '%')
    AND (category_filter IS NULL OR p.category = category_filter)
    AND GREATEST(
      similarity(p.name, search_query),
      word_similarity(search_query, p.name),
      similarity(p.description, search_query) * 0.7,
      similarity(p.brand, search_query) * 0.9
    ) >= min_similarity
  ORDER BY score DESC
  LIMIT result_limit;
END;
$$;

```

```sql

User Interface (Search Bar)
    ↓
Application Layer
    ↓ REST API: GET /search?q=Barca&limit=10&min_score=0.3
Backend Service
    ↓ Query Construction & Parameter Validation
PostgreSQL with pg_trgm
    ↓ Query: SELECT ... WHERE name % 'Barca' AND similarity() > 0.3
GIN Trigram Index Scan
    ↓ Index Lookup & Candidate Selection
Similarity Scoring & Ranking
    ↓ Result Filtering & Pagination
Ranked, Fuzzy Matched Results
    ↓ JSON Response to Client

```

🧭 Key Takeaways

- ✅ Always use % operator in WHERE to leverage trigram indexes
- ✅ Tune similarity thresholds for your use case
- ✅ Prefer GIN indexes for read-heavy systems
- ✅ Combine multiple strategies for robust matching
- ✅ Monitor index usage and query performance regularly
- ✅ Use transaction blocks for temporary threshold overrides
