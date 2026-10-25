# home-wallet-api

Backend (Go + Fiber + Postgres) de la app de finanzas del hogar. Cliente: [home-wallet-android](https://github.com/luismoralesarg/home-wallet-android).

Ver `CLAUDE.md` para arquitectura y reglas, y `openapi.yaml` para el contrato de la API.

## Validar la spec

```sh
npm install
npm run lint:api
```

## Base de datos

Migraciones en `migrations/` (golang-migrate):

```sh
migrate -path migrations -database "$DATABASE_URL" up
psql "$DATABASE_URL" -f scripts/seed_users.sql   # después de poner las passwords reales
```

## Correr en desarrollo

```sh
docker compose up --build        # db + migraciones + api en :8080
docker compose exec db psql -U wallet -c "call sp_create_user('luis', '...')"   # crear usuarios
go test ./...
# tests de integración de repositorios (necesitan DB migrada):
TEST_DATABASE_URL=postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable go test ./internal/adapters/postgres
```

Variables de entorno: ver `.env.example` (`DATABASE_URL`, `JWT_SECRET`, `PORT`, `JWT_TTL`, `DOLARAPI_URL` opcional).
