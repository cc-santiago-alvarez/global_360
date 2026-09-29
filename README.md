# Global 360 API

Backend de **Global 360**, una plataforma B2B de logística internacional que conecta empresas clientes con proveedores (distribuidores). El primer corredor es Colombia ↔ Costa Rica.

Esta API cubre dos fases:

| Fase | Qué incluye |
|---|---|
| **1. Foundation** | Países y monedas, personas, empresas, usuarios, login con JWT y refresh token, roles y permisos por empresa (RBAC) y auditoría de cada cambio |
| **2. Comercio (marketplace B2B)** | Perfiles públicos de distribuidores y sus servicios, categorías, búsqueda, revisión y publicación por Global 360, y solicitudes de contacto (leads) |

Está hecha con **Go + Gin** sobre **MongoDB**, en arquitectura hexagonal.

---

## Cómo arrancar el proyecto

### Paso 1: instala los requisitos

| Herramienta | Versión | Para qué |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.22 o superior | Compilar y ejecutar la API |
| [Git](https://git-scm.com/) | cualquiera | Descargar el repositorio |
| MongoDB | 4.4 o superior, **idealmente un replica set** | Base de datos |
| `curl` y `jq` | cualquiera | Solo para el script de pruebas de humo (opcional) |

Comprueba que Go está instalado:

```bash
go version   # go version go1.22.x (o superior)
```

> Si la terminal responde `go: command not found`, Go no está en el `PATH`. Con la instalación oficial, añade `export PATH=$PATH:/usr/local/go/bin` a tu `~/.bashrc` o `~/.zshrc` y abre una terminal nueva.

### Paso 2: ten un MongoDB disponible

Si ya tienes un MongoDB, anota su cadena de conexión y pasa al paso 3.

Si no, la forma más rápida es levantarlo con Docker como replica set de un solo nodo:

```bash
docker run -d --name global360-mongo -p 27017:27017 mongo:7 --replSet rs0
docker exec global360-mongo mongosh --quiet --eval 'rs.initiate()'
```

La cadena de conexión será `mongodb://localhost:27017/?directConnection=true`.

> **¿Por qué un replica set?** Cada cambio se guarda junto con su registro de auditoría en una transacción, y MongoDB solo admite transacciones en un replica set. Con un servidor standalone la API funciona igual, pero sin transacciones, y al arrancar lo avisa en el log.

### Paso 3: descarga el repositorio

```bash
git clone https://github.com/cc-santiago-alvarez/global_360.git
cd global_360
go mod download   # descarga las dependencias de Go
```

### Paso 4: crea tu archivo `.env`

```bash
cp .env.example .env
```

Abre `.env` y completa los valores obligatorios:

| Variable | Qué poner | Ejemplo |
|---|---|---|
| `MONGO_URI` | Tu cadena de conexión, **entre comillas** (contiene `&`) | `"mongodb://localhost:27017/?directConnection=true"` |
| `JWT_SECRET` | Un secreto aleatorio de 32 caracteres o más | La salida de `openssl rand -base64 48` |
| `SEED_SUPERADMIN_EMAIL` | Email del primer administrador | `admin@miempresa.com` |
| `SEED_SUPERADMIN_PASSWORD` | Su contraseña, de 10 a 72 caracteres | `Una-Clave-Larga-2026` |

El resto de variables tiene valores por defecto que sirven para desarrollo (ver [Configuración](#configuración-env)).

### Paso 5: arranca la API

```bash
go run ./cmd/api
```

Si todo va bien, el log (en JSON) termina con `"msg":"server listening","addr":":8080"`.

En el primer arranque, la API prepara la base de datos automáticamente:
1. Crea las colecciones con sus validadores e índices.
2. Carga los datos iniciales: países CO y CR; monedas COP, CRC y USD; los permisos; los 8 roles del sistema y 8 categorías de logística.
3. Crea el superadministrador del paso 4.
4. Aplica las migraciones pendientes.

Todo esto es idempotente, así que se puede arrancar las veces que haga falta.

### Paso 6: comprueba que funciona

En otra terminal:

```bash
curl localhost:8080/health   # {"status":"ok"}
curl localhost:8080/ready    # {"database":"up","status":"ok"}  ← confirma la conexión a MongoDB
```

Luego inicia sesión con el superadmin (ver [Primeros pasos con la API](#primeros-pasos-con-la-api)).

### Uso diario

| Tarea | Comando |
|---|---|
| Arrancar | `go run ./cmd/api` |
| Detener | `Ctrl+C` (espera a que terminen las peticiones en curso) |
| Aplicar cambios de código | Detén la API y vuelve a arrancarla: `go run` no recarga solo |
| Usar otro puerto o base de datos | `PORT=9090 MONGO_DATABASE=pruebas go run ./cmd/api` |
| Compilar un binario | `go build -o global_360 ./cmd/api && ./global_360` |
| Ejecutar las pruebas | `go test ./...` (ver [Pruebas](#pruebas)) |

> **Frontend en desarrollo:** la API no tiene CORS habilitado. El frontend debe llamarla a través de un proxy de su servidor de desarrollo (por ejemplo, el `proxy` de Vite apuntando a `http://localhost:8080`).

---

## Primeros pasos con la API

Todas las rutas están bajo `/api/v1`, usan JSON en `snake_case` y se autentican con `Authorization: Bearer <access_token>`.

```bash
# 1. Login con el superadmin del .env
curl -s localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"superadmin@tu-dominio.com","password":"tu-contraseña"}'
# -> {"token_type":"Bearer","access_token":"...","refresh_token":"...", ...}

# 2. Usar el access token (dura 15 minutos)
TOKEN=...
curl -s localhost:8080/api/v1/auth/me -H "Authorization: Bearer $TOKEN"

# 3. Renovarlo con el refresh token (dura 7 días; cada uso emite uno nuevo)
curl -s localhost:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' -d '{"refresh_token":"..."}'

# 4. El comercio es público: no necesita token
curl -s 'localhost:8080/api/v1/commerce/distributors?q=aduanas'
```

**Errores.** Todos usan el mismo formato:

```json
{"error": {"code": "validation_error", "message": "legal_name is required", "request_id": "..."}}
```

| HTTP | `code` | Cuándo |
|---|---|---|
| 400 | `validation_error` | Datos inválidos o una transición de estado no permitida |
| 401 | `unauthorized` | Sin token, token inválido o vencido, o credenciales incorrectas |
| 403 | `forbidden` | Falta el permiso necesario, o la cuenta está bloqueada o inactiva |
| 404 | `not_found` | El recurso o la ruta no existen |
| 405 | `method_not_allowed` | La ruta existe pero no admite ese método HTTP |
| 409 | `conflict` | Un duplicado (email, documento, código…) |

Las listas se paginan con `?page=1&page_size=20` (máximo 100) y devuelven `{"items":[...],"total":N,"page":1,"page_size":20}`.

---

## Endpoints

### Fase 1: Foundation

| Recurso | Endpoints | Permiso |
|---|---|---|
| Salud | `GET /health`, `GET /ready`, `GET /api/v1/ping` | Público |
| Autenticación | `POST /auth/login`, `POST /auth/refresh` | Público |
| | `POST /auth/logout`, `GET /auth/me` | Autenticado |
| Países y monedas | `GET /countries`, `GET /currencies` | `catalog.read` |
| | `POST` y `PATCH /countries[/:id]`, `POST` y `PATCH /currencies[/:id]` | `catalog.manage` |
| Empresas | `GET /companies`, `GET /companies/:id` | `company.read` |
| | `POST /companies`, `PATCH /companies/:id`, `PATCH /companies/:id/status` | `company.manage` |
| Usuarios | `GET /users`, `GET /users/:id`, `GET /users/:id/roles` | `user.read` (o uno mismo) |
| | `POST /users`, `PATCH /users/:id/status`, `POST /users/:id/roles`, `DELETE /users/:id/roles/:assignmentId` | `user.manage` |
| | `PATCH /users/:id`, `PUT /users/:id/password` | `user.manage` (o uno mismo) |
| Roles | `GET /permissions`, `GET /roles`, `GET /roles/:id` | `role.read` |
| | `POST /roles`, `PATCH /roles/:id`, `PUT /roles/:id/permissions` | `role.manage` |
| Auditoría | `GET /audit-logs?entity=&entity_id=&user_id=&action=&from=&to=` | `audit.read` |

### Fase 2: Comercio (`/api/v1/commerce`)

| Quién | Endpoints |
|---|---|
| **Público** (el token es opcional: si se envía, se muestran los datos de contacto) | `GET /categories`, `GET /distributors?q=&category_id=&country_id=`, `GET /distributors/:companyId` |
| **Clientes** (`commerce.contact`) | `POST /distributors/:companyId/contact-requests`, `GET /contact-requests` (las solicitudes enviadas) |
| **Distribuidores** (`distributor.manage`) | `GET` y `PUT /distributors/:companyId/profile`, `POST /distributors/:companyId/profile/submit`, `GET` y `POST /distributors/:companyId/services`, `PATCH /distributors/:companyId/services/:serviceId` |
| **Distribuidores** (`lead.read` y `lead.manage`) | `GET /distributors/:companyId/contact-requests`, `PATCH /distributors/:companyId/contact-requests/:requestId` |
| **Global 360** (`commerce.manage`) | `GET /profiles?status=pending_review`, `POST /distributors/:companyId/profile/{approve,reject,suspend,reinstate}`, `POST /categories`, `PATCH /categories/:id` |

**Ciclo de vida de un distribuidor:**

1. El administrador del proveedor crea su perfil con `PUT /profile`, y el perfil queda como borrador.
2. Añade sus servicios y lo envía a revisión con `POST /profile/submit`.
3. Global 360 lo aprueba o lo rechaza con un motivo. Si lo rechaza, el proveedor corrige y lo reenvía.
4. Una vez publicado, aparece en el comercio.

```
draft ──submit──▶ pending_review ──approve──▶ published ──suspend──▶ suspended
  ▲                    │                          ▲                     │
  └──── rejected ◀─reject                         └─────reinstate───────┘
```

- Solo pueden tener perfil las empresas de tipo `provider` o `both`.
- Si la empresa deja de estar activa, su perfil sale del comercio automáticamente.
- Un cliente solo puede tener una solicitud sin atender (`new`) por distribuidor. El distribuidor la pasa a `contacted` y luego a `closed`.

---

## Roles y permisos

Los permisos se consultan en la base de datos en cada petición, no van dentro del JWT. Por eso un cambio de rol surte efecto de inmediato.

Un rol **global** aplica a toda la plataforma. Un rol **de empresa** solo aplica a la empresa en la que se asignó, y un usuario puede tener roles en varias empresas.

| Rol | Alcance | Resumen |
|---|---|---|
| `superadmin` | global | Todo |
| `global360_admin` | global | Todo excepto gestionar roles |
| `operations` | global | Empresas, lectura general, auditoría y revisión del comercio |
| `finance` | global | Lectura y auditoría |
| `client_admin` | empresa | Usuarios de su empresa y contacto con distribuidores |
| `client_operator` | empresa | Consulta y contacto con distribuidores |
| `provider_admin` | empresa | Usuarios de su empresa, su perfil de distribuidor, sus servicios y sus solicitudes recibidas |
| `provider_operator` | empresa | Atender las solicitudes recibidas |

Reglas importantes:
- Un administrador de empresa solo asigna roles de su empresa y solo con permisos que él mismo tenga.
- Nadie puede cambiar su propio estado ni quitarse sus propios roles.
- Tras 5 intentos fallidos de login (`MAX_FAILED_LOGIN_ATTEMPTS`) la cuenta queda bloqueada.
- Cambiar la contraseña o desactivar un usuario cierra todas sus sesiones.

---

## Configuración (`.env`)

| Variable | Por defecto | Descripción |
|---|---|---|
| `PORT` | `8080` | Puerto HTTP |
| `GIN_MODE` | `release` | `debug` o `release` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` o `error`. Los logs se escriben en JSON |
| `MONGO_URI` | — | **Obligatoria** |
| `MONGO_DATABASE` | `global360` | Nombre de la base de datos |
| `JWT_SECRET` | — | **Obligatoria**, de al menos 32 caracteres |
| `JWT_ISSUER` | `global360` | Emisor del JWT |
| `JWT_ACCESS_TTL` | `15m` | Vigencia del access token |
| `REFRESH_TOKEN_TTL` | `168h` | Vigencia del refresh token. Debe ser mayor que `JWT_ACCESS_TTL` |
| `BCRYPT_COST` | `12` | Entre 10 y 15 |
| `MAX_FAILED_LOGIN_ATTEMPTS` | `5` | Intentos fallidos antes de bloquear la cuenta |
| `SEED_SUPERADMIN_EMAIL` y `SEED_SUPERADMIN_PASSWORD` | — | Si no se definen, no se crea el superadmin (y queda un aviso en el log) |

Las variables del entorno del sistema tienen prioridad sobre `.env`. Por ejemplo, `PORT=9090 MONGO_DATABASE=pruebas go run ./cmd/api`.

`.env` está en `.gitignore`: **nunca subas credenciales reales**.

---

## Pruebas

```bash
# Unitarias y HTTP (no necesitan MongoDB)
go test ./...

# Integración contra un MongoDB real: usa MONGO_URI del .env y crea una base
# temporal global360_test_<n> que se borra al terminar
go test -tags integration ./internal/infrastructure/persistence/mongodb/...

# Humo: recorre todos los endpoints de ambas fases contra una API en marcha
# (más de 360 verificaciones). Crea datos, así que usa una base desechable:
MONGO_DATABASE=global360_smoke PORT=18081 go run ./cmd/api   # en otra terminal
BASE_URL=http://localhost:18081 scripts/smoke_test.sh
```

---

## Estructura del proyecto

Arquitectura hexagonal organizada por capas. El dominio no conoce ni Gin ni MongoDB.

```
cmd/api/main.go                  Punto de entrada: cablea config → Mongo → repositorios → servicios → HTTP
internal/
  domain/                        Entidades, reglas de negocio y máquinas de estado (Go puro)
    shared/ catalog/ identity/ company/ access/ audit/ commerce/
  application/
    port/                        Interfaces que la aplicación necesita (repositorios, hashing, tokens…)
    service/                     Casos de uso: autorización, transacciones y auditoría
    dto/                         Entradas y salidas de los casos de uso
    actor/                       Quién hace la petición (usuario, permisos, IP)
  infrastructure/
    config/ logger/ security/    Configuración, logs en JSON, bcrypt y JWT
    persistence/mongodb/         Repositorios, esquema (validadores e índices), seed y migraciones
    httpapi/                     Router Gin, middleware y handlers
  testutil/                      Implementaciones en memoria y fixtures para los tests
scripts/smoke_test.sh            Pruebas de humo de extremo a extremo
```

Para añadir un módulo nuevo, sigue el mismo recorrido:
1. Entidad en `domain/`.
2. Interfaz en `application/port/`.
3. Caso de uso en `application/service/`.
4. Repositorio y esquema en `persistence/mongodb/`.
5. Handler y ruta en `httpapi/`.
6. Cablearlo en `cmd/api/main.go`.

Todo en inglés: código, colecciones, campos y valores de enum.

**Cosas a tener en cuenta sobre la base de datos:**
- **No hay borrado físico:** los registros se desactivan o cambian de estado. La única excepción son los refresh tokens, que se purgan 30 días después de vencer.
- **Validadores `$jsonSchema`:** cada colección tiene uno, así que MongoDB rechaza documentos que no cumplan el esquema aunque se inserten a mano.
- **Migraciones:** los cambios de datos para bases existentes se registran en `schema_migrations` y se aplican una sola vez al arrancar.

---

## Solución de problemas

| Síntoma | Causa y solución |
|---|---|
| `invalid configuration: JWT_SECRET is required…` | Falta `JWT_SECRET` o tiene menos de 32 caracteres |
| En el log aparece `mongodb is a standalone server` | MongoDB no es un replica set. Funciona, pero sin transacciones (ver [Requisitos](#requisitos)) |
| `404 route not found` en rutas que sí existen | Una API vieja sigue corriendo en ese puerto. Detenla y vuelve a arrancar |
| `source .env` deja `MONGO_URI` vacía en bash | El `&` de la URI sin comillas se interpreta como un comando. Pon el valor entre comillas. La API lee `.env` por su cuenta, así que no hace falta cargarlo con `source` |
| VS Code marca `undefined: ...` pero `go build` compila | La caché del analizador está desactualizada: ejecuta **Go: Restart Language Server** |
| No puedo iniciar sesión con el superadmin | Solo se crea si ese email no existe todavía. Revisa `SEED_SUPERADMIN_*` y el log de arranque |
