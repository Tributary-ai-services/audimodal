# Embeddings API

The Embeddings API provides vector operations for semantic search, similarity analysis, and AI-powered document intelligence.

## Base URL

```
/v1/tenants/{tenant_id}/embeddings
```

## Search Documents

Perform semantic search across document embeddings.

### Request

```http
POST /v1/tenants/{tenant_id}/embeddings/search
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "query": "contract terms and payment schedules",
  "options": {
    "top_k": 10,
    "threshold": 0.7,
    "include_content": true,
    "include_metadata": true,
    "filters": {
      "document_type": "contract",
      "classification": ["confidential", "restricted"],
      "created_after": "2024-01-01T00:00:00Z"
    },
    "boost": {
      "document_type:contract": 1.5,
      "tags:legal": 1.2
    }
  }
}
```

### Request Parameters

- `query` (string, required): Search query text
- `options` (object, optional): Search configuration
  - `top_k` (integer): Number of results (1-100, default: 10)
  - `threshold` (float): Similarity threshold (0.0-1.0, default: 0.0)
  - `include_content` (boolean): Include document content (default: true)
  - `include_metadata` (boolean): Include metadata (default: true)
  - `filters` (object): Metadata filters
  - `boost` (object): Score boosting for specific metadata
  - `rerank` (boolean): Apply reranking (default: false)
  - `hybrid_search` (boolean): Combine semantic and keyword search (default: false)

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "query": "contract terms and payment schedules",
  "results": [
    {
      "file_id": "file_abc123",
      "chunk_id": "chunk_def456",
      "score": 0.92,
      "rank": 1,
      "content": "PAYMENT TERMS\n\nPayment shall be made within 30 days of invoice date. The total contract value is $150,000...",
      "content_preview": "Payment shall be made within 30 days of invoice date...",
      "file_metadata": {
        "filename": "contract_acme.pdf",
        "document_type": "contract",
        "classification": "confidential",
        "tags": ["contract", "legal", "payment"],
        "custom_fields": {
          "client_name": "Acme Inc",
          "contract_value": 150000
        }
      },
      "chunk_metadata": {
        "chunk_index": 5,
        "page_numbers": [3, 4],
        "section": "payment_terms",
        "word_count": 245
      },
      "highlights": [
        {
          "field": "content",
          "matched_text": "payment schedules",
          "context": "...contract value and payment schedules are defined..."
        }
      ]
    }
  ],
  "pagination": {
    "total_results": 1,
    "returned_results": 1,
    "has_more": false
  },
  "search_metadata": {
    "query_time_ms": 45,
    "total_documents_searched": 1250,
    "embedding_model": "openai-text-embedding-ada-002",
    "search_type": "semantic"
  }
}
```

## Vector Search

Search using a pre-computed vector.

### Request

```http
POST /v1/tenants/{tenant_id}/embeddings/vector-search
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "vector": [0.123, -0.456, 0.789, ...],
  "options": {
    "top_k": 5,
    "threshold": 0.8,
    "include_content": true,
    "filters": {
      "file_type": "pdf"
    }
  }
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "results": [
    {
      "file_id": "file_abc123",
      "chunk_id": "chunk_def456",
      "score": 0.95,
      "distance": 0.05,
      "rank": 1,
      "content": "Similar content based on vector similarity...",
      "file_metadata": {
        "filename": "similar_document.pdf",
        "file_type": "pdf"
      }
    }
  ],
  "search_metadata": {
    "query_time_ms": 23,
    "vector_dimension": 1536,
    "distance_metric": "cosine"
  }
}
```

## Hybrid Search

Combine semantic and keyword search for better results.

### Request

```http
POST /v1/tenants/{tenant_id}/embeddings/hybrid-search
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "query": "contract payment terms",
  "options": {
    "top_k": 10,
    "semantic_weight": 0.7,
    "keyword_weight": 0.3,
    "fusion_method": "rrf",
    "include_content": true,
    "filters": {
      "document_type": "contract"
    }
  }
}
```

### Request Parameters

- `semantic_weight` (float): Weight for semantic search (0.0-1.0)
- `keyword_weight` (float): Weight for keyword search (0.0-1.0)
- `fusion_method` (string): Result fusion method (`rrf`, `weighted`, `combined`)

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "query": "contract payment terms",
  "results": [
    {
      "file_id": "file_abc123",
      "chunk_id": "chunk_def456",
      "combined_score": 0.89,
      "semantic_score": 0.92,
      "keyword_score": 0.83,
      "rank": 1,
      "content": "CONTRACT PAYMENT TERMS...",
      "match_explanation": {
        "semantic_matches": ["contract terms", "payment conditions"],
        "keyword_matches": ["contract", "payment", "terms"],
        "fusion_method": "rrf"
      }
    }
  ],
  "search_metadata": {
    "fusion_method": "rrf",
    "semantic_weight": 0.7,
    "keyword_weight": 0.3
  }
}
```

