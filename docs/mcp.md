# Contrato del servidor MCP local

Este documento define el contrato previo a la implementacion del issue 48. El README describe correctamente la version actual como una CLI sin servidor MCP. Este contrato define toda la superficie que implementara el issue 48.

## Alcance

El servidor expondra consultas read-only sobre un snapshot privado del indice SQLite perenne. El servidor reutilizara `internal/services.QueryService`. El servidor no leera archivos fuente durante una solicitud.

El servidor implementara estas operaciones MCP:

- `server/discover`;
- `tools/list`;
- `tools/call`.

El servidor publicara solo estas tools:

- `backscroll_search`;
- `backscroll_context`;
- `backscroll_status`.

El servidor no publicara resources. El servidor no publicara prompts. El servidor no implementara subscriptions. El servidor no implementara MCP Tasks. El servidor no aceptara `initialize`. El servidor no aceptara revisiones legacy.

## Revision y SDK

El servidor aceptara solo la revision `2026-07-28`. Todo POST debe incluir los headers MCP de transporte. Solo un request con `id` debe declarar la revision tambien en `params._meta.io.modelcontextprotocol/protocolVersion`.

Un guard de revision se ejecutara antes del handler del SDK. El guard exigira el valor exacto `2026-07-28`. El guard rechazara una revision distinta. Esta capa impedira que una ruta de compatibilidad del SDK active semantica legacy.

El servidor configurara tambien `ServerOptions.SupportedProtocolVersions` con una lista que contenga solo `2026-07-28`. `server/discover` devolvera `supportedVersions: ["2026-07-28"]`.

La implementacion usara el [SDK oficial Go v1.8.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0). La implementacion fijara esa version exacta cuando agregue la dependencia. Este cambio de documentacion no modifica `go.mod` ni `go.sum`.

El servidor construira `mcp.Implementation{Name: "backscroll", Version: buildVersion}`. `buildVersion` sera la misma version que usa el binario. El valor `dev` sera valido. El SDK publicara esos valores en `_meta.io.modelcontextprotocol/serverInfo` para `server/discover` y para cada resultado moderno.

La configuracion del transporte usara estos valores:

| Opcion | Valor |
| --- | --- |
| `StreamableHTTPOptions.Stateless` | `true` |
| `StreamableHTTPOptions.JSONResponse` | `true` |
| `StreamableHTTPOptions.PropagateRequestCancellation` | `true` |
| `StreamableHTTPOptions.MaxRequestBodyBytes` | `1 << 20` |
| `ServerOptions.SupportedProtocolVersions` | `[]string{"2026-07-28"}` |

`PropagateRequestCancellation` conectara la cancelacion HTTP con el contexto del handler. Los servicios de consulta propagaran ese contexto a SQLite. El servidor detendra el trabajo cuando el cliente cierre la solicitud.

## Transporte y endpoint

El servidor usara Streamable HTTP stateless. El endpoint por defecto sera:

```text
http://127.0.0.1:8377/mcp
```

El servidor aceptara solo direcciones IP loopback. El valor por defecto usara IPv4. Toda configuracion admitida por el issue 48 quedara limitada a una direccion loopback. El contrato no requiere un flag de bind.

Los clientes descubriran el endpoint mediante la URL fija. Backscroll no escribira un archivo de puerto. Backscroll no usara mDNS. Backscroll no registrara el endpoint en un servicio. `server/discover` descubre capacidades despues de que el cliente conoce la URL. No descubre la URL.

El servidor respondera a solicitudes MCP POST aceptadas con `application/json`. El cliente debe anunciar `application/json` y `text/event-stream` en `Accept`. El error de preflight de `Accept` sera la excepcion `text/plain`. El servidor no abrira streams SSE porque esta superficie no emite progreso ni notificaciones.

El servidor no asignara, leera ni devolvera `Mcp-Session-Id`. El servidor ignorara `Last-Event-ID`. Una ruta distinta de `/mcp` devolvera `404 Not Found` despues de superar los controles comunes de origen y autenticacion.

## Requests y notifications

Un request contiene una clave `id` valida. Cada request debe incluir esta metadata en `params._meta`:

- `io.modelcontextprotocol/protocolVersion` con `2026-07-28`;
- `io.modelcontextprotocol/clientCapabilities` como objeto.

`io.modelcontextprotocol/clientInfo` es opcional en un request. Cuando exista, debe ser un objeto. Debe contener `name` y `version` como strings. Los strings vacios son validos conforme al schema MCP. El servidor no usara `clientInfo` para autorizacion ni para cambiar el comportamiento.

Una notification omite la clave `id`. Una notification no requiere `params._meta.io.modelcontextprotocol/protocolVersion`. Una notification no requiere `params._meta.io.modelcontextprotocol/clientCapabilities`. Si una notification incluye protocol metadata, esa metadata debe tener forma valida y debe coincidir con los headers. Si incluye `clientInfo`, debe ser un objeto con `name` y `version` como strings. Los strings vacios son validos.

Los headers MCP de transporte siguen siendo obligatorios para todo POST. Esta regla incluye requests y notifications.

El servidor no usara metadata de un mensaje anterior. El servidor no inferira una tarea, una conversacion, un proyecto ni una identidad desde la conexion.

## Token bearer

El servidor exigira un bearer token en toda solicitud a `/mcp`. El token tendra un unico scope logico:

```text
backscroll:read
```

Ese scope permite `server/discover`, `tools/list` y las tres tools. El token no concede escritura. El issue 48 no implementara operaciones de escritura.

La forma valida del token es `^[A-Za-z0-9_-]{43}$`. El valor representa 32 bytes codificados con `base64.RawURLEncoding`. `BACKSCROLL_MCP_TOKEN` debe cumplir esa forma exacta. Backscroll no recortara espacios ni saltos de linea del override.

### Token en Unix

En Unix, Backscroll resolvera el token con este orden:

1. Si `BACKSCROLL_MCP_TOKEN` contiene un valor no vacio, Backscroll validara y usara ese valor.
2. En otro caso, Backscroll leera `<config_dir>/backscroll/mcp-token`.
3. Si el archivo no existe, Backscroll leera exactamente 32 bytes desde `crypto/rand`.
4. Backscroll codificara esos bytes con `base64.RawURLEncoding`.
5. Backscroll creara el archivo de forma exclusiva con modo `0600`.
6. El archivo contendra los 43 caracteres y un solo LF final.
7. Backscroll verificara el contenido, el tipo de archivo y el modo antes de usar un archivo existente.

El archivo Unix tendra exactamente 44 bytes. Backscroll eliminara el LF final cuando lea el archivo. El valor de `Authorization: Bearer` contendra solo los 43 caracteres.

Backscroll fallara cerrado si no puede crear, leer o proteger el archivo Unix. Backscroll fallara cerrado si el archivo contiene otra forma. Backscroll comparara tokens con una comparacion de tiempo constante.

### Token en Windows

El comando MCP se compilara para Windows. Windows exigira `BACKSCROLL_MCP_TOKEN`. El valor debe cumplir la forma de 43 caracteres.

La implementacion Windows no leera, creara, modificara ni validara `<config_dir>/backscroll/mcp-token`. La biblioteca estandar no puede garantizar el contrato Unix de modo `0600` mediante permisos POSIX. El contrato no sustituira ese requisito por una comprobacion parcial.

Un valor ausente, vacio o invalido en Windows terminara el proceso antes de `net.Listen`. El error ira a stderr. El error no incluira el valor recibido.

El override de entorno no se escribira al archivo Unix. El operador debe proteger el entorno del supervisor. El token permanecera estable entre reinicios mientras permanezca la fuente seleccionada.

En Unix, el operador podra rotar el archivo mediante reemplazo atomico. En Windows, el operador rotara `BACKSCROLL_MCP_TOKEN` en el supervisor. Toda rotacion requerira reinicio. El servidor no recargara el token durante la ejecucion. Los clientes deben actualizar su configuracion en el mismo cambio operativo.

Backscroll no aceptara el token como argumento de linea de comandos. Backscroll no imprimira el token. Backscroll no incluira el token en logs ni errores.

Una credencial ausente o invalida devolvera `401 Unauthorized`. La respuesta incluira:

```text
WWW-Authenticate: Bearer realm="backscroll-mcp", scope="backscroll:read"
```

El servidor no implementara endpoints OAuth. La autorizacion local usa la opcion de autenticacion personalizada que permite MCP.

## Orden del pipeline HTTP

Un middleware explicito aplicara este orden a todas las rutas y a todos los metodos:

1. Validara `Origin`.
2. Validara el bearer token.
3. Validara que el path sea exactamente `/mcp`.
4. Validara el metodo HTTP.
5. Comprobara el presupuesto de headers aceptados.
6. Para POST, validara `Content-Type`.
7. Para POST, validara `Accept`.
8. Para POST, intentara entrar al gate de lifecycle.
9. Leera un body acotado a 1 MiB.
10. Validara el body con `utf8.Valid`.
11. Preextraera y clasificara el `id` raw.
12. Prevalidara el envelope JSON-RPC.
13. Validara la revision, la metadata y las cabeceras reflejadas.
14. Aplicara la allowlist MCP.
15. Preparara el body y el capturador de respuesta antes de delegar un metodo permitido.
16. El SDK ejecutara el metodo permitido.

Una fase puede terminar la solicitud. Una solicitud que falla en una fase no alcanza la fase siguiente. La precedencia sera Origin, auth, path, metodo HTTP, header budget, content negotiation, gate, body, UTF-8, envelope, revision y headers, allowlist, SDK.

### Validacion de Origin

El middleware explicito validara `Origin` antes de cualquier otro control. Esta regla se aplica tambien a metodos seguros y rutas desconocidas.

Una solicitud sin `Origin` estara permitida para clientes no browser. Una solicitud con `Origin` debe identificar el mismo scheme, IP y puerto que el listener. El valor por defecto valido sera:

```text
http://127.0.0.1:8377
```

El middleware analizara `Origin` como URL. Normalizara el scheme a minusculas. Normalizara una IP con `net/netip`. Comparara el puerto explicito como numero. No resolvera nombres DNS. El valor no puede contener path, query, fragment ni userinfo. Un valor malformed recibira `403 Forbidden`. El valor `null` recibira `403 Forbidden`. Una IP o puerto distintos recibiran `403 Forbidden`. Esta normalizacion acepta casing de scheme e IPv6 equivalente sin debilitar la coincidencia de host y puerto.

El middleware no emitira cabeceras CORS. La proteccion de `Host` local del SDK permanecera activa.

