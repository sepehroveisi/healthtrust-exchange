# Contributing

HealthTrust Exchange is an engineering prototype. Contributions should remain focused, testable, and honest about healthcare and security limitations.

## Local setup

1. Install Docker Desktop.
2. Run `docker compose up --build`.
3. Open `http://localhost:3000`.

Use `docker compose down --volumes --remove-orphans` only when you intend to erase disposable local demo data.

## Checks

Backend:

```sh
gofmt -w cmd internal
go vet ./...
go test ./...
go test -race ./...
```

Frontend, from `web/` with Node.js 22 or newer:

```sh
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm run e2e
```

## Pull requests

- Keep changes limited to one coherent concern.
- Add or update tests for behavior changes.
- Never commit secrets, private keys, local databases, or real clinical information.
- Run formatting and relevant verification before opening a pull request.
- Explain security and data-model tradeoffs without claiming production or regulatory readiness.
