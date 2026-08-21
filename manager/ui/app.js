const VIEW_MODE_KEY = "pluginkit-manager-view";

const state = {
  catalog: [],
  catalogByKind: new Map(),
  document: {
    rootId: "agent",
    plugin: { use: "agent", config: {}, deps: {} },
    shared: {},
  },
  viewMode: loadViewMode(),
  picker: null,
  pickerMode: "inline",
  configTarget: null,
};

const els = {
  rootId: document.getElementById("rootId"),
  rootKind: document.getElementById("rootKind"),
  catalogList: document.getElementById("catalogList"),
  catalogFilter: document.getElementById("catalogFilter"),
  sharedList: document.getElementById("sharedList"),
  graph: document.getElementById("graph"),
  status: document.getElementById("status"),
  yamlPreview: document.getElementById("yamlPreview"),
  pickerDialog: document.getElementById("pickerDialog"),
  pickerTitle: document.getElementById("pickerTitle"),
  pickerMode: document.getElementById("pickerMode"),
  pickerModeInline: document.getElementById("pickerModeInline"),
  pickerModeRef: document.getElementById("pickerModeRef"),
  pickerList: document.getElementById("pickerList"),
  pickerFilter: document.getElementById("pickerFilter"),
  configDialog: document.getElementById("configDialog"),
  configTitle: document.getElementById("configTitle"),
  configFields: document.getElementById("configFields"),
  configForm: document.getElementById("configForm"),
  yamlDialog: document.getElementById("yamlDialog"),
  yamlInput: document.getElementById("yamlInput"),
  sharedDialog: document.getElementById("sharedDialog"),
  sharedForm: document.getElementById("sharedForm"),
  sharedId: document.getElementById("sharedId"),
  sharedKind: document.getElementById("sharedKind"),
  btnViewTree: document.getElementById("btnViewTree"),
  btnViewFlat: document.getElementById("btnViewFlat"),
};

function loadViewMode() {
  const saved = localStorage.getItem(VIEW_MODE_KEY);
  return saved === "flat" ? "flat" : "tree";
}

function isFlatView() {
  return state.viewMode === "flat";
}

function syncViewModeButtons() {
  els.btnViewTree?.classList.toggle("active", state.viewMode === "tree");
  els.btnViewFlat?.classList.toggle("active", state.viewMode === "flat");
}

function setViewMode(mode) {
  const prev = state.viewMode;
  state.viewMode = mode === "flat" ? "flat" : "tree";
  localStorage.setItem(VIEW_MODE_KEY, state.viewMode);
  syncViewModeButtons();
  let statusMsg = null;
  if (state.viewMode === "tree" && prev !== "tree") {
    const inlined = inlineSingleUseShared(state.document);
    if (inlined > 0) {
      statusMsg = `已内联 ${inlined} 个仅引用一次的共享实例`;
    }
  }
  renderGraph();
  renderSharedList();
  refreshPreview();
  if (statusMsg) {
    setStatus(statusMsg, "ok");
  }
}

function clonePluginNode(node) {
  return structuredClone(node);
}

function countReferencesInDocument(document, instanceId) {
  let count = 0;
  function walk(node) {
    for (const raw of Object.values(node.deps || {})) {
      if (typeof raw === "string") {
        if (raw === instanceId) count += 1;
      } else if (Array.isArray(raw)) {
        for (const item of raw) {
          if (typeof item === "string") {
            if (item === instanceId) count += 1;
          } else if (item && typeof item === "object") {
            walk(item);
          }
        }
      } else if (raw && typeof raw === "object") {
        walk(raw);
      }
    }
  }
  walk(document.plugin);
  for (const node of Object.values(document.shared || {})) {
    walk(node);
  }
  return count;
}

