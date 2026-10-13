> **Untriaged audit archive — not a current defect register.** This file preserves
> the original untracked `backscroll-audit-findings.md` from the main worktree,
> captured on 2026-10-13. Its assertions have not all been reproduced or checked
> against the current implementation; some may be obsolete. No build, test,
> lint, formatter, or smoke check was run while preserving this report.
>
> Review this artifact as research evidence, not as a product contract or an
> implementation PR. Revalidate and deduplicate each actionable finding before
> filing a focused bug report with reproduction steps, expected behavior, and
> version/environment evidence, as required by `CONTRIBUTING.md` and
> `.github/ISSUE_TEMPLATE/bug_report.md`. Security findings follow `SECURITY.md`.
>
> The other untracked main-worktree artifact, `.pi/synagent.json`, contained
> exactly the empty JSON object `{}`. This preserves its configuration payload
> without turning local agent runtime state into a repository configuration
> contract. Both original main-worktree files were left untouched because that
> worktree has a live session.
>
> The original content follows verbatim. Original UTF-8 SHA-256:
> `8bd44323205d5aee630ec5e6a6d5c76526f24a7babd86687c667d182bde592f7`.

# Backscroll audit findings

- `newSearchCmd`, `validateAndParseSearchRequest`, `paginate`: `--limit -1` causa panic; `--json --robot` se acepta con formatos inconsistentes entre startup y handler; faltan validación y pruebas.
- `newSearchCmd`, `runSearch`: README, skill y `docs/search.md` omiten `--lexical-only` y `--similarity-threshold`.
- `prepareIndex`, `SetEmbeddingProvider`, `HybridSearch`: producción nunca conecta el proveedor de embeddings; la búsqueda híbrida pública es inaccesible; resultados exclusivamente vectoriales se descartan; falta prueba CLI.
- `runList`, `ListItemsV2`: las opciones V2 incluyen documentos que no son sesiones; `--offset` sin límite genera SQL inválido; `--order` inválido se acepta; faltan pruebas.
- `newListCmd`, `runList`: `--json --robot` se acepta; la salida robot no tiene un esquema documentado; las ramas text y robot ignoran errores de escritura.
- `runPatterns`, `AggregateCorrections`, `LoadToolSequences`: `--tag` se acepta para correcciones y secuencias, pero no se aplica; faltan pruebas.
- `newPatternsCmd`, `validatePatternsRequest`: aceptan `--min-support -1`, `--min-confidence 2` y `--json --robot`; faltan validaciones y pruebas.
- `AggregateCommands`, `AggregateFailures`, `AggregateCommandsTrend`, `AggregateFailuresTrend`, `AggregateCorrections`, `runPatterns`: `--limit 0 --offset N` produce SQL inválido o semánticas diferentes según el tipo.
- `AggregateCommands`, `AggregateCommandsTrend`: fallan al escanear `command_head NULL` en un `string`; falta una prueba con valores nulos.
- `AggregateFailuresTrend`, `runPatterns`: cada semana recibe una cobertura global incorrecta; la salida robot reinicia `result_N` en cada semana; las pruebas no verifican estos valores.
- `validateAnnotateRequest`, `UpsertAnnotation`: aceptan una UUID con ruta parcial conflictiva y coordenadas ambiguas; contradicen el contrato documentado; faltan pruebas.
- `AggregateTemplates`: la paginación no tiene un desempate estable cuando varias plantillas comparten frecuencia; falta prueba de estabilidad.
- `AggregateFailures`, `runPatterns`: la documentación afirma que se cuentan eventos sin señal, pero producción no devuelve ese total.
- `newPatternsCmd`: README, skill y `docs/patterns.md` omiten `--after` y `--before` para secuencias.
- `newConfigCmd`, `newStatusCmd`, `newValidateCmd`, `newRebuildCmd`: aceptan argumentos posicionales no documentados y llegan a startup; faltan pruebas de rechazo.
- `runConfig`: la salida textual descarta errores de escritura; un pipe cerrado puede terminar con éxito y salida incompleta.
- `newConfigCmd`, `runConfig`: la skill no documenta `backscroll config [--json]`.
- `newRecoverCmd`: README, skill y documentación operativa omiten o muestran incorrectamente `recover --from PATH [--dry-run]`, el backup, el post-sync y los fallos parciales.
- `buildRootCmdWithStartup`, `runOwnedStartup`: README y skill afirman que todo comando operacional ejecuta sync previo, pero `recover` omite preparación y sync por diseño.
- `runValidate`: README afirma que `validate` muestra inputs configurados, pero la función solo informa la integridad del índice.
- `runStatus`: la skill afirma que `status` muestra la versión desplegada, pero la función no la emite.
- `InputsDir`: ignora `XDG_CONFIG_HOME` y rutas nativas de macOS y Windows; la skill y `docs/configuration.md` contradicen el comportamiento; falta prueba contractual.
- `LoadInputsFromDir`, `loadFile`, `ActiveInputs`: no validan versión, identificador, fuente, raíces, patrones ni formato; manifests incompletos pueden activar Claude o `session` silenciosamente; faltan pruebas.
- `defaultStartupPolicy`, `maybeAutoSync`: un decoder no registrado supera el preflight, crea la base y termina como `index_stale` con recomendación incorrecta de recovery.
- `Load`: la documentación y el comentario no reflejan la prioridad real del entorno sobre configuración local y global; falta una prueba de colisión.
- `newFileLock`: acepta un sidecar existente con permisos `0644` y no lo corrige a `0600`; contradice el contrato documentado; falta prueba.
- `LoadGlobalRegistry`, `loadRegistryFrom`: convierten errores de lectura y TOML inválido en un registro vacío; startup continúa con identidades derivadas; el modo degradado no está documentado.
- `NormalizeRootEquivalence`, `isCrossHostEquivalent`: pueden asociar rutas no relacionadas que compartan dos componentes; faltan pruebas de colisiones.
- `maybeAutoSync`, `startupDiags`: `Total` empieza después del lock y preparación, pero suma esas fases como tiempo medido; `Unattributed` puede quedar oculto; faltan pruebas de contabilidad.
- `run`, `newUpdater`: descartan errores de aplicación y descarga del autoupdate; las pruebas solo usan builds `dev`; README y skill omiten la espera de hasta diez segundos y la ausencia de opt-out.
- `install_input_presets`: README omite `decisions.inputs.toml` del inventario y de la copia manual.
- `newPurgeCmd`, `validatePurgeBefore`: la skill omite `purge`; README y ayuda no documentan que `--before` acepta RFC3339.
- `buildRootCmdWithStartup`: README no documenta `backscroll --version`; las pruebas de help no exigen toda la superficie pública.
- `walkGlob`, `DiscoverFiles`: `follow_symlinks = true` no recorre directorios enlazados; la prueba actual produce un falso positivo.
- `OpenCodeReader.Hash`: usa solo `MAX(message.time_updated)`; pierde filas con el mismo máximo y cambios exclusivos de `part.time_updated`; faltan pruebas.
- `maybeAutoSync`, `contentHashesEqual`, `semanticsForClaim`: cambios de `source`, formato u opciones del manifest no invalidan el hash; los metadatos almacenados quedan obsoletos.
- `extractClaudeMessages`, `piTimestamp`: timestamps ausentes o inválidos se sustituyen por `time.Now()`; una repetición puede cambiar el timestamp del mismo registro; faltan pruebas.
- `OpenCodeReader.Parse`: ignora `session_id`, no asigna `ParsedFile.Cwd` y agrupa toda la base en un solo archivo lógico; proyecto y sesión quedan mal identificados; faltan pruebas y documentación.
- `selectReplayPaths`, `EmptyIndexedPaths`: solo reanalizan rutas vacías de Pi; rutas vacías de Claude, Codex, OpenCode o Markdown no reciben mejoras del parser sin cambios de contenido.
- `ParsePlan`, `DiscoverPlanFiles`: no tienen trayectoria desde la aplicación; los planes usan `MarkdownSectionsReader`; sus pruebas no validan integración.
- `ParseSessions`, `WalkSessionDirs`, `GetNoisePatterns`: son trayectorias antiguas inaccesibles desde la aplicación; solo las invocan pruebas.
- `ParseAll`: es una trayectoria Markdown antigua inaccesible; producción usa `ParseDocument` y `ParseSectioned`.
- `OpenCodeReader.Name`: `inputs/opencode.inputs.toml` recomienda el comando inexistente `backscroll sync`.
- `MarkdownDocumentReader.Name`, `MarkdownSectionsReader.Name`: README y skill no identifican ambos formatos públicos de decodificación.
- `noClobberMove`: en plataformas distintas de Darwin y Linux siempre devuelve `noClobberUnsupportedError`; Windows puede ejecutar dry-run, pero no aplicar recovery; falta prueba y documentación.
- `OpenCompatible`, `snapshotDatabase`, `pruneSnapshots`: README, skill y documentación operativa no explican nombre, ubicación ni retención de dos snapshots de migración.
- `(*Database).RebuildFTS`: el `rebuild` de FTS5 mezcla filas tool en `messages_fts` y filas text en `tool_fts`; altera la separación de tokenizadores; faltan prueba y documentación correctas.
- `(*Database).Purge`: un registro purgado reaparece cuando una fuente activa cambia y vuelve a sincronizarse; las pruebas solo verifican la eliminación inmediata.
- `(*Database).Validate`: no comprueba correspondencia ni routing entre `search_items` y los índices FTS; puede devolver `valid=true` con filas no buscables.
- `openWriteConnectionWithPragmas`: activa `foreign_keys` en una sola conexión del pool; conexiones posteriores pueden usar `foreign_keys=0` y dejar metadatos huérfanos.

