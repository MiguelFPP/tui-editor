# tui-fer-editor

Base de editor TUI en Go. Es el esqueleto del proyecto: ventana en pantalla alternativa, panel de editor y barra de estado. Todavía no implementa funciones de edición.

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
tui-fer-editor/
├── cmd/
│   └── tui-fer-editor/
│       └── main.go
├── internal/
│   └── ui/
│       ├── model.go
│       └── model_test.go
├── .air.toml
├── .dockerignore
├── .gitignore
├── Dockerfile
├── Makefile
├── compose.yaml
├── go.mod
├── go.sum
└── README.md
```

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
| `make test` | Ejecuta `go test ./...` |
| `make vet` | Ejecuta `go vet ./...` |
| `make fmt` | Ejecuta `go fmt ./...` |
| `make tidy` | Ejecuta `go mod tidy` |
| `make build` | Construye las imágenes Docker |
| `make clean` | Borra los artefactos de Air (`tmp`, `bin`) |

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

Este repositorio es solo el esqueleto: no abre archivos, no edita texto y no guarda cambios. Cualquier funcionalidad de editor hay que añadirla encima de este punto de partida.