function findReferenceLocation(document, instanceId) {
  let found = null;
  function walk(node) {
    if (found) return;
    for (const [extName, raw] of Object.entries(node.deps || {})) {
      if (typeof raw === "string") {
        if (raw === instanceId) {
          found = { node, extName, index: null };
          return;
        }
      } else if (Array.isArray(raw)) {
        for (let index = 0; index < raw.length; index++) {
          const item = raw[index];
          if (typeof item === "string" && item === instanceId) {
            found = { node, extName, index };
            return;
          }
          if (item && typeof item === "object") {
            walk(item);
            if (found) return;
          }
        }
      } else if (raw && typeof raw === "object") {
        walk(raw);
      }
    }
  }
  walk(document.plugin);
  for (const node of Object.values(document.shared || {})) {
    walk(node);
    if (found) return found;
  }
  return found;
}

function replaceReferenceWithInline(location, inlineNode) {
  const { node, extName, index } = location;
  if (index == null) {
    node.deps[extName] = inlineNode;
    return;
  }
  node.deps[extName][index] = inlineNode;
}

// 树状视图下，仅被引用一次的共享实例内联到引用处并移出 shared。
function inlineSingleUseShared(document) {
  if (!document.shared) {
    document.shared = {};
  }
  let total = 0;
  let changed = true;
  while (changed) {
    changed = false;
    for (const id of [...Object.keys(document.shared)]) {
      if (countReferencesInDocument(document, id) !== 1) {
        continue;
      }
      const location = findReferenceLocation(document, id);
      if (!location) {
        continue;
      }
      replaceReferenceWithInline(location, clonePluginNode(document.shared[id]));
      delete document.shared[id];
      total += 1;
      changed = true;
      break;
    }
  }
  if (Object.keys(document.shared).length === 0) {
    document.shared = {};
  }
  return total;
}

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

function ensureShared() {
  if (!state.document.shared) state.document.shared = {};
  return state.document.shared;
}