`http.CrossOriginProtection` no satisface por si solo este contrato. Esa API permite metodos seguros y usa reglas distintas para solicitudes sin `Origin`. La implementacion usara [`http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection) como defensa adicional solo si su configuracion conserva las reglas anteriores. Las pruebas deben demostrar esa compatibilidad. El middleware explicito seguira siendo autoritativo.

### Cardinalidad de headers

El servidor tratara `Origin`, `Authorization`, `Content-Type`, `MCP-Protocol-Version`, `Mcp-Method` y `Mcp-Name` como headers singleton. El servidor contara todos los valores que devuelve `Header.Values`. Una repeticion sera invalida aunque los valores sean iguales.

Dos o mas valores de `Origin` devolveran HTTP 403. Dos o mas valores de `Authorization` devolveran HTTP 401.

Dos o mas valores de `Content-Type` devolveran HTTP 400. La respuesta usara `Content-Type: text/plain; charset=utf-8`. La respuesta usara exactamente este body:

```text
Bad Request: Content-Type must appear exactly once
```

Dos o mas valores de `MCP-Protocol-Version`, `Mcp-Method` o `Mcp-Name` se validaran despues de preextraer el `id`. Un request recibira HTTP 400 y `HeaderMismatch`, codigo `-32020`, con el mismo `id`. Una notification valida recibira HTTP 400 sin body.

`Accept` no es singleton. El servidor combinara todas sus lineas en una sola lista HTTP antes de analizar media types y parametros.

### Casing y parsing

Los nombres de headers siguen la comparacion case-insensitive de HTTP. Los valores MCP no siguen esa regla.

El servidor analizara `Authorization` como scheme y credential. Comparara el scheme `Bearer` con `strings.EqualFold`. Comparara el token de 43 caracteres de forma case-sensitive y en tiempo constante. Rechazara credentials adicionales.

El servidor analizara `Content-Type` con `mime.ParseMediaType`. Comparara el type y subtype con `strings.EqualFold` contra `application/json`. Los parametros deben ser sintacticamente validos.

El servidor dividira la lista combinada de `Accept` en media ranges. Analizara cada media range con `mime.ParseMediaType`. Comparara type y subtype de forma case-insensitive. Los valores `q` conservaran la semantica documentada.

`protocolVersion`, `method`, `name` y los valores de sus headers reflejados seran case-sensitive. Las keys `io.modelcontextprotocol/protocolVersion`, `io.modelcontextprotocol/clientCapabilities` e `io.modelcontextprotocol/clientInfo` seran case-sensitive. Una key con otro casing sera una key distinta y no satisfara un campo obligatorio.

### Cabeceras MCP

Una solicitud POST que alcanza el handler debe incluir estas cabeceras:

| Cabecera | Regla |
| --- | --- |
| `Authorization` | Debe contener `Bearer <token>`. |
| `Content-Type` | Debe ser `application/json`. |
| `Accept` | Debe admitir `application/json` y `text/event-stream`. |
| `MCP-Protocol-Version` | Debe ser exactamente `2026-07-28`. |
| `Mcp-Method` | Debe coincidir con `method` en el body. |
| `Mcp-Name` | Es obligatoria para `tools/call`, `prompts/get` y `resources/read`. Debe coincidir con el campo fuente del body. |

Los nombres de cabecera no distinguen mayusculas. Los valores MCP reflejados distinguen mayusculas. `Authorization`, `Content-Type`, `Accept` y `Origin` siguen las reglas HTTP documentadas. El servidor decodificara el sentinel Base64 de `Mcp-Name` antes de comparar cuando el protocolo lo requiera.

El control de path ocurrira despues de auth y antes del metodo HTTP. Una ruta distinta de `/mcp` devolvera `404 Not Found` para cualquier metodo. En `/mcp`, el control de metodo HTTP ocurrira antes de leer el body y antes de validar la revision.

### Path y metodos HTTP

El unico path es `/mcp`. Una ruta distinta devolvera `404 Not Found` despues de superar Origin y auth. Esta regla se aplica a GET, POST y cualquier otro metodo.

POST es el unico metodo HTTP permitido en `/mcp`. GET, HEAD, OPTIONS, PUT, PATCH y DELETE devolveran `405 Method Not Allowed` cuando superen los controles anteriores. Cualquier otro metodo HTTP no POST tambien devolvera `405`. La respuesta incluira `Allow: POST`.

### Accept

POST `/mcp` debe incluir un header `Accept` que liste de forma explicita `application/json` y `text/event-stream`. El parser tratara los media types sin distinguir mayusculas. El parser admitira cualquier orden. El parser admitira parametros validos. Un media type con `q=0` no cuenta como admitido. Un wildcard no sustituye ninguno de los dos media types.

Si `Accept` falta, es malformed o no admite ambos media types, el servidor devolvera HTTP 400. La respuesta usara `Content-Type: text/plain; charset=utf-8`. La respuesta usara exactamente este body:

```text
Bad Request: Accept must include application/json and text/event-stream
```

La respuesta incluira `Cache-Control: no-store`. La respuesta no sera un error `HeaderMismatch`. La respuesta no contendra JSON-RPC ni datos internos.

### Allowlist MCP

Un middleware HTTP explicito aplicara una allowlist al valor case-sensitive de `Mcp-Method`. El middleware se ejecutara antes del handler SDK. La allowlist contendra solo estos valores:

- `server/discover`;
- `tools/list`;
- `tools/call`.

El middleware envolvera el body con `http.MaxBytesReader` configurado en 1 MiB. El middleware leera como maximo 1 MiB de payload mediante `io.ReadAll`. `http.MaxBytesReader` detectara el exceso. Un body que exceda 1 MiB recibira `413 Request Entity Too Large`.

### Validacion UTF-8

Inmediatamente despues de `io.ReadAll`, el middleware ejecutara `utf8.Valid(body)`. Esta comprobacion ocurrira antes de JSON decode y antes de capturar `id` raw.

Un body con UTF-8 invalido devolvera HTTP 400 y el parse error JSON-RPC `-32700`. La respuesta omitira la clave `id` porque no existira preextraccion valida. El middleware no preservara ni reflejara bytes del body invalido.

La respuesta se construira desde constantes UTF-8 validas mediante `encoding/json`. Por eso el error siempre sera JSON y UTF-8 validos. El body invalido no llegara al decoder, allowlist ni SDK.

### Preextraccion de `id`

El middleware conservara los bytes del body despues de validar UTF-8. El middleware ejecutara el primer `Decode` con `json.Decoder.UseNumber`. Cuando el valor superior sea un objeto, el middleware buscara la clave `id` antes de validar cualquier otro campo. Un decoder auxiliar sobre los mismos bytes capturara el valor de `id` como `json.RawMessage`.

La ausencia de la clave `id` definira una notification. Un string sera un `id` valido. Un JSON number sera valido solo cuando su valor matematico sea un integer. La notacion puede contener punto decimal o exponente. No existira un limite de magnitud para ese integer. Los limites integer de los schemas de tools no se aplicaran a RequestId.

El middleware clasificara integridad con un scanner lexico de una sola pasada. El scanner tendra tiempo O(n) y memoria O(1) adicional. Procesara signo, parte entera, fraccion y exponente. Contara digitos fraccionarios y ceros finales. Comparara el exponente con esos contadores mediante magnitud decimal saturada. No expandira el exponente y no materializara la magnitud.

Cero con cualquier exponente sera integer. Para un valor no cero, el number sera integer cuando el desplazamiento decimal deje todos los digitos no cero a la izquierda del punto. `1.0`, `1e1000000000`, `10e-1` y `1000e-3` seran validos. `1.5`, `1e-1` y `1000e-4` seran invalidos.

`id: null`, boolean, objeto y array tambien seran IDs invalidos.

El middleware clasificara el `id` como ausente, valido, invalido o ilegible. Solo el estado valido permitira incluir la clave `id` en un error posterior. El middleware omitira la clave `id` cuando el estado sea ausente, invalido o ilegible. El schema de error MCP 2026-07-28 permite omitir `id`. El middleware nunca serializara `id: null`.

Despues de confirmar un unico valor JSON sin trailing data, una clave `id` invalida devolvera HTTP 400 y `Invalid Request`, codigo `-32600`, sin clave `id`. Esta decision ocurrira antes de validar los otros campos del envelope.

El middleware conservara los bytes JSON originales de un `id` valido. El middleware usara el `json.RawMessage` al construir una respuesta propia. Esta regla preservara strings, integers y notacion exponencial sin truncacion ni normalizacion.

### Adaptacion de RequestId para el SDK

Todo `id` numerico se sustituira en el body delegado por el string `"backscroll-surrogate-id"`. El middleware no intentara clasificar su magnitud. El surrogate sera fijo porque cada POST contiene un solo mensaje. El middleware no mantendra un mapa global.

Un `id` string se delegara sin surrogate. El capturador restaurara de todos modos su `json.RawMessage` exacto en la respuesta.

`StreamableHTTPOptions.JSONResponse` permanecera en true. El middleware capturara por completo la respuesta JSON del SDK antes de escribir al cliente. El middleware validara que la respuesta sea un unico objeto JSON-RPC. Para un request adaptado, exigira que el response use el surrogate. Despues sustituira el surrogate por el `id` raw original.

El middleware capturara tambien respuestas para IDs string. Antes de enviar, reinsertara siempre el `id` raw original. Esta regla conserva notacion exponencial y escapes de string. El middleware conservara el status HTTP del SDK.

El middleware copiara solo `Content-Type`, `Cache-Control` y `MCP-Protocol-Version`. `MCP-Protocol-Version` sera el unico header MCP de respuesta necesario para esta revision stateless. El middleware descartara `Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade`, `Set-Cookie`, `Server` y cualquier header no incluido en la allowlist. El middleware recalculara `Content-Length` despues de restaurar el `id`.

La captura de cada respuesta se limitara a 2 MiB. Con 16 requests activas, el limite agregado de buffers de respuesta sera 32 MiB. El middleware no enviara bytes parciales. Una respuesta mayor, malformed, con trailing data o con un `id` inesperado se descartara.

Despues de descartar la captura, el middleware devolvera HTTP 500 y `Content-Type: application/json`. El body sera un error JSON-RPC pequeño con el `id` raw original, codigo `-32603` y mensaje exacto `Internal error`. La respuesta incluira `Cache-Control: no-store`. El detalle ira a stderr.

```json
{
  "jsonrpc": "2.0",
  "id": 123456789012345678901234567890,
  "error": {
    "code": -32603,
    "message": "Internal error"
  }
}
```

Cada POST usara su propio buffer, surrogate y `id` raw. Requests concurrentes no compartiran estado de correlacion.

### Prevalidacion del envelope

El primer `Decode` debe producir un unico objeto JSON. El segundo `Decode` debe devolver `io.EOF`. Whitespace despues del objeto estara permitido. Cualquier otro trailing data producira parse error.

El objeto debe contener `jsonrpc: "2.0"`. El objeto debe contener un `method` string no vacio. `params` puede estar ausente. Cuando `params` exista, debe ser un objeto JSON. `params: null`, arrays y valores primitivos seran invalid request. Un batch sera invalid request porque el valor superior es un array.

Un JSON sintacticamente malformado, un body vacio o trailing data devolveran HTTP 400. El body de respuesta sera un error JSON-RPC `-32700`. El error incluira el `id` raw solo cuando la preextraccion obtuvo un `id` valido. El error omitira `id` en los otros casos. El middleware no delegara estos casos al SDK.

Un envelope que incumpla la forma anterior devolvera HTTP 400. El body de respuesta sera un error JSON-RPC `-32600`. El error incluira el `id` raw solo cuando la preextraccion obtuvo un `id` valido. El error omitira `id` cuando el `id` sea ausente, invalido o ilegible. El middleware no delegara estos casos al SDK.

### Revision, metadata y cabeceras reflejadas

El middleware aplicara esta precedencia despues de prevalidar el envelope.

1. Para un request, validara la metadata del body. `params._meta.io.modelcontextprotocol/protocolVersion` debe ser un string no vacio. `params._meta.io.modelcontextprotocol/clientCapabilities` debe ser un objeto. `clientInfo` sera opcional. Cuando exista, debe ser un objeto con `name` y `version` como strings. Los strings vacios seran validos. Una ausencia obligatoria o una forma invalida devolvera HTTP 400 y `Invalid params`, codigo `-32602`, con el mismo `id` raw.
2. Validara `MCP-Protocol-Version`. Debe existir una sola vez. Debe tener la forma `YYYY-MM-DD`. Una ausencia, duplicacion o forma invalida devolvera HTTP 400 y `HeaderMismatch`, codigo `-32020`, con el mismo `id` raw para un request.
3. Comparara el header con `params._meta.io.modelcontextprotocol/protocolVersion`. Una diferencia devolvera HTTP 400 y `HeaderMismatch`, codigo `-32020`, aunque uno de los valores no sea soportado.
4. Si los valores coinciden pero no son `2026-07-28`, devolvera HTTP 400 y `UnsupportedProtocolVersion`, codigo `-32022`. `requested` sera el valor coincidente. `supported` sera `["2026-07-28"]`.
5. Solo el valor coincidente `2026-07-28` continuara a las demas cabeceras.

Una notification no requiere request metadata. El middleware validara sus headers MCP de transporte. Si `params._meta` existe, debe ser un objeto. Si `protocolVersion` existe, debe ser un string y debe coincidir con el header. Si `clientCapabilities` existe, debe ser un objeto. Si `clientInfo` existe, debe ser un objeto con `name` y `version` como strings. Los strings vacios seran validos. Una notification valida con error de metadata o revision recibira HTTP 400 sin body.

Despues de la revision, el middleware comparara `Mcp-Method` con `method`.

El middleware exigira `Mcp-Name` para `tools/call`, `prompts/get` y `resources/read` antes de aplicar la allowlist. Para `tools/call` y `prompts/get`, comparara el valor decodificado con `params.name`. Para `resources/read`, comparara el valor decodificado con `params.uri`, como exige Streamable HTTP.

`Mcp-Name` debe tener una sola representacion valida. Un valor plain debe usar ASCII visible sin whitespace inicial ni final. Un valor no representable como plain debe usar exactamente `=?base64?{Base64StdEncoding}?=`. El sentinel distingue mayusculas. El payload usara `base64.StdEncoding`. El valor decodificado debe ser UTF-8 valido y no vacio.

Una cabecera obligatoria ausente, malformed o distinta del body producira `HeaderMismatch`, codigo `-32020`, con HTTP 400. Un request conservara su `id` raw. Una notification valida recibira HTTP 400 sin body. El middleware producira este error antes de la allowlist.

Para `tools/call`, `params.name` debe ser exactamente `backscroll_search`, `backscroll_context` o `backscroll_status`. Estos tres nombres son ASCII. Un request con otro nombre recibira HTTP 400 y `Invalid params`, codigo `-32602`, con el mismo `id`. Una notification con otro nombre recibira HTTP 400 sin body.

Cuando el nombre de tool sea valido, el middleware reescribira `Mcp-Name` al nombre ASCII canonico mediante `Header.Set`. El SDK recibira una sola cabecera canonica. Esta regla se aplica aunque el cliente haya usado el sentinel Base64 para codificar un nombre ASCII valido.

Un nombre Unicode valido en `prompts/get` o `resources/read` puede superar la decodificacion de `Mcp-Name`. El metodo bloqueado recibira despues HTTP 404 y `-32601`. El request no llegara al SDK.

### Gate de lifecycle

El servidor usara un gate con `sync.Mutex`, `sync.Cond`, `accepting bool`, `active int` y limite 16. El gate no usara `sync.WaitGroup` para admission.

Despues de Origin, auth, path, metodo HTTP, `Content-Type` y `Accept`, cada POST llamara `enter`. `enter` adquirira el mutex. Comprobara `accepting`. Comprobara `active < 16`. Incrementara `active` bajo el mismo mutex antes de leer el body. Despues liberara el mutex.

Si `accepting` es false, `enter` devolvera HTTP 503 sin leer el body. Si `active` es 16, `enter` devolvera HTTP 429 sin leer el body. No existira cola.

La respuesta 429 usara `Content-Type: text/plain; charset=utf-8` y `Cache-Control: no-store`. Usara exactamente este body:

```text
Too Many Requests
```

La respuesta 503 usara los mismos headers y el body fijo `Service Unavailable\n`.

El gate se aplicara a requests y notifications. Un rechazo ocurre antes de conocer `id`, `method` o la clase request/notification. Por eso sera un rechazo HTTP de transporte y no una respuesta JSON-RPC. No ejecutara parser, allowlist ni SDK.

Un `defer` llamara `leave` cuando termine la respuesta completa. `leave` adquirira el mutex, decrementara `active`, ejecutara `cond.Broadcast` y liberara el mutex. El defer tambien cubrira errores y cancelaciones.

Cada slot podra conservar una copia raw de body de 1 MiB, una copia delegada de 1 MiB para un ID numerico y un buffer de respuesta de 2 MiB. El SDK y el adapter podran conservar copias transitorias dentro de esos mismos limites de entrada y salida. El techo conservador para las copias de bytes enumeradas y los textos materializados de 16 handlers sera 320 MiB. No es un limite de RSS. Excluye overhead de objetos Go, SQLite page cache, indices, memoria del driver y memoria del modelo de embeddings.

Los 16 buffers de respuesta tendran un limite individual de 2 MiB. Su limite agregado especifico sera 32 MiB.

### Decision de allowlist

Si la prevalidacion y las cabeceras tienen exito, el middleware aplicara la allowlist.

`server/discover`, `tools/list` y `tools/call` son operaciones request-only. Una notification con cualquiera de esos metodos recibira HTTP `202 Accepted` sin body. El middleware no invocara el handler SDK.

Una notification con un metodo bloqueado tambien recibira HTTP `202 Accepted` sin body. El middleware no creara un objeto JSON-RPC Response para ninguna notification valida.

Si un request usa un metodo permitido, el middleware preparara el body para el SDK. Un `id` string conservara el body original. Todo `id` numerico usara un body equivalente con el surrogate. El middleware reemplazara `Request.Body` y actualizara `Request.ContentLength`. El middleware delegara despues al SDK mediante el capturador de respuesta.

Un request bloqueado no llegara al SDK. El middleware devolvera HTTP `404 Not Found`. El body sera un error JSON-RPC con codigo `-32601`, mensaje saneado y el mismo `id` raw.

La allowlist bloqueara prompts, resources, subscriptions, completion, logging, ping, initialize y notifications. Esta regla se aplica aunque el SDK v1.8.0 registre un handler estandar para el metodo.

Este request bloqueado usa `Mcp-Method: prompts/list`. El request conserva su `id`:

```json
{
  "jsonrpc": "2.0",
  "id": "blocked-1",
  "method": "prompts/list",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientCapabilities": {}
    }
  }
}
```

El middleware responde con HTTP 404 y este body:

```json
{
  "jsonrpc": "2.0",
  "id": "blocked-1",
  "error": {
    "code": -32601,
    "message": "Method not found"
  }
}
```

Esta notification bloqueada usa `Mcp-Method: notifications/progress`. La notification no contiene `id`:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/progress",
  "params": {
    "progressToken": "probe",
    "progress": 1
  }
}
```

