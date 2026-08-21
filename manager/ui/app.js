const state = {
  catalog: [],
  catalogByKind: new Map(),
  document: {
    rootId: "agent",
    plugin: { use: "agent", config: {}, deps: {} },
  },
  picker: null,
  configTarget: null,
};

const els = {
  rootId: document.getElementById("rootId"),
  rootKind: document.getElementById("rootKind"),
  catalogList: document.getElementById("catalogList"),
  catalogFilter: document.getElementById("catalogFilter"),
  graph: document.getElementById("graph"),
  status: document.getElementById("status"),
  yamlPreview: document.getElementById("yamlPreview"),
  pickerDialog: document.getElementById("pickerDialog"),
  pickerTitle: document.getElementById("pickerTitle"),
  pickerList: document.getElementById("pickerList"),
  pickerFilter: document.getElementById("pickerFilter"),
  configDialog: document.getElementById("configDialog"),
  configTitle: document.getElementById("configTitle"),
  configFields: document.getElementById("configFields"),
  configForm: document.getElementById("configForm"),
  yamlDialog: document.getElementById("yamlDialog"),
  yamlInput: document.getElementById("yamlInput"),
};

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `${res.status} ${res.statusText}`);
  }
  return data;
}

function setStatus(text, kind = "info") {
  els.status.textContent = text;
  els.status.className = `status ${kind}`;
}

function currentDocument() {
  return {
    rootId: els.rootId.value.trim(),
    plugin: structuredClone(state.document.plugin),
  };
}

function renderCatalog(filter = "") {
  const q = filter.trim().toLowerCase();
  els.catalogList.innerHTML = "";
  for (const item of state.catalog) {
    if (q && !item.kind.includes(q)) continue;
    const li = document.createElement("li");
    li.className = "catalog-item";
    li.innerHTML = `
      <div class="kind">${item.kind}</div>
      <div class="meta">${item.returnType || "unknown return"}</div>
    `;
    li.addEventListener("click", () => {
      els.rootKind.value = item.kind;
      newDocument(item.kind);
    });
    els.catalogList.appendChild(li);
  }
}

function fillRootKindOptions() {
  els.rootKind.innerHTML = "";
  for (const item of state.catalog) {
    const opt = document.createElement("option");
    opt.value = item.kind;
    opt.textContent = item.kind;
    els.rootKind.appendChild(opt);
  }
}

async function loadTemplate(kind) {
  return api(`/api/template/${encodeURIComponent(kind)}`, { method: "POST" });
}

async function newDocument(kind) {
  const tmpl = await loadTemplate(kind);
  state.document.plugin = {
    use: tmpl.use || kind,
    config: tmpl.config || {},
    deps: {},
  };
  renderGraph();
  refreshPreview();
  setStatus(`已新建 root kind: ${kind}`);
}

function getDescribe(kind) {
  return state.catalogByKind.get(kind);
}

function renderGraph() {
  els.graph.innerHTML = "";
  const root = document.createElement("div");
  root.className = "graph-root";
  root.appendChild(renderNode(state.document.plugin, "root", true));
  els.graph.appendChild(root);
}

function renderNode(node, path, isRoot = false) {
  const card = document.createElement("div");
  card.className = `node-card${isRoot ? " root" : ""}`;
  card.dataset.path = path;

  const desc = getDescribe(node.use) || { extensions: [], config: [] };
  const header = document.createElement("div");
  header.className = "node-header";
  header.innerHTML = `
    <div class="node-title">
      <div class="kind">${node.use}</div>
      <div class="path">${path}</div>
    </div>
    <div class="node-actions">
      <button type="button" data-action="config">config</button>
      ${isRoot ? "" : `<button type="button" data-action="remove">删除</button>`}
    </div>
  `;
  header.querySelector('[data-action="config"]').addEventListener("click", () => openConfig(node, path));
  card.addEventListener("dblclick", () => openConfig(node, path));

  const removeBtn = header.querySelector('[data-action="remove"]');
  if (removeBtn) {
    removeBtn.addEventListener("click", () => removeNode(path));
  }

  card.appendChild(header);

  const extensions = document.createElement("div");
  extensions.className = "extensions";

  for (const ext of desc.extensions || []) {
    if (ext.optional && !node.deps?.[ext.name]) {
      extensions.appendChild(renderOptionalExtension(node, path, ext));
      continue;
    }
    extensions.appendChild(renderExtension(node, path, ext));
  }

  card.appendChild(extensions);
  return card;
}

function renderOptionalExtension(node, path, ext) {
  const optional = document.createElement("div");
  optional.className = "extension";
  optional.innerHTML = `
    <div class="extension-head">
      <div>
        <div class="extension-name">${ext.name} <span class="extension-meta">optional · ${ext.type}</span></div>
      </div>
      <button type="button" class="add-btn" title="添加可选依赖">+</button>
    </div>
  `;
  optional.querySelector(".add-btn").addEventListener("click", () =>
    openPicker(node, path, ext.name, ext.list)
  );
  return optional;
}

