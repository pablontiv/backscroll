---
tipo: adr
estado: accepted
fecha: '2026-10-07'
contexto: 'El skill y los presets dependen de un instalador Python, de un clon estable y de hooks. El issue 123 exige una interfaz nativa sin inicialización ordinaria del producto.'
decision: 'Incorporar el skill oficial y los presets en el binario. Añadir skill, skill --install, skill --restore RECEIPT y presets install. Conservar el alias --skill durante una versión mayor. Mantener recibos v1 compatibles con el instalador Python.'
alternativas: 'Conservar el instalador Python: mantiene dependencias y un clon estable. Instalar desde hooks: mezcla automatización del repositorio con estado del usuario. Crear un marco para varios skills: excede el único skill oficial.'
consecuencias: 'La instalación deja de depender de Python y de un clon. La CLI debe conservar aprobación, respaldo, diario, bloqueo, durabilidad, reversión y restore. La migración retira scripts y mutaciones de hooks solo después de alcanzar paridad.'
sustituye: 'ADR 0008'
---
# 0010. Integrar el skill y los presets en la CLI

## Contexto

Backscroll tiene un solo skill oficial instalable. Su nombre es `backscroll`.

El binario ya contiene las instrucciones autoritativas. El shim de descubrimiento delega en `backscroll --skill`.

El instalador actual usa Python. También exige un clon estable como origen del shim. Los hooks instalan presets como efecto secundario.

El issue 123 exige una interfaz nativa. También exige conservar las garantías del instalador actual.

## Estado de la decisión anterior

Esta ADR sustituye de forma explícita la decisión de [ADR 0008](0008-instalar-skills-con-aprobacion-explicita.md).

ADR 0008 rechazó un subcomando en Go. Esa alternativa deja de ser válida porque el issue 123 incorpora la instalación al producto.

ADR 0008 permanece como registro histórico. Esta ADR conserva sus garantías de aprobación, respaldo y restauración.

## Decisión

El binario incorpora el shim y los presets. La instalación no depende de Python, Git ni un clon del repositorio.

La interfaz pública gestiona solo el skill `backscroll`. La interfaz no crea un registro genérico para otros skills. `backscroll-doctor` queda fuera de esta interfaz.

La primera implementación añade estas formas:

```text
backscroll skill
backscroll skill --install [--approve SHA256] [--home HOME] [--config-home CONFIG_HOME]
backscroll skill --restore RECEIPT --approve SHA256 [--home HOME]
backscroll presets install [--overwrite] [--config-home CONFIG_HOME]
```

`--install` y `--restore` son opciones excluyentes. Las opciones de instalación no se aceptan en `backscroll skill` sin una operación.

`HOME` debe ser un directorio absoluto existente. El valor predeterminado es el hogar del usuario actual.

`CONFIG_HOME` debe ser absoluto. El valor predeterminado es `XDG_CONFIG_HOME` cuando la variable está definida. Esa variable debe contener una ruta absoluta. Cuando la variable no está definida, el valor es `<home>/.config`.

## Contrato de salida del skill

`backscroll skill` escribe en stdout los bytes exactos de las instrucciones autoritativas incorporadas. No añade cabeceras ni formato. Deja stderr vacío cuando tiene éxito.

`backscroll --skill` conserva los mismos bytes exactos que produce hoy. Su stdout es idéntico al de `backscroll skill`. Deja stderr vacío. No produce efectos secundarios.

El alias se mantiene durante toda la versión mayor que incorpore `backscroll skill`. Solo una versión mayor posterior puede retirarlo.

El shim nuevo puede delegar en `backscroll skill`. La compatibilidad no exige cambiar de inmediato un shim ya instalado que delegue en `backscroll --skill`.

## Aislamiento de inicio

El despachador reconoce todas las formas de `skill` antes del inicio ordinario.

Ninguna forma de `skill` carga `backscroll.toml` ni la configuración global. Ninguna forma abre, crea, inspecciona o migra SQLite. Ninguna forma descubre entradas ni ejecuta sincronización. Ninguna forma inicia o espera la actualización automática.

