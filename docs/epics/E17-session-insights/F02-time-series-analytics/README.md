# F02: Time-Series Analytics

**Epic**: [E17 Session Insights](../README.md)
**Objetivo**: Proveer comando `insights` con agregaciones temporales sobre el corpus indexado
**Satisface**: P2 (actividad por dia, distribucion de categorias)
**Milestone**: `backscroll insights` muestra tabla con sesiones/dia y distribucion de tags

## Invariantes

- INV1: Datos derivados de queries SQL sobre tablas existentes (search_items + session_tags)
- INV2: Output --robot/--json consistente con otros subcomandos
- INV3: Zero dependencias nuevas

## Stories

| Story | Descripcion |
|-------|-------------|
| S077 | Queries de agregacion: sesiones por dia, mensajes por hora, projects por actividad |
| S078 | Distribucion de tags: porcentaje de sesiones por categoria |
| S079 | Subcomando `insights` con output text/json/robot |
| S080 | Tests: queries de agregacion sobre corpus de test |