El middleware responde con HTTP 202. La respuesta contiene `Cache-Control: no-store`. La respuesta no contiene body. El middleware no crea un Response JSON-RPC.

Un parse error cuyo `id` no puede leerse devuelve HTTP 400 con esta forma:

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32700,
    "message": "Parse error"
  }
}
```

Un invalid request con `id: "bad-params"` valido conserva el `id`:

```json
{
  "jsonrpc": "2.0",
  "id": "bad-params",
  "error": {
    "code": -32600,
    "message": "Invalid Request"
  }
}
```

Un invalid request cuyo `id` es invalido omite la clave `id`:

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32600,
    "message": "Invalid Request"
  }
}
```

### Limites POST

El middleware de allowlist limitara cada body a 1 MiB durante la lectura. El limite se aplicara a `Content-Length`, transferencia chunked y HTTP/2. El SDK conservara tambien `StreamableHTTPOptions.MaxRequestBodyBytes: 1 << 20` como defensa interna.

El body debe contener un solo request o notification JSON-RPC. El servidor no aceptara batches. La superficie no define notifications de aplicacion. Un request debe contener un `id` string o un JSON number cuyo valor matematico sea integer. Una notification debe omitir la clave `id`.

El servidor enviara `Cache-Control: no-store` en toda respuesta HTTP. Esta regla incluye exitos, errores, `401`, `403`, `404`, `405`, `413`, `429`, `431`, `500` y `503`.

## Configuracion del servidor HTTP

La implementacion configurara `http.Server` con estos valores exactos:

| Campo | Valor |
| --- | --- |
| `ReadHeaderTimeout` | 5 segundos |
| `ReadTimeout` | 30 segundos |
| `WriteTimeout` | 30 segundos |
| `IdleTimeout` | 30 segundos |
| `MaxHeaderBytes` | `60 << 10` bytes |

El servidor no dejara los zero values para estos campos. Go 1.26.2 puede leer hasta `MaxHeaderBytes + 4096` bytes durante parsing. `60 << 10` mas ese margen es 64 KiB.

Despues del parsing, el handler calculara bytes de headers aceptados como `2 + sum(len(name)+2+len(value)+2)` para cada valor. El `2` inicial representa la linea vacia final. Si el total supera `60 << 10`, devolvera HTTP 431, `Content-Type: text/plain; charset=utf-8`, `Cache-Control: no-store` y el body fijo `Request Header Fields Too Large\n`. No ejecutara el gate ni el handler MCP.

`MaxHeaderBytes` sera un upper bound de configuracion, no una promesa de que todo request de un tamaño exacto sea aceptado. `net/http` puede rechazar antes por sintaxis, request line u otros limites.

Los timeouts HTTP seran adicionales al timeout del handler.

`http.Server.BaseContext` derivara cada request del contexto raiz. Despues de entrar al gate, el middleware creara `context.WithTimeout(request.Context(), 30*time.Second)`. Reemplazara el contexto del request antes de continuar. Propagara ese contexto al SDK, al adapter, a `QueryService`, a SQLite y al provider. Un `defer` cancelara el contexto del handler.

Todo provider configurable debe respetar `ctx.Done()` y devolver `context.Canceled` o `context.DeadlineExceeded`. La suite de la factory validara esta propiedad para el provider real disponible y para cada fake. Un provider que no cumpla no podra registrarse como provider configurado.

Una prueba slowloris enviara headers incompletos por mas de cinco segundos y exigira cierre. Una prueba comprobara que todo request aceptado por el handler tiene como maximo `60 << 10` bytes segun la formula. Otra prueba enviara headers que exceden 64 KiB y exigira rechazo antes del handler. Las pruebas trataran 64 KiB como upper bound y no exigiran aceptacion exacta en el borde. Una prueba mantendra un body por mas de 30 segundos y exigira timeout. Una prueba mantendra una respuesta por mas de 30 segundos y exigira timeout. Una prueba verificara cierre de una conexion idle despues de 30 segundos.

Una prueba bloqueara una query SQLite y exigira `context.DeadlineExceeded` antes o al cumplir 30 segundos. Otra prueba bloqueara `Embed` y exigira la misma cancelacion. La prueba del provider real se ejecutara cuando su build tag este disponible. Los fakes de provider tendran la misma prueba contractual.

## Capabilities, discovery y cache MCP

La implementacion configurara `ServerOptions.Capabilities` de forma explicita. No dejara ese campo en nil. La configuracion tendra `Tools: &mcp.ToolCapabilities{}`. Los campos `Resources`, `Prompts` y `Logging` permaneceran nil.

Esta configuracion evita la capability `logging` que el SDK agrega cuando `Capabilities` es nil. Esta configuracion tambien evita la inferencia `listChanged: true` cuando se registran tools.

El resultado serializado de `server/discover` contendra `capabilities.tools` como objeto vacio. No contendra `listChanged`. No contendra `resources`, `prompts` ni `logging`. El contrato acepta el uso de `omitempty` del SDK. El contrato no exige serializar `listChanged: false`.

`tools/list` devolvera las tres tools en orden lexicografico. La lista no cambiara durante la vida del proceso. El servidor no registrara handlers de subscriptions.

`ServerOptions.SetCacheable` fijara estos valores para `server/discover` y `tools/list`:

```json
{
  "ttlMs": 0,
  "cacheScope": "private"
}
```

`ttlMs: 0` marca el resultado como inmediatamente stale. `cacheScope: "private"` impide compartir el resultado entre contextos de autorizacion. Las respuestas de `tools/call` no llevan hints MCP de cache.

`tools/list` devolvera las tres tools en una pagina. La respuesta no incluira `nextCursor`.

## Resultados de tools

Todo resultado de `tools/call` incluira `resultType: "complete"` y `content`. El SDK v1.8.0 añadira `resultType` para una solicitud moderna.

Un resultado exitoso incluira `structuredContent`. El valor validara contra el `outputSchema` de la tool. El adapter creara `Content` de forma explicita. `Server.AddTool` no postprocesa `structuredContent` ni crea un fallback textual.

`Content` contendra exactamente un `TextContent`. Su texto sera JSON canonico y saneado del mismo resultado publico. La forma canonica sera JSON UTF-8 compacto, sin indentacion y sin LF final. El adapter ordenara las keys de objetos de forma lexicografica. El adapter no incluira campos internos.

Un resultado exitoso puede omitir `isError`. El SDK usa `omitempty` y omite `isError` cuando el valor es false. El contrato no exige `isError: false` en el wire.

Un error de tool incluira `isError: true`. El error omitira `structuredContent`. El error no debe validar contra el `outputSchema` de exito. El error no tiene que incluir metadata de exito.

El adapter creara tambien el `Content` de error. Contendra exactamente un `TextContent`. Su texto sera el JSON canonico de un objeto con solo `code` y `message`. Ambos valores vendran de la taxonomia cerrada. El adapter no llamara `CallToolResult.SetError` con un error interno. El adapter nunca devolvera el texto de un error interno.

Las tres tools tendran estas annotations:

```json
{
  "readOnlyHint": true,
  "destructiveHint": false,
  "idempotentHint": true,
  "openWorldHint": false
}
```

## Adapter de tools

La implementacion registrara cada tool con `Server.AddTool`. No usara el helper generico `mcp.AddTool`.

`Server.AddTool` entrega argumentos raw y no valida schemas. Un adapter comun sera propietario de esta frontera. El adapter compilara los seis schemas durante startup. Un schema invalido impedira readiness.

Para cada llamada, el adapter tratara `arguments` ausente como `{}`. Cuando `arguments` exista, debe ser un objeto JSON. El adapter decodificara el objeto como `map[string]any` con `json.Decoder.UseNumber`.

`arguments: null`, array, string, boolean o number devolvera `invalid_arguments`. El adapter no invocara `QueryService` en esos casos.

El adapter aplicara defaults del `inputSchema`. El adapter validara el mapa contra el `inputSchema`. El adapter rechazara propiedades desconocidas. El adapter realizara estas acciones antes de invocar `QueryService`.

`backscroll_status` aceptara `arguments` ausente y `{}`. Su schema rechazara cualquier propiedad.

Los limites documentados hacen segura la representacion `map[string]any`. `limit`, `offset`, `before`, `after` y `max_tokens` se validan como integers exactos antes de convertirlos a tipos Go. `ordinal` se valida como string decimal antes de convertirlo. El adapter no convierte estos valores mediante `float64`.

El mapeo minimo sera:

| Tool | Mapeo al servicio |
| --- | --- |
| `backscroll_search` | `query` va a `SearchRequest.Query`; `relax` va a `SearchRequest.Relax`; `project`, `all_projects`, `source`, `source_path`, `after`, `before`, `role`, `content_type`, `tag`, `limit`, `offset`, `lexical_only` y `similarity_threshold` van a los campos correspondientes de `models.SearchOptions`; las fechas se parsean antes del mapeo; `max_tokens` queda en el adapter de presentacion. |
| `backscroll_context` | `uuid` va a `ContextRequest.UUID`; `source_path` va a `ContextRequest.SourcePath`; `ordinal` se parsea y va a `ContextRequest.Ordinal`; `before` y `after` van a los campos del mismo nombre; `ContextRequest.MaxTokens` usa `services.DefaultContextMaxTokens`; el `max_tokens` MCP queda en el adapter de presentacion. |
| `backscroll_status` | El objeto vacio forma `services.StatusRequest{}`. |

Antes de `QueryService.Search`, el adapter llamara `services.ValidateExplicitSearchScope`, que compone `services.ValidateSearchRequest` y exige exactamente un scope explicito. En `origin/main` eeb9e8f2b0435ac07e3f00f91ea1eb24176df4b3, `services.ValidateSearchRequest` ya incorpora `storage.ValidateRelaxationQuery` cuando `Relax` es true. El issue 48 reutilizara esa frontera; no implementara otro validador de relax.

Un error de `services.ValidateSearchRequest` devolvera `invalid_arguments` y el texto fijo `invalid arguments`. El adapter no invocara `QueryService.Search` despues de ese error. El adapter no devolvera el texto de `storage.ValidateRelaxationQuery`.

Antes de `QueryService.Context`, el adapter llamara `services.ValidateContextRequest`. Esa frontera ya valida selector, ventana y `ContextRequest.MaxTokens` en `64..16384`. `QueryService.Context` no aplica ese presupuesto al payload: devuelve los records para que el caller los presente. MCP suministrara `services.DefaultContextMaxTokens` al request tipado y aplicara por separado su presupuesto de presentacion `1..16384` al JSON publico completo. Asi conserva los schemas MCP y la validacion compartida sin modificar el rango de la CLI ni enviar un zero value invalido.

El adapter convertira la respuesta del servicio al `structuredContent` documentado. El adapter aplicara `max_tokens` antes de serializar. Search eliminara resultados completos del final. Context eliminara records completos de los extremos y conservara el anchor. Si el objeto minimo no cabe, el adapter devolvera `invalid_arguments` y `invalid arguments`.

`max_tokens` sera obligatorio despues de defaults. Su rango sera `1..16384`. Su default sera `2000`. El adapter usara el mismo estimador de tokens que la CLI.

El adapter validara `structuredContent` contra el `outputSchema` antes de devolver exito. El JSON canonico completo de `content` y `structuredContent` debera caber tambien en el limite duro de respuesta de 2 MiB.

Un fallo de input devolvera `invalid_arguments` con texto fijo y saneado. El adapter no incluira el error del validador, JSON pointers, nombres de tipos Go, SQL, valores recibidos ni stack traces. Un fallo interno de output devolvera `query_failed` con texto fijo y saneado. El proceso enviara el detalle interno solo a stderr.

Una prueba con un spy de `QueryService` exigira cero invocaciones cuando falle el schema de input. Otra prueba inyectara errores internos del validador. Esa prueba exigira que ningun texto interno aparezca en `content`, `structuredContent`, headers ni logs dirigidos al cliente.

## Runtime de busqueda y embeddings

El wiring actual de Search CLI sigue esta ruta:

1. `runSearch` recibe la configuracion resuelta.
2. `prepareIndex` abre la base preparada.
3. `services.QueryService.Search` llama `Database.HybridSearchContext` cuando `relax` es false.
4. `HybridSearchContext` ejecuta BM25 y decide si usa el provider de embeddings.

En la base de este contrato, el codigo de produccion de `cmd/backscroll` no llama `embedding.NewOnnxProvider` ni `Database.SetEmbeddingProvider`. Solo los tests conectan un provider. Por eso Search CLI deja el provider en nil y `HybridSearchContext` usa el fallback lexico aunque `lexical_only` sea false.

La implementacion MCP no creara una segunda politica. La implementacion del issue 48 compartira la construccion y ownership del provider entre Search CLI y MCP. Ambos consumiran la misma `config.Embedding` y la misma factory. El caller suministrara la DB que corresponde a su superficie. Search CLI conservara la DB canonica preparada. MCP suministrara el snapshot privado.

La invocacion MCP de esa construccion ejecutara esta secuencia:

1. Resolvera la misma `config.Embedding` que Search CLI.
2. Si embeddings estan habilitados, construira el provider con esa configuracion.
3. Abrira el snapshot privado con `OpenReadOnly`.
4. Si existe provider, instalara el wrapper mutex sobre ese provider.
5. Conectara el wrapper mediante `Database.SetEmbeddingProvider`.
6. Publicara `QueryService` solo despues de completar los pasos anteriores.

Cuando embeddings esten deshabilitados, la secuencia seleccionara provider nil de forma explicita y continuara con `OpenReadOnly` sobre el snapshot. MCP no abrira la DB canonica mediante esta construccion.

Un fallo de construccion del provider terminara antes de abrir la DB. Un fallo de `OpenReadOnly` sobre el snapshot cerrara el provider ya creado. Un fallo posterior cerrara primero el provider y despues la DB. Cada cierre ocurrira como maximo una vez. Ninguno de estos fallos publicara readiness ni se convertira en resultado de tool.

Despues de readiness, `HybridSearchContext` conservara sus fallbacks de produccion:

- `lexical_only: true` omite el provider;
- provider nil usa BM25;
- cero vectores usa BM25;
- un error no relacionado con cancelacion al contar o leer vectores usa BM25;
- un error no relacionado con cancelacion al ejecutar `Embed` usa BM25;
- cero resultados despues de `similarity_threshold` usa BM25;
- `context.Canceled` y `context.DeadlineExceeded` no usan fallback y conservan su taxonomia.

El runtime envolvera el provider compartido con un mutex. `Embed` comprobara el contexto antes de `Lock` y justo despues de adquirirlo. Mantendra el mutex durante toda la llamada. Solo una llamada `Embed` estara activa. El mutex no cubrira BM25 ni queries con `lexical_only: true`.

