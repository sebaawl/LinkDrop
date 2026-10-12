# LinkDrop 1.1 — descarga rápida de componentes

**Windows 10/11, 64 bits.** App local que abre su interfaz en el navegador.

Esta versión **no lleva FFmpeg ni yt-dlp incorporados dentro del EXE**. Se mejoró para evitar volver a descargar los componentes existentes:

- Al iniciar, busca `bin/yt-dlp.exe`, `bin/ffmpeg.exe` y `bin/ffprobe.exe` junto a LinkDrop.exe (versión portátil).
- Si no están, reutiliza los componentes de instalaciones anteriores bajo `%APPDATA%/LinkDrop/bin` (o la carpeta de configuración de Windows) y las herramientas disponibles en PATH.
- Si hace falta FFmpeg, primero prueba el paquete oficial compacto de Gyan (~34 MB), extrayéndolo con `tar.exe` de Windows. Si la extracción de 7z no es compatible con tu versión de Windows, utiliza el ZIP como alternativa.
- Si yt-dlp está instalado, no lo vuelve a descargar.
- La primera preparación necesita conexión solo si faltan componentes. Las siguientes ejecuciones los reutilizan.

## Inicio

1. Ejecutar `LinkDrop_1.1_Rapido.exe`.
2. Si dice LISTO, no hay que instalar nada.
3. Si dice NO LISTO, pulsar **Preparar componentes**.
4. Pegar un enlace de contenido disponible y autorizado, elegir audio/video, calidad, y descargar.

## Crear paquete portátil completo manualmente

Cuando tengas `yt-dlp.exe`, `ffmpeg.exe` y `ffprobe.exe` de sus distribuidores originales, colócalos en una carpeta `bin` junto al .exe:

```
LinkDrop/
    LinkDrop_1.1_Rapido.exe
    bin/
        yt-dlp.exe
        ffmpeg.exe
        ffprobe.exe
```

En ese caso LinkDrop detecta las tres herramientas al iniciar **sin descargarlas ni instalarlas**. Es posible mover toda la carpeta a otro PC Windows x64.

### Origen de motores externos

- yt-dlp: https://github.com/yt-dlp/yt-dlp/releases/latest
- FFmpeg Windows: https://www.gyan.dev/ffmpeg/builds/

## Limitaciones

- Solo para contenido que tienes derecho a descargar. No permite sortear DRM.
- No ha sido probado directamente en un PC Windows ni en sitios reales desde este entorno.
- No se garantiza el funcionamiento en cualquier sitio ni la extracción 7z con versiones antiguas de Windows.