`backscroll skill` tampoco consulta el hogar del usuario. `--install` y `--restore` solo consultan las rutas necesarias para su operación de archivos.

Los errores de argumentos ocurren antes de crear estado. Una prueba con configuración inválida, una ruta de base de datos inexistente y entradas inexistentes debe demostrar este límite.

## Copia gestionada del shim

El binario incorpora el árbol completo del shim. La instalación materializa una copia gestionada en esta ruta:

```text
<home>/.local/share/backscroll/skill/backscroll/<tree_sha256>/backscroll
```

`tree_sha256` usa la representación de preimagen definida en esta ADR. El directorio contiene solo archivos incorporados por el binario. La implementación escribe archivos con modos fijos. Después marca los archivos como `0444` y los directorios como `0555`. La implementación trata el árbol como inmutable y nunca lo actualiza en el mismo lugar.

La implementación crea el árbol en un directorio temporal hermano. Hace `fsync` de cada archivo y directorio. Después renombra el directorio temporal a su nombre final. Este renombrado publica el árbol de forma atómica. La implementación hace `fsync` del directorio padre después del renombrado.

Un árbol existente debe coincidir en contenido, tipo y modo. La implementación rechaza un árbol gestionado que tenga cambios.

Los tres destinos apuntan mediante enlaces simbólicos absolutos a la misma copia gestionada:

| Consumidor | Destino |
| --- | --- |
| Claude Code | `<home>/.claude/skills/backscroll` |
| Agents y Codex | `<home>/.agents/skills/backscroll` |
| OpenCode | `<config-home>/opencode/skills/backscroll` |

La instalación gestiona solo estas tres entradas. No recorre otros nombres ni otros directorios. No elimina `backscroll-doctor`.

Una entrada es idéntica cuando es un enlace léxico a la copia gestionada actual y el árbol gestionado es válido. Una instalación con tres entradas idénticas devuelve `changed: 0`. No crea un recibo, un diario, un respaldo ni un archivo de bloqueo nuevo.

Una entrada ausente se puede instalar sin digest de aprobación. Una entrada gestionada de una versión anterior se puede actualizar sin digest de aprobación. La implementación respalda la entrada anterior antes de actualizarla.

Cualquier otra entrada es contenido no reconocido. Esta categoría incluye archivos, directorios, enlaces rotos y enlaces a un clon. La implementación no interpreta el texto de `SKILL.md` para declarar propiedad.

Después de una aprobación válida, la instalación retira cada entrada diferente de los tres destinos activos. Primero mueve cada preimagen a su respaldo. Después publica el enlace a la copia gestionada.

La operación trata los tres destinos como una transacción. Si un destino contiene contenido no reconocido, una ejecución sin `--approve` no cambia ningún destino. La ejecución imprime el inventario y su digest de aprobación. El operador repite la misma orden con `--approve SHA256`.

El digest cubre los tres destinos. También cubre el hogar, el directorio de configuración, el origen incorporado y cada preimagen. Un cambio en cualquier valor invalida la aprobación.

## Inventario y preimagen

La implementación usa `lstat`. Nunca sigue un enlace al inventariar un destino.

La preimagen usa estas formas JSON:

```json
{"kind":"absent"}
{"kind":"link","target":"lexical-target"}
{"kind":"file","mode":384,"sha256":"hex"}
{"kind":"directory","mode":448,"entries":{"name":{"kind":"file","mode":384,"sha256":"hex"}}}
```

`mode` contiene solo los bits de permiso. `entries` contiene cada hijo de forma recursiva. Los nombres se ordenan para calcular el digest.

La implementación rechaza un archivo regular cuando su número de enlaces físicos no es uno. También rechaza sockets, dispositivos, tuberías y otros archivos especiales.

La implementación rechaza un ancestro de destino, estado, recibo o copia gestionada que sea un enlace simbólico. También rechaza un ancestro existente que no sea un directorio.

La implementación rechaza solapamientos entre destinos, estado, copia gestionada y respaldos. También rechaza destinos duplicados.

## Aprobación

La implementación construye primero un plan v1. El campo `approval` es el SHA-256 del plan sin el propio campo `approval`.

La serialización canónica conserva el algoritmo de Python v1:

