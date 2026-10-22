# CLAUDE.md

Backend de una app Android de finanzas personales para dos usuarios (Luis y su esposa).
Stack: Go + Fiber + Postgres (pgx). Arquitectura hexagonal. El cliente móvil vive en `home-wallet-android` (Expo + TypeScript) y consume esta API.

## Estructura

- `internal/domain`: entidades y reglas de negocio puras (sin I/O).
- `internal/application`: casos de uso / servicios.
- `internal/ports`: interfaces (repositorios, clientes externos).
- `internal/adapters/http`: handlers Fiber, middleware, DTOs.
- `internal/adapters/postgres`: implementación de repositorios con pgx.
- `cmd/api`: main y wiring.
- `migrations`: SQL con golang-migrate.
- `openapi.yaml`: contrato de la API.

## Reglas

- **`openapi.yaml` es la fuente de verdad.** Si un endpoint cambia, se actualiza la spec en el mismo cambio. El repo mobile genera sus tipos desde ese archivo.
- **Dinero:** `NUMERIC(14,2)` en DB y string decimal en la API. Nunca float (usar `shopspring/decimal`).
- **Monedas:** `ARS` y `USD`. Todo movimiento guarda moneda original, `fx_rate` usado y `amount_ars`. En ARS, `fx_rate = 1`.
- **Cotización:** dólar oficial, valor de **venta**, de dolarapi.com, una vez por día.
- **Zona horaria:** `America/Argentina/Mendoza` para definir "día" y "mes".
- **Usuarios:** solo dos, sin registro. Se crean con `sp_create_user`.
- **Auditoría:** toda entidad que se carga guarda `created_by` (user id) y `created_at`.
- **Tarjetas:** las compras con tarjeta NO afectan el indicador diario; impactan cuando vence el resumen.
- Errores en JSON con schema `{code, message}`.

## Comandos

- Validar spec: `npm run lint:api` (redocly).
- Tests: `go test ./...`
