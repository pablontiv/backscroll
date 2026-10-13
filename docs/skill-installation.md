# Instalación del skill de Backscroll

Backscroll distribuye un solo skill oficial. Su nombre es `backscroll`.

El archivo `.claude/skills/backscroll/SKILL.md` es un shim de descubrimiento. El archivo `cmd/backscroll/backscroll_skill.md` contiene las instrucciones autoritativas. El binario incorpora ambos artefactos.

`backscroll skill` imprime las instrucciones autoritativas en stdout. `backscroll --skill` conserva los mismos bytes por compatibilidad. Ambos comandos dejan stderr vacío cuando terminan correctamente. Ambos comandos omiten la configuración, SQLite, la sincronización y la actualización automática.

## Plataformas y destinos compatibles

`backscroll skill --install` y `backscroll skill --restore` admiten macOS y Linux. Windows rechaza ambas operaciones antes de modificar archivos o directorios.

La instalación gestiona estas entradas:

| Consumidor | Destino |
| --- | --- |
| Claude Code | `<home>/.claude/skills/backscroll` |
| Agents y Codex | `<home>/.agents/skills/backscroll` |
| OpenCode | `<config-home>/opencode/skills/backscroll` |

Los tres destinos son enlaces simbólicos absolutos. Los tres enlaces apuntan a una copia inmutable que gestiona el binario. La copia está bajo `<home>/.local/share/backscroll/skill/backscroll/`.

La operación no gestiona `backscroll-doctor`. La operación no recorre otros nombres. La operación no necesita Python, Git ni un clon del repositorio.

## Instalar

Use un hogar absoluto existente. Puede indicar también el hogar de configuración de OpenCode.

```bash
backscroll skill --install \
  --home /ruta/absoluta/al/hogar \
  --config-home /ruta/absoluta/a/config
```

`--home` usa el hogar del usuario cuando se omite. `--config-home` usa `XDG_CONFIG_HOME` cuando la variable contiene una ruta absoluta. En otro caso usa `<home>/.config`.

Una instalación limpia crea la copia gestionada y los tres enlaces. Una instalación idéntica devuelve `changed: 0`. Esta operación no crea otro recibo.

Una versión gestionada anterior se actualiza sin aprobación. El instalador conserva el enlace anterior en el respaldo de la transacción.

## Revisar y aprobar contenido desconocido

El instalador usa `lstat`. El instalador no sigue un enlace al inventariar un destino. El instalador considera desconocido cualquier archivo, directorio o enlace que no apunte a una copia gestionada válida.

Una ejecución sin `--approve` no cambia ningún destino desconocido. La ejecución devuelve `approval_required: true`, el plan completo y el digest `plan.approval`.

Revise el hogar, el hogar de configuración, la copia gestionada y las tres preimágenes. Después repita la misma orden con el digest aprobado.

```bash
backscroll skill --install \
  --home /ruta/absoluta/al/hogar \
  --config-home /ruta/absoluta/a/config \
  --approve SHA256_APROBADO
```

Un cambio en una ruta o preimagen invalida el digest. Un digest incorrecto no crea estado operativo. La aprobación autoriza el respaldo y la sustitución de las tres entradas descritas en el plan.

El instalador rechaza ancestros simbólicos, destinos solapados, enlaces físicos y archivos especiales. El instalador toma un bloqueo exclusivo por hogar. El instalador escribe el recibo y el evento `prepared` antes de sustituir destinos. Cada evento se añade a `events.jsonl` y se sincroniza en disco.

El instalador publica cada enlace mediante una sustitución atómica. Un fallo posterior inicia la reversión de los destinos ya cambiados. El instalador conserva el recibo, el diario y los respaldos.

## Recibos y restore

Una instalación con cambios devuelve estos campos:

```json
{
  "changed": 3,
  "receipt": "/ruta/al/receipt.json",
  "restore_approval": "SHA256"
}
```

El estado privado está bajo `<home>/.local/state/backscroll/skill-install/`. Cada transacción conserva `receipt.json`, `events.jsonl` y los respaldos `backup-N`.

Guarde `receipt` y `restore_approval`. Use ambos valores para restaurar la preimagen:

```bash
backscroll skill \
  --restore /ruta/absoluta/al/receipt.json \
  --approve SHA256_DE_RESTORE \
  --home /ruta/absoluta/al/hogar
```

Restore valida todos los destinos y todos los respaldos antes de cambiar uno. Restore no sobrescribe trabajo nuevo del operador. Restore es idempotente. Una segunda ejecución válida devuelve `restored: 0`.

La implementación nativa escribe recibos v2. La implementación restaura recibos nativos v2 y recibos Python v1 existentes. El clon registrado por un recibo Python v1 puede no existir durante restore. La ruta Python ya no participa en instalaciones nuevas.

## Instalar presets

La instalación de presets es una operación separada.

```bash
backscroll presets install
```

La operación instala estos seis manifiestos incorporados:

- `claude.inputs.toml`
- `pi.inputs.toml`
- `omp.inputs.toml`
- `decisions.inputs.toml`
- `opencode.inputs.toml`
- `codex.inputs.toml`

El destino es `<config-home>/backscroll/inputs`. Use una ruta explícita cuando sea necesario:

```bash
backscroll presets install --config-home /ruta/absoluta/a/config
```

La operación conserva todos los archivos existentes por defecto. La operación no modifica otros nombres. Use `--overwrite` para sustituir de forma explícita los seis nombres gestionados.

```bash
backscroll presets install --config-home /ruta/absoluta/a/config --overwrite
```

`install.sh` instala el binario y después ejecuta esta operación nativa. `BACKSCROLL_CONFIG_DIR` se traduce a `--config-home`. `BACKSCROLL_FORCE_INPUTS=1` se traduce a `--overwrite` para conservar la compatibilidad del bootstrap.

Los hooks de Git no instalan skills ni presets. Los hooks solo aplican las comprobaciones del repositorio y la actualización de agregados de documentación.

## Verificación

Compruebe los enlaces y el shim:

```bash
readlink "$HOME/.claude/skills/backscroll"
readlink "$HOME/.agents/skills/backscroll"
cat "$HOME/.claude/skills/backscroll/SKILL.md"
backscroll skill
```

El shim debe indicar `backscroll --skill`. El shim no debe contener el manual completo.

Ejecute las pruebas herméticas:

```bash
just test-skills
```

Las pruebas usan hogares temporales en Linux y macOS. Las pruebas no modifican el hogar real. La prueba opcional `tests/skill_runtime_probe.py` comprueba runtimes externos instalados en macOS. Esa prueba no forma parte de CI y no invoca modelos.
