const state = {
  catalog: [],
  document: { rootId: "", plugin: { use: "" }, shared: {} },
  view: { root: null, shared: [] },
  diagnostics: [],
  yaml: "",
  selectedPath: "root",
  buildDiagnostics: null,
};

const els = {
  rootId: document.getElementById("rootId"),
  rootKind: document.getElementById("rootKind"),
  issueCount: document.getElementById("issueCount"),
  tree: document.getElementById("tree"),
  inspectorBody: document.getElementById("inspectorBody"),
  yamlPreview: document.getElementById("yamlPreview"),
  importDialog: document.getElementById("importDialog"),
  importForm: document.getElementById("importForm"),
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

function activeDiagnostics() {
  return state.buildDiagnostics ?? state.diagnostics ?? [];
}

function diagsForPath(path) {
  return activeDiagnostics().filter((d) => d.path === path || path.startsWith(d.path + ".") || d.path.startsWith(path + "."));
}

function errorCount() {
  return activeDiagnostics().filter((d) => d.severity === "error").length;
}

function hasError(path) {
  return diagsForPath(path).some((d) => d.severity === "error");
}

async function edit(op) {
  const data = await api("/api/edit", {
    method: "POST",
    body: JSON.stringify({ document: state.document, op }),
  });
  state.document = data.document;
  state.view = data.view;
  state.diagnostics = data.diagnostics || [];
  state.yaml = data.yaml || "";
  state.buildDiagnostics = null;
  els.rootId.value = state.document.rootId || "";
  if (state.document.plugin?.use) {
    els.rootKind.value = state.document.plugin.use;
  }
  render();
}

async function runBuild() {
  const data = await api("/api/build", {
    method: "POST",
    body: JSON.stringify({ document: state.document }),
  });
  state.buildDiagnostics = data.diagnostics || [];
  render();
}

function render() {
  renderIssueCount();
  renderTree();
  renderInspector();
  els.yamlPreview.textContent = state.yaml;
}

function renderIssueCount() {
  const n = errorCount();
  els.issueCount.textContent = n === 0 ? "0 个问题" : `${n} 个问题`;
  els.issueCount.className = `issue-btn ${n === 0 ? "ok" : "err"}`;
}

function renderTree() {
  els.tree.innerHTML = "";
  if (!state.view?.root) {
    els.tree.innerHTML = `<p class="empty-hint">正在加载…</p>`;
    return;
  }
  els.tree.appendChild(renderNodeCard(state.view.root, true));
}

function renderNodeCard(node, isRoot = false) {
  const card = document.createElement("div");
  card.className = `node-card${isRoot ? " root" : ""}${state.selectedPath === node.path ? " selected" : ""}${hasError(node.path) ? " has-error" : ""}`;
  card.dataset.path = node.path;

  const head = document.createElement("div");
  head.className = "node-head";
  head.innerHTML = `
    <div>
      <div class="node-kind">${escapeHtml(node.kind || node.role)}</div>
      <div class="node-path">${escapeHtml(node.path)}</div>
    </div>
  `;
  head.addEventListener("click", () => {
    state.selectedPath = node.path;
    render();
  });
  card.appendChild(head);

  if (node.slots?.length) {
    const slots = document.createElement("div");
    slots.className = "slots";
    for (const slot of node.slots) {
      slots.appendChild(renderSlot(slot));
    }
    card.appendChild(slots);
  }
  return card;
}

function renderSlot(slot) {
  const wrap = document.createElement("div");
  wrap.className = `slot${slot.status === "empty" ? " empty" : ""}${hasError(slot.path) ? " has-error" : ""}`;
  wrap.dataset.path = slot.path;

  const head = document.createElement("div");
  head.className = "slot-head";
  head.innerHTML = `
    <div>
      <div class="slot-name">${escapeHtml(slot.name)}</div>
      <div class="slot-meta">${slot.list ? "[]" : ""}${escapeHtml(slot.type || "")}${slot.optional ? " · optional" : ""}</div>
    </div>
  `;
  wrap.appendChild(head);

  if (slot.status === "empty") {
    const hint = document.createElement("div");
    hint.className = "empty-hint";
    hint.textContent = "未配置 · 点击选择插件";
    hint.style.cursor = "pointer";
    hint.addEventListener("click", () => {
      state.selectedPath = slot.path;
      render();
    });
    wrap.appendChild(hint);
    return wrap;
  }

  const items = document.createElement("div");
  items.className = "slot-items";
  for (const item of slot.items || []) {
    items.appendChild(renderItem(item, slot));
  }
  if (slot.list) {
    const add = document.createElement("button");
    add.type = "button";
    add.textContent = "+ 添加";
    add.addEventListener("click", () => {
      state.selectedPath = slot.path;
      render();
    });
    items.appendChild(add);
  }
  wrap.appendChild(items);
  return wrap;
}

function renderItem(item, slot) {
  if (item.role === "ref") {
    const chip = document.createElement("div");
    chip.className = `ref-chip${state.selectedPath === item.path ? " selected" : ""}`;
    chip.textContent = `→ ${item.refId} (${item.kind || "?"})`;
    chip.addEventListener("click", (e) => {
      e.stopPropagation();
      state.selectedPath = item.path;
      render();
    });
    return chip;
  }
  const link = document.createElement("div");
  link.className = `inline-link${state.selectedPath === item.path ? " selected" : ""}`;
  link.innerHTML = `<div class="node-kind">${escapeHtml(item.kind)}</div><div class="node-path">${escapeHtml(item.path)}</div>`;
  link.addEventListener("click", (e) => {
    e.stopPropagation();
    state.selectedPath = item.path;
    render();
  });
  if (item.slots?.length) {
    const nested = document.createElement("div");
    nested.className = "slots";
    nested.style.marginTop = "8px";
    for (const s of item.slots) {
      nested.appendChild(renderSlot(s));
    }
    link.appendChild(nested);
  }
  return link;
}

function renderInspector() {
  const path = state.selectedPath;
  const diags = diagsForPath(path);
  const body = els.inspectorBody;
  body.innerHTML = "";

  if (path === "root" || !path) {
    renderRootOverview(body, diags);
    return;
  }

  const node = findViewNode(path);
  const slot = findViewSlot(path);

  if (slot && slot.status === "empty") {
    renderEmptySlotInspector(body, slot, diags);
    return;
  }

  if (node) {
    renderNodeInspector(body, node, diags);
    return;
  }

  body.innerHTML = `<p class="empty-hint">选中路径：${escapeHtml(path)}</p>`;
  appendDiagnostics(body, diags);
}

function renderRootOverview(body, diags) {
  body.innerHTML = `
    <div class="inspector-title">Root 概览</div>
    <div class="inspector-sub">${escapeHtml(state.document.rootId)} · ${escapeHtml(state.document.plugin?.use || "")}</div>
    <p class="empty-hint">在左侧装配树选择空槽或节点进行编辑。共享实例通过「提取为共享」创建，不会单独占一栏。</p>
  `;
  const shared = state.view?.shared || [];
  if (shared.length) {
    const list = document.createElement("ul");
    list.className = "candidate-list";
    for (const s of shared) {
      const li = document.createElement("li");
      li.className = "candidate-item";
      li.innerHTML = `<div class="kind">${escapeHtml(s.path)}</div><div class="meta">${escapeHtml(s.kind)} · 引用 ${s.refCount ?? 0} 处</div>`;
      li.addEventListener("click", () => {
        state.selectedPath = s.path;
        render();
      });
      list.appendChild(li);
    }
    body.appendChild(list);
  }
  appendDiagnostics(body, diags);
}

function renderEmptySlotInspector(body, slot, diags) {
  body.innerHTML = `
    <div class="inspector-title">填槽：${escapeHtml(slot.name)}</div>
    <div class="inspector-sub">${escapeHtml(slot.path)}</div>
  `;

  if (slot.refs?.length) {
    const h = document.createElement("h3");
    h.textContent = "引用已有";
    body.appendChild(h);
    const refs = document.createElement("ul");
    refs.className = "candidate-list";
    for (const ref of slot.refs) {
      const li = document.createElement("li");
      li.className = "candidate-item";
      li.innerHTML = `<div class="kind">${escapeHtml(ref.id)}</div><div class="meta">${escapeHtml(ref.kind)} · 引用共享</div>`;
      li.addEventListener("click", () => edit({ type: "attachRef", path: slot.path, refId: ref.id }));
      refs.appendChild(li);
    }
    body.appendChild(refs);
  }

  const h2 = document.createElement("h3");
  h2.textContent = "新建内联";
  body.appendChild(h2);
  const kinds = document.createElement("ul");
  kinds.className = "candidate-list";
  for (const kind of slot.kinds || []) {
    const li = document.createElement("li");
    li.className = "candidate-item";
    li.innerHTML = `<div class="kind">${escapeHtml(kind)}</div>`;
    li.addEventListener("click", () => edit({ type: "attach", path: slot.path, kind }));
    kinds.appendChild(li);
  }
  body.appendChild(kinds);
  appendDiagnostics(body, diags);
}

function renderNodeInspector(body, node, diags) {
  body.innerHTML = `
    <div class="inspector-title">${escapeHtml(node.kind || node.role)}</div>
    <div class="inspector-sub">${escapeHtml(node.path)}</div>
  `;

  if (node.role === "ref") {
    const actions = document.createElement("div");
    actions.className = "node-actions";
    actions.style.marginBottom = "12px";
    const gotoBtn = document.createElement("button");
    gotoBtn.type = "button";
    gotoBtn.textContent = "跳到定义";
    gotoBtn.addEventListener("click", () => {
      state.selectedPath = `shared.${node.refId}`;
      render();
    });
    const removeBtn = document.createElement("button");
    removeBtn.type = "button";
    removeBtn.textContent = "解除引用";
    removeBtn.addEventListener("click", () => edit({ type: "remove", path: node.path }));
    actions.append(gotoBtn, removeBtn);
    body.appendChild(actions);
    appendDiagnostics(body, diags);
    return;
  }

  if (node.config?.length) {
    const form = document.createElement("div");
    form.className = "field-list";
    for (const field of node.config) {
      const row = document.createElement("div");
      row.className = "field-row";
      const label = document.createElement("label");
      label.textContent = `${field.name} (${field.type})`;
      const input = document.createElement("input");
      input.type = "text";
      input.value = field.value ?? "";
      input.dataset.field = field.name;
      let timer;
      input.addEventListener("input", () => {
        clearTimeout(timer);
        timer = setTimeout(() => {
          const cfg = Object.fromEntries(
            [...form.querySelectorAll("input[data-field]")].map((el) => [el.dataset.field, el.value])
          );
          edit({ type: "setConfig", path: node.path, config: cfg });
        }, 300);
      });
      label.appendChild(input);
      row.appendChild(label);
      form.appendChild(row);
    }
    body.appendChild(form);
  }

  if (node.role === "inline" && node.path !== "root") {
    const actions = document.createElement("div");
    actions.className = "node-actions";
    const hoistBtn = document.createElement("button");
    hoistBtn.type = "button";
    hoistBtn.textContent = "提取为共享";
    hoistBtn.addEventListener("click", async () => {
      const id = prompt("共享实例 ID", node.kind);
      if (!id?.trim()) return;
      await edit({ type: "hoist", path: node.path, id: id.trim() });
    });
    const removeBtn = document.createElement("button");
    removeBtn.type = "button";
    removeBtn.textContent = "删除";
    removeBtn.addEventListener("click", () => edit({ type: "remove", path: node.path }));
    actions.append(hoistBtn, removeBtn);
    body.appendChild(actions);
  }

  if (node.role === "shared" && node.slots?.length) {
    const slots = document.createElement("div");
    slots.className = "slots";
    for (const slot of node.slots) {
      slots.appendChild(renderSlot(slot));
    }
    body.appendChild(slots);
  }

  appendDiagnostics(body, diags);
}

function appendDiagnostics(body, diags) {
  if (!diags.length) return;
  const h = document.createElement("h3");
  h.textContent = "问题";
  body.appendChild(h);
  const list = document.createElement("ul");
  list.className = "diag-list";
  for (const d of diags) {
    const li = document.createElement("li");
    li.className = "diag-item";
    li.innerHTML = `<div class="path">${escapeHtml(d.path)} · ${escapeHtml(d.stage)}</div>${escapeHtml(d.message)}`;
    li.addEventListener("click", () => {
      state.selectedPath = d.path;
      render();
    });
    list.appendChild(li);
  }
  body.appendChild(list);
}

function findViewNode(path) {
  if (state.view?.root?.path === path) return state.view.root;
  const fromRoot = searchItems(state.view?.root, path);
  if (fromRoot) return fromRoot;
  for (const s of state.view?.shared || []) {
    if (s.path === path) return s;
    const found = searchItems(s, path);
    if (found) return found;
  }
  return null;
}

function searchItems(node, path) {
  if (!node?.slots) return null;
  for (const slot of node.slots) {
    for (const item of slot.items || []) {
      if (item.path === path) return item;
      const nested = searchItems(item, path);
      if (nested) return nested;
    }
  }
  return null;
}

function findViewSlot(path) {
  const nodes = [state.view?.root, ...(state.view?.shared || [])];
  for (const node of nodes) {
    const slot = findSlotInNode(node, path);
    if (slot) return slot;
  }
  return null;
}

function findSlotInNode(node, path) {
  if (!node?.slots) return null;
  for (const slot of node.slots) {
    if (slot.path === path) return slot;
    for (const item of slot.items || []) {
      const nested = findSlotInNode(item, path);
      if (nested) return nested;
    }
  }
  return null;
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

function escapeHtml(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function confirmIfDirty() {
  const hasContent =
    state.document.plugin?.use &&
    (Object.keys(state.document.plugin.deps || {}).length > 0 ||
      Object.keys(state.document.shared || {}).length > 0 ||
      Object.keys(state.document.plugin.config || {}).length > 0);
  if (!hasContent) return true;
  return confirm("当前文档将被替换，确定继续？");
}

document.getElementById("btnNew").addEventListener("click", async () => {
  if (!confirmIfDirty()) return;
  await edit({ type: "newRoot", kind: els.rootKind.value });
  state.selectedPath = "root";
});

document.getElementById("btnBuild").addEventListener("click", () => runBuild().catch(alert));

document.getElementById("btnExport").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(state.yaml);
  } catch {
    alert("复制失败");
  }
});