## Generate Embeddings

Generate embeddings for custom text.

### Request

```http
POST /v1/tenants/{tenant_id}/embeddings/generate
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "texts": [
    "This is the first text to embed",
    "This is the second text to embed"
  ],
  "model": "openai-text-embedding-ada-002",
  "normalize": true
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "embeddings": [
    {
      "text": "This is the first text to embed",
      "vector": [0.123, -0.456, 0.789, ...],
      "model": "openai-text-embedding-ada-002",
      "dimensions": 1536
    },
    {
      "text": "This is the second text to embed",
      "vector": [0.321, -0.654, 0.987, ...],
      "model": "openai-text-embedding-ada-002",
      "dimensions": 1536
    }
  ],
  "usage": {
    "total_tokens": 24,
    "model": "openai-text-embedding-ada-002"
  }
}
```

## Similarity Analysis

Compare documents or text for similarity.

### Request

```http
POST /v1/tenants/{tenant_id}/embeddings/similarity
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "source": {
    "type": "file",
    "file_id": "file_abc123"
  },
  "targets": [
    {
      "type": "file",
      "file_id": "file_def456"
    },
    {
      "type": "text",
      "content": "This is comparison text"
    }
  ],
  "options": {
    "aggregation": "max",
    "chunk_level": true
  }
}
```

### Request Parameters

- `source`: Source document or text
- `targets`: Array of target documents or texts
- `options`:
  - `aggregation` (string): How to aggregate chunk similarities (`max`, `mean`, `median`)
  - `chunk_level` (boolean): Return chunk-level similarities

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "source": {
    "type": "file",
    "file_id": "file_abc123",
    "filename": "document1.pdf"
  },
  "similarities": [
    {
      "target": {
        "type": "file",
        "file_id": "file_def456",
        "filename": "document2.pdf"
      },
      "similarity_score": 0.87,
      "distance": 0.13,
      "most_similar_chunks": [
        {
          "source_chunk_id": "chunk_123",
          "target_chunk_id": "chunk_456",
          "similarity": 0.95,
          "source_content": "Section about payment terms...",
          "target_content": "Similar payment terms section..."
        }
      ]
    },
    {
      "target": {
        "type": "text",
        "content": "This is comparison text"
      },
      "similarity_score": 0.34,
      "distance": 0.66
    }
  ],
  "metadata": {
    "aggregation_method": "max",
    "comparison_model": "openai-text-embedding-ada-002"
  }
}
```

## Get Document Clusters

Get document clusters based on semantic similarity.

### Request

```http
POST /v1/tenants/{tenant_id}/embeddings/cluster
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "filters": {
    "document_type": "contract",
    "created_after": "2024-01-01T00:00:00Z"
  },
  "options": {
    "num_clusters": 5,
    "algorithm": "kmeans",
    "min_cluster_size": 3,
    "include_outliers": false
  }
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "clusters": [
    {
      "cluster_id": 0,
      "label": "Employment Contracts",
      "size": 15,
      "centroid_terms": ["employment", "salary", "benefits", "termination"],
      "files": [
        {
          "file_id": "file_abc123",
          "filename": "employment_contract_smith.pdf",
          "distance_to_centroid": 0.12
        }
      ],
      "similarity_score": 0.85
    }
  ],
  "metadata": {
    "total_files": 67,
    "clustered_files": 62,
    "outliers": 5,
    "algorithm": "kmeans",
    "silhouette_score": 0.73
  }
}
```

## Manage Embeddings Datasets

### Create Dataset

```http
POST /v1/tenants/{tenant_id}/embeddings/datasets
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "name": "legal_documents",
  "description": "Legal documents embedding dataset",
  "dimensions": 1536,
  "metric_type": "cosine",
  "filters": {
    "document_type": ["contract", "agreement", "policy"]
  }
}
```

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "dataset_id": "dataset_123",
  "name": "legal_documents",
  "description": "Legal documents embedding dataset",
  "dimensions": 1536,
  "metric_type": "cosine",
  "vector_count": 0,
  "created_at": "2024-01-15T14:30:00Z",
  "status": "active"
}
```