# Repeated trajectory findings

- `[EXACTA]` `newSearchCmd`, `newContextCmd`, `newListCmd`, `newPatternsCmd`, `newAnnotateCmd`, `newPurgeCmd`, `newRebuildCmd`, `newConfigCmd`, `newStatusCmd`, `newValidateCmd`: repiten la obtención y validación de `startup.Config`; `newRecoverCmd` mantiene una excepción de remediación.
- `[DIVERGENTE]` constructores de `list`, `patterns`, `annotate`, `context`, `purge` y `recover`: repiten el rechazo de argumentos posicionales con implementaciones y mensajes distintos.
- `[DIVERGENTE]` `runSearch`, `runList`, `runPatterns`: resuelven `--project` y `--all-projects`; solo `patterns` rechaza la combinación.
- `[DIVERGENTE]` `validateAndParseSearchRequest`, `validatePurgeBefore`, `runPatterns`: implementan tres contratos de fecha distintos para `--after` y `--before`.
- `[DIVERGENTE]` `runSearch`, `runContext`, `runList`, `runPatterns`, `refuseIndexWithCause`: seleccionan text, JSON y robot; solo `context` rechaza `--json --robot`, y startup usa una precedencia distinta.
- `[DIVERGENTE]` `runSearch`, `runList`, `runPatterns`: repiten la representación de resultados vacíos y la emisión de hints con esquemas distintos.
- `[DIVERGENTE]` handlers de CLI: algunas ramas propagan errores de escritura y otras los descartan. `runList`, `runStatus`, `runConfig`, `runValidate`, `runPatterns`, `runAnnotate`, `runPurge`, `runRebuild` y `printRecoveryReport` pueden terminar con éxito tras una escritura incompleta.
- `[JUSTIFICADA]` `refuseIndexWithCause`, `refuseDiagnostics`: repiten el diagnóstico estructurado para uno o varios problemas, con diferencias en robot, causas y JSON.
- `[DIVERGENTE]` `defaultStartupPolicy`, `maybeAutoSync`, `runConfig`, `resolveInputsForStatus`: cargan manifests hasta tres veces. Validación, sync y salida pueden observar conjuntos distintos.
- `[DIVERGENTE]` `canonicalDatabasePath`, `resolveActiveIndexPath`, `canonicalizeDBPath`, `canonicalizeExistingDBPath`: canonizan el mismo path de base con reglas distintas para padres inexistentes y enlaces simbólicos.
- `[DIVERGENTE]` `maybeAutoSync`, `runRebuild`, `ReresolveProjectsWithRegistry`, `Identify`: asignan proyectos mediante CWD, paths Claude decodificados y normalización repetida. La indexación y el rebuild pueden producir identidades distintas.
- `[REPETIDA]` `runOwnedStartup`, `maybeAutoSync` y handlers que llaman a `prepareIndex`: abren y validan la misma base hasta tres veces por comando.
- `[DIVERGENTE]` `ValidateNoLegacySources`, `legacySourcesInFile`, `Load`, `loadConfigFile`: leen y analizan dos veces los mismos archivos de configuración con contratos distintos para `[sources]`.
- `[JUSTIFICADA]` `validateCommandBeforeStartup`, `PersistentPreRunE` y validadores dentro de handlers: repiten validación para impedir efectos tempranos y proteger llamadas directas a `run*`.
- `[JUSTIFICADA]` métodos `Discover` de Claude, Pi, Codex, OpenCode y Markdown: repiten el contrato `SessionReader` y delegan en `DiscoverFiles`.
- `[ANTIGUA]` `plans.DiscoverPlanFiles`: duplica el descubrimiento activo de Markdown con menos capacidades y sin caller de producción.
- `[ACTIVA]` métodos `Hash` y `Parse` de todos los lectores: `maybeAutoSync` calcula el hash antes del parseo y cada `Parse` vuelve a calcularlo.
- `[ACTIVA]` `ClaudeReader.Parse`, `PiReader.Parse`, `CodexReader.Parse`: repiten el bucle JSONL mediante `IterateJSONLFile`; la decodificación nativa difiere por formato.
- `[ANTIGUA]` `sync.ParseSessions`: duplica el bucle JSONL de Claude y Pi, pierde metadatos modernos y no tiene caller de producción.
- `[DIVERGENTE]` `extractClaudeMessages`, `extractPiMessages`, `codexMessage`, `OpenCodeReader.Parse`: repiten normalización y clasificación textual con separadores y limpieza distintos.
- `[ANTIGUA]` `sync.extractContent`, `extractFromBlocks`, `classifyContentType`: duplican la normalización activa y solo sirven a `ParseSessions`.
- `[ANTIGUA]` `IsNoiseType`, `IsNoiseRecord`, `GetNoisePatterns`: mantienen listas o reglas paralelas de ruido; `GetNoisePatterns` no tiene caller de producción.
- `[JUSTIFICADA]` extractores Claude, Pi, Codex y OpenCode: repiten la adaptación de herramientas, pero centralizan el formato en `SerializeToolInput` y `SerializeToolOutput`.
- `[DIVERGENTE]` `extractClaudeMessages`, `piTimestamp`, `codexMessage`, `OpenCodeReader.Parse`, `parseMarkdownFile`: aplican cinco políticas de timestamp; Claude y Pi usan `time.Now()` ante errores y Codex descarta el registro.
- `[DIVERGENTE]` `messageOriginForRole`, `piMessageOrigin`, `openCodeOriginForRole`, `claudeRecordOrigin`, `codexMessage`: repiten el mapeo de roles; OpenCode trata `system` y `tool` de forma distinta.
- `[JUSTIFICADA]` `extractPiReasoning`, `codexMessage`: repiten el contrato de razonamiento opcional con formatos nativos distintos.
- `[DIVERGENTE]` segundo recorrido de `ClaudeReader.Parse` y `CodexReader.Parse`: enlazan llamadas y resultados; Claude propaga error y código de salida, mientras Codex solo propaga `SearchEcho`.
- `[DIVERGENTE]` `blockUUID`, extractores de lectores y `parseMarkdownFile`: todas las rutas construyen mensajes, pero solo Claude conserva identidad y metadatos ricos.
- `[ANTIGUA]` `sources.ParseSectioned`, `plans.ParsePlan`: implementan dos parsers de secciones `##`; `internal/plans` no tiene caller de producción.
- `[JUSTIFICADA]` `Search`, `SearchRelaxed`, `HybridSearch`: búsqueda estricta y relajada repiten la trayectoria de filtros y paginación, pero convergen en helpers compartidos.
- `[JUSTIFICADA]` `searchTables`, `searchTable`, `searchTableQuery`: repiten la consulta para `messages_fts` y `tool_fts` con tokenizadores distintos y un contrato compartido.
- `[JUSTIFICADA]` `runSearch`, `runList`, `effectiveProject`: repiten la resolución del ámbito de proyecto mediante un helper común.
- `[DIVERGENTE]` `ListSessions`, `ListItemsV2`: implementan dos listados de sesiones. La ruta V2 puede incluir documentos, generar SQL inválido con offset sin límite e ignorar `recent`.
- `[JUSTIFICADA]` `resolveContextRecordAnchor`: implementa selectores paralelos por UUID y por ruta más ordinal con el mismo control de ausencia y ambigüedad.
- `[REPETIDA]` salida de `search` y entrada de `context`: publican y consumen la misma identidad mediante UUID o ruta más ordinal, con claves JSON distintas según el modo.
- `[DIVERGENTE]` límites de tokens de `search` y `context`: repiten conteo y truncado por registros completos, pero seleccionan registros y tratan presupuestos insuficientes de forma distinta.
- `[CRÍTICA]` `RebuildFTS`, triggers `sqlV7Triggers`, `sqlV4Repopulate`, `verifyRecoveryDestinationFTS`: derivan los índices desde `search_items`; `RebuildFTS` contamina ambos índices con todos los tipos.
- `[DIVERGENTE]` `mineTemplatesForFile`, `backfillTemplatesForFile`: repiten selección, normalización, agrupación e inserción de plantillas, pero usan señales y extractores distintos.
- `[RIESGO LATENTE]` bloque de correcciones de `SyncFiles`, `backfillCorrectionsForFile`: repiten eliminación, detección e inserción; actualmente coinciden, pero construyen `models.Message` con campos diferentes.
- `[DIVERGENTE]` `SyncFiles`, `backfillToolEventsForFile`: crean `tool_events`; sync conserva metadatos estructurados y backfill reconstruye una versión incompleta desde texto.
- `[ANTIGUA]` `sqlV4Triggers`, `sqlV7Triggers`: duplican mantenimiento FTS durante migraciones; V4 acepta cualquier contenido no tool y V7 restringe text, code y reasoning.
- `[DIVERGENTE]` `AggregateCommands`, `AggregateCommandsTrend`, `AggregateFailures`, `AggregateFailuresTrend`, `LoadToolSequences`: repiten filtros de eventos; joins directos, subconsultas y coordenadas ambiguas pueden producir resultados distintos.
- `[DIVERGENTE]` paginación de commands, failures, trends, corrections, templates y sequences: cada tipo interpreta `limit=0, offset>0` de forma diferente.
- `[DIVERGENTE]` filtros temporales de commands, failures, templates y sequences: repiten SQL con bordes inferiores `>=` o `>`; la CLI solo conecta fechas para sequences.
- `[DIVERGENTE]` `AggregateFailures`, `AggregateFailuresTrend`: repiten el cálculo de cobertura; la ruta semanal asigna el total global a cada semana.
- `[REPETIDA]` ramas semanales de commands y failures en `runPatterns`: duplican agrupación, orden y formatos; ambas reinician índices robot por semana.
- `[DIVERGENTE]` `UpsertAnnotation`, `resolveContextRecordAnchor`: resuelven mensajes por UUID o coordenadas; contexto detecta ambigüedad y anotaciones acepta la primera fila.
- `[EXACTA]` `extractBashErrors`, `extractGenericErrors`: contienen la misma implementación y solo cambia la ruta seleccionada por nombre de herramienta.
- `[REPETIDA]` `Mine`, `mineExtensions`: duplican el conteo de soporte por sesión en PrefixSpan para la base completa y la base proyectada.
- `[DIVERGENTE]` `mergeRRF`, fase RRF de `HybridSearch`: repiten adaptación y reconstrucción de resultados; `HybridSearch` descarta elementos exclusivamente vectoriales.
- `[EXACTA]` `getVersion`, `parseMapper`: decodifican dos veces los mismos bytes TOML de categorías al mismo tipo.
- `[ANTIGUA]` `RunDetectors`, `RunDetectorsFiltered`: duplican la orquestación de detectores; solo la variante filtrada tiene callers de producción.
- `[REPETIDA]` `ParseDocument`, `ParseSectioned`: duplican lectura, extracción de frontmatter y creación de `SourceItem`; divergen solo al dividir secciones.
- `[DIVERGENTE]` `InspectIndex`, `VerifyCurrentShape`, `Database.Validate`, `verifyRecoveryDestinationQueryer`: mantienen tres autoridades parciales sobre la validez de una base.
- `[REPETIDA]` `setupNewDatabaseSchema`, `allMigrationSteps`, `applyMigrationPlanLocked`, `migrationPlanDispatch`: mantienen dos secuencias de evolución de esquema para bases nuevas y existentes.
- `[JUSTIFICADA]` `snapshotDatabase`, `replaceActiveWithBackup`: preservan el estado anterior con contratos distintos de retención, restauración y durabilidad.
- `[JUSTIFICADA]` `startuplock.TryAcquire`, reservas SQLite de migración y `openActiveReservation`: repiten exclusión de escritores en capas y ámbitos diferentes.
- `[DIVERGENTE]` `OpenImmutableReadOnly`, `ensureSourceSidecarsSafe`, `readImmutableRecoveryInput`: deciden si una lectura inmutable es segura; recovery aplica controles más completos.
- `[EXACTA]` `validatePurgeBefore`, `Database.Purge`: validan dos veces los mismos formatos de fecha con mensajes distintos.
- `[JUSTIFICADA]` `runOwnedStartup`, `recoverPostInstallSync`: ejecutan `maybeAutoSync` antes de comandos ordinarios y después de recovery; el orden distinto pertenece al contrato de remediación.
