# guardia

[![ci](https://github.com/KlGamers/guardia/actions/workflows/ci.yml/badge.svg)](https://github.com/KlGamers/guardia/actions/workflows/ci.yml)

Hook de pre-commit en un solo binario de Go. Antes de cada commit revisa
formato, lint y pruebas de lo que vas a subir, y si algo falla cancela el
commit. Detecta solo el lenguaje de cada proyecto, así que en la mayoría de
los repos no hay nada que configurar.

```
$ git commit -m "add Mul"
guardia · 3 checks · 1 staged file
  ✗ gofmt    42ms  reported problems
  ✓ go vet   510ms
  ✗ go test  1.3s  exit status 1

── gofmt ──
calc.go

── go test ──
--- FAIL: TestAdd (0.00s)
    calc_test.go:7: 2 + 2 should be 4
FAIL
FAIL	demo	0.143s
FAIL

2 checks failed
commit cancelled. fix the problems above, or bypass with git commit --no-verify
```

## Por qué otra herramienta

Husky, pre-commit y lefthook ya existen. guardia se enfoca en tres cosas que
con ellas requieren configuración o plugins extra:

- **Revisa lo que vas a commitear, no tu carpeta.** Si un archivo tiene una
  parte en staging y otra no, guardia aparta temporalmente lo que no está en
  staging, corre los chequeos y lo regresa. Un error en código que todavía no
  agregas no te bloquea, y un error en lo que sí agregaste no se esconde
  aunque ya lo hayas arreglado en tu copia de trabajo.
- **Cero configuración.** Reconoce Go, Python, Node, Rust, Dart y Flutter,
  C y C++ con `.clang-format` y contratos Vyper, también en subcarpetas de un
  monorepo. Usa automáticamente el `.venv` o `node_modules/.bin` de cada
  proyecto.
- **Lo mismo en local y en CI.** `guardia ci` corre exactamente los mismos
  chequeos sobre todos los archivos del repo, para que tu laptop y GitHub
  Actions nunca opinen distinto.

## Instalación

```sh
go install github.com/KlGamers/guardia/cmd/guardia@latest
cd tu-repo
guardia init
```

`guardia init` instala el hook en `.git/hooks/pre-commit`, o en la carpeta de
`core.hooksPath` si la configuraste. Si ya había un hook, lo respalda y
`guardia uninstall` lo devuelve a su lugar.

## Comandos

| Comando | Qué hace |
|---|---|
| `guardia init [--force] [--write-config]` | Instala el hook. Con `--write-config` guarda los chequeos detectados en `guardia.json` para editarlos |
| `guardia run [-p N] [--no-isolate] [-v]` | Revisa los archivos en staging. Es lo que corre el hook |
| `guardia ci [-p N] [-v]` | Revisa todos los archivos versionados. Para CI |
| `guardia list` | Muestra qué chequeos correrían |
| `guardia uninstall` | Quita el hook y restaura el anterior |

Códigos de salida: `0` todo bien, `1` falló algún chequeo, `2` error de uso o
de git, `130` cancelado con Ctrl-C.

## Qué detecta

| Si encuentra | Corre |
|---|---|
| `go.mod` | `gofmt -l`, `go vet ./...`, `go test ./...` |
| `pyproject.toml`, `setup.py` o `requirements*.txt` | `ruff check`, `ruff format --check` y `pytest` si hay pruebas |
| `package.json` | Los scripts `lint`, `format:check`, `typecheck` y `test` que existan |
| `Cargo.toml` | `cargo fmt --check`, `cargo clippy -D warnings`, `cargo test` |
| `pubspec.yaml` | `dart format`, `dart analyze` o `flutter analyze`, y las pruebas |
| `.clang-format` | `clang-format --dry-run -Werror` |
| archivos `.vy` | Compila los contratos con `vyper` |

Busca hasta dos niveles de profundidad e ignora `node_modules`, `.venv`,
`build`, `dist`, `target` y carpetas ocultas. Un chequeo solo corre si en el
commit hay archivos que le corresponden, y si la herramienta no está
instalada se marca como omitido en lugar de fallar.

## Configuración

Si los chequeos detectados no te sirven, crea `guardia.json`:

```json
{
  "parallel": 4,
  "timeout": "5m",
  "checks": [
    { "name": "eslint", "dir": "web", "run": "eslint {files}", "glob": ["*.ts", "*.tsx"], "need": "eslint" },
    { "name": "api tests", "dir": "api", "run": "pytest -q", "glob": ["*.py"], "timeout": "2m" },
    { "name": "sin TODO", "run": "! grep -n TODO {files}", "glob": ["*.go"] }
  ]
}
```

| Campo | Qué hace |
|---|---|
| `run` | Comando de shell. `{files}` se reemplaza por los archivos que tocan, ya escapados |
| `dir` | Carpeta donde corre, relativa a la raíz del repo |
| `glob` | Patrones de archivo que activan el chequeo |
| `need` | Programa requerido. Si falta, el chequeo se omite |
| `fail_on_output` | Falla si el comando imprime algo, para herramientas como `gofmt -l` |
| `timeout` | Límite por chequeo |
| `isolate` | `false` para revisar la carpeta tal cual, sin apartar lo que no está en staging |
| `detect` | `true` para sumar los chequeos detectados a los tuyos |

## Cómo funciona

```
git commit
   │
   ▼
pre-commit ──> guardia run
                 │
                 ├─ archivos en staging (git diff --cached)
                 ├─ aparta lo que no está en staging (parche en .git/)
                 ├─ chequeos en paralelo, cada uno en su grupo de procesos
                 ├─ regresa lo apartado
                 └─ resumen y código de salida
```

- **Apartar sin perder trabajo.** Lo que no está en staging se guarda como
  parche binario dentro de `.git/` antes de tocar nada. Si al regresarlo hay
  un conflicto, el parche se conserva y guardia te dice dónde está. Si una
  ejecución anterior se interrumpió, guardia se niega a seguir hasta que lo
  recuperes.
- **Nada queda colgado.** Cada chequeo corre en su propio grupo de procesos.
  Un timeout o un Ctrl-C mata también a los procesos hijos, y lo apartado se
  regresa antes de salir.
- **Durante un merge o rebase** no se aparta nada, para no interferir con git.

## Pruebas

```sh
go test -race ./...
```

Las pruebas crean repositorios temporales en rutas con espacios y acentos y
cubren staging parcial, archivos borrados y binarios, conflictos al
restaurar, timeouts, cancelación, paralelismo, detección en monorepos,
respaldo de hooks existentes, merges en curso, parches que quedaron de una
ejecución interrumpida y un `git commit` real que el hook rechaza. Todos los
paquetes superan el 91% de cobertura y el CI falla si el total baja de 90%.

## Límites

- Funciona en macOS y Linux. Windows no está soportado.
- No corrige archivos automáticamente, solo los revisa.
- Un hook local se puede saltar con `git commit --no-verify`. Para proteger
  `main` de verdad, corre `guardia ci` en tu pipeline y activa la protección
  de rama en GitHub.

## Licencia

MIT
