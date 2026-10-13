---
tipo: adr
estado: accepted
fecha: "2026-10-07"
contexto: El issue 48 requiere una superficie MCP compartida para las consultas de Backscroll sin convertir cada conversacion en un proceso ni crear otra ruta de sincronizacion.
decision: Exponer solo MCP 2026-07-28 mediante Streamable HTTP stateless en /mcp, con tres tools de lectura, autenticacion bearer local, bind loopback y una sola sincronizacion al iniciar.
consecuencias: Un proceso foreground servira consultas concurrentes sobre SQLite read-only; un supervisor externo controlara su vida util; los clientes legacy, los resources, las subscriptions y el sync por solicitud quedaran fuera.
---

# 0010. Exponer MCP local stateless

## Contexto

Backscroll ya ofrece servicios tipados para busqueda, contexto exacto y estado. SQLite es la unica fuente publica de consulta. ADR 0003 coordina la preparacion, las migraciones y el startup sync entre procesos.

El issue 48 requiere que varios clientes reutilicen un servidor local. MCP 2026-07-28 elimina el handshake `initialize`. La revision exige `protocolVersion` y `clientCapabilities` en cada request con `id`. La revision define `clientInfo` como opcional. Las notifications no requieren esa request metadata. Streamable HTTP permite que un proceso atienda varias conexiones sin asociar estado a una conversacion.

## Decision

Backscroll expondra solo la revision MCP `2026-07-28`. El servidor usara Streamable HTTP stateless. El endpoint sera `http://127.0.0.1:8377/mcp` por defecto. Toda direccion aceptada sera loopback.

El servidor implementara `server/discover`, `tools/list` y `tools/call`. Expondra solo estas tools:

- `backscroll_search`;
- `backscroll_context`;
- `backscroll_status`.

El servidor no expondra resources, prompts, subscriptions ni revisiones legacy. El servidor rechazara `initialize`. Un guard externo al SDK validara la revision exacta antes del despacho.

El pipeline validara Origin, auth, path y metodo HTTP en ese orden. Solo `/mcp` estara permitido. Una ruta distinta devolvera HTTP 404 antes del control de metodo. Para POST `/mcp`, el pipeline validara `Content-Type` y `Accept`. Un `Accept` invalido devolvera HTTP 400 con texto fijo y no devolvera JSON-RPC.

Despues de entrar al gate de lifecycle, un middleware leera un body acotado a 1 MiB. Validara `utf8.Valid` antes de JSON decode y antes de preextraer el `id` raw. UTF-8 invalido devolvera `-32700` sin `id`. El middleware prevalidara despues el envelope. El middleware validara despues revision, metadata y cabeceras reflejadas. El middleware aplicara despues la allowlist de `Mcp-Method`. La allowlist aceptara solo `server/discover`, `tools/list` y `tools/call`.

El middleware usara `json.Decoder.UseNumber`. El middleware exigira un unico objeto JSON-RPC sin trailing data, `jsonrpc: "2.0"`, un `method` no vacio y `params` ausente o como objeto. El middleware devolvera `-32700` para JSON malformado. El middleware devolvera `-32600` para un envelope invalido. El middleware no delegara estos errores al SDK.

La ausencia de la clave `id` definira una notification. Un `id` sera un string o un JSON number cuyo valor matematico sea integer. La notacion puede usar punto decimal o exponente. El middleware usara un scanner lexico O(n) con memoria O(1) adicional para clasificar integridad sin expandir la magnitud y conservara el JSON raw. Solo un `id` valido aparecera en un error posterior. Un `id` ausente, invalido o ilegible hara que el error omita la clave `id`. El middleware nunca serializara `id: null`.

Para todo `id` numerico, el middleware delegara un surrogate string. `JSONResponse` permanecera activo. El middleware capturara la respuesta SDK y restaurara el `id` raw antes de enviarla. Cada POST usara estado local. La captura tendra un limite de 2 MiB y fallara sin bytes parciales. Un overflow devolvera HTTP 500, `-32603`, `Internal error` y el `id` raw. Dieciseis capturas limitaran los buffers de respuesta agregados a 32 MiB.

Solo un request con `id` exigira `protocolVersion` y `clientCapabilities` en `params._meta`. `clientInfo` sera opcional. Cuando exista, tendra `name` y `version` como strings. Los strings vacios seran validos. Una notification no exigira request metadata. Todos los POST exigiran los headers MCP de transporte.

Para un request, la metadata obligatoria ausente ganara con `-32602`. Un header de revision ausente o malformed ganara despues con `-32020`. Un mismatch de revision ganara despues con `-32020`, aunque un valor no sea soportado. Una revision coincidente pero no soportada ganara despues con `-32022`. Una notification con headers invalidos recibira el status de transporte sin body.

