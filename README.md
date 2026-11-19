# tui-editor

TUI multiplataforma en Go que unifica el flujo diario de desarrollo: exploración
y edición rápida de archivos, control completo de Git y monitorización de
contenedores Docker. Funciona con teclado y ratón.

Estado actual: **v0.5.0 (temas y atajos configurables)**. Sobre v0.4.0, esta
versión añade un catálogo de cuatro temas, configuración JSON en XDG para tema y
atajos, ayuda/footers generados desde el registro y un overlay `,` para
consultar la configuración. Los instaladores no cambian.

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
- **Edición rápida:** `e` edita el archivo abierto conservando el resaltado de
  sintaxis, con cursor visible, scroll con rueda y clic para posicionar;
  `ctrl+s` guarda con sello de la última lectura, así que un archivo cambiado en
  disco no se sobrescribe. `E`/`ctrl+e` lanza el editor externo.
- **Git completo:** staging por archivo y por hunk, commits multilínea, ramas
  (crear/borrar/merge), stash (push/pop), resolución de conflictos
  (`ours`/`theirs`/abort) y `fetch`/`pull`/`push`.
- **Docker:** lista acotada al proyecto abierto mediante las etiquetas de
  Compose (nombre, imagen, estado, health, puertos), con `a` para ver todos los
  contenedores del daemon. Muestra puntual de `stats`, `logs` paginados sin
  `follow` y acciones `start`/`stop`/`restart` con confirmación.
- **Temas y atajos configurables:** cuatro temas (Tokyo Night, Catppuccin Mocha,
  Dracula y Gruvbox Dark) y overrides de teclado mediante un `config.json` en
  XDG; `--list-themes` y `--list-keys` muestran los catálogos.
- **Configuración a la vista:** `,` abre un overlay read-only con la ruta del
  archivo, el tema activo y los overrides; `--init-config` y `--print-config`
  escriben un archivo base o imprimen la configuración resuelta.
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
│       ├── components/    # filetree, editor, gitpanel y dockerpanel
│       ├── keymap/        # registro de acciones, parser de teclas, overrides y ayuda
│       └── styles/        # catálogo de temas y tokens visuales
├── scripts/               # install.sh (Linux/macOS) e install.ps1 (Windows)
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
├── config.example.json
├── go.mod
└── go.sum
```

`domain` y `ports` son puros; `adapters` y `tui` implementan la funcionalidad por
fases. La documentación de arquitectura (ADRs y presupuestos de rendimiento)
se mantiene fuera del repositorio.

## Requisitos

Para **usar** el binario no hace falta nada más: es estático (`CGO_ENABLED=0`),
así que solo necesitas Linux, macOS o Windows en amd64 o arm64.

- `git` en el `PATH` si vas a usar las pestañas Git (el adaptador usa el binario
  nativo, no una librería).
- Un daemon Docker accesible si vas a usar la pestaña Docker; si no está, el
  panel muestra un estado accionable y la TUI sigue funcionando.

Para **desarrollar** el proyecto sí necesitas Docker y Docker Compose v2 o
superior: todo se compila y se prueba dentro de contenedores, sin Go en el host.

## Instalación

### Instalador (recomendado)

Linux y macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.ps1 | iex
```

El instalador detecta el sistema y la arquitectura, descarga el archivo de la
última release, **verifica el SHA-256** contra `checksums.txt` y deja el binario
en:

| Sistema | Destino por defecto |
| --- | --- |
| Linux/macOS, como root | `/usr/local/bin` |
| Linux/macOS, usuario normal | `~/.local/bin` (avisa si falta en el `PATH`) |
| Windows | `%LOCALAPPDATA%\Programs\tui-editor` (lo añade al `PATH` de usuario) |

Opciones de `install.sh`:

```bash
# Versión concreta y directorio propio
curl -fsSL https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.sh \
  | sh -s -- --version v0.5.0 --dir "$HOME/bin"

# Sin verificar el checksum (no recomendado)
curl -fsSL https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.sh \
  | sh -s -- --no-verify
```

Opciones de `install.ps1`: `-Version v0.5.0`, `-Dir C:\Tools\tui-editor`,
`-NoVerify` y `-NoPath` (no tocar el `PATH` de usuario).

Ningún instalador requiere `sudo` por sí mismo: si escribes en un directorio del
sistema, ejecútalo con los permisos que ese directorio necesite.

### Actualizar

El mismo comando sirve para actualizar: siempre instala la última release en el
destino que corresponda y reemplaza el binario anterior sin dejar copias.

```bash
curl -fsSL https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.ps1 | iex
```

