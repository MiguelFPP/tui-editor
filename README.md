# tui-editor

TUI multiplataforma en Go que unifica el flujo diario de desarrollo: exploración
y edición rápida de archivos, control completo de Git y monitorización de
contenedores Docker. Funciona con teclado y ratón.

Estado actual: **v0.3.0 (panel Docker, release engineering y documentación
pública)**. Sobre el MVP con árbol de archivos, preview, edición rápida y panel
Git interactivo, esta versión añade el panel Docker sobre el cliente de Moby y
el andamiaje de release (GoReleaser, workflow de tags y demo reproducible).

## Demo

El flujo completo (Files, preview, Git y Docker) está descrito como una cinta de
[VHS](https://github.com/charmbracelet/vhs) en [`demo/demo.tape`](demo/demo.tape).
Se reproduce con:

```bash
docker compose --profile prod build   # construir la imagen de runtime
vhs demo/demo.tape                    # escribe demo/demo.gif
```

La cinta conduce la TUI real a través de un pseudo-terminal, así que necesita un
daemon Docker. Los paneles Files y Git corren con `make run`; el panel Docker
solo se puebla con `make dev-docker` (ver [Docker y socket](#docker-y-socket)).

## Features

- **Archivos:** árbol con ignores, filtro por nombre/ruta, y preview con
  resaltado de sintaxis (Chroma), números de línea, scroll y detección de
  binarios y tamaño.
- **Edición rápida:** `e` edita el archivo abierto y `ctrl+s` guarda con sello de
  la última lectura, así que un archivo cambiado en disco no se sobrescribe.
  `E`/`ctrl+e` lanza el editor externo.
- **Git completo:** staging por archivo y por hunk, commits multilínea, ramas
  (crear/borrar/merge), stash (push/pop), resolución de conflictos
  (`ours`/`theirs`/abort) y `fetch`/`pull`/`push`.
- **Docker:** lista de contenedores (nombre, imagen, estado, health, puertos),
  muestra puntual de `stats`, `logs` paginados sin `follow` y acciones
  `start`/`stop`/`restart` con confirmación.
- **Multiplataforma y bajo consumo:** binarios estáticos (`CGO_ENABLED=0`) para
  linux/darwin/windows en amd64 y arm64.

## Stack

| Componente | Versión | Import |
| --- | --- | --- |
| Go | 1.27.4 | — |
| Bubble Tea | v2.1.2 | `charm.land/bubbletea/v2` |
| Lip Gloss | v2.1.1 | `charm.land/lipgloss/v2` |
| Bubbles | v2.2.1 | `charm.land/bubbles/v2` |
| Chroma | v2.27.0 | `github.com/alecthomas/chroma/v2` |
| Moby client | v0.6.3 | `github.com/moby/moby/client` |
| Moby API | v1.57.0 | `github.com/moby/moby/api` |
| GoReleaser | v2.20.2 | — |
| Air (hot reload) | v1.67.4 | — |
| Alpine (imagen de runtime) | 3.24.4 | — |

Probado con Docker 29.9.3 y Docker Compose v5.7.0.

Las dependencias usan los paths canónicos v2 (`charm.land/...`), no los antiguos
`github.com/charmbracelet/...`.

## Estructura del proyecto

```
tui-editor/
├── .github/workflows/     # ci.yml (gate) y release.yml (tags v*)
├── cmd/tui-editor/        # composition root
├── demo/                  # cinta VHS y GIF de demo
├── internal/
│   ├── adapters/          # fs, git, editor externo y docker
│   ├── config/            # flags y validación
│   ├── domain/            # tipos, límites y errores puros
│   ├── ports/             # interfaces de servicios
│   ├── testutil/fakes/    # dobles de test
│   └── tui/
│       ├── app/           # modelo raíz, pestañas, layout y ratón
│       ├── common/        # piezas compartidas (diff)
│       └── components/    # filetree, editor, gitpanel y dockerpanel
├── .air.toml
├── .dockerignore
├── .goreleaser.yaml
├── CHANGELOG.md
├── CONTRIBUTING.md
├── Dockerfile
├── LICENSE
├── Makefile
├── README.md
├── SECURITY.md
├── compose.yaml
├── go.mod
└── go.sum
```

`domain` y `ports` son puros; `adapters` y `tui` implementan la funcionalidad por
fases. La documentación de arquitectura (ADRs y presupuestos de rendimiento)
se mantiene fuera del repositorio.

## Requisitos

- Docker
- Docker Compose v2 o superior

No hace falta Go ni Node en el host: todo se ejecuta dentro de contenedores.

## Instalación

### Desde GitHub Releases

Cada tag `vX.Y.Z` publica archivos `tar.gz` (zip en Windows) para
linux/darwin/windows × amd64/arm64, más un `checksums.txt`. Descarga el
correspondiente desde la página de
[releases](https://github.com/MiguelFPP/tui-editor/releases).

```bash
# Linux amd64: sustituye <version> por la versión publicada, p. ej. 0.3.0
curl -fL -O https://github.com/MiguelFPP/tui-editor/releases/latest/download/tui-editor_<version>_linux_amd64.tar.gz
tar -xzf tui-editor_<version>_linux_amd64.tar.gz
./tui-editor --version
```

### Build local (recomendado para desarrollo)

```bash
make build   # construye las imágenes Docker (dev y runtime)
make run     # ejecuta el TUI desde la imagen de producción (requiere TTY)
```

Para generar binarios sueltos en `dist/` sin publicar (snapshot de GoReleaser):

```bash
docker compose run --rm --no-deps -T dev sh -c \
  'go install github.com/goreleaser/goreleaser/v2@v2.20.2 && /go/bin/goreleaser build --snapshot --clean'
```

## Quickstart

```bash
make dev      # TUI con hot reload (requiere TTY)
make run      # TUI desde la imagen de producción (requiere TTY)
```

El TUI necesita un TTY real: usa `docker compose run` (o los targets de arriba)
y no `docker compose up -d`. Sin terminal interactiva no se puede dibujar la
interfaz.

## Docker y socket

Para desarrollar el panel Docker existe `make dev-docker`, que arranca el mismo
servicio dev con el perfil `dev-docker`. Ese perfil es el único que monta
`/var/run/docker.sock`: acceso al socket equivale a acceso root en el host, así
que no se monta en `dev` ni en `app` y el daemon nunca se publica por TCP.
Úsalo solo para desarrollar el panel; si trabajas en otra cosa, `make dev` es
suficiente. Ver [`SECURITY.md`](SECURITY.md).

Si el daemon no está disponible (socket ausente, permiso denegado) el panel
muestra un mensaje accionable y `r` reintenta sin tumbar la TUI. En `dev` y `app`
el socket no está montado, así que el panel aparece como “no disponible”.

## Pestañas y atajos

| Atajo | Acción |
| --- | --- |
| `1` / `2` / `3` | Cambiar de pestaña (`Files`, `Git`, `Docker`) |
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
| `e` | Editar el archivo abierto en la TUI |
| `ctrl+s` | Guardar el borrador |
| `E` / `ctrl+e` | Abrir el archivo en el editor externo (`--editor`) |
| `esc` (editando) | Volver al preview; con cambios sin guardar pide confirmación (`y` descarta, `n` cancela) |

El clic izquierdo selecciona y el doble clic expande o abre. La rueda desplaza el
panel bajo el cursor. El preview usa resaltado de sintaxis con Chroma, números de
línea, scroll y detección de binarios y de tamaño.

La edición rápida solo se abre en archivos de texto completos: los binarios, los
truncados y los diffs se quedan en preview. `ctrl+s` escribe con el sello de la
última lectura, así que un archivo cambiado en disco no se sobrescribe: el
borrador se conserva y se avisa del conflicto. Con un borrador pendiente, `q` y
`ctrl+c` avisan en lugar de salir, y elegir otro archivo no descarta los cambios.

El editor externo se resuelve con `exec.LookPath` y se lanza sin shell, con la
ruta absoluta del archivo: el binario de `--editor` tiene que existir en el
entorno donde corre la TUI. Dentro del contenedor eso significa instalarlo en la
imagen (igual que Git); si falta, la TUI muestra el error y sigue funcionando.

### Git

| Atajo | Acción |
| --- | --- |
| `tab` / `shift+tab` | Cambiar de sección (`Cambios`, `Diff`, `Ramas`, `Stash`, `Historial`) |
| `↑`/`↓`, `j`/`k` | Mover el cursor |
| `enter` | Cargar el diff del archivo, hacer checkout de la rama, aplicar stash o abrir el commit |
| `d` | Alternar entre `worktree` y `staged` |
| `s` / `u` | Preparar o quitar el archivo seleccionado; en `Diff`, el hunk marcado |
| `[` / `]` / `espacio` | Mover el cursor de hunk y marcarlo |
| `c` | Abrir el modal de commit (`ctrl+s` confirma, `esc` cancela) |
| `n` / `x` / `m` | Nueva rama, borrar rama (con confirmación), merge de la rama seleccionada |
| `z` | Stash push (en Cambios) o stash pop (en Stash) |
| `o` / `t` / `A` | Resolver conflicto ours/theirs o abortar el merge (con confirmación) |
| `f` / `p` / `P` | `fetch`, `pull`, `push` |
| `r` | Refrescar estado, ramas, historial y stash |
| `esc` | Cerrar el modal/ayuda o volver a `Cambios` |

El panel Git es interactivo: prepara archivos y hunks, crea commits con mensaje
multilínea, gestiona ramas y stash, detecta conflictos y sincroniza con el
upstream. Las operaciones de escritura se serializan y nunca fuerzan push ni
borran ramas sin confirmación.

### Docker

| Atajo | Acción |
| --- | --- |
| `↑`/`↓`, `j`/`k` | Mover el cursor por la tabla |
| `pgup`/`pgdown`, `home`/`end` | Moverse por páginas o al principio y al final |
| `enter` | Leer una muestra de `stats` del contenedor seleccionado |
| `l` | Abrir los `logs` paginados del contenedor seleccionado |
| `s` / `x` / `R` | `start`, `stop` y `restart` del contenedor seleccionado |
| `y`/`enter`, `n`/`esc` | Confirmar o cancelar la acción pendiente |
| `r` | Refrescar la lista (`dev` sin socket reintenta la conexión) |
| `esc` | Cerrar los `logs` o la confirmación |

La tabla muestra nombre, imagen, estado, health y puertos. `start`, `stop` y
`restart` nunca se ejecutan sin confirmación y se cancelan con `n`/`esc`. Los
`logs` usan `Tail=100` y `Follow=false`: nunca hay streaming continuo; `k`/`pgup`
en la parte superior carga la página anterior con el cursor
`Before`/`NextBefore` del adaptador.

## Arquitectura

Arquitectura **hexagonal (puertos y adaptadores)** para el I/O y **Elm
Architecture (MVU)** para la TUI; `main` es el único composition root.

- `internal/domain/` y `internal/ports/` son puros: no importan Bubble Tea, Lip
  Gloss, `os/exec`, el SDK de Moby ni el filesystem.
- `internal/adapters/` implementa los puertos (fs, git nativo, editor externo,
  Moby). `internal/tui/` compone los sub-modelos MVU y solo conoce `domain` y
  `ports`.
- `Update`/`View` no bloquean con I/O: toda operación externa va en un `tea.Cmd`
  con `context.Context` que devuelve un mensaje tipado.
- Git se ejecuta con el binario nativo vía `os/exec` (`--porcelain=v2 -z`, sin
  shell); Docker usa el cliente modular de Moby, no el CLI.

La documentación detallada de arquitectura, ADRs y presupuestos vive fuera del
repositorio.

## Límites y métricas

- **Preview:** acotado por bytes (`1 MiB`) y líneas (`2000`); los binarios se
  detectan y no se renderizan.
- **Árbol e historial:** límites de entradas, profundidad, commits, ramas y
  stashes para no agotar memoria.
- **Docker:** el ticker solo existe con la pestaña visible (no hay polling en
  segundo plano), `stats` es una muestra puntual con timeout y los `logs` están
  acotados por página y buffer. Un tick de una pestaña oculta no llama al daemon
  ni reserva memoria: `BenchmarkInactiveTickNoPoll` reporta **0 allocs/op**.
- **Objetivo de consumo:** CPU inactiva ≈ 0 % con la pestaña Docker visible y RAM
  **< 30 MB**.

## Comandos

| Comando | Descripción |
| --- | --- |
| `make help` | Lista los targets disponibles (target por defecto) |
| `make dev` | Ejecuta el TUI con hot reload |
| `make dev-docker` | Ejecuta el TUI con hot reload y el socket Docker montado (perfil `dev-docker`) |
| `make run` | Ejecuta el TUI desde la imagen de producción |
| `make build` | Construye las imágenes Docker (dev y runtime) |
| `make fmt` | Ejecuta `go fmt ./...` |
| `make tidy` | Ejecuta `go mod tidy` |
| `make clean` | Borra los artefactos de Air (`tmp`, `bin`) |
| `make test` | Ejecuta `go test ./...`: solo CI y fase final |
| `make vet` | Ejecuta `go vet ./...`: solo CI y fase final |

### Targets por fase

Cada fase ejecuta solo los paquetes que toca. La suite completa está reservada al
CI y a la fase final.

| Comando | Descripción |
| --- | --- |
| `make test-pkg PKG='./internal/<pkg>/...'` | Tests de un paquete |
| `make test-run PKG='./internal/<pkg>/...' RUN='TestNombre'` | Tests que casan con `RUN` |
| `make vet-pkg PKG='./internal/<pkg>/...'` | `go vet` de un paquete |
| `make test-integration PKG='./internal/<pkg>/...' [RUN='TestIntegration...']` | Tests con build tag `integration` |
| `make bench-pkg PKG='./internal/<pkg>/...'` | Benchmarks de un paquete |
| `make test-race` | Suite completa con el detector de carreras: solo fase final |

`PKG` y `RUN` son obligatorios donde aplica: si faltan, el target falla con un
mensaje claro.

## Release

El release se dispara con un tag `v*`:

```bash
# el usuario crea el tag; el workflow .github/workflows/release.yml publica
git tag v0.3.0
git push origin v0.3.0
```

El workflow usa GoReleaser v2.20.2 (`.goreleaser.yaml`) y `GITHUB_TOKEN` para
crear la release en GitHub. No publiques tags desde el agente: el flujo de PR y
el gate de CI (`test`) siguen siendo la vía de merge.

## Flags

| Flag | Default | Descripción |
| --- | --- | --- |
| `--version` | — | Imprime `tui-editor 0.3.0` (builds de release añaden el commit) y sale con código 0 |
| `--repo DIR` | directorio actual | Raíz del repositorio. Debe existir y ser un directorio; se guarda en ruta absoluta |
| `--editor CMD` | `$EDITOR` o `vim` | Comando del editor externo; el binario debe existir en el `PATH` del entorno donde corre la TUI |
| `--docker-host URL` | `$DOCKER_HOST` | Endpoint del daemon Docker. Solo se aceptan los esquemas `unix`, `tcp` y `npipe`; el socket no se valida al arrancar |
| `--debug-log FILE` | vacío | Archivo de log de depuración |

`--help` muestra el uso. Los errores de validación se imprimen en `stderr` y el
proceso termina con código 1. `--version` no valida el resto de flags.

```bash
docker compose run --rm --no-deps -T dev go run ./cmd/tui-editor --version
```

## Alcance y limitaciones

- Edición rápida: `e` edita el archivo abierto, `ctrl+s` guarda con detección de
  cambios externos y `E` abre el binario de `--editor`. Los binarios, los
  truncados y los diffs no se editan en la TUI.
- Git interactivo: staging por archivo/hunk, commit, ramas, stash, merge,
  conflictos y sync.
- Panel Docker: lista de contenedores, stats de una muestra bajo demanda, logs
  paginados sin `follow` y acciones `start`/`stop`/`restart` con confirmación. El
  ticker solo corre con la pestaña visible.
- Previews y listados acotados por tamaño y número de líneas; los binarios se
  detectan y no se renderizan.
- El staging por hunk cubre archivos modificados; los binarios, untracked y
  renames se preparan por archivo.

## Cómo añadir dependencias

1. Añade la dependencia con su versión exacta en `go.mod`.
2. Ejecuta `make tidy` para generar/actualizar `go.sum` y los `require`
   indirectos.
3. Ejecuta `make build` para reconstruir las imágenes.

Todo el trabajo de módulos ocurre dentro del contenedor. Nunca instales Go ni
gestores de paquetes en el host.

`go.sum` ya está generado y debe versionarse junto con `go.mod`. Si añades
dependencias, `make tidy` lo actualiza.

## Contribuir y seguridad

- Guía de contribución y workflow: [`CONTRIBUTING.md`](CONTRIBUTING.md).
- Política de seguridad y riesgo del socket Docker: [`SECURITY.md`](SECURITY.md).
- Historial de cambios: [`CHANGELOG.md`](CHANGELOG.md).

## Licencia

MIT. Ver [`LICENSE`](LICENSE).