El middleware validara `Mcp-Name` antes de la allowlist para `tools/call`, `prompts/get` y `resources/read`. El middleware decodificara el sentinel Base64 de MCP. Para una tool permitida, reescribira `Mcp-Name` al nombre ASCII canonico antes del SDK. El middleware producira `HeaderMismatch` cuando una cabecera y el body no coincidan. Un request conservara su `id` raw. Una notification valida con error de revision o cabecera recibira HTTP 400 sin body.

`Origin`, `Authorization`, `Content-Type`, `MCP-Protocol-Version`, `Mcp-Method` y `Mcp-Name` seran singleton. Un duplicado fallara en la fase del header. `Accept` sera una lista HTTP combinable. Los valores MCP y sus metadata keys seran case-sensitive. El scheme Bearer sera case-insensitive. `Content-Type` y `Accept` usaran `mime.ParseMediaType`. Origin normalizara scheme e IP y conservara coincidencia exacta de IP y puerto.

Despues de Origin, auth, path, metodo y media headers, cada POST intentara entrar a un gate con mutex, condicion, `accepting` y `active`. `enter` reservara uno de 16 slots e incrementara `active` bajo el mutex antes de leer el body. No existira cola. Una saturacion devolvera HTTP 429 con texto fijo y no leera el body ni ejecutara el SDK. Notifications usaran el mismo gate.

Un metodo bloqueado con `id` valido recibira HTTP 404, codigo JSON-RPC `-32601` y el mismo `id` raw. Una notification valida recibira HTTP 202 sin body. Esta regla tambien se aplicara a `server/discover`, `tools/list` y `tools/call` porque son request-only. Ninguna notification invocara esos handlers. Ningun metodo bloqueado llegara al SDK. El middleware restaurara el body original para IDs string y usara el body con surrogate para todo ID numerico.

La implementacion configurara `ServerOptions.Capabilities` de forma explicita. La capability `tools` se serializara como un objeto vacio. Las capabilities `resources`, `prompts` y `logging` se omitiran. La respuesta no prometera un campo `listChanged: false`.

La implementacion registrara las tools con `Server.AddTool`. No usara `mcp.AddTool`. Un adapter validara input schema, defaults, argumentos y mapeo antes de `QueryService`. `arguments` ausente equivaldra a `{}`. Solo un objeto sera valido. El adapter validara el output schema antes del exito. El adapter creara explicitamente un `TextContent` con JSON canonico y saneado. Los errores dirigidos al cliente no incluiran texto interno del validador.

Antes de buscar, el adapter llamara `services.ValidateExplicitSearchScope`, que compone `services.ValidateSearchRequest`. Esa frontera compartida ya incorpora `storage.ValidateRelaxationQuery` cuando `Relax` es true. Un relax invalido devolvera `invalid_arguments` y no invocara `QueryService.Search`. Para context reutilizara `services.ValidateContextRequest` con `ContextRequest.MaxTokens=services.DefaultContextMaxTokens`; el presupuesto MCP quedara en el adapter de presentacion, sin modificar el rango CLI.

Cada resultado de tool incluira `resultType: "complete"` y `content`. Un exito incluira `structuredContent` validado por su `outputSchema`. El wire puede omitir `isError` cuando sea false. Un error de dominio usara `isError: true` y contenido saneado. Un error de dominio omitira `structuredContent` y metadata de exito.

La taxonomia de tool se limitara a validacion, los tres sentinels de context, cancelacion, deadline y fallo interno. Cada categoria tendra `code` y `message` fijos. Los fallos de apertura o indice antes de readiness seran errores de proceso.

Los inputs limitaran `offset` a `0..2147483647`. `ordinal` sera un string decimal canonico no negativo hasta `MaxInt64`; el adapter lo convertira directamente a `int64` sin depender del tamaño de `int`. Search y context exigiran `max_tokens` positivo despues de defaults, con default `2000` y maximo `16384`. Los outputs serializaran ordinals y contadores integer como strings decimales. Las fronteras storage afectadas migraran a `int64`. Esta frontera evitara truncacion JSON y diferencias de tamaño de `int`.

MCP fijara `MaxTextRunes=16384` en las opciones internas de search y context. La CLI mantendra cero. Storage aplicara `substr` en SQL antes de `Scan`. Search aplicara tambien `substr(snippet(...),1,1024)`. Search limitara `limit` a 20 y context conservara un maximo de 101 records. Cada texto MCP tendra como maximo 65536 bytes UTF-8. Context materializara como maximo 101 records. Con 16 handlers y copias acotadas, el calculo conservador del ciclo request sera 320 MiB, sin contar SQLite ni el modelo.