El reemplazo es atómico, así que puedes actualizar **con la TUI abierta**: el
proceso en marcha sigue funcionando y la versión nueva se usa en el siguiente
arranque. Comprueba la versión con `tui-editor --version`. Para fijar una versión
concreta —por ejemplo para volver atrás— usa `--version vX.Y.Z`.

### Descarga manual

Cada tag `vX.Y.Z` publica archivos `tar.gz` (zip en Windows) para
linux/darwin/windows × amd64/arm64, más un `checksums.txt`, desde la página de
[releases](https://github.com/MiguelFPP/tui-editor/releases).

El nombre del asset **incluye la versión**, así que sustituye `<version>` por la
versión publicada (por ejemplo `0.5.0`; el prefijo `v` no forma parte del
nombre):

```bash
# Linux amd64
version=0.5.0
curl -fL -O "https://github.com/MiguelFPP/tui-editor/releases/download/v${version}/tui-editor_${version}_linux_amd64.tar.gz"
tar -xzf "tui-editor_${version}_linux_amd64.tar.gz"
./tui-editor --version
```

Para consultar la última versión publicada sin la API de GitHub:

```bash
curl -fsSL -o /dev/null -w '%{url_effective}\n' \
  https://github.com/MiguelFPP/tui-editor/releases/latest | sed 's#.*/tag/v##'
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

### Desde el código fuente

Si prefieres compilar tú mismo y ya tienes Go 1.27.4 o superior:

```bash
git clone https://github.com/MiguelFPP/tui-editor.git
cd tui-editor
go build -trimpath -o tui-editor ./cmd/tui-editor
```

Con `go build` sin ldflags el binario funciona pero `--version` informa `dev`.
Para inyectar la versión igual que GoReleaser:

```bash
go build -trimpath \
  -ldflags "-s -w -X main.version=0.5.0 -X main.commit=$(git rev-parse --short HEAD)" \
  -o tui-editor ./cmd/tui-editor
```

Requiere `git` en el `PATH` para las pestañas Git, igual que la instalación por
binario.

## Quickstart

Con el binario instalado, basta con apuntarlo a un repositorio:

```bash
tui-editor --repo .        # abre el directorio actual
tui-editor                 # o usa el directorio actual por defecto
```

Para desarrollo del proyecto, el flujo equivalente es:

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

## Configuración

La configuración vive en un único archivo JSON (tema, atajos y layout del árbol)
y se resuelve en tres niveles —flags, variables de entorno y archivo— con esta
correspondencia:

| Dato | Flag | Env | Archivo | Default |
| :--- | :--- | :--- | :--- | :--- |
| Ruta del archivo | `--config FILE` | `TUI_EDITOR_CONFIG` | — | `$XDG_CONFIG_HOME/tui-editor/config.json` (`os.UserConfigDir()/tui-editor/config.json`) |
| Tema | `--theme NAME` | `TUI_EDITOR_THEME` | `theme` | `tokyonight-night` |
| Atajos | — | — | `keys` | defaults del registro |
| Layout del árbol | — | — | `tree_width` / `tree_collapsed` | `0` / `false` (`0` = automático, 35 %) |

La precedencia es **flags > env > archivo > defaults**: el flag gana a la variable
de entorno, ambos ganan al archivo, y lo que no se define en ningún nivel usa el
default. El layout del árbol solo vive en el archivo: no tiene flag ni variable de
entorno.

El repositorio incluye [`config.example.json`](config.example.json) como
plantilla versionada con overrides de ejemplo; no se carga automáticamente.

- Si el archivo por defecto **no existe, se ignora** sin error: arrancas con los
  defaults del registro.
- Si `--config` o `TUI_EDITOR_CONFIG` apuntan a un archivo inexistente, es un
  **error**: la ruta explícita debe existir.
- El JSON es **estricto**: campos desconocidos, JSON inválido, acciones
  desconocidas, teclas inválidas, conflictos dentro de un mismo scope o teclas
  reservadas producen un error accionable en `stderr` (código 1).
- No hay recarga en caliente: reinicia la TUI para aplicar los cambios.

Ejemplo de archivo:

```json
{
  "theme": "catppuccin-mocha",
  "tree_width": 42,
  "tree_collapsed": false,
  "keys": {
    "quit": ["q"],
    "help": ["?"],
    "tab_files": ["1"],
    "tab_git": ["2"],
    "tab_docker": ["3"],
    "tree_up": ["up", "k"],
    "tree_down": ["down", "j"],
    "git_up": ["k", "up"],
    "git_down": ["j", "down"],
    "docker_up": ["k", "up"],
    "docker_down": ["j", "down"],
    "git_stage": ["s"],
    "docker_logs": ["l"]
  }
}
```

Sobre el bloque `keys`:

- Una acción omitida conserva su default; si la incluyes, tu array **sustituye la
  lista completa** de esa acción (no se fusiona con el default).
- Cada acción admite una cadena o una lista: `"quit": "q"` equivale a
  `"quit": ["q"]`.
- Las acciones modales (confirmaciones, commit, prompt y logs) **no son
  configurables**.
- `ctrl+c` siempre sale; `esc` y `enter` están reservados para modales y ayuda y
  no se pueden asignar en overrides.
- `--list-keys` imprime todos los IDs, su scope, sus teclas por defecto y si son
  configurables o fijas.
- `--list-themes` imprime el catálogo y marca el default.

Fuera de `keys`, el archivo guarda el layout del árbol: `tree_width` es el ancho
exterior de la columna del árbol (`0` = automático, el 35 % del ancho),
`tree_collapsed` arranca la pestaña `Files` con el árbol minimizado y un
`tree_width` negativo es un error.

### Layout del árbol

El reparto de ancho entre el árbol y el preview de la pestaña `Files` se ajusta
de tres maneras:

- Arrastrar con el ratón el borde compartido: al soltar, el ancho nuevo se
  guarda.
- `F2`, o el botón `[<]`/`[>]` de la fila de pestañas, minimiza el árbol o lo
  vuelve a mostrar; el ancho se conserva mientras está oculto.
- `shift+left` / `shift+right` reducen o amplían el árbol dos celdas por
  pulsación. El ancho se acota en ambos extremos para que los dos paneles
  conserven un mínimo.

El layout se guarda solo en `config.json` al cambiarlo, con `tree_width` y
`tree_collapsed`: el resto del archivo (tema y atajos) se conserva y, si no
existía, se crea. No hay recarga en caliente.

### Archivo base

```bash
tui-editor --init-config
tui-editor --init-config --force
tui-editor --print-config
```

- `--init-config` escribe un archivo base mínimo (`theme: tokyonight-night`,
  `tree_width: 0`, `tree_collapsed: false`, `keys: {}`) en la ruta resuelta (XDG o
  `--config`); no sobrescribe sin `--force`.
- `--print-config` imprime la configuración resuelta en JSON con `config_path`,
  `theme`, `tree_width`, `tree_collapsed` y `keys`; no arranca la TUI.
- `config.example.json` es una plantilla con overrides de ejemplo; no se carga
  automáticamente.

### Configuración desde la TUI

- `,` abre el overlay read-only.
- Muestra el archivo cargado (o “sin archivo (defaults)”), el tema activo y el
  número y la lista de overrides.
- `,` o `esc` lo cierran; el ratón se ignora.
- Es read-only: edita el archivo y reinicia para aplicar; no hay recarga en
  caliente.
- La tecla `,` es configurable como el resto de acciones (ID `config`).

### Temas

| Nombre | Estilo Chroma | Descripción | Default |
| :--- | :--- | :--- | :---: |
| `tokyonight-night` | `tokyonight-night` | Tokyo Night | Sí |
| `catppuccin-mocha` | `catppuccin-mocha` | Catppuccin Mocha | — |
| `dracula` | `dracula` | Dracula | — |
| `gruvbox-dark` | `gruvbox` | Gruvbox Dark | — |

Un `--theme`/`TUI_EDITOR_THEME`/`theme` fuera de este catálogo es un error que
lista los nombres válidos. El tema colorea los tokens de la interfaz y el
resaltado de sintaxis del preview (Chroma).

### Ejemplos de uso

```bash
tui-editor --theme dracula
TUI_EDITOR_THEME=gruvbox-dark tui-editor
tui-editor --config ~/.config/tui-editor/config.json
tui-editor --list-themes
tui-editor --list-keys
```

## Pestañas y atajos

Los atajos de las tablas son los **defaults**: se pueden sobrescribir en
`config.json` (ver [Configuración](#configuración)) y `--list-keys` muestra el ID
de cada acción. La ayuda `?` refleja siempre las teclas **efectivas**, es decir,
las que resultan de aplicar los overrides sobre los defaults. Las teclas modales
(`enter`, `esc`, `y`, `n`, `ctrl+s`) permanecen **fijas**, igual que `ctrl+c` para
salir.

| Atajo | Acción |
| --- | --- |
| `1` / `2` / `3` | Cambiar de pestaña (`Files`, `Git`, `Docker`) |
| clic en la pestaña | Cambiar de pestaña, con el mismo efecto que `1`/`2`/`3` |
| `tab` / `shift+tab` | Alternar el foco en `Files`; cambiar de sección en `Git` |
| `F2` | Minimizar el árbol de `Files` o volver a mostrarlo |
| `shift+left` / `shift+right` | Redimensionar el árbol de `Files` dos celdas por pulsación |
| `?` | Mostrar u ocultar la ayuda |
| `,` | Ver la configuración (overlay read-only) |
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
| `↑`/`↓`/`←`/`→`, `home`/`end` | Mover el cursor dentro del borrador |
| rueda, `pgup`/`pgdown` | Desplazar el borrador sin mover el cursor |
| clic en el borrador | Colocar el cursor en esa posición |
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
| `a` | Alternar entre los contenedores del proyecto y todos los del daemon |
| `s` / `x` / `R` | `start`, `stop` y `restart` del contenedor seleccionado |
| `y`/`enter`, `n`/`esc` | Confirmar o cancelar la acción pendiente |
| `r` | Refrescar la lista (`dev` sin socket reintenta la conexión) |
| `esc` | Cerrar los `logs` o la confirmación |

La tabla muestra nombre, imagen, estado, health y puertos. Por defecto solo
lista los contenedores del proyecto abierto: compara la etiqueta de Compose
`com.docker.compose.project.working_dir` con la raíz del repositorio y, si esa
etiqueta falta, cae al nombre del proyecto. `a` alterna a todos los contenedores
del daemon y conserva la selección; el modo activo aparece en el título y en el
pie. Si la raíz del repo no se conoce, la lista no se filtra.

`start`, `stop` y `restart` nunca se ejecutan sin confirmación y se cancelan con
`n`/`esc`. Los `logs` usan `Tail=100` y `Follow=false`: nunca hay streaming
continuo; `k`/`pgup` en la parte superior carga la página anterior con el cursor
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
- **Medición local:** smoke con la TUI real en pseudo-TTY (`--repo /src`, pestaña
  Files): proceso `tui-editor` con **22.8 MB RSS** y salida limpia (exit 0). El
  benchmark del tick Docker inactivo reporta **0 allocs/op**.

## Calidad

| Comprobación | Resultado |
| :--- | :--- |
| `make test` (`-count=1`) | suite completa en verde |
| `make test-race` (`-count=1`) | detector de carreras en verde |
| `make test-integration PKG='./...'` | integración Git/Docker opt-in en verde |
| `make vet` | `go vet ./...` limpio |
| `make build` / `goreleaser build --snapshot --clean` | imágenes y 6 binarios |
| Cobertura crítica | domain 98.9 %, git 92.9 %, docker 90.5 %, app 93.6 % |
| Smoke TUI | pseudo-TTY, exit 0; VHS/GIF pendiente de herramienta externa |

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
git tag v0.5.0
git push origin v0.5.0
```

El workflow usa GoReleaser v2.20.2 (`.goreleaser.yaml`) y `GITHUB_TOKEN` para
crear la release en GitHub. No publiques tags desde el agente: el flujo de PR y
el gate de CI (`test`) siguen siendo la vía de merge.

## Flags

| Flag | Default | Descripción |
| --- | --- | --- |
| `--version` | — | Imprime `tui-editor 0.5.0` (builds de release añaden el commit) y sale con código 0 |
| `--repo DIR` | directorio actual | Raíz del repositorio. Debe existir y ser un directorio; se guarda en ruta absoluta |
| `--editor CMD` | `$EDITOR` o `vim` | Comando del editor externo; el binario debe existir en el `PATH` del entorno donde corre la TUI |
| `--docker-host URL` | `$DOCKER_HOST` | Endpoint del daemon Docker. Solo se aceptan los esquemas `unix`, `tcp` y `npipe`; el socket no se valida al arrancar |
| `--debug-log FILE` | vacío | Archivo de log de depuración |
| `--config FILE` | `$XDG_CONFIG_HOME/tui-editor/config.json` | Archivo JSON de temas y atajos; si se indica explícitamente debe existir |
| `--theme NAME` | `tokyonight-night` | Tema de color; `--list-themes` muestra los válidos |
| `--list-themes` | — | Lista los temas disponibles y sale |
| `--list-keys` | — | Lista las acciones, su scope, sus teclas por defecto y si son configurables, y sale |
| `--init-config` | — | Escribe un archivo base y sale; no sobrescribe sin `--force` |
| `--force` | — | Permite sobrescribir con `--init-config` |
| `--print-config` | — | Imprime la configuración resuelta en JSON y sale |

`--help` muestra el uso. Los errores de validación se imprimen en `stderr` y el
proceso termina con código 1. `--version`, `--list-themes`, `--list-keys`,
`--init-config` y `--print-config` no validan repositorio, editor ni Docker.

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