El runtime compartido sera propietario del provider que cree. `Database.Close` solo cierra `sql.DB`; no cierra el provider. El servidor no cerrara el provider por request. Un `sync.Once` protegera `Close`.

Durante shutdown, el gate impedira admission nueva y drenara `active` a cero. Solo despues, el runtime adquirira el mutex del provider y ejecutara `Close` exactamente una vez. Despues cerrara `Database`. Esta secuencia impide una carrera entre `Embed` y `Close`. Los errores de cierre se combinaran para el diagnostico de stderr y no se enviaran al cliente.

Una prueba E2E ejecutara Search CLI y `backscroll_search` sobre la misma fixture con una factory que devuelve `embedding.MockEmbeddingProvider`. La prueba usara `lexical_only: false`. La prueba comprobara que ambos caminos invocan el provider y devuelven el mismo orden.

Otra prueba E2E almacenara vectores conocidos y enviara dos valores de `similarity_threshold`. La prueba comprobara la misma seleccion y el mismo fallback en CLI y MCP. Una prueba adicional inyectara un error ordinario de `Embed` y exigira fallback lexico. Una prueba de cancelacion exigira ausencia de fallback.

Una prueba lanzara dos busquedas vectoriales concurrentes. El provider mock medira una concurrencia maxima de `Embed` igual a 1. Otra prueba bloqueara dos busquedas `lexical_only: true` en SQLite y comprobara que ambas avanzan de forma concurrente. Una prueba de shutdown comprobara que `Close` ocurre despues de drenar requests y nunca corre junto con `Embed`.

## Vector search streaming para MCP

MCP no usara `LoadChunkEmbeddings` ni construira un slice del corpus. El issue 48 añadira una opcion interna de vector streaming para MCP. La CLI conservara su conducta actual sin deferir requisitos del servidor.

La ruta streaming ejecutara este orden:

1. Obtendra la dimension `D` del provider.
2. Adquirira el gate vectorial de capacidad 1 con un select sobre el contexto del handler.
3. Ejecutara `provider.Embed` bajo el wrapper mutex.
4. Abrira un cursor SQL sobre embeddings del snapshot.
5. Ordenara el cursor por `ItemID ASC`, `chunk_idx ASC` y chunk rowid ASC.
6. Procesara grupos contiguos del mismo `ItemID`.
7. Leera y decodificara un solo BLOB por iteracion.
8. Calculara cosine similarity para el chunk.
9. Mantendra solo el `ItemID` actual y su score maximo.
10. Al terminar el grupo, filtrara e insertara como maximo una entrada en el min-heap top-K.
11. Cerrara rows y liberara el gate vectorial.
12. Aplicara RRF con los candidatos lexicos acotados.

El gate vectorial cubrira desde antes de `Embed` hasta despues de cerrar el scan vectorial. Cuando su capacidad este ocupada, la busqueda vectorial esperara dentro de su deadline de 30 segundos. Esta saturacion vectorial no devolvera HTTP 429 porque la request ya entro al gate general. Si vence, devolvera `query_deadline_exceeded` cuando la respuesta pueda entregarse. Una cancelacion devolvera `query_canceled`. No habra un segundo scan vectorial activo.

Una busqueda `lexical_only: true` no adquirira el gate vectorial. Una busqueda con provider nil tampoco lo adquirira. Esas busquedas podran usar otros slots del gate general de forma concurrente.

El candidate cap sera 100. Para conservar la politica existente, `K` sera `min(100,max(1,2*limit))`. Como MCP limita `limit` a 20, `K` sera como maximo 40 en el contrato inicial.

El cursor unira `search_items` y `mcp_visible_rowids` antes de leer el BLOB. Para search sin `content_type`, exigira `unfiltered_visible=1`. La query exigira `length(embedding)=4*D` antes de proyectar el BLOB. Una dimension incompatible no se cargara.

Al cerrar un grupo, el algoritmo descartara un score maximo `<=0`. Tambien descartara un score menor que `similarity_threshold`. Solo un score positivo que cumpla el threshold podra entrar al heap.

El loop comprobara cancelacion antes de `rows.Next`, despues de `Scan` y despues de calcular cada score. Un error de cursor no relacionado con cancelacion conservara el fallback lexico de produccion.

La memoria Go controlada del algoritmo sera O(D+K). En cada iteracion podra existir el query vector de `4*D` bytes, un BLOB de `4*D`, un vector decodificado de `4*D`, el `ItemID` actual, su score maximo y el heap. `vectorCandidate` contendra `itemID int64` y `score float64`. Su tamaño esperado sera 16 bytes. El heap usara como maximo `100*16=1600` bytes de backing entries, mas el header de slice.

El algoritmo no construira un slice ni un map del corpus de embeddings. El map de RRF solo contendra candidatos lexicos y top-K acotados. El modelo del provider, SQLite page cache y buffers internos del driver quedan fuera de esta memoria Go controlada.

El orden vectorial final sera `Similarity DESC, ItemID ASC`. El min-heap tratara como peor entrada el menor score y, ante empate, el mayor `ItemID`. Esa regla hace estable el reemplazo del heap.

La ruta load-all actual ordena solo por score y no garantiza el desempate entre ItemIDs. El issue 48 compartira ese comparador determinista con la ruta CLI de referencia para que CLI y MCP conserven el mismo top-K tambien ante empates, sin cambiar los scores ni la politica de threshold.

RRF conservara su desempate actual: score RRF DESC y string de ID ASC. Dentro del top-K documentado, el score, filtro `similarity_threshold`, orden y RRF seran iguales a la ruta de referencia. Items vectoriales sin candidato BM25 seguiran sin producir snippet y conservaran la exclusion observable actual.

Las pruebas sembraran un corpus mayor que K. Compararan la ruta streaming con una referencia load-all que use los mismos desempates. Exigiran el mismo top-K y el mismo orden RRF.

Una fixture tendra varios chunks para un ItemID y comprobara que solo entra su score maximo. Otra fixture incluira maximos cero y negativos y comprobara su descarte. Otra aplicara `similarity_threshold` antes del heap. Otra creara scores iguales y comprobara `ItemID ASC` y el desempate string de RRF.

Un detector de allocations verificara que el crecimiento no sea proporcional al numero de filas. Una prueba cancelara en cada posicion del cursor. Otra prueba ejecutara dos busquedas vectoriales y comprobara un solo scan activo. Una busqueda lexica concurrente debera completar mientras la vectorial espera o escanea.

## Materializacion acotada en storage

El issue 48 añadira `MaxTextRunes int` a la opcion interna de search y a la query interna de context. El adapter MCP fijara `MaxTextRunes=16384`. La CLI fijara `MaxTextRunes=0` y conservara su comportamiento actual.

`QueryService.Search` y `QueryService.Context` propagaran el valor a storage. Storage aplicara el limite en SQL antes de `Scan`. La proyeccion usara `substr(text,1,MaxTextRunes)` cuando el limite sea positivo. Un valor cero proyectara `text` completo.

Search aplicara la proyeccion solo al payload materializado. BM25, FTS, filtros, RRF, `similarity_threshold` y orden se calcularan antes de esa proyeccion. La truncacion no cambiara ranking ni seleccion.

Search proyectara el snippet como `substr(snippet(...),1,1024)` antes de `Scan`. Cada snippet tendra como maximo 1024 runes y 4096 bytes UTF-8. El adapter no recibira un snippet completo mayor que ese limite.

Context mantendra `before<=50` y `after<=50`. Por eso materializara como maximo 101 records. `storage.ContextRecord` añadira `TextTruncated bool`. SQL calculara esa marca con `length(text)>MaxTextRunes`. MCP publicara la marca como `text_truncated`.

Cada texto materializado por MCP tendra como maximo 16384 runes. UTF-8 usa como maximo cuatro bytes por rune. Cada texto tendra como maximo 65536 bytes.

El peor caso de context materializara `101 * 65536 = 6619136` bytes de texto por handler. Search materializara como maximo `20 * 4096 = 81920` bytes de snippets. El adapter podra conservar una segunda copia de los textos o snippets antes de aplicar `max_tokens`. Cada slot podra conservar ademas 1 MiB de body raw, 1 MiB de body delegado, 1 MiB de copia SDK, 2 MiB de JSON canonico y 2 MiB de captura.

Las copias de bytes enumeradas seran menores de 20 MiB por handler. Su techo derivado para 16 handlers sera 320 MiB. Este valor no promete un limite total de proceso.

La cardinalidad de objetos de resultados estara acotada por 20 resultados de search o 101 records de context. Su overhead Go dependera del build y no recibira una cifra inventada.

El vector gate añadira una sola memoria vectorial controlada de `12*D` bytes mas un heap de hasta 1600 bytes y headers. La memoria del modelo provider, los buffers internos del driver, SQLite page cache, indices y archivos mmap se documentan por separado y no forman parte del calculo de copias Go.

La construccion de visibilidad ocurre antes de readiness. Su memoria sera una fila broad-candidate completa a la vez y no tendra un bound numerico falso. El limite de respuesta seguira siendo 2 MiB.

Las fronteras internas MCP usaran `int64` antes del adapter. `storage.SearchResult.Ordinal` y `models.SearchResult.Ordinal` migraran a `int64`. Los contadores expuestos por `storage.Stats` migraran a `int64`. `storage.ContextRecord.Ordinal` ya usa `int64`.

El adapter MCP convertira ordinals y contadores a strings decimales. Si una ruta CLI conserva un destino `int`, esa ruta usara una conversion comprobada y devolvera overflow de forma explicita. Ninguna frontera MCP dependera de `strconv.IntSize`.

## Metadata de exito

Cada `structuredContent` exitoso incluira `metadata`.

`vcs_revision` se omitira cuando el binario no contenga `vcs.revision` en su build info. El servidor no inventara una revision.

`indexed_at` se omitira cuando `storage.Stats.IndexedAt` sea zero. El servidor no serializara el tiempo zero de Go. Search y context obtendran este valor mediante `QueryService.Status` despues de su consulta. El valor informa el ultimo sync confirmado. No afirma que ambas consultas usaron una sola transaccion.

Los demas campos de metadata siempre estaran presentes:

- `protocol_version`;
- `server_version`;
- `database_mode`;
- `sync_policy`;
- `server_started_at`;
- `initial_sync_completed_at`;
- `queried_at`.

`server_version` sera exactamente `buildVersion`. Coincidira con `mcp.Implementation.Version` y con `_meta.io.modelcontextprotocol/serverInfo.version`.

La metadata describe datos confirmados. No promete que los archivos fuente no hayan cambiado despues de `indexed_at`.

## Schemas de `backscroll_search`

La tool ejecutara `QueryService.Search`. La tool conservara ranking, filtros, query echo y relajacion. La tool nunca leera archivos fuente.

El cliente debe enviar `project` o `all_projects: true`. Esta regla evita usar el directorio de trabajo del proceso como estado implicito.

### Input schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "query": {"type": "string", "minLength": 1},
    "project": {"type": "string", "minLength": 1},
    "all_projects": {"type": "boolean", "const": true},
    "source": {"type": "string", "minLength": 1},
    "source_path": {"type": "string", "minLength": 1},
    "after": {"type": "string", "format": "date"},
    "before": {"type": "string", "format": "date"},
    "role": {"type": "string", "minLength": 1},
    "content_type": {
      "type": "string",
      "enum": ["text", "code", "tool", "reasoning"]
    },
    "tag": {"type": "string", "minLength": 1},
    "limit": {"type": "integer", "minimum": 1, "maximum": 20, "default": 20},
    "offset": {
      "type": "integer",
      "minimum": 0,
      "maximum": 2147483647,
      "default": 0
    },
    "max_tokens": {
      "type": "integer",
      "minimum": 1,
      "maximum": 16384,
      "default": 2000
    },
    "lexical_only": {"type": "boolean", "default": false},
    "similarity_threshold": {
      "type": "number",
      "minimum": 0,
      "maximum": 1,
      "default": 0.3
    },
    "relax": {"type": "boolean", "default": false}
  },
  "required": ["query"],
  "oneOf": [
    {
      "required": ["project"],
      "not": {"required": ["all_projects"]}
    },
    {
      "required": ["all_projects"],
      "not": {"required": ["project"]}
    }
  ]
}
```

Los filtros ausentes no se aplican. `before` conserva la comparacion exclusiva de la CLI. `relax: true` aplica las reglas de [busqueda con opt-in](search.md#opt-in-lexical-relaxation). `relax` no amplia el scope.

Este input tiene una frase relax sin cierre y es invalido:

```json
{
  "query": "\"unterminated",
  "all_projects": true,
  "relax": true
}
```

`services.ValidateSearchRequest` devolvera error antes de `QueryService.Search`. El resultado de tool usara `invalid_arguments` y `invalid arguments`. Un spy de `QueryService` registrara cero invocaciones.

`offset` queda limitado a `0..2147483647`. El adapter de `Server.AddTool` valida este valor antes del handler. El limite evita diferencias entre tamaños de `int` y evita overflow al construir la paginacion SQL. `limit` queda limitado a `1..20` para MCP. `max_tokens` usa default `2000`, exige un valor positivo y no supera `16384`. Los demas integers conservan sus limites existentes.

### Output schema de `structuredContent`

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "$defs": {
    "metadata": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "protocol_version": {"type": "string", "const": "2026-07-28"},
        "server_version": {"type": "string", "minLength": 1},
        "vcs_revision": {"type": "string", "minLength": 1},
        "database_mode": {"type": "string", "const": "read_only"},
        "sync_policy": {"type": "string", "const": "startup_only"},
        "server_started_at": {"type": "string", "format": "date-time"},
        "initial_sync_completed_at": {"type": "string", "format": "date-time"},
        "indexed_at": {"type": "string", "format": "date-time"},
        "queried_at": {"type": "string", "format": "date-time"}
      },
      "required": [
        "protocol_version",
        "server_version",
        "database_mode",
        "sync_policy",
        "server_started_at",
        "initial_sync_completed_at",
        "queried_at"
      ]
    },
    "result": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "source": {"type": "string"},
        "source_path": {"type": "string"},
        "uuid": {"type": ["string", "null"]},
        "ordinal": {
          "type": "string",
          "pattern": "^(0|[1-9][0-9]*)$"
        },
        "project": {"type": "string"},
        "role": {"type": "string"},
        "origin": {
          "type": "string",
          "enum": ["human", "assistant", "system", "automation", "unknown"]
        },
        "content_type": {
          "type": "string",
          "enum": ["text", "code", "tool", "reasoning"]
        },
        "timestamp": {"type": ["string", "null"], "format": "date-time"},
        "snippet": {"type": "string"},
        "score": {"type": "number"},
        "match_stage": {"type": "string", "enum": ["drop-terms"]},
        "dropped_terms": {
          "type": "array",
          "items": {"type": "string"}
        }
      },
      "required": [
        "source",
        "source_path",
        "uuid",
        "ordinal",
        "project",
        "role",
        "origin",
        "content_type",
        "timestamp",
        "snippet",
        "score"
      ],
      "dependentRequired": {
        "match_stage": ["dropped_terms"],
        "dropped_terms": ["match_stage"]
      }
    }
  },
  "properties": {
    "results": {
      "type": "array",
      "items": {"$ref": "#/$defs/result"}
    },
    "relaxation_stages": {
      "type": "array",
      "items": {"type": "string"}
    },
    "metadata": {"$ref": "#/$defs/metadata"}
  },
  "required": ["results", "relaxation_stages", "metadata"]
}
```