document.getElementById("btnImport").addEventListener("click", () => {
  els.yamlInput.value = state.yaml;
  els.importDialog.showModal();
});

els.importForm.addEventListener("submit", async (e) => {
  if (e.submitter?.id !== "importConfirm") return;
  if (!confirmIfDirty()) return;
  try {
    await edit({ type: "importYAML", yaml: els.yamlInput.value });
    state.selectedPath = "root";
  } catch (err) {
    alert(err.message);
  }
});

els.rootId.addEventListener("change", async () => {
  const id = els.rootId.value.trim();
  if (!id || id === state.document.rootId) return;
  try {
    await edit({ type: "setRootId", id });
  } catch (err) {
    alert(err.message);
    els.rootId.value = state.document.rootId;
  }
});

els.rootKind.addEventListener("change", async () => {
  if (!confirmIfDirty()) {
    els.rootKind.value = state.document.plugin?.use || "";
    return;
  }
  await edit({ type: "newRoot", kind: els.rootKind.value });
  state.selectedPath = "root";
});

els.issueCount.addEventListener("click", () => {
  const first = activeDiagnostics().find((d) => d.severity === "error");
  if (first) {
    state.selectedPath = first.path;
    render();
  }
});

async function boot() {
  const data = await api("/api/catalog");
  state.catalog = data.kinds || [];
  fillRootKindOptions();
  const kind = state.catalog.some((k) => k.kind === "agent")
    ? "agent"
    : state.catalog[0]?.kind;
  if (kind) {
    await edit({ type: "newRoot", kind });
    state.selectedPath = "root";
  } else {
    render();
  }
}

boot().catch((err) => {
  els.tree.innerHTML = `<p class="empty-hint">${escapeHtml(err.message)}</p>`;
});
