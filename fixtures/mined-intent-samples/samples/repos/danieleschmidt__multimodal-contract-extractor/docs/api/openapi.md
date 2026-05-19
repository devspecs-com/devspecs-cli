# OpenAPI Specification

Interactive REST API documentation with OpenAPI/Swagger.

## API Specification

<!-- swagger-ui config="swagger_config.json" -->

<!-- stripped fenced code block: yaml -->

<!-- /swagger-ui -->

## Try It Out

You can test the API endpoints directly from this documentation using the interactive Swagger UI above.

### Authentication

For production deployments, API endpoints may require authentication:

<!-- stripped fenced code block: bash -->

### Rate Limiting

The API implements rate limiting to ensure fair usage:

- **Development**: 100 requests per minute
- **Production**: 1000 requests per minute

Rate limit headers are included in responses:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
```

### Error Codes

| Code | Description |
|------|-------------|
| 400  | Bad Request - Invalid parameters |
| 401  | Unauthorized - Missing or invalid authentication |
| 413  | Payload Too Large - File exceeds size limit |
| 422  | Unprocessable Entity - Unsupported file format |
| 429  | Too Many Requests - Rate limit exceeded |
| 500  | Internal Server Error - Processing failed |

## SDK Examples

### Python

<!-- stripped fenced code block: python -->

### JavaScript

<!-- stripped fenced code block: javascript -->

### cURL

<!-- stripped fenced code block: bash -->