`results` sera un array vacio cuando no existan resultados. `relaxation_stages` sera un array vacio cuando no se intente relajacion. `match_stage` y `dropped_terms` se omitiran cuando la consulta no devuelva una etapa con terminos eliminados. `uuid` y `timestamp` siempre estaran presentes y pueden ser null. `vcs_revision` e `indexed_at` son las unicas propiedades opcionales de metadata.

`ordinal` se serializara como string decimal no negativo. El adaptador usara conversion integer a decimal y no pasara por `float64`. Esta forma conserva valores `int64` sin perdida.

Este excerpto conserva el valor maximo de `int64`:

```json
{
  "ordinal": "9223372036854775807"
}
```

`storage.SearchResult.ID`, `Text` y `SearchEcho` no forman parte del resultado publico. El orden del array conserva el ranking de `SearchResponse.Results`.

## Schemas de `backscroll_context`

La tool ejecutara `QueryService.Context`. El cliente debe enviar un UUID o una coordenada exacta.

### Input schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "uuid": {"type": "string", "minLength": 1},
    "source_path": {"type": "string", "minLength": 1},
    "ordinal": {
      "type": "string",
      "pattern": "^(0|[1-9][0-9]{0,18})$",
      "description": "Decimal canonico no negativo; el adapter exige valor <= 9223372036854775807."
    },
    "before": {"type": "integer", "minimum": 0, "maximum": 50, "default": 5},
    "after": {"type": "integer", "minimum": 0, "maximum": 50, "default": 5},
    "max_tokens": {
      "type": "integer",
      "minimum": 1,
      "maximum": 16384,
      "default": 2000
    }
  },
  "oneOf": [
    {
      "required": ["uuid"],
      "not": {
        "anyOf": [
          {"required": ["source_path"]},
          {"required": ["ordinal"]}
        ]
      }
    },
    {
      "required": ["source_path", "ordinal"],
      "not": {"required": ["uuid"]}
    }
  ]
}
```

`source_path` debe ser exacto. No admite glob. Los valores ausentes de `before` y `after` usan los defaults del schema.

El input `ordinal` sera un string decimal canonico no negativo. El limite superior sera `9223372036854775807` (`MaxInt64`). El adapter lo parseara directamente a `int64` sin pasar por `float64` ni `int`. La frontera MCP no dependera del tamaño de palabra del build. `before` y `after` conservan el rango `0..50`. `max_tokens` usa default `2000`, exige un valor positivo y no supera `16384`.

Este selector usa el limite superior exacto:

```json
{
  "source_path": "/indexed/session.jsonl",
  "ordinal": "9223372036854775807"
}
```

### Output schema de `structuredContent`

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "$defs": {
    "metadata": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "protocol_version": {"type": "string", "const": "2026-07-28"},
        "server_version": {"type": "string", "minLength": 1},
        "vcs_revision": {"type": "string", "minLength": 1},
        "database_mode": {"type": "string", "const": "read_only"},
        "sync_policy": {"type": "string", "const": "startup_only"},
        "server_started_at": {"type": "string", "format": "date-time"},
        "initial_sync_completed_at": {"type": "string", "format": "date-time"},
        "indexed_at": {"type": "string", "format": "date-time"},
        "queried_at": {"type": "string", "format": "date-time"}
      },
      "required": [
        "protocol_version",
        "server_version",
        "database_mode",
        "sync_policy",
        "server_started_at",
        "initial_sync_completed_at",
        "queried_at"
      ]
    },
    "record": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "uuid": {"type": ["string", "null"]},
        "source_path": {"type": "string"},
        "ordinal": {
          "type": "string",
          "pattern": "^(0|[1-9][0-9]*)$"
        },
        "role": {"type": "string"},
        "origin": {
          "type": "string",
          "enum": ["human", "assistant", "system", "automation", "unknown"]
        },
        "timestamp": {"type": ["string", "null"], "format": "date-time"},
        "content_type": {"type": "string", "minLength": 1},
        "source": {"type": "string"},
        "text": {"type": "string"},
        "text_truncated": {"type": "boolean"},
        "is_anchor": {"type": "boolean"}
      },
      "required": [
        "uuid",
        "source_path",
        "ordinal",
        "role",
        "origin",
        "timestamp",
        "content_type",
        "source",
        "text",
        "text_truncated",
        "is_anchor"
      ]
    }
  },
  "properties": {
    "records": {
      "type": "array",
      "minItems": 1,
      "items": {"$ref": "#/$defs/record"},
      "contains": {
        "type": "object",
        "properties": {
          "is_anchor": {"const": true}
        },
        "required": ["is_anchor"]
      },
      "minContains": 1,
      "maxContains": 1
    },
    "metadata": {"$ref": "#/$defs/metadata"}
  },
  "required": ["records", "metadata"]
}
```

La tool devolvera todos los records de `ContextResponse.Records`. El schema exige exactamente un record con `is_anchor: true`. `uuid` y `timestamp` siempre estaran presentes y pueden ser null. Todos los campos de `storage.ContextRecord` tienen una representacion directa. `Anchor` se serializa como `is_anchor`. `TextTruncated` se serializa como `text_truncated`.

`content_type` preservara el valor no vacio almacenado. El schema no cerrara ese campo con un enum. Esta regla permite valores compatibles como `text/markdown`.

`ordinal` se serializara como string decimal no negativo. Esta forma permite el round-trip exacto de cualquier valor almacenado en `storage.ContextRecord.Ordinal`. Un valor fuera del rango seguro JSON no pasara por un number JSON.

## Schemas de `backscroll_status`

La tool ejecutara `QueryService.Status`. La tool no aceptara argumentos. La tool no iniciara sync. La tool no volvera a cargar manifests.

### Input schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "description": "No acepta argumentos ni integers de input.",
  "type": "object",
  "additionalProperties": false,
  "maxProperties": 0
}
```

### Output schema de `structuredContent`

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "$defs": {
    "metadata": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "protocol_version": {"type": "string", "const": "2026-07-28"},
        "server_version": {"type": "string", "minLength": 1},
        "vcs_revision": {"type": "string", "minLength": 1},
        "database_mode": {"type": "string", "const": "read_only"},
        "sync_policy": {"type": "string", "const": "startup_only"},
        "server_started_at": {"type": "string", "format": "date-time"},
        "initial_sync_completed_at": {"type": "string", "format": "date-time"},
        "indexed_at": {"type": "string", "format": "date-time"},
        "queried_at": {"type": "string", "format": "date-time"}
      },
      "required": [
        "protocol_version",
        "server_version",
        "database_mode",
        "sync_policy",
        "server_started_at",
        "initial_sync_completed_at",
        "queried_at"
      ]
    },
    "index": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "total_files": {"type": "string", "pattern": "^(0|[1-9][0-9]*)$"},
        "total_messages": {"type": "string", "pattern": "^(0|[1-9][0-9]*)$"},
        "total_chunks": {"type": "string", "pattern": "^(0|[1-9][0-9]*)$"},
        "total_embeddings": {"type": "string", "pattern": "^(0|[1-9][0-9]*)$"},
        "total_vectors": {"type": "string", "pattern": "^(0|[1-9][0-9]*)$"}
      },
      "required": [
        "total_files",
        "total_messages",
        "total_chunks",
        "total_embeddings",
        "total_vectors"
      ]
    }
  },
  "properties": {
    "index": {"$ref": "#/$defs/index"},
    "metadata": {"$ref": "#/$defs/metadata"}
  },
  "required": ["index", "metadata"]
}
```

Los cinco contadores proceden directamente de `storage.Stats`. La tool los serializara como strings decimales no negativos. Esta forma evita perdida cuando un contador excede el rango seguro JSON. Por ejemplo, `total_messages` puede serializarse como `"9007199254740992"` sin perdida. `storage.Stats.IndexedAt` se serializa una sola vez como `metadata.indexed_at`. La tool omite `indexed_at` cuando el valor es zero. La tool no expone path de base, tamaño de archivo ni manifests porque `QueryService.Status` no devuelve esos campos.

## Errores

El servidor separara errores HTTP, errores JSON-RPC y errores de tool.

### Errores HTTP y JSON-RPC

La primera condicion aplicable termina la solicitud.

| Fase | Condicion | Resultado unico |
| --- | --- | --- |
| 1 | `Origin` duplicado, malformed, `null` o distinto | HTTP 403. |
| 2 | `Authorization` ausente, duplicado o invalido | HTTP 401. |
| 3 | Path distinto de `/mcp` | HTTP 404 para cualquier metodo. |
| 4 | Metodo HTTP no POST en `/mcp` | HTTP 405 con `Allow: POST`. |
| 5 | Headers aceptados superan `60 << 10` segun la formula | HTTP 431 y body fijo. |
| 6 | `Content-Type` duplicado | HTTP 400, texto fijo y `text/plain; charset=utf-8`. |
| 6 | `Content-Type` ausente o no admitido | HTTP 415. |
| 7 | Lista combinada de `Accept` ausente, malformed o incompleta | HTTP 400, texto fijo y `text/plain; charset=utf-8`. |
| 8 | Gate con `accepting=false` antes de leer body | HTTP 503 y body fijo; cero bytes leidos. |
| 8 | Gate con `active=16` antes de leer body | HTTP 429 y body fijo; cero bytes leidos y cero llamadas al SDK. |
| 9 | Body mayor de 1 MiB | HTTP 413. |
| 10 | Body con UTF-8 invalido | HTTP 400 y `-32700` sin clave `id`; respuesta UTF-8 valida. |
| 11 | JSON malformado, body vacio o trailing data | HTTP 400 y `-32700`; conserva un `id` valido preextraido y omite otro `id`. |
| 12 | Objeto JSON con clave `id` invalida | HTTP 400 y `-32600` sin clave `id`. |
| 13 | Batch, envelope o `params` con forma invalida | HTTP 400 y `-32600`; conserva solo un `id` valido preextraido. |
| 14 | Metadata obligatoria ausente o invalida en request | HTTP 400 y `-32602` con el mismo `id`. |
| 14 | Metadata opcional malformed en notification | HTTP 400 sin body. |
| 15 | `MCP-Protocol-Version` ausente, duplicado o malformed | Request: HTTP 400 y `-32020` con el mismo `id`. Notification: HTTP 400 sin body. |
| 15 | Header y revision de `_meta` distintos | Request: HTTP 400 y `-32020` con el mismo `id`. Notification: HTTP 400 sin body. |
| 15 | Revisiones coincidentes pero no soportadas | Request: HTTP 400 y `-32022` con el mismo `id`. Notification: HTTP 400 sin body. |
| 16 | `Mcp-Method` o `Mcp-Name` ausente, duplicado, malformed o distinto | Request: HTTP 400 y `-32020` con el mismo `id`. Notification: HTTP 400 sin body. |
| 17 | `tools/call` con nombre fuera de las tres tools | Request: HTTP 400 y `-32602` con el mismo `id`. Notification: HTTP 400 sin body. |
| 18 | Notification valida con metodo permitido o bloqueado | HTTP 202 sin body y cero handlers invocados. |
| 18 | Request con `Mcp-Method` bloqueado | HTTP 404 y `-32601` con el mismo `id`. |
| 19 | Request con metodo permitido | Delegacion al SDK con captura y restauracion de `id`. |
| 20 | Respuesta SDK mayor de 2 MiB, malformed, con trailing data o `id` inesperado | HTTP 500, `application/json`, `-32603`, `Internal error` y `id` raw. |

El error `UnsupportedProtocolVersion` correlacionara el request e incluira ambos campos de data:

```json
{
  "jsonrpc": "2.0",
  "id": "revision-1",
  "error": {
    "code": -32022,
    "message": "Unsupported protocol version",
    "data": {
      "requested": "revision-recibida",
      "supported": ["2026-07-28"]
    }
  }
}
```

Una solicitud `initialize` con metadata moderna recibira `Method not found`. Una solicitud legacy sin metadata moderna fallara antes. El servidor no respondera con una revision legacy.

### Errores de tool

El adapter aplicara esta taxonomia cerrada mediante `errors.Is` cuando exista un sentinel:

| Fuente tipada o fase | `code` | `message` exacto |
| --- | --- | --- |
| Forma de `arguments`, input schema, defaults, fecha, conversion de input o validadores compartidos de `services` | `invalid_arguments` | `invalid arguments` |
| `storage.ErrInvalidContextRecordQuery` | `invalid_arguments` | `invalid arguments` |
| `storage.ErrContextRecordNotFound` | `context_not_found` | `context not found` |
| `storage.ErrContextRecordAmbiguous` | `context_ambiguous` | `context ambiguous` |
| `context.Canceled` | `query_canceled` | `query canceled` |
| `context.DeadlineExceeded` | `query_deadline_exceeded` | `query deadline exceeded` |
| Cualquier otro error de `QueryService`, conversion de output o `outputSchema` | `query_failed` | `query failed` |

Esta tabla es exhaustiva para errores de tool. El adapter no creara otras categorias.

Un error de tool tendra esta forma:

```json
{
  "resultType": "complete",
  "content": [
    {
      "type": "text",
      "text": "{\"code\":\"context_not_found\",\"message\":\"context not found\"}"
    }
  ],
  "isError": true
}
```

`structuredContent` estara ausente. La respuesta no incluira metadata de exito.

El adapter devolvera los resultados de `context.Canceled` y `context.DeadlineExceeded` solo cuando el transporte todavia pueda entregar la respuesta. Si el request HTTP ya termino, el servidor no intentara escribir una respuesta.

Los fallos de configuracion, apertura de SQLite, compatibilidad, migracion o startup sync ocurren antes de readiness. El proceso los escribira en stderr y terminara con status distinto de cero. Esos fallos no son resultados de tool.

El servidor no devolvera SQL, tokens, paths internos, texto del validador ni stack traces. El servidor escribira el detalle operativo en stderr sin secretos.

## Snapshot SQLite privado

El startup creara el snapshot despues del sync canonico y mientras conserva el lock de startup. Usara la API backup de SQLite sobre la conexion canonica abierta. No copiara bytes del archivo con `io.Copy`.

El destino inicial sera un archivo partial con nombre criptograficamente aleatorio dentro del directorio privado de runtime de Backscroll. En Unix, el archivo tendra modo `0600`. En Windows, el archivo tendra una ACL equivalente de solo owner mediante la implementacion especifica de plataforma.