function renderExtension(node, path, ext) {
  const wrap = document.createElement("div");
  wrap.className = "extension";
  const head = document.createElement("div");
  head.className = "extension-head";
  head.innerHTML = `
    <div>
      <div class="extension-name">${ext.name}</div>
      <div class="extension-meta">${ext.list ? "[]" : ""}${ext.type}${ext.optional ? " · optional" : ""}</div>
    </div>
    <button type="button" class="add-btn" title="添加插件">+</button>
  `;
  head.querySelector(".add-btn").addEventListener("click", () =>
    openPicker(node, path, ext.name, ext.list)
  );
  wrap.appendChild(head);

  const body = document.createElement("div");
  body.className = "extension-body";

  if (ext.list) {
    const items = Array.isArray(node.deps?.[ext.name]) ? node.deps[ext.name] : [];
    if (items.length === 0) {
      body.appendChild(renderEmptySlot(node, path, ext.name, true));
    } else {
      items.forEach((item, index) => {
        body.appendChild(renderNode(item, `${path}.${ext.name}[${index}]`));
      });
      body.appendChild(renderEmptySlot(node, path, ext.name, true));
    }
  } else if (node.deps?.[ext.name]) {
    body.appendChild(renderNode(node.deps[ext.name], `${path}.${ext.name}`));
  } else if (!ext.optional) {
    body.appendChild(renderEmptySlot(node, path, ext.name, false));
  }

  wrap.appendChild(body);
  return wrap;
}

function renderEmptySlot(node, path, extName, isList) {
  const slot = document.createElement("div");
  slot.className = "empty-slot";
  slot.innerHTML = `<span>${isList ? "空列表扩展点" : "未配置"} · ${extName}</span><button type="button" class="add-btn">+</button>`;
  slot.querySelector(".add-btn").addEventListener("click", () => openPicker(node, path, extName, isList));
  return slot;
}

function ensureDeps(node) {
  if (!node.deps) node.deps = {};
  return node.deps;
}

function resolveNode(path) {
  if (path === "root") return state.document.plugin;
  const parts = path.slice(5).split(".");
  let current = state.document.plugin;
  for (const part of parts) {
    const match = part.match(/^(.+)\[(\d+)\]$/);
    if (match) {
      const [, name, index] = match;
      current = current.deps[name][Number(index)];
    } else {
      current = current.deps[part];
    }
  }
  return current;
}

function resolveParent(path) {
  if (path === "root") return { node: state.document.plugin, extName: null, index: null };
  const parts = path.slice(5).split(".");
  const last = parts.pop();
  let current = state.document.plugin;
  for (const part of parts) {
    const match = part.match(/^(.+)\[(\d+)\]$/);
    if (match) {
      const [, name, index] = match;
      current = current.deps[name][Number(index)];
    } else {
      current = current.deps[part];
    }
  }
  const listMatch = last.match(/^(.+)\[(\d+)\]$/);
  if (listMatch) {
    return {
      node: current,
      extName: listMatch[1],
      index: Number(listMatch[2]),
    };
  }
  return { node: current, extName: last, index: null };
}

function removeNode(path) {
  const { node, extName, index } = resolveParent(path);
  if (index == null) {
    delete node.deps[extName];
  } else {
    node.deps[extName].splice(index, 1);
    if (node.deps[extName].length === 0) delete node.deps[extName];
  }
  renderGraph();
  refreshPreview();
}

async function openPicker(parentNode, parentPath, extName, isList) {
  state.picker = { parentNode, parentPath, extName, isList };
  els.pickerTitle.textContent = `为 ${parentNode.use}.${extName} 选择插件`;
  els.pickerFilter.value = "";
  await renderPicker("");
  els.pickerDialog.showModal();
}

async function renderPicker(filter) {
  const { parentNode, extName } = state.picker;
  const kinds = await api(`/api/compatible?parent=${encodeURIComponent(parentNode.use)}&ext=${encodeURIComponent(extName)}`);
  const q = filter.trim().toLowerCase();
  els.pickerList.innerHTML = "";
  for (const kind of kinds.kinds || []) {
    if (q && !kind.includes(q)) continue;
    const li = document.createElement("li");
    li.className = "picker-item";
    const info = getDescribe(kind);
    li.innerHTML = `
      <div class="kind">${kind}</div>
      <div class="meta">${info?.returnType || ""}</div>
    `;
    li.addEventListener("click", async () => {
      await attachPlugin(kind);
      els.pickerDialog.close();
    });
    els.pickerList.appendChild(li);
  }
}

async function attachPlugin(kind) {
  const { parentNode, extName, isList } = state.picker;
  const tmpl = await loadTemplate(kind);
  const child = {
    use: tmpl.use || kind,
    config: tmpl.config || {},
    deps: normalizeDepsFromTemplate(tmpl.deps),
  };
  const deps = ensureDeps(parentNode);
  if (isList) {
    if (!Array.isArray(deps[extName])) deps[extName] = [];
    deps[extName].push(child);
  } else {
    deps[extName] = child;
  }
  renderGraph();
  refreshPreview();
  setStatus(`已添加 ${kind} 到 ${parentNode.use}.${extName}`, "ok");
}