function currentDocument() {
  const shared = state.document.shared || {};
  const hasShared = Object.keys(shared).length > 0;
  return {
    rootId: els.rootId.value.trim(),
    plugin: structuredClone(state.document.plugin),
    ...(hasShared ? { shared: structuredClone(shared) } : {}),
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
  els.sharedKind.innerHTML = "";
  for (const item of state.catalog) {
    for (const select of [els.rootKind, els.sharedKind]) {
      const opt = document.createElement("option");
      opt.value = item.kind;
      opt.textContent = item.kind;
      select.appendChild(opt);
    }
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
  state.document.shared = {};
  renderAll();
  setStatus(`已新建 root kind: ${kind}`);
}

function getDescribe(kind) {
  return state.catalogByKind.get(kind);
}

function hasConfigFields(kind) {
  const desc = getDescribe(kind);
  return Array.isArray(desc?.config) && desc.config.length > 0;
}

function renderConfigButton(node, path) {
  if (!hasConfigFields(node.use)) {
    return "";
  }
  return `<button type="button" data-action="config">config</button>`;
}

function bindConfigActions(card, node, path) {
  if (!hasConfigFields(node.use)) {
    return;
  }
  card.querySelector('[data-action="config"]')?.addEventListener("click", () => openConfig(node, path));
  card.addEventListener("dblclick", () => openConfig(node, path));
}

function resolveSharedNode(refId) {
  if (refId === els.rootId.value.trim()) {
    return { node: state.document.plugin, source: "root" };
  }
  const shared = ensureShared();
  if (shared[refId]) {
    return { node: shared[refId], source: "shared" };
  }
  return { node: null, source: "missing" };
}

function countReferences(instanceId) {
  return countReferencesInDocument(state.document, instanceId);
}

function renderAll() {
  renderSharedList();
  renderGraph();
  refreshPreview();
}

function renderSharedList() {
  if (!els.sharedList) return;
  els.sharedList.innerHTML = "";
  const shared = ensureShared();
  const ids = Object.keys(shared).sort();
  if (ids.length === 0) {
    els.sharedList.innerHTML =
      `<div class="shared-empty">暂无共享实例。添加后可被多个依赖引用（<code>deps: instance-id</code>）。</div>`;
    return;
  }
  for (const id of ids) {
    els.sharedList.appendChild(renderSharedCard(id, shared[id]));
  }
}

function renderSharedCard(id, node) {
  const card = document.createElement("div");
  card.className = "shared-card";
  card.dataset.sharedId = id;

  const refCount = countReferences(id);
  const header = document.createElement("div");
  header.className = "node-header";
  header.innerHTML = `
    <div class="node-title">
      <div class="kind">${node.use}</div>
      <div class="path">${id}${refCount ? ` · 引用 ${refCount} 处` : ""}</div>
    </div>
    <div class="node-actions">
      ${renderConfigButton(node, `@shared:${id}`)}
      <button type="button" data-action="remove">删除</button>
    </div>
  `;
  header.querySelector('[data-action="remove"]').addEventListener("click", () =>
    removeSharedInstance(id)
  );
  card.appendChild(header);
  bindConfigActions(card, node, `@shared:${id}`);
  card.appendChild(renderExtensions(node, `@shared:${id}`, { flat: isFlatView() }));
  return card;
}

function removeSharedInstance(id) {
  const refs = countReferences(id);
  if (refs > 0) {
    setStatus(`无法删除：${id} 仍被引用 ${refs} 处`, "err");
    return;
  }
  delete ensureShared()[id];
  renderAll();
  setStatus(`已删除共享实例 ${id}`, "ok");
}

function renderGraph() {
  els.graph.innerHTML = "";
  els.graph.classList.toggle("flat-view", isFlatView());
  if (isFlatView()) {
    renderFlatGraph();
    return;
  }
  renderTreeGraph();
}

function renderTreeGraph() {
  const root = document.createElement("div");
  root.className = "graph-root";
  root.appendChild(renderNode(state.document.plugin, "root", true));
  els.graph.appendChild(root);
}

function collectInlineEntries(node, path, out) {
  out.push({ node, path, isRoot: path === "root" });
  const desc = getDescribe(node.use) || { extensions: [] };
  for (const ext of desc.extensions || []) {
    const dep = node.deps?.[ext.name];
    if (dep == null) continue;
    if (ext.list) {
      const items = Array.isArray(dep) ? dep : [];
      items.forEach((item, index) => {
        if (typeof item === "string") return;
        collectInlineEntries(item, `${path}.${ext.name}[${index}]`, out);
      });
      continue;
    }
    if (typeof dep === "string") continue;
    collectInlineEntries(dep, `${path}.${ext.name}`, out);
  }
}

function renderFlatGraph() {
  const entries = [];
  collectInlineEntries(state.document.plugin, "root", entries);
  for (const entry of entries) {
    els.graph.appendChild(renderFlatNode(entry.node, entry.path, entry.isRoot));
  }
}

function renderFlatNode(node, path, isRoot = false) {
  const card = document.createElement("div");
  card.className = `node-card${isRoot ? " root" : ""}`;
  card.dataset.path = path;

  const header = document.createElement("div");
  header.className = "node-header";
  header.innerHTML = `
    <div class="node-title">
      <div class="kind">${node.use}</div>
      <div class="path">${path}</div>
    </div>
    <div class="node-actions">
      ${renderConfigButton(node, path)}
      ${isRoot ? "" : `<button type="button" data-action="remove">删除</button>`}
    </div>
  `;
  const removeBtn = header.querySelector('[data-action="remove"]');
  if (removeBtn) {
    removeBtn.addEventListener("click", () => removeNode(path));
  }

  card.appendChild(header);
  bindConfigActions(card, node, path);
  card.appendChild(renderExtensions(node, path, { flat: true }));
  return card;
}

function scrollToPath(path) {
  const target = document.querySelector(`[data-path="${CSS.escape(path)}"]`);
  target?.scrollIntoView({ behavior: "smooth", block: "center" });
}

function renderFlatLink(node, path) {
  const link = document.createElement("div");
  link.className = "flat-link";
  link.innerHTML = `
    <span class="kind">${node.use}</span>
    <span class="path">${path}</span>
  `;
  link.addEventListener("click", () => scrollToPath(path));
  return link;
}

function renderNode(node, path, isRoot = false) {
  const card = document.createElement("div");
  card.className = `node-card${isRoot ? " root" : ""}`;
  card.dataset.path = path;

  const header = document.createElement("div");
  header.className = "node-header";
  header.innerHTML = `
    <div class="node-title">
      <div class="kind">${node.use}</div>
      <div class="path">${path}</div>
    </div>
    <div class="node-actions">
      ${renderConfigButton(node, path)}
      ${isRoot ? "" : `<button type="button" data-action="remove">删除</button>`}
    </div>
  `;
  const removeBtn = header.querySelector('[data-action="remove"]');
  if (removeBtn) {
    removeBtn.addEventListener("click", () => removeNode(path));
  }

  card.appendChild(header);
  bindConfigActions(card, node, path);
  card.appendChild(renderExtensions(node, path));
  return card;
}

function renderExtensions(node, path, options = {}) {
  const extensions = document.createElement("div");
  extensions.className = "extensions";
  const desc = getDescribe(node.use) || { extensions: [] };

  for (const ext of desc.extensions || []) {
    if (ext.optional && !node.deps?.[ext.name]) {
      extensions.appendChild(renderOptionalExtension(node, path, ext));
      continue;
    }
    extensions.appendChild(renderExtension(node, path, ext, options));
  }
  return extensions;
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

function renderExtension(node, path, ext, options = {}) {
  const flat = options.flat === true;
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
        const childPath = `${path}.${ext.name}[${index}]`;
        if (typeof item === "string") {
          body.appendChild(renderRefNode(item, childPath, ext.name, index));
        } else if (flat) {
          body.appendChild(renderFlatLink(item, childPath));
        } else {
          body.appendChild(renderNode(item, childPath));
        }
      });
      body.appendChild(renderEmptySlot(node, path, ext.name, true));
    }
  } else {
    const dep = node.deps?.[ext.name];
    const childPath = `${path}.${ext.name}`;
    if (typeof dep === "string") {
      body.appendChild(renderRefNode(dep, childPath, ext.name));
    } else if (dep) {
      if (flat) {
        body.appendChild(renderFlatLink(dep, childPath));
      } else {
        body.appendChild(renderNode(dep, childPath));
      }
    } else if (!ext.optional) {
      body.appendChild(renderEmptySlot(node, path, ext.name, false));
    }
  }

  wrap.appendChild(body);
  return wrap;
}