MCP y Search CLI compartiran una sola construccion del runtime de busqueda. La secuencia resolvera `config.Embedding`, construira el provider configurado, abrira el snapshot privado con `OpenReadOnly`, instalara el wrapper mutex y publicara `QueryService`. Si un paso falla, cerrara primero el provider creado y despues la DB abierta. La construccion sera propietaria de ambos recursos. Embeddings deshabilitados seleccionaran fallback lexico. Un fallo de construccion configurada impedira readiness. Los fallos ordinarios del provider despues de readiness conservaran el fallback de `HybridSearchContext`; cancelacion y deadline no haran fallback.

Un mutex serializara `Embed` sobre el provider compartido. MCP sustituira el vector load-all por un cursor ordenado por ItemID y chunk ordinal. Procesara grupos contiguos, conservara solo el maximo positivo por ItemID y mantendra un heap top-K con cap 100. Los empates usaran score DESC e ItemID ASC, con el mismo comparador en la ruta CLI de referencia. Un gate vectorial de capacidad 1 cubrira `Embed` y todo el scan vectorial. Las busquedas lexicas no tomaran ese gate. Shutdown drenara requests antes de cerrar el provider. `sync.Once` y el mismo mutex impediran doble cierre y carreras entre `Embed` y `Close`.

El proceso reservara el listener antes de preparar el indice. Un fallo de bind impedira una segunda instancia en la misma direccion. El proceso ejecutara una sincronizacion inicial mediante el coordinador de ADR 0003. Mientras conserva el lock y la DB canonica abierta, creara un snapshot privado mediante la API backup de SQLite. Cerrara la DB canonica y liberara el lock solo despues de completar y hacer durable el snapshot.

El snapshot usara nombre aleatorio y permisos `0600` en Unix o ACL owner-only en Windows. Startup limpiara snapshots huerfanos del mismo owner solo cuando pueda adquirir su lease. Shutdown normal eliminara snapshot, lease y sidecars privados.

Antes de readiness, el proceso creara en el snapshot una tabla exhaustiva de visibilidad de rowids. Solo filas `content_type='tool'` podran quedar ocultas por `search_echo` o `directsearch.IsSerializedDirectSearchCall`. Text, code y reasoning permaneceran visibles. Search page e IDF de relax uniran la misma tabla. `MaxTextRunes` se aplicara despues a la proyeccion.

El servidor abrira solo el snapshot. Ninguna solicitud iniciara discovery, hashing, parseo, migracion ni sync. Ninguna solicitud observara commits canonicos posteriores.

El proceso se ejecutara en foreground. Un supervisor externo iniciara, detendra y reiniciara el proceso. El servidor no creara daemon, PID file ni mecanismo de descubrimiento dinamico. Los clientes usaran la URL fija.

El servidor usara el SDK oficial Go `github.com/modelcontextprotocol/go-sdk` v1.8.0. Usara `mcp.Implementation{Name: "backscroll", Version: buildVersion}`. `dev` sera una version valida. El transporte activara `Stateless`, `JSONResponse`, `PropagateRequestCancellation` y un limite de body de 1 MiB. El servidor restringira `SupportedProtocolVersions` a `2026-07-28`.

El servidor exigira un bearer token en cada solicitud. El token concedera solo el scope logico `backscroll:read`. En Unix, Backscroll generara 32 bytes con `crypto/rand` cuando no exista override ni archivo. Backscroll los codificara con `base64.RawURLEncoding` como 43 caracteres ASCII sin padding. El archivo Unix estable tendra modo `0600` y contendra el token seguido por un LF. El bearer excluira el LF. `BACKSCROLL_MCP_TOKEN` reemplazara el token del archivo Unix y debera cumplir la misma forma.

El comando MCP se compilara para Windows. Windows exigira `BACKSCROLL_MCP_TOKEN`. Windows no leera ni generara el token file. Un token de entorno ausente o invalido terminara el proceso antes del bind.

Un middleware explicito validara primero `Origin` en todos los metodos. La ausencia de `Origin` estara permitida. Un `Origin` presente se normalizara como URL y debera identificar la misma IP y puerto del listener. El middleware rechazara valores malformed, `null` o distintos con `403 Forbidden`. La autenticacion ocurrira despues. El path ocurrira despues de auth. El control de metodo HTTP ocurrira despues del path. La revision y las cabeceras se validaran despues de leer y prevalidar un POST.