```text
sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode("utf-8"))
```

La implementación nativa debe reproducir el escape ASCII predeterminado de `json.dumps`. Esta regla mantiene el digest para rutas con caracteres no ASCII.

La implementación compara `--approve` antes de crear estado operativo. Después obtiene el bloqueo y repite el inventario. Cualquier diferencia detiene la operación antes del primer reemplazo.

La salida de una solicitud de aprobación es JSON. Contiene `approval_required: true` y el plan completo. Esta salida termina con código cero porque no ocurrió un fallo ni una mutación.

## Bloqueo y estado durable

La implementación conserva esta raíz de estado para mantener compatibilidad:

```text
<home>/.local/state/backscroll/skill-install
```

La raíz tiene modo `0700`. El archivo `lock` tiene modo `0600`. La implementación toma un bloqueo asesor exclusivo por hogar. La implementación falla si otro proceso conserva el bloqueo.

Cada cambio usa un directorio de transacción con un identificador aleatorio. El directorio contiene `receipt.json`, `events.jsonl` y los respaldos `backup-N`.

El recibo se escribe antes del primer cambio. La implementación hace `fsync` del recibo, del directorio de transacción y de la raíz de estado. Después añade un evento `prepared` que incluye `restore_approval`. La implementación hace `fsync` del diario después de cada evento.

Antes de cada cambio, la implementación añade `install-intent`. Después mueve la preimagen a `backup-N` mediante un renombrado en el mismo sistema de archivos. Hace `fsync` del directorio de respaldo y del padre del destino.

La implementación crea un enlace temporal en el directorio padre del destino. Después renombra el enlace temporal al nombre final. Este renombrado publica el reemplazo de forma atómica. El destino nunca expone un árbol parcial. La implementación hace `fsync` del padre después del renombrado. Después añade `installed` al diario.

Los respaldos y su destino deben estar en el mismo sistema de archivos. La implementación rechaza la transacción antes de cambiar destinos cuando esta condición no se cumple.

El diario usa los eventos v1 existentes: `prepared`, `install-intent`, `installed`, `failed`, `complete`, `restore-intent`, `restored` y `restore-complete`. Cada línea es un objeto JSON completo. El diario solo crece.

## Recibo v1

La implementación nativa lee y escribe el formato Python v1. No cambia el significado de una preimagen.

```json
{
  "version": 1,
  "plan": {
    "version": 1,
    "action": "install",
    "home": "/absolute/home",
    "config_home": "/absolute/config",
    "source": {
      "root": "/absolute/home/.local/share/backscroll/skill/backscroll",
      "path": "/absolute/home/.local/share/backscroll/skill/backscroll/TREE/backscroll",
      "commit": "native:SHIM_SHA256",
      "tree_sha256": "TREE"
    },
    "targets": [
      {
        "path": "/absolute/destination",
        "before": {"kind": "absent"},
        "change": true
      }
    ],
    "approval": "PLAN_SHA256"
  },
  "run": "TRANSACTION_ID"
}
```

El valor `source.commit` distingue el origen nativo. El lector v1 no usa este campo para restaurar. Los recibos Python conservan su SHA de commit Git sin conversión.

`restore_approval` es el digest canónico del recibo completo. No se guarda dentro del recibo. La salida de una instalación aplicada contiene `changed`, `receipt` y `restore_approval`.

Un no-op contiene `changed: 0`. También contiene `receipt: null` y `restore_approval: null`.

El lector nativo acepta recibos Python v1 con acciones `install` y `uninstall`. Valida `version`, `run`, `home`, `config_home`, destinos, preimágenes, respaldos y digest. No exige que el clon indicado por `source.path` todavía exista.

El instalador Python puede restaurar un recibo nativo v1 porque la ubicación, la forma JSON, el digest y las preimágenes permanecen compatibles.

## Reversión y restore

`backscroll skill --restore RECEIPT` exige `--approve RESTORE_SHA256`. El recibo debe estar dentro de la raíz de estado del hogar indicado. El identificador `run` debe coincidir con el directorio del recibo.

