---
tipo: adr
estado: accepted
fecha: 2026-08-20
contexto: Las guías operativas vigentes conservaron comandos y flags eliminados porque el contrato Cobra solo validaba el skill de Backscroll.
decision: Mantener una lista explícita de documentación CLI vigente y validarla estáticamente contra buildRootCmd, excluyendo registros históricos declarados.
consecuencias: La CI detectará deriva en ejemplos ejecutables con archivo y línea; al agregar una guía operativa deberá incorporarse a la frontera, mientras documentos históricos conservarán su contexto original.
---

# Declarar la frontera de documentación CLI vigente

## Contexto

El árbol Cobra es la fuente de verdad del CLI público. El contrato existente comprobaba únicamente el skill distribuido, por lo que otras guías orientadas a usuarios podían conservar comandos o flags retirados y presentarlos como workflows actuales.

Validar todos los archivos Markdown no es correcto: planes, especificaciones, investigaciones, evaluaciones históricas y roadmaps registran superficies anteriores de forma intencional.

## Decisión

Se declara en el test contractual una lista explícita de guías vigentes. Sus invocaciones de Backscroll se analizan estáticamente y se comparan con `buildRootCmd`; los snippets nunca se ejecutan y las violaciones informan archivo y línea.

Los directorios y archivos históricos se excluyen de forma explícita junto a esa lista. Las menciones narrativas de comandos se distinguen de los snippets ejecutables para evitar falsos positivos.

## Alternativas descartadas

- **Validar todo Markdown:** obligaría a reescribir registros históricos y eliminaría contexto útil.
- **Ejecutar snippets:** introduciría efectos laterales, dependencias del entorno y pruebas no herméticas.
- **Mantener un segundo catálogo manual del CLI:** duplicaría Cobra y volvería a crear la misma deriva.
- **Aplicar reemplazos globales:** comandos como autosync y `rebuild` no tienen semánticas intercambiables.

## Consecuencias

Las guías vigentes fallarán en CI cuando documenten comandos o flags inexistentes. Añadir una nueva guía operativa exige incorporarla a la frontera declarada. El parser permanece deliberadamente pequeño y puede requerir nuevos casos sintéticos cuando aparezcan formas Markdown distintas, pero Cobra continúa siendo la única fuente de verdad del CLI.