POST sera el unico metodo permitido. GET, HEAD, OPTIONS, PUT, PATCH y DELETE devolveran `405 Method Not Allowed` cuando superen los controles anteriores. Cada handler tendra `context.WithTimeout` de 30 segundos y lo propagara a SDK, consultas, SQLite y provider. Todo provider configurado debera respetar cancelacion. `http.Server` usara `ReadHeaderTimeout` de 5 segundos, `ReadTimeout`, `WriteTimeout` e `IdleTimeout` de 30 segundos, y `MaxHeaderBytes=60<<10`. El margen interno de 4096 bytes mantendra el limite de lectura de Go dentro de 64 KiB. La implementacion usara `http.CrossOriginProtection` como defensa adicional solo si su configuracion conserva estas reglas. No sustituira el middleware explicito.

El proceso iniciara `Serve` y ejecutara un self-probe autenticado de `server/discover`. El probe tendra un timeout de cinco segundos. El proceso enviara readiness a stderr solo despues del probe exitoso.

Shutdown fijara `accepting=false` y cancelara el contexto raiz bajo el mutex del gate. Despues ejecutara `http.Server.Shutdown` con cinco segundos y llamara `http.Server.Close`. Esperara `active=0` mediante `sync.Cond` otros cinco segundos. Solo `active=0` permitira cerrar provider y SQLite. Un handler no cooperativo producira un error fatal sin cerrar recursos en uso. El gate evita Add-after-Wait.

El documento [Contrato del servidor MCP](../mcp.md) define cabeceras, schemas, errores, metadata, cache, amenazas, ciclo de vida, upgrade y pruebas.

## Alternativas descartadas

- **stdio por conversacion:** vincula el proceso a una conversacion y duplica procesos.
- **Servidor stateful:** agrega sesiones que MCP 2026-07-28 no requiere.
- **Soporte dual de revisiones:** aumenta la superficie y permite semantica legacy con `initialize`.
- **Resources y subscriptions:** agregan nombres, cache e invalidacion sin una necesidad inicial.
- **Confiar en handlers no anunciados del SDK:** deja accesibles metodos fuera de la superficie aunque las capabilities los omitan.
- **Sync por solicitud:** repite trabajo y contradice el objetivo del proceso compartido.
- **Bind externo:** expone datos locales sin TLS ni una frontera de identidad remota.
- **Daemon propio y PID file:** duplican funciones del supervisor y crean recuperacion de estado stale.
- **Descubrimiento de puerto:** agrega un archivo o servicio mutable. La URL fija es suficiente.
- **Confiar solo en `http.CrossOriginProtection`:** permite metodos seguros antes de aplicar la politica estricta de `Origin`.
- **Publicar readiness antes del probe:** no demuestra que el endpoint autenticado procese MCP.

## Consecuencias

### Positivas

- Varios clientes comparten un proceso y un snapshot SQLite estable.
- La DB canonica queda aislada de todas las requests.
- Cada solicitud conserva metadata de protocolo completa.
- La superficie inicial es read-only y pequena.
- La allowlist mantiene cerrados los handlers adicionales del SDK.
- La autenticacion, el origen y el bind reducen la exposicion local.
- La cancelacion HTTP llega a las consultas SQLite.
- ADR 0003 sigue siendo el unico coordinador de startup sync.
- Los schemas describen los tipos reales de `QueryService`.

### Negativas

- Los clientes MCP legacy no pueden usar el servidor.
- El servidor no actualiza inputs por solicitud.
- Cada startup copia la DB a un snapshot privado y consume espacio adicional.
- La visibilidad de query echoes agrega trabajo antes de readiness.
- La memoria de ese paso depende del mayor broad candidate y no tiene un bound numerico.
- Un cambio de binario, schema o token requiere reinicio.
- La disponibilidad depende de un supervisor externo.
- El startup requiere un self-probe local autenticado.
- Un proceso con el mismo usuario puede leer la base y puede leer el token si compromete la cuenta.

### Riesgo aceptado

Se acepta HTTP sin TLS porque el listener solo admite loopback. El bearer token sigue siendo obligatorio. La frescura queda fijada al snapshot creado despues del sync inicial. Los commits canonicos posteriores requieren restart para aparecer en MCP.

## Referencias

- [MCP 2026-07-28: protocolo base](https://modelcontextprotocol.io/specification/2026-07-28/basic)
- [MCP 2026-07-28: Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)
- [MCP 2026-07-28: discovery](https://modelcontextprotocol.io/specification/2026-07-28/server/discover)
- [MCP 2026-07-28: tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)
- [SDK oficial Go v1.8.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)
- [SQLite Online Backup API](https://www.sqlite.org/backup.html)
- [`http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection)
