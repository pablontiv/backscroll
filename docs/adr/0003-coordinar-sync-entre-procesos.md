---
tipo: adr
estado: accepted
fecha: 2026-08-21
contexto: Varias sesiones pueden invocar Backscroll al mismo tiempo y cada proceso repite discovery, hashing, parseo y sync; el costo cuadrático al agregar texto y la serialización de escritores SQLite convierten esa duplicación en latencia y presión de recursos.
decision: Eliminar la agregación completa de texto, coordinar preparación y startup sync con un lock advisory del sistema operativo por base canónica y permitir que los procesos read-safe no propietarios consulten una snapshot confirmada compatible.
consecuencias: Habrá un solo owner de preparación, sync y mutaciones por base; las lecturas concurrentes conservarán disponibilidad a cambio de observar temporalmente una snapshot anterior y de introducir un sidecar persistente, diagnósticos de frescura y una dependencia de locking portable.
---

# Coordinar el sync entre procesos

## Contexto

La política vigente exige un intento de sync incremental antes de toda operación pública. Esa garantía fue diseñada para una invocación aislada, pero varias sesiones de agentes pueden ejecutar Backscroll simultáneamente contra la misma base.

Cada proceso repite de forma independiente discovery, hashing, parseo y agregación de texto para tagging antes de competir por una transacción SQLite. WAL permite que lectores y un escritor coexistan, pero no habilita múltiples escritores ni deduplica el trabajo previo. El `busy_timeout` limita la espera ante contención SQLite, pero no evita que varios procesos consuman CPU, disco y memoria sobre los mismos inputs.

El problema se agrava por la concatenación repetida de strings inmutables en `maybeAutoSync`, cuyo costo crece de forma cuadrática con el texto de una sesión modificada. Una sola invocación puede ser lenta; la concurrencia multiplica ese costo.

Esta decisión limita ADR 0002 para invocaciones concurrentes. El intento de sync sigue siendo obligatorio, pero un proceso read-safe puede continuar contra la última transacción confirmada cuando otro proceso ya posee la responsabilidad de sincronizar. SQLite continúa siendo la única fuente pública de consulta.

La alternativa seleccionada fue validada contra documentación vigente a agosto de 2026. `gofrs/flock` usa `flock` en sistemas Unix soportados y `LockFileEx` en Windows. Ambos modelos vinculan ownership al descriptor o handle abierto y permiten recuperación por el sistema operativo al terminar el proceso. SQLite confirma que WAL permite lectores y escritor concurrentes mediante snapshot isolation y que todos los procesos deben residir en el mismo host.

## Decisión

La corrección se implementará en tres capas y en este orden:

1. Eliminar la construcción de un string completo de sesión. `internal/tagging` acumulará categorías por mensaje sin retener el contenido total.
2. Coordinar preparación, migración y startup sync mediante `github.com/gofrs/flock` v0.13.0. El lock se identificará por la ruta canónica de la base y usará el sidecar `<db>.startup-sync.lock`, creado con permisos `0600`.
3. Cuando un proceso read-safe encuentre un owner activo, validará la base existente en modo read-only y ejecutará su handler contra la snapshot SQLite confirmada. Emitirá `sync_in_progress` en stderr sin modificar stdout JSON, robot o texto.

El sidecar permanecerá en disco y nunca se borrará. Su existencia no representa ownership; solo el lock advisory mantenido por el descriptor o handle abierto lo representa. Los caminos normales liberarán explícitamente el recurso y la terminación del proceso actuará como recuperación final.

Los comandos se clasificarán explícitamente:

- read-safe: `search`, `list`, `patterns`, `status`, `validate`, `config`;
- mutation: `annotate`, `purge`, `rebuild`, `recover`.

Un follower mutante esperará hasta cinco segundos por el lock. Si no lo adquiere, fallará con diagnóstico reintentable y no ejecutará su handler. Cuando lo adquiere, conservará el lock durante preparación, sync y handler. `recover` conservará ownership durante reemplazo, verificación y post-install sync.

Un follower read-safe no esperará. Solo continuará si la snapshot existente es compatible sin migración. Una base ausente, incompatible o que requiera migración producirá un diagnóstico reintentable; el follower no creará ni modificará schema.

El alcance se limita a filesystems locales confiables, consistente con la restricción de SQLite WAL. La optimización de hashing/discovery, la interfaz de progreso, un servidor MCP compartido y MCP Tasks quedan fuera de esta decisión.

## Alternativas descartadas

- **Lease dentro de SQLite:** obliga a abrir y escribir la misma base cuya creación o migración debe coordinarse, y agrega heartbeat, expiración, reloj y recuperación de owners.
- **Archivo con `O_EXCL`, PID y timestamp:** la presencia sobrevive crashes y exige detección de procesos stale, con riesgos de PID reuse y carreras al robar ownership.
- **Conservar frescura estricta y hacer esperar a todos los procesos:** evita snapshots anteriores, pero bloquea comandos interactivos detrás de un sync largo.
- **Confiar solamente en WAL y `busy_timeout`:** SQLite coordina acceso transaccional, pero no evita discovery, hashing, parseo y tagging duplicados antes de escribir.
- **Convertir Backscroll en MCP:** MCP 2026-07-28 no define exclusión, single-flight ni coordinación de escritores SQLite. Un servidor compartido futuro consumirá esta decisión en vez de reemplazarla.
- **Borrar el sidecar al liberar:** puede crear carreras de identidad de archivo; un nuevo proceso podría bloquear un inodo distinto mientras el anterior continúa bloqueado.

## Consecuencias

### Positivas

- Solo un proceso realiza preparación y startup sync costosos para una base determinada.
- La agregación de tags escala linealmente y no duplica el texto completo de la sesión.
- Las consultas concurrentes permanecen disponibles sobre una snapshot transaccional consistente.
- La terminación del owner no deja ownership permanente.
- Las mutaciones y recovery quedan serializados con startup sync.
- El diseño funciona tanto para clientes CLI como para una futura superficie MCP.

### Negativas

- Una consulta concurrente puede no incluir inputs que el owner todavía está procesando.
- El contrato de ADR 0002 deja de significar frescura estricta por invocación cuando ya existe un owner activo.
- El sidecar, sus diagnósticos y las pruebas multiproceso agregan complejidad operativa.
- Se añade una dependencia portable de locking.
- Windows puede tardar en liberar un lock después de una terminación anormal; una mutación puede requerir reintento después de su timeout de cinco segundos.
- Backscroll no promete coordinación sobre filesystems remotos.

### Riesgo aceptado

Se acepta frescura eventual durante una ventana de sync concurrente para evitar trabajo duplicado y conservar latencia interactiva. SQLite sigue siendo la única fuente pública de consulta y solo expone transacciones confirmadas.
