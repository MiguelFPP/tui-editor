# tui-editor

TUI multiplataforma en Go que unifica el flujo diario de desarrollo: exploración y edición rápida de archivos, control completo de Git y monitorización de contenedores Docker.

Estado actual: **v0.1.0 (MVP de lectura)**. La TUI arranca con pantalla alternativa y ratón, une el árbol de archivos con el preview y añade un panel de Git de solo lectura, todo en pestañas. La edición y el panel Docker llegan en fases posteriores.

## Stack

| Componente | Versión | Import |
| --- | --- | --- |
| Go | 1.27.4 | — |
| Bubble Tea | v2.1.2 | `charm.land/bubbletea/v2` |
| Lip Gloss | v2.1.1 | `charm.land/lipgloss/v2` |
| Bubbles | v2.2.1 | `charm.land/bubbles/v2` |
| Chroma | v2.27.0 | `github.com/alecthomas/chroma/v2` |
| Air (hot reload) | v1.67.4 | — |
| Alpine (imagen de runtime) | 3.24.4 | — |

Probado con Docker 29.9.3 y Docker Compose v5.7.0.

Las dependencias usan los paths canónicos v2 (`charm.land/...`), no los antiguos `github.com/charmbracelet/...`.

## Estructura del proyecto

```
tui-editor/
├── cmd/tui-editor/        # composition root
├── internal/
│   ├── adapters/          # fs y git (docker en F12)
│   ├── config/            # flags y validación
│   ├── domain/            # tipos, límites y errores puros
│   ├── ports/             # interfaces de servicios
│   ├── testutil/fakes/    # dobles de test
│   └── tui/
│       ├── app/           # modelo raíz, pestañas, layout y ratón
│       ├── common/        # piezas compartidas (diff)
│       └── components/    # filetree, editor y gitpanel
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

`domain` y `ports` son puros; `adapters` y `tui` implementan la funcionalidad por fases. La documentación de arquitectura se mantiene fuera del repositorio.

## Requisitos

- Docker
- Docker Compose v2 o superior

No hace falta Go ni Node en el host: todo se ejecuta dentro de contenedores.

## Quickstart

```bash
make dev      # TUI con hot reload (requiere TTY)
make run      # TUI desde la imagen de producción (requiere TTY)
```

El TUI necesita un TTY real: usa `docker compose run` (o los targets de arriba) y no `docker compose up -d`. Sin terminal interactiva no se puede dibujar la interfaz.

## Pestañas y atajos

| Atajo | Acción |
| --- | --- |
| `1` / `2` | Cambiar de pestaña (`Files`, `Git`) |
| `tab` / `shift+tab` | Alternar el foco en `Files`; cambiar de sección en `Git` |
| `?` | Mostrar u ocultar la ayuda |
| `esc` | Cerrar la ayuda o reenviar la tecla al panel |
| `q` / `ctrl+c` | Salir |

### Files: árbol y preview

| Atajo | Acción |
| --- | --- |
| `↑`/`↓`, `j`/`k` | Mover el cursor por el árbol |
| `→` / `l`, `←` / `h` | Expandir o colapsar un directorio |
| `enter` / `espacio` | Alternar el directorio o abrir el archivo |
| `/` | Filtrar por nombre o ruta |
| `esc` | Limpiar el filtro |

El clic izquierdo selecciona y el doble clic expande o abre. La rueda desplaza el panel bajo el cursor. El preview usa resaltado de sintaxis con Chroma, números de línea, scroll y detección de binarios y de tamaño.

### Git (solo lectura)

| Atajo | Acción |
| --- | --- |
| `tab` / `shift+tab` | Cambiar de sección (`Cambios`, `Diff`, `Ramas`, `Historial`) |
| `↑`/`↓`, `j`/`k` | Mover el cursor |
| `enter` | Cargar el diff del archivo seleccionado |
| `d` | Alternar entre `worktree` y `staged` |
| `r` | Refrescar estado, ramas, historial y diff |
| `esc` | Cerrar la ayuda o volver a `Cambios` |

Todo el panel Git es de solo lectura: no prepara, no confirma y no toca el índice.

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
| `--version` | — | Imprime `tui-editor 0.1.0` y sale con código 0 |
| `--repo DIR` | directorio actual | Raíz del repositorio. Debe existir y ser un directorio; se guarda en ruta absoluta |
| `--editor CMD` | `$EDITOR` o `vim` | Comando del editor externo |
| `--docker-host URL` | `$DOCKER_HOST` | Endpoint del daemon Docker. Solo se aceptan los esquemas `unix`, `tcp` y `npipe`; el socket no se valida al arrancar |
| `--debug-log FILE` | vacío | Archivo de log de depuración |

`--help` muestra el uso. Los errores de validación se imprimen en `stderr` y el proceso termina con código 1. `--version` no valida el resto de flags.

```bash
docker compose run --rm --no-deps -T dev go run ./cmd/tui-editor --version
```

## Alcance y limitaciones

- Solo lectura: no se edita ni se guardan archivos; el preview es de consulta.
- Git de solo lectura: `status`, `diff`, ramas e historial; sin staging, commit ni merge.
- Sin panel Docker: llega en una fase posterior y no se monta el socket por defecto.
- Previews y listados acotados por tamaño y número de líneas; los binarios se detectan y no se renderizan.

## Cómo añadir dependencias

1. Añade la dependencia con su versión exacta en `go.mod`.
2. Ejecuta `make tidy` para generar/actualizar `go.sum` y los `require` indirectos.
3. Ejecuta `make build` para reconstruir las imágenes.

Todo el trabajo de módulos ocurre dentro del contenedor. Nunca instales Go ni gestores de paquetes en el host.

`go.sum` ya está generado y debe versionarse junto con `go.mod`. Si añades dependencias, `make tidy` lo actualiza.

## Licencia

MIT. Ver `LICENSE`.