La backup API copiara una vista SQLite consistente, incluidas transacciones WAL confirmadas. El startup completara todos los pasos de backup. Cerrara la conexion destino. Ejecutara fsync del archivo. Renombrara el partial de forma atomica dentro del mismo filesystem. Ejecutara fsync del directorio cuando la plataforma lo permita.

Solo despues de completar esos pasos, el startup cerrara la DB canonica. Solo despues liberara el lock de startup. Un fallo anterior mantendra el lock hasta cerrar conexiones y limpiar el partial.

El nombre final tendra el prefijo `.backscroll-mcp-snapshot-`, una identidad de owner y un sufijo aleatorio. El proceso mantendra un lease advisory abierto para el snapshot. El lease no sera un PID file.

Al iniciar, despues de reservar el listener, Backscroll inspeccionara solo archivos con ese prefijo en su directorio privado. Eliminara un snapshot partial o final solo cuando pertenezca al mismo owner, sea un archivo regular, no sea symlink y su lease pueda adquirirse. Esta regla conserva snapshots activos de otro proceso.

Un fallo de startup despues de publicar el snapshot cerrara conexiones privadas y eliminara snapshot, lease y sidecars antes de terminar.

Durante shutdown normal, Backscroll cerrara la DB snapshot. Eliminara el snapshot, su lease y sidecars privados. Ejecutara fsync del directorio cuando la plataforma lo permita. Un crash dejara un snapshot huerfano para la limpieza segura del siguiente startup.

El snapshot no reemplazara ni mutara la DB canonica. Las tablas internas MCP existiran solo en el snapshot.

## Tabla de visibilidad MCP

Antes de readiness, Backscroll abrira el snapshot en modo privado de mantenimiento. Creara `mcp_visible_rowids` dentro de una transaccion. La tabla tendra una fila por cada `search_items.id` y una marca `unfiltered_visible`.

La construccion sera exhaustiva. Una fila tendra `unfiltered_visible=0` por provenance solo cuando `content_type='tool'` y `search_echo!=0`. Esa marca conserva el pairing exacto que ya produjo ingestion.

Una fila con `content_type` igual a `text`, `code` o `reasoning` tendra `unfiltered_visible=1` aunque `search_echo` sea true. La marca `search_echo` no ocultara contenido no tool.

Para filas tool con `COALESCE(search_echo,0)=0`, el constructor usara el broad prefilter actual: `content_type='tool'` y texto que contiene el literal `backscroll`. Leera el texto completo de cada candidato, una fila a la vez. Aplicara `directsearch.IsSerializedDirectSearchCall` como frontera estricta. Un match tendra `unfiltered_visible=0`. Las demas filas tendran valor 1.

El constructor no adivinara pairing para outputs sin evidencia. Conservara las limitaciones actuales de fuentes expiradas y provenance historico. No introducira una regla nueva de query echo.

Toda consulta MCP de search o relax unira sus rowids con `mcp_visible_rowids`. Una consulta sin `content_type` exigira `unfiltered_visible=1`. Una consulta explicita por `content_type`, incluida `tool`, unira la tabla pero no aplicara esa marca. Esta regla conserva la visibilidad explicita actual de echoes de tool.

La page de search sin filtro y el IDF de relax sin filtro uniran `mcp_visible_rowids` y exigiran `unfiltered_visible=1`. El IDF con `content_type` explicito conservara todas las filas. Search y IDF usaran asi el mismo conjunto exacto.

La tabla se construira con texto completo antes de aplicar `MaxTextRunes`. Despues de construirla, las queries podran usar `substr` en la proyeccion sin cambiar seleccion, pairing, visibilidad ni IDF.

La memoria de startup para este paso sera O(tamaño del mayor row broad-candidate). El constructor procesara una fila a la vez y no retendra una coleccion de textos. No se declara un bound numerico porque una fila canonica puede tener tamaño no acotado. Esta memoria de startup no forma parte de la memoria concurrente posterior a readiness.

Despues del commit, Backscroll cerrara el modo de mantenimiento. Hara fsync del snapshot. Verificara que no queden sidecars de mantenimiento. El runtime abrira solo el snapshot read-only.

## Ciclo de vida y readiness

El proceso seguira este orden:

1. Cargara y validara la configuracion.
2. Resolvera el token.
3. Validara que la direccion sea loopback.
4. Ejecutara `net.Listen` para reservar el listener.
5. Fallara si el bind ya esta ocupado.
6. Limpiara snapshots huerfanos seguros del mismo owner.
7. Intentara adquirir durante cinco segundos el lock existente de startup sync.
8. Preparara o migrara la DB canonica mediante la ruta existente.
9. Ejecutara exactamente un sync inicial sobre la DB canonica.
10. Mantendra abierta la DB canonica y mantendra el lock.
11. Creara un snapshot SQLite privado mediante la API backup de SQLite.
12. Completara, cerrara y hara durable el snapshot.
13. Cerrara la DB canonica.
14. Liberara el lock de startup solo despues del cierre canonico.
15. Abrira el snapshot en modo privado de mantenimiento.
16. Construira la tabla interna de visibilidad.
17. Cerrara el modo de mantenimiento.
18. Construira el provider desde la configuracion compartida.
19. Abrira solo el snapshot con `OpenReadOnly()`.
20. Instalara el wrapper mutex del provider.
21. Construira `QueryService` y el servidor MCP.
22. Iniciara `http.Server.Serve` sobre el listener reservado.
23. Ejecutara un self-probe autenticado de `server/discover`.
24. Escribira readiness en stderr solo despues del self-probe exitoso.

El self-probe tendra un timeout total de cinco segundos. El self-probe usara la URL loopback efectiva. El self-probe enviara el bearer token. El self-probe enviara la revision exacta, `Mcp-Method`, `io.modelcontextprotocol/clientCapabilities` y el resto de cabeceras POST obligatorias. El self-probe no enviara `Origin`.

El self-probe debe recibir HTTP 200. El body debe contener `resultType: "complete"`, `supportedVersions: ["2026-07-28"]` y `capabilities.tools`. El proceso cerrara el servidor y terminara con status distinto de cero si el probe falla o vence.

El servidor no estara ready antes del paso 24. El inicio de `Serve` no constituye readiness. Un fallo anterior no emitira la linea de readiness.

Si otro proceso posee el lock, el servidor esperara hasta cinco segundos. El servidor fallara con un diagnostico reintentable si no adquiere el lock. El servidor no creara otro lock. El servidor no ejecutara sync en paralelo. La cancelacion del contexto interrumpira la espera.

Despues de readiness, cada request consultara solo el snapshot privado. Ninguna request abrira la DB canonica. Ninguna request observara escrituras externas ni commits futuros. Todas las requests del proceso observaran el mismo estado de datos.

El proceso permanecera en foreground. El proceso no hara fork. El proceso no creara un PID file. El proceso no se instalara como daemon. Un supervisor externo conservara el proceso.

Todos los logs del proceso iran a stderr. stdout permanecera vacio. La linea de readiness incluira la URL, la revision MCP, la version del binario y `database_mode=read_only`. La linea no incluira el token ni el path de la base.

## Shutdown

El servidor tratara `SIGINT`, `SIGTERM` o la cancelacion equivalente como inicio de shutdown.

El owner ejecutara este orden:

1. Adquirira el mutex del gate.
2. Fijara `accepting=false`.
3. Cancelara el contexto raiz bajo el mismo mutex.
4. Liberara el mutex.
5. Llamara `http.Server.Shutdown` con un timeout de cinco segundos.
6. Llamara `http.Server.Close` para cerrar conexiones restantes.
7. Esperara otros cinco segundos a que `active` llegue a cero.
8. Solo si `active` es cero, cerrara el provider exactamente una vez bajo su mutex.
9. Solo despues cerrara la DB snapshot.
10. Eliminara el snapshot, su lease y sidecars privados, y hara fsync del directorio cuando aplique.

La espera de `active` usara el mismo mutex y `sync.Cond`. `leave` ejecutara `cond.Broadcast`. Un timer despertara la condicion al vencer el segundo plazo. `enter` no podra incrementar `active` despues de `accepting=false` porque ambas operaciones usan el mismo mutex. No existira Add-after-Wait.

Si `active` no llega a cero en el segundo plazo, el owner no cerrara el provider ni SQLite. Esos recursos pueden seguir en uso por un handler no cooperativo. El owner devolvera un error fatal y terminara el proceso. El sistema operativo recuperara los recursos del proceso.

Una prueba graceful comprobara el orden completo y `active=0` antes de `Close` del provider. Una prueba de fake no cooperativo se ejecutara en un subprocess. El fake ignorara la cancelacion y superara ambos plazos. El subprocess comprobara el error fatal, la ausencia de `provider.Close` y `Database.Close`, y la conservacion del snapshot mientras el proceso sigue vivo. El proceso de tests no retendra el goroutine bloqueado.

## Upgrade y restart

Un upgrade seguira este orden:

1. El supervisor detendra el proceso anterior.
2. El operador o el mecanismo de release instalara el nuevo binario.
3. El supervisor iniciara el nuevo proceso.
4. El nuevo proceso ejecutara su unico startup sync.
5. El nuevo proceso completara su self-probe.
6. El nuevo proceso publicara readiness.
7. Los clientes volveran a ejecutar `server/discover`.

El servidor no hara hot reload. El servidor no cambiara de binario durante la ejecucion. El servidor no migrara schema durante una solicitud. El servidor no mantendra sesiones que deban recuperarse despues del restart.

El token persistira durante un upgrade. Una rotacion de token sera una operacion separada. Un cambio de schema, revision MCP o tool schema requiere restart. `ttlMs: 0` evita una promesa de cache entre procesos.

El fallo de bind es el guard de instancia. El proceso nuevo no tomara el puerto mientras el proceso anterior lo conserve. No se usara un PID file para decidir ownership.

## Modelo de amenazas

| Amenaza | Control |
| --- | --- |
| Acceso desde otra maquina | Bind exclusivo a loopback. |
| DNS rebinding | Bind loopback, comparacion exacta de `Origin` y proteccion de `Host`. |
| Pagina web maliciosa | Middleware de `Origin` anterior a auth. |
| Proceso local sin autorizacion | Bearer token de 32 bytes; archivo `0600` en Unix y entorno obligatorio en Windows. |
| Robo de token por argv | No existe argumento de token. |
| Robo de token por logs o errores | El servidor nunca registra ni devuelve el token. |
| Robo de token desde el entorno | El archivo es el valor por defecto. El override queda bajo control del supervisor. |
| Body que agota memoria | Limite de 1 MiB durante la lectura. |
| Consulta abandonada | `PropagateRequestCancellation` y contextos SQLite. |
| Escritura por tool | Solo tres services read-only y snapshot abierto con `OpenReadOnly()`. |
| Escritura canonica concurrente | Requests consultan un snapshot privado estable y no abren la DB canonica. |
| Snapshot huerfano | Nombre aleatorio, permisos owner-only, lease y cleanup seguro del mismo owner. |
| Confusion de revision | Guard exacto en header y `_meta`. |
| Confusion entre cabecera y body | Validacion de `Mcp-Method`, `Mcp-Name` y revision. |
| Exfiltracion mediante resources | La allowlist rechaza los metodos de resources. |
| Handler estandar adicional del SDK | La allowlist rechaza todo `Mcp-Method` no aprobado. |
| Estado de otra conversacion | El servidor no conserva estado de protocolo entre requests. |
| Segunda instancia | Reserva atomica del puerto. |
| Cache compartida | `Cache-Control: no-store` y `cacheScope: private`. |

La frontera local no protege contra un proceso que ya ejecuta como el mismo usuario y puede leer la base, el entorno o la memoria. Esa amenaza equivale a comprometer la cuenta local. El contrato no expone el servidor fuera de loopback y no define TLS remoto.

## Pruebas de aceptacion del issue 48

La implementacion debe agregar pruebas hermeticas para estas areas.

### Protocolo y serializacion