### List Datasets

```http
GET /v1/tenants/{tenant_id}/embeddings/datasets
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "datasets": [
    {
      "dataset_id": "dataset_123",
      "name": "legal_documents",
      "description": "Legal documents embedding dataset",
      "dimensions": 1536,
      "vector_count": 1250,
      "last_updated": "2024-01-15T16:30:00Z",
      "status": "active"
    }
  ]
}
```

## Embedding Models

### Available Models

| Model | Provider | Dimensions | Max Tokens | Use Case |
|-------|----------|------------|------------|----------|
| `openai-text-embedding-ada-002` | OpenAI | 1536 | 8191 | General purpose |
| `openai-text-embedding-3-small` | OpenAI | 1536 | 8191 | Efficient, cost-effective |
| `openai-text-embedding-3-large` | OpenAI | 3072 | 8191 | High performance |
| `cohere-embed-english-v3.0` | Cohere | 1024 | 512 | English text |
| `cohere-embed-multilingual-v3.0` | Cohere | 1024 | 512 | Multilingual |

### Model Selection

```http
POST /v1/tenants/{tenant_id}/embeddings/search
Content-Type: application/json

{
  "query": "search query",
  "model": "openai-text-embedding-3-large",
  "options": {
    "top_k": 10
  }
}
```

## Advanced Filtering

### Metadata Filters

```json
{
  "filters": {
    "document_type": "contract",
    "classification": ["confidential", "restricted"],
    "tags": {
      "$in": ["legal", "compliance"]
    },
    "created_after": "2024-01-01T00:00:00Z",
    "custom_fields.contract_value": {
      "$gte": 100000
    },
    "$and": [
      {"department": "legal"},
      {"status": "active"}
    ]
  }
}
```

### Filter Operators

- `$eq`: Equal to
- `$ne`: Not equal to
- `$gt`: Greater than
- `$gte`: Greater than or equal
- `$lt`: Less than
- `$lte`: Less than or equal
- `$in`: In array
- `$nin`: Not in array
- `$and`: Logical AND
- `$or`: Logical OR
- `$not`: Logical NOT

## Performance Optimization

### Search Optimization

```http
POST /v1/tenants/{tenant_id}/embeddings/search
Content-Type: application/json

{
  "query": "search query",
  "options": {
    "top_k": 10,
    "ef_search": 100,
    "approximate": true,
    "cache_results": true,
    "timeout_ms": 5000
  }
}
```

### Batch Operations

```http
POST /v1/tenants/{tenant_id}/embeddings/batch-search
Content-Type: application/json

{
  "queries": [
    {
      "id": "query_1",
      "query": "first search query",
      "options": {"top_k": 5}
    },
    {
      "id": "query_2", 
      "query": "second search query",
      "options": {"top_k": 3}
    }
  ]
}
```

## Error Responses

### Vector Dimension Mismatch

```http
HTTP/1.1 400 Bad Request

{
  "error": {
    "code": "VECTOR_DIMENSION_MISMATCH",
    "message": "Vector dimension mismatch",
    "expected_dimensions": 1536,
    "provided_dimensions": 512
  }
}
```

### Model Not Available

```http
HTTP/1.1 400 Bad Request

{
  "error": {
    "code": "MODEL_NOT_AVAILABLE",
    "message": "Embedding model not available",
    "model": "invalid-model",
    "available_models": [
      "openai-text-embedding-ada-002",
      "openai-text-embedding-3-small",
      "openai-text-embedding-3-large"
    ]
  }
}
```

### Search Timeout

```http
HTTP/1.1 408 Request Timeout

{
  "error": {
    "code": "SEARCH_TIMEOUT",
    "message": "Search request timed out",
    "timeout_ms": 5000,
    "suggestion": "Try reducing top_k or using approximate search"
  }
}
```