# tui-editor

TUI multiplataforma en Go que unifica el flujo diario de desarrollo: exploración y edición rápida de archivos, control completo de Git y monitorización de contenedores Docker.

Este repositorio está en construcción: la TUI ya arranca (pantalla alternativa, panel de editor y barra de estado) y el módulo, el binario y el tooling por fases son definitivos. Las features de producto llegan en las fases siguientes.

## Stack

| Componente | Versión | Import |
| --- | --- | --- |
| Go | 1.27.4 | — |
| Bubble Tea | v2.1.2 | `charm.land/bubbletea/v2` |
| Lip Gloss | v2.1.1 | `charm.land/lipgloss/v2` |
| Air (hot reload) | v1.67.4 | — |
| Alpine (imagen de runtime) | 3.24.4 | — |

Probado con Docker 29.9.3 y Docker Compose v5.7.0.

Las dependencias usan los paths canónicos v2 (`charm.land/...`), no los antiguos `github.com/charmbracelet/...`.

## Estructura del proyecto

```
tui-editor/
├── cmd/
│   └── tui-editor/
│       └── main.go
├── internal/
│   ├── adapters/
│   │   ├── docker/
│   │   │   └── doc.go
│   │   ├── fs/
│   │   │   └── doc.go
│   │   └── git/
│   │       └── doc.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── domain/
│   │   └── doc.go
│   ├── ports/
│   │   └── doc.go
│   ├── testutil/
│   │   └── fakes/
│   │       └── doc.go
│   └── tui/
│       ├── app/
│       │   ├── model.go
│       │   └── model_test.go
│       └── components/
│           └── doc.go
├── docs/
│   └── adr/
│       └── 0001-arquitectura-hexagonal-mvu.md
├── .air.toml
├── .dockerignore
├── .gitignore
├── Dockerfile
├── LICENSE
├── Makefile
├── README.md
├── compose.yaml
├── go.mod
└── go.sum
```

Los paquetes `domain`, `ports`, `adapters/*`, `tui/components` y `testutil/fakes` son esqueletos vacíos: solo fijan el boundary de cada capa (ver `docs/adr/0001-arquitectura-hexagonal-mvu.md`).

## Requisitos

- Docker
- Docker Compose v2 o superior

No hace falta Go ni Node en el host: todo se ejecuta dentro de contenedores.

## Comandos

| Comando | Descripción |
| --- | --- |
| `make help` | Lista los targets disponibles (target por defecto) |
| `make dev` | Ejecuta el TUI con hot reload |
| `make run` | Ejecuta el TUI desde la imagen de producción |
| `make build` | Construye las imágenes Docker (dev y runtime) |
| `make fmt` | Ejecuta `go fmt ./...` |
| `make tidy` | Ejecuta `go mod tidy` |
| `make clean` | Borra los artefactos de Air (`tmp`, `bin`) |
| `make test` | Ejecuta `go test ./...`: solo CI y fase final |
| `make vet` | Ejecuta `go vet ./...`: solo CI y fase final |

### Targets por fase

Cada fase ejecuta solo los paquetes que toca. La suite completa está reservada al CI y a la fase final.

| Comando | Descripción |
| --- | --- |
| `make test-pkg PKG='./internal/<pkg>/...'` | Tests de un paquete |
| `make test-run PKG='./internal/<pkg>/...' RUN='TestNombre'` | Tests que casan con `RUN` |
| `make vet-pkg PKG='./internal/<pkg>/...'` | `go vet` de un paquete |
| `make test-integration PKG='./internal/<pkg>/...' [RUN='TestIntegration...']` | Tests con build tag `integration` |
| `make bench-pkg PKG='./internal/<pkg>/...'` | Benchmarks de un paquete |
| `make test-race` | Suite completa con el detector de carreras: solo fase final |

`PKG` y `RUN` son obligatorios donde aplica: si faltan, el target falla con un mensaje claro.

## Flags

| Flag | Default | Descripción |
| --- | --- | --- |
| `--repo DIR` | directorio actual | Raíz del repositorio. Debe existir y ser un directorio; se guarda en ruta absoluta |
| `--editor CMD` | `$EDITOR` o `vim` | Comando del editor externo |
| `--docker-host URL` | `$DOCKER_HOST` | Endpoint del daemon Docker. Solo se aceptan los esquemas `unix`, `tcp` y `npipe`; el socket no se valida al arrancar |
| `--debug-log FILE` | vacío | Archivo de log de depuración |

`--help` muestra el uso. Los errores de validación se imprimen en `stderr` y el proceso termina con código 1.

## La aplicación es interactiva

El TUI necesita un TTY real. Usa `docker compose run` (o `make dev` / `make run`) y no `docker compose up -d`: sin terminal interactiva la interfaz no se puede dibujar y el contenedor termina enseguida.

Para salir del programa, pulsa `q` o `ctrl+c`.

## Cómo añadir dependencias

1. Añade la dependencia con su versión exacta en `go.mod`.
2. Ejecuta `make tidy` para generar/actualizar `go.sum` y los `require` indirectos.
3. Ejecuta `make build` para reconstruir las imágenes.

Todo el trabajo de módulos ocurre dentro del contenedor. Nunca instales Go ni gestores de paquetes en el host.

`go.sum` ya está generado y debe versionarse junto con `go.mod`. Si añades dependencias, `make tidy` lo actualiza.

## Alcance

Además del arranque de la TUI y de la validación de flags, el repo solo contiene boundaries de paquete: no abre archivos, no edita texto, no guarda cambios y todavía no habla con Git ni con Docker.

## Licencia

MIT. Ver `LICENSE`.