- `server/discover` anuncia solo `2026-07-28`.
- `server/discover` publica `_meta.io.modelcontextprotocol/serverInfo.name: "backscroll"`.
- `server/discover` publica `_meta.io.modelcontextprotocol/serverInfo.version` con `buildVersion`.
- Cada resultado moderno publica el mismo `serverInfo`.
- `buildVersion: "dev"` se publica sin rechazo ni sustitucion.
- `server/discover` serializa `capabilities.tools` como `{}`.
- `server/discover` omite `resources`, `prompts` y `logging`.
- `server/discover` omite `listChanged`.
- `tools/list` devuelve exactamente tres tools en orden estable.
- `tools/list` omite `nextCursor`.
- `initialize` no abre una sesion.
- Una revision legacy recibe rechazo.
- Una revision desconocida en un request recibe `-32022` con `requested`, `supported` y el mismo `id` raw.
- Una revision desconocida en una notification valida recibe HTTP 400 sin body.
- Un header y `_meta` distintos en un request reciben `-32020` con el mismo `id` raw.
- Un `Mcp-Method` permitido que no coincide con el body recibe `-32020` del middleware y el mismo `id` raw.
- Un `Mcp-Name` incorrecto en un request recibe `-32020` del middleware y el mismo `id` raw.
- Un error de revision o header en una notification valida recibe HTTP 400 sin body.
- La allowlist acepta `server/discover`, `tools/list` y `tools/call`.
- La allowlist rechaza un request arbitrario con HTTP 404 y `-32601`.
- Ningun metodo bloqueado llega al handler SDK.
- Un request bloqueado devuelve exactamente el mismo `id` string o integer.
- Un header bloqueado con un body permitido recibe `-32020` del middleware y no llega al SDK.
- Un header permitido con un body bloqueado recibe `-32020` del middleware y no llega al SDK.
- Un body JSON con strings Unicode validos supera `utf8.Valid` y conserva su contenido.
- Un body que contiene byte `0xff` falla antes de JSON decode.
- UTF-8 invalido recibe HTTP 400 y `-32700` sin clave `id`, aunque bytes previos parezcan formar un `id`.
- La respuesta a UTF-8 invalido pasa `utf8.Valid` y `json.Valid`.
- Un JSON sintacticamente invalido sin `id` legible recibe HTTP 400 y `-32700` sin clave `id`.
- Un body con un primer objeto valido, `id` valido y trailing JSON recibe `-32700` con el mismo `id` raw.
- Un batch recibe HTTP 400 y `-32600` sin clave `id`.
- Un envelope JSON-RPC invalido con `id` valido recibe `-32600` con el mismo `id` raw.
- `params: null`, array o primitivo con `id` valido recibe `-32600` con el mismo `id` raw.
- Una notification bloqueada recibe HTTP 202.
- Una notification bloqueada no contiene body, bytes JSON ni Response JSON-RPC.
- Notifications con `server/discover`, `tools/list` y `tools/call` reciben HTTP 202 sin body.
- Un spy confirma cero invocaciones de los tres handlers request-only para esas notifications.
- `id: null`, boolean, objeto o array recibe `-32600` sin clave `id`.
- `id: 1.5` recibe `-32600` sin clave `id`.
- `id: 1.0` es valido, usa el surrogate y se restaura como `1.0`.
- `id: 1e0` es valido, usa el surrogate y se restaura como `1e0`.
- `id: 1e1000000000` es valido, no expande memoria y usa el surrogate.
- `id: -1e1000000000` es valido, conserva el signo y usa el surrogate.
- `id: 1e-1` e `id: 1e-1000000000` reciben `-32600` sin clave `id`.
- `id: 10e-1` e `id: 1000e-3` son validos y se restauran sin normalizacion.
- `id: 1000e-4` recibe `-32600` sin clave `id`.
- `id: 0e-1000000000` es el integer valido 0 y conserva su lexeme.
- `id: 1.0000000000000001` recibe `-32600` porque no representa un integer.
- Los tokens `01`, `1.`, `.5` y `1e` son JSON malformed y reciben `-32700`.
- Un test de allocation confirma memoria O(1) adicional para exponentes largos.
- `id: 9007199254740991` e `id: -9007199254740991` son validos y usan el surrogate como todos los IDs numericos.
- `id: 9007199254740992` e `id: -9007199254740992` son validos y usan el surrogate al llegar al SDK.
- Un integer positivo de cientos de digitos conserva exactamente todos sus digitos en la respuesta.
- Un integer negativo de cientos de digitos conserva exactamente el signo y todos sus digitos.
- Un `id` string conserva exactamente su valor en una respuesta bloqueada.
- Un request permitido con integer grande recibe una respuesta con el `id` raw y nunca con `backscroll-surrogate-id`.
- Dos requests concurrentes con IDs grandes distintos restauran sus IDs correctos sin estado compartido.
- Una respuesta SDK de 2 MiB se acepta.
- Una respuesta SDK de 2 MiB mas un byte devuelve HTTP 500 sin bytes parciales.
- El overflow devuelve `application/json`, codigo `-32603`, mensaje `Internal error` y el `id` raw exacto.
- Dieciseis capturas simultaneas no superan 32 MiB de buffers de respuesta.
- Una respuesta SDK con surrogate inesperado devuelve el mismo error JSON-RPC pequeño.
- Un request sin `clientInfo` se acepta.
- Un request con `clientInfo: {"name":"client","version":"1.0"}` se acepta.
- Un request con `clientInfo` null, array o primitivo recibe `-32602` con el mismo `id`.
- Un request con `clientInfo.name` o `clientInfo.version` ausente o no string recibe `-32602` con el mismo `id`.
- Un request con `clientInfo: {"name":"","version":""}` se acepta.
- Una notification con `clientInfo` invalido recibe HTTP 400 sin body.
- Un request sin `protocolVersion` en `_meta` recibe `-32602` con el mismo `id`.
- Un request sin `clientCapabilities` recibe `-32602` con el mismo `id`.
- Una notification sin `_meta`, `protocolVersion` ni `clientCapabilities` supera la fase de metadata.
- Una notification sin metadata y con headers validos recibe HTTP 202 sin body.
- Una notification con metadata opcional malformed o contradictoria recibe HTTP 400 sin body.
- Una notification con cualquier header MCP de transporte invalido recibe el status definido y cero bytes de body.
- Todo resultado de tool incluye `resultType: "complete"` y `content`.
- Un exito puede omitir `isError`.
- Un error de tool incluye `isError: true` y contenido saneado.
- Un error de tool puede omitir `structuredContent` y metadata.
- Cada exito valida contra su `outputSchema`.
- `vcs_revision` se omite cuando build info no lo contiene.
- `indexed_at` se omite cuando `Stats.IndexedAt` es zero.

La prueba parametrizada de la allowlist cubrira todos los metodos adicionales que `serverMethodInfos` registra en SDK v1.8.0.

Los casos de request seran:

| `Mcp-Method` bloqueado | Resultado esperado |
| --- | --- |
| `completion/complete` | HTTP 404, `-32601`, mismo `id` |
| `initialize` | HTTP 404, `-32601`, mismo `id` |
| `ping` | HTTP 404, `-32601`, mismo `id` |
| `prompts/list` | HTTP 404, `-32601`, mismo `id` |
| `prompts/get` | HTTP 404, `-32601`, mismo `id` |
| `resources/list` | HTTP 404, `-32601`, mismo `id` |
| `resources/templates/list` | HTTP 404, `-32601`, mismo `id` |
| `resources/read` | HTTP 404, `-32601`, mismo `id` |
| `logging/setLevel` | HTTP 404, `-32601`, mismo `id` |
| `resources/subscribe` | HTTP 404, `-32601`, mismo `id` |
| `subscriptions/listen` | HTTP 404, `-32601`, mismo `id` |
| `resources/unsubscribe` | HTTP 404, `-32601`, mismo `id` |

Cada request usara el mismo valor en `Mcp-Method` y `method`. Cada request incluira un `id` valido. Los casos `prompts/get` y `resources/read` incluiran un `Mcp-Name` valido y coincidente. La prueba verificara el tipo y valor exactos del `id` en el error.

Los casos de notification seran:

| `Mcp-Method` bloqueado | Resultado esperado |
| --- | --- |
| `notifications/cancelled` | HTTP 202, body ausente |
| `notifications/initialized` | HTTP 202, body ausente |
| `notifications/roots/list_changed` | HTTP 202, body ausente |
| `notifications/progress` | HTTP 202, body ausente |

Cada notification usara el mismo valor en `Mcp-Method` y `method`. Cada notification omitira `id`. La prueba verificara cero bytes de body. La prueba verificara que el middleware no construyo un Response JSON-RPC.

Las operaciones request-only tendran esta matriz de notifications:

| Metodo MCP | Resultado esperado | Invocaciones del handler |
| --- | --- | --- |
| `server/discover` | HTTP 202, body ausente | 0 |
| `tools/list` | HTTP 202, body ausente | 0 |
| `tools/call` con `Mcp-Name` valido | HTTP 202, body ausente | 0 |

La matriz de `Mcp-Name` incluira estos casos:

| Metodo MCP | Caso | Resultado esperado |
| --- | --- | --- |
| `tools/call` | Header ausente, malformed o distinto de `params.name` | HTTP 400, `-32020`, mismo `id` |
| `tools/call` | Header valido y coincidente | Reescritura canonica y delegacion al SDK |
| `tools/call` | `=?base64?YmFja3Njcm9sbF9zZWFyY2g=?=` con `params.name: "backscroll_search"` | Header reescrito a `backscroll_search`; delegacion al SDK |
| `tools/call` | Nombre ASCII o Unicode fuera de las tres tools | HTTP 400, `-32602`, mismo `id` |
| `prompts/get` | Header ausente, malformed o distinto de `params.name` | HTTP 400, `-32020`, mismo `id` |
| `prompts/get` | Header valido y coincidente | HTTP 404, `-32601`, mismo `id` |
| `prompts/get` | `=?base64?5aSp5rCU?=` coincide con `params.name: "天气"` | HTTP 404, `-32601`, mismo `id`; cero llamadas al SDK |
| `resources/read` | Header ausente, malformed o distinto de `params.uri` | HTTP 400, `-32020`, mismo `id` |
| `resources/read` | Header valido y coincidente | HTTP 404, `-32601`, mismo `id` |
| `prompts/get` notification | Header o revision invalida | HTTP 400, body ausente |
| `prompts/get` notification | Header y revision validas | HTTP 202, body ausente |
| `resources/read` notification | Header o revision invalida | HTTP 400, body ausente |
| `resources/read` notification | Header y revision validas | HTTP 202, body ausente |

La matriz de precedencia de revision usara requests con `id: "revision-1"`:

| Metadata del body | Header | Resultado esperado |
| --- | --- | --- |
| Ausente | Ausente | HTTP 400, `-32602`, mismo `id` |
| Valida | Ausente o malformed | HTTP 400, `-32020`, mismo `id` |
| `2026-07-28` | `1900-01-01` | HTTP 400, `-32020`, mismo `id` |
| `1900-01-01` | `2026-07-28` | HTTP 400, `-32020`, mismo `id` |
| `1900-01-01` | `1900-01-01` | HTTP 400, `-32022`, mismo `id`, `requested` y `supported` |
| `2026-07-28` | `2026-07-28` | Continua a `Mcp-Method` y `Mcp-Name` |

Las variantes notification de header ausente, mismatch y revision no soportada exigiran HTTP 400 y cero bytes de body.

La matriz de cardinalidad sera:

| Header | Caso | Resultado esperado |
| --- | --- | --- |
| `Origin` | Dos lineas, iguales o distintas | HTTP 403 |
| `Authorization` | Dos lineas, iguales o distintas | HTTP 401 |
| `Content-Type` | Dos lineas, iguales o distintas | HTTP 400 y body fijo |
| `MCP-Protocol-Version` | Dos lineas en request | HTTP 400, `-32020`, mismo `id` |
| `MCP-Protocol-Version` | Dos lineas en notification | HTTP 400, body ausente |
| `Mcp-Method` | Dos lineas en request | HTTP 400, `-32020`, mismo `id` |
| `Mcp-Method` | Dos lineas en notification | HTTP 400, body ausente |
| `Mcp-Name` | Dos lineas en request | HTTP 400, `-32020`, mismo `id` |
| `Mcp-Name` | Dos lineas en notification | HTTP 400, body ausente |
| `Accept` | Dos lineas que combinadas contienen ambos media types | Supera el control de `Accept` |

Una prueba de inventario fallara si una futura version del SDK registra otro metodo y la matriz no lo clasifica.

### Cache

- Toda respuesta HTTP contiene `Cache-Control: no-store`.
- `server/discover` devuelve `ttlMs: 0` y `cacheScope: "private"`.
- `tools/list` devuelve `ttlMs: 0` y `cacheScope: "private"`.
- `tools/call` no recibe hints MCP de cache.

### Seguridad y prioridad HTTP

- Un request sin token recibe `401` cuando `Origin` es valido o ausente.
- Un token incorrecto recibe `401`.
- El token correcto concede solo `backscroll:read`.
- El archivo Unix nuevo contiene 43 caracteres, un LF final, 44 bytes totales y modo `0600`.
- El token decodifica a 32 bytes.
- El bearer excluye el LF.
- Un archivo con permisos inseguros bloquea el startup.
- Un archivo con forma invalida bloquea el startup.
- `BACKSCROLL_MCP_TOKEN` exige 43 caracteres Base64 URL sin padding.
- `BACKSCROLL_MCP_TOKEN` reemplaza el archivo Unix sin modificarlo.
- Unix crea o lee el archivo con modo `0600` cuando no existe override.
- `GOOS=windows GOARCH=amd64 go build ./cmd/backscroll` compila el comando MCP.
- Windows sin `BACKSCROLL_MCP_TOKEN` falla antes de invocar `net.Listen`.
- Windows con token invalido falla antes de invocar `net.Listen`.
- Windows con token valido no ejecuta operaciones sobre el token file.
- Windows ignora un token file existente.
- El token no aparece en argv, stdout, logs ni errores.
- Un request sin `Origin` alcanza auth.
- El origin exacto alcanza auth.
- Un `Origin` malformed recibe `403`.
- `Origin: null` recibe `403`.
- Un origin distinto recibe `403`.
- Un origin invalido con token invalido y path desconocido recibe `403`.
- Un origin valido con token invalido y path desconocido recibe `401`.
- GET y POST con credenciales validas sobre un path desconocido reciben `404`.
- GET, HEAD, OPTIONS, PUT, PATCH y DELETE sobre `/mcp` reciben `405` con credenciales validas.
- El control de path ocurre antes del control de metodo.
- Un request con token valido, revision invalida e `id` valido recibe `400` y conserva el `id`.
- Un POST `/mcp` sin `Accept` recibe HTTP 400 y el body fijo documentado.
- Un POST `/mcp` con solo `application/json` recibe HTTP 400.
- Un POST `/mcp` con solo `text/event-stream` recibe HTTP 400.
- Un POST `/mcp` con uno de los media types en `q=0` recibe HTTP 400.
- `Accept: */*` recibe HTTP 400.
- Un `Accept` con ambos media types, cualquier orden y parametros validos supera el control.
- El error de `Accept` usa `text/plain; charset=utf-8` y no contiene JSON-RPC.
- Dos valores de `Content-Type` reciben HTTP 400 y el body fijo documentado.
- Dos valores de `Origin` reciben HTTP 403 antes de auth.
- Dos valores de `Authorization` reciben HTTP 401.
- Dos valores de cada header MCP singleton producen el resultado correlacionado de la matriz.
- Dos lineas de `Accept` se combinan como una lista HTTP.
- `Authorization: bEaReR <token>` acepta el scheme y conserva el token case-sensitive.
- Un cambio de casing en el token recibe HTTP 401.
- `Content-Type: Application/JSON` supera el control.
- `Content-Type` malformed recibe HTTP 415.
- `Content-Type: application/json; charset=utf-8` supera el control.
- `Accept: Application/JSON, Text/Event-Stream` supera el control.
- Un media range malformed en `Accept` recibe HTTP 400 y el body fijo.
- `Mcp-Method: Tools/List` con body `method: "tools/list"` recibe `-32020`.
- Header y body `method: "Tools/List"` coincidentes reciben `-32601` porque la allowlist es case-sensitive.
- `params.name: "Backscroll_Search"` recibe `-32602` aunque el header coincida.
- Una key metadata `ProtocolVersion`, `clientcapabilities` o `clientinfo` no satisface la key MCP.
- Los nombres de headers con casing distinto conservan la semantica HTTP.
- `HTTP://127.0.0.1:8377` normaliza al origin por defecto.
- Dos formas canonicas de la misma IPv6 y el mismo puerto coinciden.
- Una IP o puerto diferente recibe HTTP 403.
- `http://localhost:8377` no sustituye `http://127.0.0.1:8377` y recibe HTTP 403.
- Un origin sin el puerto explicito recibe HTTP 403.
- Un Host no local recibe rechazo.
- Un body de 1 MiB se acepta cuando es valido.
- Un body de 1 MiB mas un byte recibe `413`.

### Concurrencia y limites HTTP