function normalizeDepsFromTemplate(deps) {
  if (!deps) return {};
  const out = {};
  for (const [key, value] of Object.entries(deps)) {
    if (Array.isArray(value)) {
      out[key] = value.map((item) => ({
        use: "",
        config: {},
        deps: {},
        ...(typeof item === "object" ? { use: item.use || "" } : {}),
      })).filter((item) => item.use);
    } else if (value && typeof value === "object") {
      if (value.use) {
        out[key] = { use: value.use, config: value.config || {}, deps: {} };
      }
    }
  }
  return out;
}

function openConfig(node, path) {
  state.configTarget = { node, path };
  const desc = getDescribe(node.use) || { config: [] };
  els.configTitle.textContent = `编辑 ${node.use} config`;
  els.configFields.innerHTML = "";

  if (!desc.config || desc.config.length === 0) {
    els.configFields.innerHTML = `<p class="extension-meta">该插件没有 config 字段。</p>`;
  } else {
    if (!node.config) node.config = {};
    for (const field of desc.config) {
      els.configFields.appendChild(renderConfigField(node, field));
    }
  }
  els.configDialog.showModal();
}

function renderConfigField(node, field) {
  const row = document.createElement("div");
  row.className = "field-row";
  const label = document.createElement("label");
  label.textContent = `${field.name} (${field.type})`;
  const input = document.createElement("input");
  input.type = "text";
  input.value = node.config?.[field.name] ?? "";
  input.dataset.field = field.name;
  label.appendChild(input);
  row.appendChild(label);
  return row;
}

els.configForm.addEventListener("submit", (event) => {
  if (event.submitter?.id !== "configSave") return;
  const { node } = state.configTarget;
  if (!node.config) node.config = {};
  for (const input of els.configFields.querySelectorAll("input[data-field]")) {
    const key = input.dataset.field;
    const desc = getDescribe(node.use)?.config?.find((f) => f.name === key);
    if (desc?.type === "bool") {
      node.config[key] = input.value === "true";
    } else if (desc?.type === "int" || desc?.type === "int64") {
      node.config[key] = Number.parseInt(input.value, 10) || 0;
    } else if (desc?.type === "float64" || desc?.type === "float32") {
      node.config[key] = Number.parseFloat(input.value) || 0;
    } else {
      node.config[key] = input.value;
    }
  }
  renderGraph();
  refreshPreview();
  setStatus("config 已更新", "ok");
});

async function refreshPreview() {
  try {
    const data = await api("/api/export", {
      method: "POST",
      body: JSON.stringify(currentDocument()),
    });
    els.yamlPreview.textContent = data.yaml;
  } catch (err) {
    els.yamlPreview.textContent = `# ${err.message}`;
  }
}

async function validateDocument() {
  try {
    const data = await api("/api/validate", {
      method: "POST",
      body: JSON.stringify(currentDocument()),
    });
    if (data.ok) {
      setStatus("校验通过", "ok");
    } else {
      setStatus(data.error || "校验失败", "err");
    }
  } catch (err) {
    setStatus(err.message, "err");
  }
}

document.getElementById("btnNew").addEventListener("click", () => {
  newDocument(els.rootKind.value);
});

document.getElementById("btnValidate").addEventListener("click", validateDocument);

document.getElementById("btnExport").addEventListener("click", async () => {
  try {
    const data = await api("/api/export", {
      method: "POST",
      body: JSON.stringify(currentDocument()),
    });
    await navigator.clipboard.writeText(data.yaml);
    setStatus("YAML 已复制到剪贴板", "ok");
  } catch (err) {
    setStatus(err.message, "err");
  }
});

document.getElementById("btnImport").addEventListener("click", () => {
  els.yamlInput.value = els.yamlPreview.textContent;
  els.yamlDialog.showModal();
});

document.getElementById("yamlForm").addEventListener("submit", async (event) => {
  if (event.submitter?.id !== "yamlImportConfirm") return;
  try {
    const doc = await api("/api/import", {
      method: "POST",
      body: JSON.stringify({ yaml: els.yamlInput.value }),
    });
    els.rootId.value = doc.rootId;
    els.rootKind.value = doc.plugin.use;
    state.document.plugin = doc.plugin;
    renderGraph();
    refreshPreview();
    setStatus("导入成功", "ok");
  } catch (err) {
    setStatus(err.message, "err");
  }
});

els.rootId.addEventListener("change", refreshPreview);
els.rootKind.addEventListener("change", () => newDocument(els.rootKind.value));
els.catalogFilter.addEventListener("input", (e) => renderCatalog(e.target.value));
els.pickerFilter.addEventListener("input", (e) => renderPicker(e.target.value));

async function boot() {
  const data = await api("/api/catalog");
  state.catalog = data.kinds || [];
  state.catalogByKind = new Map(state.catalog.map((item) => [item.kind, item]));
  fillRootKindOptions();
  renderCatalog();
  if (state.catalog.some((item) => item.kind === "agent")) {
    await newDocument("agent");
  } else if (state.catalog[0]) {
    await newDocument(state.catalog[0].kind);
  } else {
    renderGraph();
    refreshPreview();
  }
}

boot().catch((err) => setStatus(err.message, "err"));
