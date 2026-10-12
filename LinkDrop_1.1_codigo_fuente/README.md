# LinkDrop 2.0 — aplicación de escritorio

**Windows 10/11, 64 bits.** Abre `LinkDrop_Desktop.exe` para usar LinkDrop en su propia ventana. No abre el navegador ni inicia un servidor en `localhost`: la interfaz va incluida en el ejecutable y se comunica con Go mediante un puente interno.

Necesita **Microsoft Edge WebView2 Runtime**, normalmente presente en Windows 10/11 actualizados. Si falta, LinkDrop muestra un mensaje para instalarlo desde [Microsoft](https://developer.microsoft.com/microsoft-edge/webview2/). No necesitas instalar Go para ejecutar la aplicación.

El archivo anterior `LinkDrop_1.1_Rapido.exe` conserva el comportamiento antiguo; usa el nuevo `LinkDrop_Desktop.exe`. Al cerrar la ventana se cancela la descarga activa y termina la aplicación.

Esta versión **no lleva FFmpeg ni yt-dlp incorporados dentro del EXE**. Se mejoró para evitar volver a descargar los componentes existentes:

- Al iniciar, busca `bin/yt-dlp.exe`, `bin/ffmpeg.exe` y `bin/ffprobe.exe` junto a LinkDrop.exe (versión portátil).
- Si no están, reutiliza los componentes de instalaciones anteriores bajo `%APPDATA%/LinkDrop/bin` (o la carpeta de configuración de Windows) y las herramientas disponibles en PATH.
- Si hace falta FFmpeg, primero prueba el paquete oficial compacto de Gyan (~34 MB), extrayéndolo con `tar.exe` de Windows. Si la extracción de 7z no es compatible con tu versión de Windows, utiliza el ZIP como alternativa.
- Si yt-dlp está instalado, no lo vuelve a descargar.
- La primera preparación necesita conexión solo si faltan componentes. Las siguientes ejecuciones los reutilizan.

## Buscar en YouTube y preparar una cola

1. Abre **YouTube** en la barra lateral, escribe una búsqueda y pulsa **Buscar**. La consulta devuelve hasta 20 videos con título, canal, duración y miniatura. No se abre la página de YouTube ni se necesita una clave de API.
2. En cada resultado, abre el menú **⋯** y elige **Añadir a la cola**.
3. Abre **Cola**. También puedes pegar uno o varios enlaces, uno por línea, y pulsar **Añadir a la cola**. **Pegar enlaces** lee el portapapeles de Windows. Se omiten duplicados que ya estén pendientes o descargándose.
4. Abre **Configuración global** para elegir video/audio, resolución, formato, calidad y carpeta de destino. **Guardar ajustes** cambia los videos que heredan la configuración; **Aplicar a todos** elimina las excepciones de los videos sin terminar. La descarga activa y los videos terminados mantienen sus ajustes originales.
5. Para personalizar un video, abre su menú **⋯ → Opciones de descarga**, desmarca **Usar la configuración global** y guarda tus opciones. Puedes elegir otra carpeta con **Examinar…**.
6. Pulsa **Iniciar cola**. Los videos se descargan uno a uno, en el orden de la lista. Puedes subir, bajar o quitar pendientes desde sus menús. Si un video falla, queda marcado con el error y se continúa con el siguiente.
7. **Detener** cancela el video actual y conserva los pendientes. Para repetir uno detenido, fallido o terminado, usa **Volver a poner pendiente**. **Quitar terminados** limpia la lista y conserva los archivos descargados.

La cola y sus ajustes se guardan en `%APPDATA%/LinkDrop/queue.json` (o la carpeta de configuración del sistema). Al volver a abrir la aplicación, una descarga interrumpida vuelve a pendiente y no empieza automáticamente: pulsa Iniciar cola cuando estés listo. Admite hasta 500 elementos.

Los enlaces pegados consultan el título y la miniatura pública de YouTube en segundo plano, con un máximo de dos consultas simultáneas. Si YouTube no entrega esos datos, el enlace se conserva y se identifica al descargar; puedes usar **Actualizar título y datos** desde su menú. No se consultan cookies ni cuentas personales.

La búsqueda usa `ytsearch20:` de yt-dlp; los títulos de los enlaces pegados usan el endpoint público oEmbed de YouTube. Necesitas Internet y los componentes preparados. Las fuentes privadas, restringidas o no disponibles pueden fallar.

## Controles de escritorio

- **Modo oscuro**: interruptor en la esquina superior derecha. Cambia el tema de la interfaz y la barra de título; recuerda la elección para la próxima ejecución. Antes de elegir un tema sigue la configuración de Windows.
- **Pegar**: lee el texto del portapapeles de Windows al pulsar el botón y lo coloca en el campo del enlace. Usa la API nativa, incluso si la interfaz integrada no permite leer el portapapeles con JavaScript. También puedes usar Ctrl+V.
- **Examinar…**: abre el selector de carpetas de Windows, con la ventana de LinkDrop como propietaria. Elige el destino sin copiar la ruta. Cancelar conserva la carpeta anterior. El icono de carpeta junto a Examinar abre el destino en el Explorador.
- **Icono de la aplicación**: la referencia al grupo de iconos incorporado se corrigió y se asignan iconos grandes y pequeños a la ventana; se actualizan con el DPI. El ejecutable mantiene su icono multirresolución.

El tema se guarda en `%APPDATA%/LinkDrop/settings.json` (o la carpeta de configuración que proporcione Windows). El portapapeles se consulta solo al pulsar Pegar. Estas funciones usan las APIs de Windows y no requieren PowerShell ni herramientas de terceros.

## Interfaz y pantallas de alta resolución

La interfaz usa una interfaz compacta inspirada en los controles de Windows: tipografía Segoe UI, iconos vectoriales, panel de actividad, controles de formato y barra de estado. El tema claro/oscuro sigue inicialmente la configuración del sistema, con un interruptor para cambiarlo manualmente. La barra de título nativa sigue el tema elegido.

El ejecutable declara **PerMonitorV2** antes de crear la ventana, ajusta su tamaño al área de trabajo y procesa cambios de DPI al moverlo entre monitores. Evita depender del escalado de un mapa de bits de toda la ventana. El icono incluye tamaños desde 16 hasta 256 píxeles. No actives manualmente una anulación de escalado DPI para este ejecutable en las propiedades de compatibilidad de Windows.

La interfaz se revisó en Chromium con escalas de rasterización de 100 %, 150 % y 200 %, en ambos temas y en ventanas pequeñas. Estos controles no sustituyen la prueba del escalado nativo en Windows: el comportamiento real de WebView2, ClearType y el cambio entre monitores queda pendiente de validar en un equipo Windows.

## Inicio

1. Ejecutar `LinkDrop_Desktop.exe`.
2. Si los componentes aparecen listos, no hay que instalar nada.
3. Si los componentes aparecen pendientes, pulsar **Preparar componentes**.
4. Pegar un enlace de contenido disponible y autorizado, elegir audio/video, calidad, y descargar.

## Crear paquete portátil completo manualmente

Cuando tengas `yt-dlp.exe`, `ffmpeg.exe` y `ffprobe.exe` de sus distribuidores originales, colócalos en una carpeta `bin` junto al .exe:

```
LinkDrop/
    LinkDrop_Desktop.exe
    bin/
        yt-dlp.exe
        ffmpeg.exe
        ffprobe.exe
```

En ese caso LinkDrop detecta las tres herramientas al iniciar **sin descargarlas ni instalarlas**. Es posible mover toda la carpeta a otro PC Windows x64.

### Origen de motores externos

- yt-dlp: https://github.com/yt-dlp/yt-dlp/releases/latest
- FFmpeg Windows: https://www.gyan.dev/ffmpeg/builds/

## Compilar desde el código fuente

Requiere Go 1.23 o posterior. El archivo de recursos Windows incluye el manifiesto DPI y el icono multirresolución; regénéralo con el comando indicado antes de compilar. Ejecutar desde `LinkDrop_1.1_codigo_fuente`:

```powershell
go mod download
go test ./...
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest windows.manifest -ico windows.ico -o resource_windows_amd64.syso
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-H windowsgui -s -w" -o ../LinkDrop_Desktop.exe .
```

Desde Linux se puede generar el mismo ejecutable:

```sh
go mod download
go test -race ./...
node desktop_bridge_test.js # opcional: prueba JavaScript, requiere Node.js
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest windows.manifest -ico windows.ico -o resource_windows_amd64.syso
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-H windowsgui -s -w' -o ../LinkDrop_Desktop.exe .
```

En Windows la interfaz se carga directamente en una ventana WebView2, sin un puerto HTTP. Las consultas lentas se ejecutan fuera del hilo de la ventana para mantenerla operativa. La compilación usa el subsistema gráfico de Windows y no muestra una consola. El cargador de WebView2 va incluido en el ejecutable; el Runtime es un requisito del sistema.

En Linux/macOS se conserva el servidor HTTP únicamente para desarrollo y pruebas del motor; no es el comportamiento de la distribución Windows. En este entorno Linux se verificaron las pruebas Go con detección de carreras, el puente JavaScript, la configuración global/individual, la persistencia y las descargas secuenciales de video MP4 y audio FLAC con videos locales generados con FFmpeg. La búsqueda y sus menús se probaron con resultados de prueba. La consulta real a YouTube fue bloqueada por el proxy del entorno (HTTP 403) y no se pudo verificar aquí. Se probaron también la persistencia del tema y los estados de selección, cancelación y error del selector en la interfaz, simulando el límite de las APIs nativas. Para validar las APIs del portapapeles, el diálogo de carpetas y el icono de la ventana se necesita un equipo Windows.

Distribuye también `LinkDrop_Desktop_LICENCIAS.txt`, incluido en el paquete ZIP, con el ejecutable.

La carpeta de código conserva el nombre `LinkDrop_1.1_codigo_fuente`; su contenido corresponde ahora a la versión 2.0.

## Limitaciones

- Solo para contenido que tienes derecho a descargar. No permite sortear DRM.
- No ha sido probado directamente en un PC Windows ni en sitios reales desde este entorno.
- No se garantiza el funcionamiento en cualquier sitio ni la extracción 7z con versiones antiguas de Windows.