Restore valida todos los destinos y todos los respaldos antes de cambiar uno. Acepta solo el estado posterior esperado, el estado ausente durante una interrupción o el estado anterior ya restaurado. Rechaza cambios del operador en un destino o respaldo.

Restore elimina solo el enlace posterior exacto. Después mueve cada respaldo a su destino. También restaura una preimagen ausente a ausencia. Hace `fsync` del padre del destino y del directorio de transacción después de cada paso.

Restore es idempotente. Una segunda ejecución válida devuelve `restored: 0`.

Si la instalación falla después de cambiar un destino, la misma operación ejecuta restore sobre todos los destinos cambiados. Si la reversión encuentra una diferencia, se detiene. Conserva el recibo, los respaldos y el diario. El error informa la ruta del recibo y `restore_approval`.

La implementación no elimina respaldos de forma automática. Una política de retención queda fuera de esta decisión.

## Instalación de presets

El binario incorpora los cinco manifiestos `*.inputs.toml` propiedad de Backscroll:

```text
claude.inputs.toml
pi.inputs.toml
decisions.inputs.toml
opencode.inputs.toml
codex.inputs.toml
```

`backscroll presets install` instala los archivos ausentes en `<config-home>/backscroll/inputs`.

La operación conserva cada archivo existente de forma predeterminada. Esta regla se aplica aunque el contenido difiera del preset incorporado.

`--overwrite` autoriza la sustitución de los cinco nombres gestionados. La operación no modifica otros archivos. Cada escritura usa un archivo temporal hermano, `fsync`, renombrado y `fsync` del directorio padre.

La operación no compila ni instala el binario. `install.sh` permanece como bootstrap de Unix. Después de instalar el binario, el script ejecuta `backscroll presets install`. Si el operador solicita la compatibilidad de sobrescritura del bootstrap, el script traduce esa solicitud a `--overwrite`.

Los hooks `pre-push` y `post-merge` dejan de instalar presets después de que las pruebas nativas demuestren paridad. Los hooks permanecen fuera de la CLI del producto.

La instalación de presets es distinta de la instalación del skill. Ninguna operación implica la otra.

## Migración

La implementación sigue este orden:

1. Añade el contrato nativo y pruebas herméticas en Go.
2. Demuestra paridad en Linux y macOS con hogares temporales.
3. Mantiene la prueba manual con runtimes externos fuera de la CLI pública.
4. Cambia `install.sh` para invocar la operación nativa de presets.
5. Retira la mutación de presets de los hooks.
6. Retira `scripts/install-skills.py` y sus pruebas Python solo después de demostrar compatibilidad v1.

La migración revisa los scripts de aceptación existentes. Porta a Go cada aserción vigente antes de retirar un script.

## Validación obligatoria de la implementación

Las pruebas deben cubrir contenido idéntico, ausente, diferente y no reconocido. También deben cubrir aprobación correcta e incorrecta.

Las pruebas deben cubrir ancestros simbólicos, enlaces físicos, archivos especiales, solapamientos y sistemas de archivos incompatibles.

Las pruebas deben cubrir bloqueo, diario, recibo, cada punto de `fsync`, reemplazo, fallo parcial, reversión, idempotencia y restore.

Las pruebas deben restaurar un recibo producido por Python v1. También deben verificar que Python v1 puede restaurar un recibo nativo v1 durante la transición.

Las pruebas deben verificar los bytes exactos de stdout y stderr vacío para `backscroll --skill`. También deben demostrar la ausencia de configuración, SQLite, sincronización, autoupdate y cambios de archivos.

Las pruebas de presets deben verificar conservación predeterminada y sobrescritura explícita. Ninguna prueba hermética usa el hogar real ni un runtime externo.

## Consecuencias

El skill instalado queda ligado al binario que lo materializó, no a un clon mutable. Los tres consumidores observan una sola copia gestionada.

La CLI adquiere una operación de archivos sensible. Esa operación debe mantener el contrato durable del instalador Python.

Los recibos v1 permiten restaurar instalaciones anteriores durante la migración. La historia de ADR 0008 permanece intacta.

La primera fase no añade desinstalación pública. Restore cubre la recuperación de una transacción. Una operación de desinstalación futura requiere otra decisión.