function renderRefNode(refId, path, extName, index = null) {
  const resolved = resolveSharedNode(refId);
  const card = document.createElement("div");
  card.className = "ref-card";
  card.dataset.path = path;
  const kind = resolved.node?.use || "unknown";
  const sourceLabel = resolved.source === "root" ? "root" : "shared";
  card.innerHTML = `
    <div>
      <div class="ref-label">→ ${refId}</div>
      <div class="ref-meta">${sourceLabel} · ${kind}</div>
    </div>
    <div class="node-actions">
      <button type="button" data-action="goto">查看</button>
      <button type="button" data-action="remove">解除</button>
    </div>
  `;
  card.querySelector('[data-action="goto"]').addEventListener("click", () => {
    if (resolved.source === "shared") {
      const target = document.querySelector(`[data-shared-id="${refId}"]`);
      target?.scrollIntoView({ behavior: "smooth", block: "center" });
    } else if (resolved.source === "root") {
      document.querySelector(".graph-root")?.scrollIntoView({ behavior: "smooth", block: "center" });
    } else {
      setStatus(`引用目标 ${refId} 不存在`, "err");
    }
  });
  card.querySelector('[data-action="remove"]').addEventListener("click", () => {
    removeRef(path, extName, index);
  });
  return card;
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
  if (path.startsWith("@shared:")) {
    return ensureShared()[path.slice(8)];
  }
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

function resolveDepParent(path) {
  if (path.startsWith("@shared:")) {
    const rest = path.slice(8);
    const dot = rest.indexOf(".");
    if (dot === -1) {
      return { node: null, extName: null, index: null };
    }
    const sharedId = rest.slice(0, dot);
    const suffix = rest.slice(dot + 1);
    const node = ensureShared()[sharedId];
    if (!node) {
      return { node: null, extName: null, index: null };
    }
    return resolveParentFromNode(node, suffix);
  }
  return resolveParent(path);
}

function resolveParentFromNode(rootNode, suffix) {
  const parts = suffix.split(".");
  const last = parts.pop();
  let current = rootNode;
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
  const { node, extName, index } = resolveDepParent(path);
  if (!node) return;
  if (index == null) {
    delete node.deps[extName];
  } else {
    node.deps[extName].splice(index, 1);
    if (node.deps[extName].length === 0) delete node.deps[extName];
  }
  renderAll();
}

function removeRef(path, extName, index) {
  const { node } = resolveDepParent(path);
  if (!node) return;
  if (index == null) {
    delete node.deps[extName];
  } else {
    node.deps[extName].splice(index, 1);
    if (node.deps[extName].length === 0) delete node.deps[extName];
  }
  renderAll();
}

function setPickerMode(mode) {
  state.pickerMode = mode;
  els.pickerModeInline.classList.toggle("active", mode === "inline");
  els.pickerModeRef.classList.toggle("active", mode === "ref");
  renderPicker(els.pickerFilter.value);
}

async function openPicker(parentNode, parentPath, extName, isList) {
  state.picker = { parentNode, parentPath, extName, isList };
  state.pickerMode = "inline";
  els.pickerTitle.textContent = `为 ${parentNode.use}.${extName} 选择插件`;
  els.pickerFilter.value = "";
  const hasShared = Object.keys(ensureShared()).length > 0;
  els.pickerMode.classList.toggle("hidden", !hasShared);
  setPickerMode("inline");
  els.pickerDialog.showModal();
}

async function renderPicker(filter) {
  if (state.pickerMode === "ref") {
    await renderPickerRef(filter);
    return;
  }
  const { parentNode, extName } = state.picker;
  const kinds = await api(
    `/api/compatible?parent=${encodeURIComponent(parentNode.use)}&ext=${encodeURIComponent(extName)}`
  );
  const q = filter.trim().toLowerCase();
  els.pickerList.innerHTML = "";
  for (const kind of kinds.kinds || []) {
    if (q && !kind.includes(q)) continue;
    const li = document.createElement("li");
    li.className = "picker-item";
    const info = getDescribe(kind);
    li.innerHTML = `
      <div class="kind">${kind}</div>
      <div class="meta">${info?.returnType || ""} · 新建内联</div>
    `;
    li.addEventListener("click", async () => {
      await attachPlugin(kind);
      els.pickerDialog.close();
    });
    els.pickerList.appendChild(li);
  }
}

async function renderPickerRef(filter) {
  const { parentNode, extName } = state.picker;
  const kinds = await api(
    `/api/compatible?parent=${encodeURIComponent(parentNode.use)}&ext=${encodeURIComponent(extName)}`
  );
  const compatible = new Set(kinds.kinds || []);
  const q = filter.trim().toLowerCase();
  els.pickerList.innerHTML = "";

  const entries = Object.entries(ensureShared())
    .filter(([, node]) => compatible.has(node.use))
    .filter(([id, node]) => !q || id.includes(q) || node.use.includes(q));

  if (entries.length === 0) {
    const li = document.createElement("li");
    li.className = "picker-item";
    li.innerHTML = `<div class="meta">没有类型匹配的共享实例，请先添加或使用「新建内联」。</div>`;
    els.pickerList.appendChild(li);
    return;
  }

  for (const [id, node] of entries) {
    const li = document.createElement("li");
    li.className = "picker-item";
    li.innerHTML = `
      <div class="kind">${id}</div>
      <div class="meta">${node.use} · 引用共享 · ${countReferences(id)} 处引用</div>
    `;
    li.addEventListener("click", () => {
      attachReference(id);
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
  renderAll();
  setStatus(`已添加内联 ${kind} 到 ${parentNode.use}.${extName}`, "ok");
}

function attachReference(refId) {
  const { parentNode, extName, isList } = state.picker;
  const deps = ensureDeps(parentNode);
  if (isList) {
    if (!Array.isArray(deps[extName])) deps[extName] = [];
    deps[extName].push(refId);
  } else {
    deps[extName] = refId;
  }
  renderAll();
  setStatus(`已引用共享实例 ${refId} 到 ${parentNode.use}.${extName}`, "ok");
}

function normalizeDepsFromTemplate(deps) {
  if (!deps) return {};
  const out = {};
  for (const [key, value] of Object.entries(deps)) {
    if (Array.isArray(value)) {
      out[key] = value
        .map((item) => ({
          use: "",
          config: {},
          deps: {},
          ...(typeof item === "object" ? { use: item.use || "" } : {}),
        }))
        .filter((item) => item.use);
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
  renderAll();
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
    state.document.shared = doc.shared || {};
    let status = "导入成功";
    if (!isFlatView()) {
      const inlined = inlineSingleUseShared(state.document);
      if (inlined > 0) {
        status = `导入成功，已内联 ${inlined} 个仅引用一次的共享实例`;
      }
    }
    renderAll();
    setStatus(status, "ok");
  } catch (err) {
    setStatus(err.message, "err");
  }
});

document.getElementById("btnAddShared").addEventListener("click", () => {
  els.sharedId.value = "";
  if (state.catalog[0]) {
    els.sharedKind.value = state.catalog[0].kind;
  }
  els.sharedDialog.showModal();
});

els.sharedForm.addEventListener("submit", async (event) => {
  if (event.submitter?.id !== "sharedSave") return;
  const id = els.sharedId.value.trim();
  const kind = els.sharedKind.value;
  const rootId = els.rootId.value.trim();
  if (!id) {
    setStatus("实例 ID 不能为空", "err");
    return;
  }
  if (id === rootId) {
    setStatus("共享实例 ID 不能与 Root ID 相同", "err");
    return;
  }
  if (ensureShared()[id]) {
    setStatus(`共享实例 ${id} 已存在`, "err");
    return;
  }
  const tmpl = await loadTemplate(kind);
  ensureShared()[id] = {
    use: tmpl.use || kind,
    config: tmpl.config || {},
    deps: normalizeDepsFromTemplate(tmpl.deps),
  };
  renderAll();
  setStatus(`已创建共享实例 ${id} (${kind})`, "ok");
});

els.pickerModeInline.addEventListener("click", () => setPickerMode("inline"));
els.pickerModeRef.addEventListener("click", () => setPickerMode("ref"));
els.btnViewTree?.addEventListener("click", () => setViewMode("tree"));
els.btnViewFlat?.addEventListener("click", () => setViewMode("flat"));

els.rootId.addEventListener("change", refreshPreview);
els.rootKind.addEventListener("change", () => newDocument(els.rootKind.value));
els.catalogFilter.addEventListener("input", (e) => renderCatalog(e.target.value));
els.pickerFilter.addEventListener("input", (e) => renderPicker(e.target.value));

async function boot() {
  const data = await api("/api/catalog");
  state.catalog = data.kinds || [];
  state.catalogByKind = new Map(state.catalog.map((item) => [item.kind, item]));
  fillRootKindOptions();
  syncViewModeButtons();
  renderCatalog();
  if (state.catalog.some((item) => item.kind === "agent")) {
    await newDocument("agent");
  } else if (state.catalog[0]) {
    await newDocument(state.catalog[0].kind);
  } else {
    renderAll();
  }
}

boot().catch((err) => setStatus(err.message, "err"));
