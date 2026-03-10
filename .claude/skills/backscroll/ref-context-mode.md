# Backscroll --context Mode Reference

Requiere rootline y context-save. Verificar disponibilidad:
```bash
command -v rootline 2>/dev/null
```

Si rootline está disponible y `.claude/session-state/` existe con un schema `.stem`:

```bash
# Contextos de sesión guardados para este proyecto
rootline query .claude/session-state/ --where "proyecto == '$(basename $(pwd))'" --output table

# Contexto más reciente
rootline query .claude/session-state/ --where "proyecto == '$(basename $(pwd))'" --output table --limit 1
```

Mostrar también el estado actual de artefactos R&D y planificación:

```bash
# Líneas de investigación activas
rootline query lines/ --where 'tipo == "question"' --output table 2>/dev/null

# Investigaciones activas
rootline query . --where 'metodo == "hypothesize"' --output table 2>/dev/null

# Progreso del roadmap (si configurado)
if [ -f .claude/roadmap.local.md ]; then
  ROADMAP_ROOT=$(grep 'roadmap-root:' .claude/roadmap.local.md | awk '{print $2}')
  rootline stats "$ROADMAP_ROOT" --output table 2>/dev/null
  rootline tree "$ROADMAP_ROOT" --output table 2>/dev/null
fi

# Teorías
rootline query theories/ --output table 2>/dev/null
```

Presentar datos de session-state junto con datos live de rootline para una imagen completa de recuperación: qué se discutió (session-state) y dónde está el proyecto ahora (rootline live).
