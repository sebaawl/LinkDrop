(() => {
  "use strict";
  const { API, getStatus, readClipboard } = window.LinkDrop;
  const $ = (id) => document.getElementById(id);
  const node = (tag, className, text) => {
    const n = document.createElement(tag);
    if (className) n.className = className;
    if (text !== undefined) n.textContent = text;
    return n;
  };
  const icon = (name) => {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS(svg.namespaceURI, "use");
    use.setAttribute("href", "#i-" + name);
    svg.append(use);
    return svg;
  };
  const notice = (id, text, error = false) => {
    $(id).textContent = text;
    $(id).className = "section-notice" + (error ? " error" : "");
  };
  let queue = { items: [], global: null, running: false },
    searchResults = [],
    searching = false,
    queueRequestSequence = 0,
    queueAppliedSequence = 0,
    globalDirty = false,
    globalKey = "",
    renderKey = "",
    editID = "";
  const viewNames = {
    single: ["Nueva descarga", "Guarda video o audio desde un enlace."],
    youtube: ["YouTube", "Busca videos y añádelos a tu cola."],
    queue: ["Cola de descargas", "Prepara tus videos y descárgalos uno a uno."],
  };
  function showView(name) {
    for (const key of Object.keys(viewNames))
      $(
        key === "single"
          ? "singleView"
          : key === "youtube"
            ? "youtubeView"
            : "queueView",
      ).hidden = key !== name;
    for (const button of document.querySelectorAll("[data-view]")) {
      const selected = button.dataset.view === name;
      button.classList.toggle("active", selected);
      if (selected) button.setAttribute("aria-current", "page");
      else button.removeAttribute("aria-current");
    }
    $("pageTitle").textContent = viewNames[name][0];
    $("pageSubtitle").textContent = viewNames[name][1];
  }
  document
    .querySelectorAll("[data-view]")
    .forEach(
      (button) => (button.onclick = () => showView(button.dataset.view)),
    );
  $("viewComponents").onclick = () => showView("single");
  document.addEventListener("click", (event) => {
    document.querySelectorAll(".result-menu[open]").forEach((menu) => {
      if (!menu.contains(event.target)) menu.open = false;
    });
  });
  function duration(seconds) {
    if (!seconds) return "";
    seconds = Math.floor(seconds);
    const hours = Math.floor(seconds / 3600);
    return (
      (hours ? hours + ":" : "") +
      String(Math.floor(seconds / 60) % 60).padStart(hours ? 2 : 1, "0") +
      ":" +
      String(seconds % 60).padStart(2, "0")
    );
  }
  function image(video) {
    const box = node("div", "video-image");
    box.append(icon("video"));
    try {
      const url = new URL(video.thumbnail);
      if (url.protocol === "https:") {
        const img = node("img");
        img.alt = "";
        img.loading = "lazy";
        img.src = url.href;
        img.onerror = () => {img.remove();box.prepend(icon("video"));};
        box.replaceChildren(img);
      }
    } catch {}
    if (video.duration)
      box.append(node("span", "duration", duration(video.duration)));
    return box;
  }
  function menu(title, actions, id) {
    const details = node("details", "result-menu");
    if (id) details.dataset.id = id;
    const summary = node("summary");
    summary.setAttribute("aria-label", "Opciones de " + title);
    summary.setAttribute("title", "Opciones del video");
    summary.append(icon("more"));
    details.append(summary);
    const popup = node("div", "popup-menu");
    for (const action of actions) {
      const button = node("button", "", action.label);
      button.disabled = Boolean(action.disabled);
      button.onclick = async () => {
        details.open = false;
        await action.run();
      };
      popup.append(button);
    }
    details.append(popup);
    details.addEventListener("toggle", () => {
      if (details.open) {
        details.classList.remove("open-up");
        if (popup.getBoundingClientRect().bottom > innerHeight - 40)
          details.classList.add("open-up");
      }
    });
    return details;
  }
  function settingsSummary(settings) {
    return settings.mode === "audio"
      ? `Audio · ${(settings.audioFormat || "mp3").toUpperCase()} · ${settings.audioQuality === "best" ? "Máxima calidad" : settings.audioQuality?.replace(/K$/, " kbps")}`
      : `Video · ${(settings.videoFormat || "mp4").toUpperCase()} · ${settings.resolution === "best" ? "Máxima resolución" : "Hasta " + settings.resolution + "p"}`;
  }
  function selectField(prefix, key, label, options) {
    const wrapper = node("div");
    const id = prefix + key;
    const lab = node("label", "label", label);
    lab.htmlFor = id;
    const select = node("select", "select");
    select.id = id;
    for (const [value, text] of options) {
      const option = node("option", "", text);
      option.value = value;
      select.append(option);
    }
    wrapper.append(lab, select);
    return wrapper;
  }
  function buildSettings(prefix, container) {
    const grid = node("div", "settings-grid");
    grid.append(
      selectField(prefix, "Mode", "Tipo de descarga", [
        ["video", "Video"],
        ["audio", "Solo audio"],
      ]),
    );
    const spacer = node("div");
    grid.append(spacer);
    const video = node("div", "settings-mode-options");
    video.id = prefix + "VideoFields";
    video.append(
      selectField(prefix, "Resolution", "Resolución", [
        ["best", "Máxima disponible"],
        ["2160", "Hasta 4K · 2160p"],
        ["1440", "Hasta 1440p"],
        ["1080", "Hasta 1080p"],
        ["720", "Hasta 720p"],
        ["480", "Hasta 480p"],
        ["360", "Hasta 360p"],
      ]),
      selectField(prefix, "VideoFormat", "Formato de video", [
        ["mp4", "MP4"],
        ["mkv", "MKV"],
        ["webm", "WebM"],
      ]),
    );
    const audio = node("div", "settings-mode-options");
    audio.id = prefix + "AudioFields";
    audio.append(
      selectField(prefix, "AudioFormat", "Formato de audio", [
        ["mp3", "MP3"],
        ["m4a", "M4A / AAC"],
        ["flac", "FLAC"],
        ["wav", "WAV"],
        ["opus", "Opus"],
        ["aac", "AAC"],
      ]),
      selectField(prefix, "AudioQuality", "Calidad de audio", [
        ["best", "Máxima calidad"],
        ["320K", "320 kbps"],
        ["256K", "256 kbps"],
        ["192K", "192 kbps"],
        ["128K", "128 kbps"],
        ["96K", "96 kbps"],
      ]),
    );
    grid.append(video, audio);
    const folder = node("div", "wide");
    const lab = node("label", "label", "Carpeta de destino");
    lab.htmlFor = prefix + "Folder";
    const row = node("div", "folderrow");
    const input = node("input", "textinput");
    input.id = prefix + "Folder";
    input.spellcheck = false;
    const browse = node("button", "button", "Examinar…");
    browse.type = "button";
    browse.id = prefix + "Browse";
    browse.onclick = async () => {
      browse.disabled = true;
      try {
        const result = await API("browse-folder", { folder: input.value });
        if (!result.canceled && result.folder) {
          input.value = result.folder;
          if (prefix === "global") globalDirty = true;
        }
      } catch (error) {
        notice(
          prefix === "global" ? "queueNotice" : "itemNotice",
          error.message,
          true,
        );
      } finally {
        browse.disabled = false;
      }
    };
    row.append(input, browse);
    folder.append(lab, row);
    grid.append(folder);
    container.append(grid);
    $(prefix + "Mode").onchange = () => updateSettingsMode(prefix);
    if (prefix === "global")
      grid.addEventListener("input", () => (globalDirty = true));
  }
  function updateSettingsMode(prefix) {
    const audio = $(prefix + "Mode").value === "audio";
    $(prefix + "VideoFields").style.display = audio ? "none" : "contents";
    $(prefix + "AudioFields").style.display = audio ? "contents" : "none";
  }
  function fillSettings(prefix, settings) {
    const defaults = {
      mode: "video",
      resolution: "best",
      videoFormat: "mp4",
      audioFormat: "mp3",
      audioQuality: "192K",
      folder: "",
    };
    for (const key of Object.keys(defaults)) {
      const id = prefix + key[0].toUpperCase() + key.slice(1);
      $(id).value = settings[key] || defaults[key];
    }
    $(prefix + "Folder").placeholder =
      getStatus().downloadFolder || "Carpeta predeterminada de descargas";
    updateSettingsMode(prefix);
  }
  function readSettings(prefix) {
    const settings = {};
    for (const key of [
      "mode",
      "resolution",
      "videoFormat",
      "audioFormat",
      "audioQuality",
      "folder",
    ])
      settings[key] = $(prefix + key[0].toUpperCase() + key.slice(1)).value;
    return settings;
  }
  buildSettings("global", $("globalSettingsFields"));
  buildSettings("item", $("itemSettingsFields"));
  function refreshControls() {
    const status = getStatus();
    $("engineNotice").hidden = Boolean(status.ready);
    $("engineNoticeText").textContent = status.installing
      ? "Preparando componentes… Puedes organizar tu cola mientras tanto."
      : "Prepara los componentes para buscar en YouTube y descargar.";
    const pending = queue.items.filter(
      (item) => item.state === "pending",
    ).length;
    $("queueStart").disabled =
      queue.running || status.downloading || !status.ready || !pending;
    $("queueStop").disabled = !queue.running;
    $("queueClearCompleted").disabled = !queue.items.some(
      (item) => item.state === "completed",
    );
    $("youtubeSearchBtn").disabled = searching || !status.ready;
  }
  window.addEventListener("linkdrop-status", refreshControls);
  function renderSearch() {
    const container = $("youtubeResults");
    container.replaceChildren();
    const queued = new Set(
      queue.items
        .filter(
          (item) => item.state === "pending" || item.state === "downloading",
        )
        .map((item) => item.url),
    );
    for (const video of searchResults) {
      const card = node("article", "card result-card");
      card.append(image(video));
      const copy = node("div", "result-copy");
      copy.append(node("h2", "video-title", video.title));
      copy.append(
        node(
          "p",
          "video-meta",
          [video.uploader, duration(video.duration)]
            .filter(Boolean)
            .join(" · "),
        ),
      );
      if (queued.has(video.url))
        copy.append(node("p", "video-queued", "En cola"));
      card.append(
        copy,
        menu(video.title, [
          {
            label: queued.has(video.url)
              ? "Ya está en la cola"
              : "Añadir a la cola",
            disabled: queued.has(video.url),
            run: async () => {
              try {
                const result = await API("queue/add", { items: [video] });
                notice(
                  "youtubeNotice",
                  result.added
                    ? "Video añadido a la cola."
                    : "Este video ya está en la cola.",
                );
                await refreshQueue();
                renderSearch();
              } catch (error) {
                notice("youtubeNotice", error.message, true);
              }
            },
          },
        ]),
      );
      container.append(card);
    }
  }
  $("youtubeSearchForm").onsubmit = async (event) => {
    event.preventDefault();
    if (searching) return;
    const query = $("youtubeQuery").value.trim();
    if (!query)
      return notice("youtubeNotice", "Escribe qué video quieres buscar.", true);
    searching = true;
    refreshControls();
    $("youtubeSearchBtn").textContent = "Buscando…";
    notice("youtubeNotice", "Buscando en YouTube…");
    try {
      const result = await API("youtube/search", { query });
      searchResults = result.results || [];
      $("youtubeEmpty").hidden = searchResults.length > 0;
      renderSearch();
      notice(
        "youtubeNotice",
        searchResults.length
          ? `${searchResults.length} resultados para “${result.query}”`
          : "No se encontraron videos. Prueba con otra búsqueda.",
      );
    } catch (error) {
      notice("youtubeNotice", error.message, true);
    } finally {
      searching = false;
      $("youtubeSearchBtn").textContent = "Buscar";
      refreshControls();
    }
  };
  async function itemAction(item, action) {
    try {
      await API("queue/item", { id: item.id, action });
      notice("queueNotice", "");
      await refreshQueue();
    } catch (error) {
      notice("queueNotice", error.message, true);
    }
  }
  function editItem(item) {
    editID = item.id;
    $("queueItemName").textContent = item.title;
    $("itemUseGlobal").checked = item.useGlobal;
    fillSettings("item", item.useGlobal ? queue.global : item.settings);
    $("itemSettingsFieldset").disabled = item.useGlobal;
    notice("itemNotice", "");
    $("queueItemDialog").showModal();
  }
  $("itemUseGlobal").onchange = () => {
    $("itemSettingsFieldset").disabled = $("itemUseGlobal").checked;
    if ($("itemUseGlobal").checked) fillSettings("item", queue.global);
  };
  $("itemCancel").onclick = () => $("queueItemDialog").close();
  $("itemSave").onclick = async () => {
    $("itemSave").disabled = true;
    try {
      await API("queue/item", {
        id: editID,
        action: "configure",
        useGlobal: $("itemUseGlobal").checked,
        settings: readSettings("item"),
      });
      $("queueItemDialog").close();
      await refreshQueue();
    } catch (error) {
      notice("itemNotice", error.message, true);
    } finally {
      $("itemSave").disabled = false;
    }
  };
  const states = {
    pending: "Pendiente",
    downloading: "Descargando",
    completed: "Completado",
    failed: "Error",
    canceled: "Detenido",
  };
  function renderQueue() {
    const pending = queue.items.filter(
      (item) => item.state === "pending",
    ).length;
    const finished = queue.items.filter(
      (item) => item.state === "completed",
    ).length;
    document
      .querySelectorAll("[data-queue-count]")
      .forEach(
        (counter) =>
          (counter.textContent = String(
            pending +
              queue.items.filter((item) => item.state === "downloading").length,
          )),
      );
    $("queueSummary").textContent = queue.items.length
      ? `${queue.items.length} videos · ${pending} pendientes · ${finished} terminados${queue.running ? " · En curso" : ""}`
      : "Tu cola está vacía";
    $("queueEmpty").hidden = queue.items.length > 0;
    if (queue.storageError) notice("queueNotice", queue.storageError, true);
    $("globalSummary").textContent = queue.global
      ? settingsSummary(queue.global)
      : "";
    const newGlobalKey = JSON.stringify(queue.global);
    if (!globalDirty && newGlobalKey !== globalKey && queue.global) {
      fillSettings("global", queue.global);
      globalKey = newGlobalKey;
    }
    const key = JSON.stringify([queue.items, queue.global]);
    if (key !== renderKey) {
      const opened = new Set(
        [...$("queueList").querySelectorAll("details[open]")].map(
          (menu) => menu.dataset.id,
        ),
      );
      $("queueList").replaceChildren();
      queue.items.forEach((item, index) => {
        const row = node("article", "card queue-row");
        row.dataset.id = item.id;
        row.append(
          node("span", "queue-number", String(index + 1)),
          image(item),
        );
        const copy = node("div", "queue-copy");
        const top = node("div", "queue-row-top");
        top.append(
          node("h3", "video-title", item.title),
          node(
            "span",
            "queue-state " + item.state,
            states[item.state] || item.state,
          ),
        );
        copy.append(
          top,
          node("p", "queue-url", item.url),
          node(
            "p",
            "queue-settings-text",
            (item.useGlobal ? "Global · " : "Personalizado · ") +
              settingsSummary(
                ["downloading", "completed"].includes(item.state)
                  ? item.settings
                  : item.useGlobal
                    ? queue.global
                    : item.settings,
              ),
          ),
        );
        if (item.state === "downloading") {
          const track = node("div", "track");
          const fill = node("div", "fill");
          fill.style.width = Math.min(100, Math.max(0, item.progress)) + "%";
          track.append(fill);
          copy.append(
            track,
            node("p", "queue-item-progress", Math.round(item.progress) + " %"),
          );
        }
        if (item.error) copy.append(node("p", "queue-error", item.error));
        if (item.file)
          copy.append(node("p", "queue-file", "Guardado: " + item.file));
        const busy = item.state === "downloading";
        const actions = [
          {
            label: "Opciones de descarga",
            disabled: busy || item.state === "completed",
            run: () => editItem(item),
          },
          {
            label: "Actualizar título y datos",
            disabled: busy,
            run: async () => {
              notice("queueNotice", "Consultando el video…");
              try {
                const data = await API("info", { url: item.url });
                await API("queue/item", {
                  id: item.id,
                  action: "metadata",
                  title: data.title,
                  thumbnail: data.thumbnail,
                  uploader: data.uploader,
                });
                await refreshQueue();
                notice("queueNotice", "Datos actualizados.");
              } catch (error) {
                notice("queueNotice", error.message, true);
              }
            },
          },
          {
            label: "Subir en la cola",
            disabled:
              busy ||
              index === 0 ||
              queue.items[index - 1]?.state === "downloading",
            run: () => itemAction(item, "up"),
          },
          {
            label: "Bajar en la cola",
            disabled:
              busy ||
              index === queue.items.length - 1 ||
              queue.items[index + 1]?.state === "downloading",
            run: () => itemAction(item, "down"),
          },
        ];
        if (["failed", "canceled", "completed"].includes(item.state))
          actions.push({
            label: "Volver a poner pendiente",
            run: () => itemAction(item, "retry"),
          });
        actions.push({
          label: "Quitar de la cola",
          disabled: busy,
          run: () => itemAction(item, "remove"),
        });
        const options = menu(item.title, actions, item.id);
        options.open = opened.has(item.id);
        row.append(copy, options);
        $("queueList").append(row);
      });
      renderKey = key;
    }
    refreshControls();
  }
  async function refreshQueue() {
    const sequence = ++queueRequestSequence;
    try {
      const state = await API("queue");
      if (sequence >= queueAppliedSequence) {
        queueAppliedSequence = sequence;
        queue = state;
        renderQueue();
      }
    } catch (error) {
      notice("queueNotice", error.message, true);
    }
  }
  $("queueAddLinks").onclick = async () => {
    const links = $("queueLinks").value.trim().split(/\s+/).filter(Boolean);
    if (!links.length)
      return notice("queueNotice", "Pega al menos un enlace.", true);
    $("queueAddLinks").disabled = true;
    try {
      const result = await API("queue/add", {
        items: links.map((url) => ({ url })),
      });
      $("queueLinks").value = "";
      notice(
        "queueNotice",
        `${result.added} videos añadidos${result.duplicates ? " · " + result.duplicates + " duplicados omitidos" : ""}.`,
      );
      await refreshQueue();
    } catch (error) {
      notice("queueNotice", error.message, true);
    } finally {
      $("queueAddLinks").disabled = false;
    }
  };
  $("queuePaste").onclick = async () => {
    $("queuePaste").disabled = true;
    try {
      const text = (await readClipboard()).trim();
      if (!text) throw new Error("El portapapeles no contiene enlaces.");
      const existing = $("queueLinks").value.trim();
      $("queueLinks").value = existing ? existing + "\n" + text : text;
      $("queueLinks").focus();
    } catch (error) {
      notice("queueNotice", error.message, true);
    } finally {
      $("queuePaste").disabled = false;
    }
  };
  async function saveGlobal(applyToAll) {
    try {
      await API("queue/config", {
        settings: readSettings("global"),
        applyToAll,
      });
      globalDirty = false;
      globalKey = "";
      await refreshQueue();
      notice(
        "queueNotice",
        applyToAll
          ? "Todos los videos pendientes usarán los ajustes globales."
          : "Ajustes globales guardados.",
      );
    } catch (error) {
      notice("queueNotice", error.message, true);
    }
  }
  $("globalSave").onclick = () => saveGlobal(false);
  $("globalApplyAll").onclick = () => saveGlobal(true);
  for (const [id, action] of [
    ["queueStart", "start"],
    ["queueStop", "stop"],
    ["queueClearCompleted", "clear-completed"],
  ])
    $(id).onclick = async () => {
      try {
        await API("queue/control", { action });
        notice(
          "queueNotice",
          action === "start"
            ? "Descargando la cola en orden."
            : action === "stop"
              ? "Deteniendo la descarga actual. Los pendientes se conservan."
              : "Videos terminados retirados de la cola.",
        );
        await refreshQueue();
      } catch (error) {
        notice("queueNotice", error.message, true);
      }
    };
  refreshQueue();
  setInterval(refreshQueue, 1000);
})();
