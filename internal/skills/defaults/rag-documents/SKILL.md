---
name: rag-documents
description: Analyze uploaded documents (PDF, Word, Excel, PPT) using local RAG system at http://localhost:9000. Use when user uploads a file and asks to analyze, summarize, or extract information from it.
---

# RAG Document Analysis

Analyze uploaded documents using the local RAG system.

## When to Use

- User uploads a file and asks to analyze/summarize it
- User asks to extract data from Excel/PDF
- User asks questions about document content

## Supported Formats

- Excel (.xlsx, .xls)
- PDF (.pdf)
- Word (.docx)
- PowerPoint (.pptx)

## Prerequisites

### 1. Get API Token

First, get a token from the RAG system:

```bash
# Register a user (one-time)
curl -X POST "http://localhost:9000/v1/users/register" \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "email": "test@test.com", "password": "testpass123"}'

# Or login to get token
curl -X POST "http://localhost:9000/v1/users/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpass123"}'
```

Save the `access_token` from response.

### 2. Create Project

```bash
curl -X POST "http://localhost:9000/v1/projects" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "my-project", "description": "For document analysis"}'
```

Save the `id` from response as `PROJECT_ID`.

## Usage Steps

### 1. Upload Document

```bash
curl -X POST "http://localhost:9000/v1/documents/upload?project_id=$PROJECT_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/path/to/file.xlsx"
```

Returns: `{"document_id": "xxx", "status": "processing"}`

### 2. Wait for Processing

```bash
# Poll until status is "completed"
curl "http://localhost:9000/v1/documents/{document_id}"
```

Wait 2-3 seconds between polls. Status: `processing` → `completed`

### 3. Analyze with RAG

```bash
curl -X POST "http://localhost:9000/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-14b",
    "messages": [{"role": "user", "content": "Summarize this document"}],
    "project_id": "temp",
    "enable_rag": true,
    "top_k": 5
  }'
```

### 4. Cleanup (Optional)

```bash
curl -X DELETE "http://localhost:9000/v1/documents/{document_id}"
```

## Complete Example

```bash
# Upload file
RESPONSE=$(curl -s -X POST "http://localhost:9000/v1/documents/upload?project_id=temp" \
  -F "file=@uploaded_file.xlsx")
DOC_ID=$(echo $RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['document_id'])")

# Wait for processing
for i in {1..30}; do
  STATUS=$(curl -s "http://localhost:9000/v1/documents/$DOC_ID" | \
    python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
  [ "$STATUS" = "completed" ] && break
  sleep 2
done

# Query document
curl -s -X POST "http://localhost:9000/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"qwen2.5-14b\",
    \"messages\": [{\"role\": \"user\", \"content\": \"分析这份文档的主要内容\"}],
    \"project_id\": \"temp\",
    \"enable_rag\": true
  }"

# Delete document
curl -s -X DELETE "http://localhost:9000/v1/documents/$DOC_ID"
```

## Important Notes

1. **No authentication required** - Local service, no tokens needed
2. **Always use project_id**: "temp" for temporary analysis
3. **Wait for processing** - Documents need 5-30 seconds for OCR/chunking
4. **Check chunk_count** - If 0, the file may be empty or image-based
5. **Always enable_rag**: Set to `true` for document-based answers

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Connection refused | Check: `docker ps \| grep lihuo` |
| Processing timeout | Large files take longer, wait up to 60s |
| Empty response | Check if `chunk_count` > 0 |
| Parse error | File may be corrupted or password-protected |
