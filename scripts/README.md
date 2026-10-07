# Scripts

Install Go 1.22+ from https://go.dev/dl/ before running the API.

```bash
docker compose up -d
cd backend && go mod tidy && go run ./cmd/api
cd frontend && npm install && npm run dev
```