- El gate admite 16 POST autenticados despues de path, metodo y media headers.
- Diecisiete POST con bodies bloqueantes se inician de forma simultanea.
- Solo 16 handlers leen sus bodies.
- El POST diecisiete recibe HTTP 429 y `Too Many Requests` sin espera.
- Un reader spy confirma que el POST saturado no lee un byte de body.
- El POST saturado no invoca parser, allowlist, SDK ni `QueryService`.
- `leave` reduce `active`, ejecuta `cond.Broadcast` y permite que el siguiente POST entre.
- Una cancelacion ejecuta `leave`.
- Una notification incrementa y decrementa `active`.
- Dieciseis notifications bloqueantes fijan `active=16` y la diecisiete recibe HTTP 429.
- Una carrera entre `enter` y shutdown nunca incrementa `active` despues de `accepting=false`.
- Un POST que observa `accepting=false` recibe HTTP 503 sin leer body.
- Dieciseis respuestas de 2 MiB no superan 32 MiB de buffers de respuesta.
- La captura conserva `Content-Type`, `Cache-Control` y `MCP-Protocol-Version`.
- La captura recalcula `Content-Length` despues de restaurar el `id`.
- La captura descarta hop-by-hop headers, `Set-Cookie`, `Server` y un header arbitrario.
- La prueba slowloris cierra headers incompletos despues de `ReadHeaderTimeout`.
- Todo request que alcanza el gate tiene como maximo `60 << 10` bytes de headers segun la formula.
- Un header mayor de 64 KiB se rechaza antes del handler.
- La prueba de borde trata 64 KiB como upper bound y no exige aceptacion exacta.
- `ReadTimeout`, `WriteTimeout` e `IdleTimeout` se prueban con los valores documentados.
- Cada handler recibe un deadline de 30 segundos en SDK, `QueryService`, SQLite y provider.
- Una query SQLite bloqueada termina por el contexto del handler.
- Un provider fake bloqueado termina por el contexto del handler.
- El provider real disponible y todos los fakes pasan la prueba contractual de cancelacion.

### Datos y concurrencia

- Una fixture con texto mayor de 16384 runes demuestra que storage devuelve como maximo 16384 runes antes del adapter MCP.
- Una fixture con runes de cuatro bytes demuestra el limite de 65536 bytes por texto.
- Context devuelve como maximo 101 records y marca `text_truncated` cuando corresponde.
- Search conserva ranking y filtros con `MaxTextRunes=16384`.
- Una fixture con un token enorme produce un snippet de como maximo 1024 runes antes de `Scan`.
- Un snippet de runes de cuatro bytes no supera 4096 bytes.
- La truncacion del snippet no cambia score ni orden.
- La CLI con `MaxTextRunes=0` conserva el texto completo.
- Dieciseis handlers con el peor caso documentado permanecen dentro del calculo conservador de 320 MiB.
- Las tools consultan solo SQLite.
- El servidor ejecuta un solo sync antes de readiness.
- Ninguna tool ejecuta sync.
- Dos clientes concurrentes reciben respuestas independientes.
- Dos conversaciones no comparten parametros ni estado.
- Una escritura canonica confirmada despues de readiness no aparece en ninguna consulta del proceso.
- Un restart crea un snapshot nuevo y puede incorporar esa escritura despues del sync inicial.
- Un segundo servidor falla por bind y no publica readiness.
- Un restart conserva el token y vuelve a ejecutar un solo startup sync.
- La backup API crea una snapshot consistente mientras el lock canonico sigue retenido.
- Un commit presente en WAL antes de backup aparece en el snapshot.
- La DB canonica se cierra antes de liberar el lock.
- El snapshot Unix y su lease usan modo `0600`.
- El nombre del snapshot usa entropia criptografica.
- Un fallo de backup elimina el partial y no publica readiness.
- Shutdown normal elimina snapshot, lease y sidecars.
- Startup elimina solo snapshots huerfanos del mismo owner cuyo lease puede adquirir.
- Un symlink o archivo de otro owner nunca se elimina.
- La construccion de visibilidad recorre exhaustivamente todos los rowids.
- Filas tool con `search_echo!=0` quedan fuera del scope unfiltered.
- Filas text, code y reasoning con `search_echo=true` permanecen visibles.
- Cada broad candidate se evalua con texto completo y `directsearch.IsSerializedDirectSearchCall`.
- Una llamada directa cuyo marcador decisivo aparece despues de 16384 runes conserva la misma clasificacion.
- Un call y output con pairing probado por `search_echo` quedan fuera juntos del scope unfiltered.
- Un output sin pairing probado no se excluye por heuristica.
- La construccion procesa una sola fila broad-candidate a la vez.
- Una prueba con un broad candidate grande confirma memoria O(tamaño de esa fila) y ausencia de acumulacion entre filas.
- La page de Search unfiltered e IDF relax usan exactamente los mismos rowids visibles.
- Search explicito `content_type: "tool"` conserva query echoes.
- `MaxTextRunes` no cambia seleccion, pairing ni IDF.

### Tools

- Un contract test confirma que las tres tools usan `Server.AddTool` y no `mcp.AddTool`.
- Cada exito contiene exactamente un `TextContent` con JSON canonico igual al resultado publico.
- `Server.AddTool` no crea ni modifica `structuredContent` o `Content`.
- `arguments` ausente se normaliza a `{}` antes del schema.
- Un objeto `arguments` se valida contra el input schema.
- `arguments: null`, array, string, boolean y number devuelve `invalid_arguments`.
- `backscroll_status` acepta `arguments` ausente y `{}`.
- `backscroll_status` rechaza cualquier propiedad.
- Un spy confirma cero invocaciones de `QueryService` cuando falla un `inputSchema`.
- Un fallo de schema devuelve solo `invalid_arguments` y `invalid arguments`.
- Ningun error dirigido al cliente contiene JSON pointers, tipos Go, SQL, valores recibidos ni texto del validador.
- Un fallo de `outputSchema` devuelve solo `query_failed` y `query failed`.
- Cada sentinel de la taxonomia produce el `code` y `message` exactos.
- Un error no tipado produce solo `query_failed` y `query failed`.
- `backscroll_search` conserva ranking, filtros, echo exclusion y relajacion de la CLI.
- Un E2E ejecuta CLI y MCP con `lexical_only: false` y el mismo `embedding.MockEmbeddingProvider` configurado.
- El E2E confirma que la factory recibe la misma `config.Embedding` en ambos caminos.
- Un test con embeddings deshabilitados confirma provider nil y cero llamadas a la factory.
- Un fallo de construccion del provider confirma cero llamadas a `OpenReadOnly`.
- Un fallo de `OpenReadOnly` confirma un solo `provider.Close`.
- Un fallo posterior confirma cierre de provider y despues DB.
- Un test confirma que `Database.SetEmbeddingProvider` recibe el wrapper mutex y no el provider raw.
- El E2E confirma la misma invocacion del provider y el mismo orden de resultados.
- Un E2E varia `similarity_threshold` sobre vectores conocidos y confirma la misma seleccion en CLI y MCP.
- Un provider mock con error ordinario confirma fallback lexico en ambos caminos.
- Un provider mock cancelado confirma que ninguno de los caminos usa fallback.
- Dos busquedas vectoriales concurrentes producen una concurrencia maxima de `Embed` igual a 1.
- Dos busquedas vectoriales producen un solo scan vectorial activo por el gate de capacidad 1.
- La segunda busqueda vectorial espera dentro de su deadline y cancela al vencer.
- Dos busquedas `lexical_only: true` avanzan de forma concurrente.
- Un corpus vectorial mayor que K produce el mismo top-K que la referencia load-all.
- Chunks duplicados del mismo ItemID producen una sola entrada con score maximo.
- Un maximo igual a cero o negativo se descarta.
- Un maximo menor que `similarity_threshold` se descarta antes del heap.
- Scores iguales se ordenan por `ItemID ASC`.
- Empates RRF se ordenan por string de ID ASC.
- La ruta MCP no llama `LoadChunkEmbeddings`.
- El heap nunca supera 100 entries y el contrato inicial usa como maximo 40.
- Un test confirma `unsafe.Sizeof(vectorCandidate{})==16` en targets soportados y backing entries <=1600 bytes.
- Una cancelacion en cada fila cierra rows y libera el gate vectorial.
- Un test de lifecycle confirma un solo `Close` del provider despues de drenar requests y el cierre posterior de `Database`.
- Un detector de carrera confirma que `Embed` y `Close` nunca se solapan.
- El adapter llama `services.ValidateExplicitSearchScope`, incluida `services.ValidateSearchRequest`, antes de `QueryService.Search`.
- El input exacto `{"query":"\"unterminated","all_projects":true,"relax":true}` devuelve `invalid_arguments` y `invalid arguments`.
- Un spy confirma cero invocaciones de `QueryService.Search` para ese input relax invalido.
- El contenido no incluye el error interno de `storage.ValidateRelaxationQuery`.
- `backscroll_search` exige un scope de proyecto explicito.
- El adapter llama `services.ValidateContextRequest` antes de `QueryService.Context` y suministra `services.DefaultContextMaxTokens` al request tipado.
- El presupuesto MCP permanece en el adapter de presentacion; un spy confirma que no se pasa `MaxTokens: 0` ni se sustituye el rango CLI por el rango MCP.
- `backscroll_search` omite campos internos de `storage.SearchResult`.
- `backscroll_search` acepta `limit: 20` y rechaza `limit: 21`.
- `backscroll_search` acepta `offset: 2147483647`.
- `backscroll_search` rechaza `offset: 2147483648` y offsets negativos.
- Search y context aplican `max_tokens: 2000` cuando se omite.
- Los input schemas aceptan `max_tokens: 1` y `max_tokens: 16384`.
- El adapter devuelve `invalid_arguments` cuando el objeto minimo no cabe en `max_tokens: 1`.
- Search y context rechazan `max_tokens: 0`, valores negativos y `16385`.
- El resultado serializado nunca supera 2 MiB.
- `backscroll_context` exige un selector exacto.
- `backscroll_context` rechaza un `ordinal` JSON number.
- En un build de 64 bits, `backscroll_context` acepta `ordinal: "9007199254740992"` y conserva el round-trip exacto.
- En un build de 64 bits, acepta `ordinal: "9223372036854775807"`.
- Rechaza `ordinal: "9223372036854775808"` por overflow de `MaxInt64`.
- Rechaza strings ordinal negativos, con signo positivo, con ceros iniciales o con caracteres no decimales.
- `GOARCH=386` y `GOARCH=amd64` aceptan `ordinal: "9223372036854775807"` en la frontera MCP.
- Tests de tipo confirman `int64` para `storage.SearchResult.Ordinal`, `models.SearchResult.Ordinal`, `storage.ContextRecord.Ordinal` y contadores de `storage.Stats`.
- Un consumidor CLI que convierta ordinal a `int` detecta overflow en `GOARCH=386` sin truncar.
- Cross-builds `GOARCH=386` y `GOARCH=amd64` compilan las rutas CLI y MCP sin conversion implicita.
- `backscroll_context` acepta y devuelve `content_type: "text/markdown"` sin modificarlo.
- El outputSchema de `backscroll_context` acepta un array no vacio con exactamente un record anchor.
- El outputSchema de `backscroll_context` rechaza un array sin anchor.
- El outputSchema de `backscroll_context` rechaza un array con dos anchors.
- Search y context serializan `ordinal: "9223372036854775807"` sin perdida.
- El round-trip de cada ordinal de output conserva todos los digitos decimales.
- `backscroll_status` devuelve solo campos de `storage.Stats` y metadata del servidor.
- `backscroll_status` serializa los cinco contadores como strings decimales exactos.
- Los seis schemas se analizan como JSON Schema 2020-12.
- Cada schema rechaza propiedades desconocidas mediante un error de tool completo y saneado.
- Cada campo nullable y opcional sigue el schema documentado.

### Readiness y shutdown

- El fallo de `net.Listen` impide iniciar una segunda instancia.
- El inicio de `Serve` no publica readiness por si solo.
- Un self-probe autenticado exitoso publica readiness.
- Un self-probe fallido impide readiness y termina el proceso.
- El timeout de cinco segundos del self-probe termina el startup.
- Una desconexion cancela una consulta SQLite activa.
- La cancelacion del contexto raiz detiene requests nuevos.
- Cada `enter` aceptado incrementa `active` bajo el mutex antes de leer el body.
- Shutdown fija `accepting=false` y cancela el contexto raiz bajo el mismo mutex.
- `Shutdown` permite hasta cinco segundos a los handlers activos.
- El owner llama `Close` despues de `Shutdown`.
- El owner espera `active=0` con `sync.Cond` durante otros cinco segundos.
- El camino graceful observa `active=0` antes de cerrar provider y DB.
- Un fake no cooperativo se ejecuta en subprocess y supera ambos plazos.
- El camino no cooperativo no llama `provider.Close` ni `Database.Close` y termina con error fatal.
- stdout permanece vacio.
- stderr contiene readiness y errores operativos sin secretos.

Las pruebas no usaran la base, el HOME ni el token reales del operador. Las pruebas usaran directorios temporales y puertos asignados por el sistema, excepto la prueba especifica del valor por defecto.

## Operacion del issue 48

La implementacion del issue 48 ofrecera un subcomando foreground. Este contrato reserva el nombre `backscroll mcp`. No define flags adicionales.

Inicio manual:

```bash
backscroll mcp
```

El proceso imprimira readiness en stderr y seguira activo. El operador usara `Ctrl-C` para un cierre interactivo. Un supervisor enviara `SIGTERM` para un cierre administrado.

Un cliente configurara esta URL:

```text
http://127.0.0.1:8377/mcp
```

Un probe operativo enviara `server/discover` mediante POST. El probe debe incluir el bearer token, las cabeceras MCP y la metadata `_meta`. GET no es un probe de readiness.

El operador consultara `backscroll_status` para revisar el indice despues de readiness. El operador reiniciara el proceso para aplicar un binario nuevo o un token nuevo. El operador no borrara un PID file porque Backscroll no creara uno.

## Referencias oficiales

- [MCP 2026-07-28: protocolo base y statelessness](https://modelcontextprotocol.io/specification/2026-07-28/basic)
- [MCP 2026-07-28: versioning](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning)
- [MCP 2026-07-28: Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)
- [MCP 2026-07-28: server discovery](https://modelcontextprotocol.io/specification/2026-07-28/server/discover)
- [MCP 2026-07-28: tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)
- [MCP 2026-07-28: caching](https://modelcontextprotocol.io/specification/2026-07-28/server/utilities/caching)
- [SDK oficial Go v1.8.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)
- [`StreamableHTTPOptions` v1.8.0](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp@v1.8.0#StreamableHTTPOptions)
- [`CallToolResult` v1.8.0](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp@v1.8.0#CallToolResult)
- [`ServerOptions` v1.8.0](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp@v1.8.0#ServerOptions)
- [`Server.AddTool` v1.8.0](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp@v1.8.0#Server.AddTool)
- [`http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection)
- [`crypto/rand`](https://pkg.go.dev/crypto/rand)
- [`base64.RawURLEncoding`](https://pkg.go.dev/encoding/base64#RawURLEncoding)
- [SQLite Online Backup API](https://www.sqlite.org/backup.html)
- [RFC 6750: Bearer Token Usage](https://www.rfc-editor.org/rfc/rfc6750)
