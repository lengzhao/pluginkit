const state = {
  catalog: [],
  catalogByKind: {},
  document: { rootId: "", plugin: { use: "" }, shared: {} },
  view: { root: null, shared: [] },
  diagnostics: [],
  yaml: "",
  selectedPath: "root",
  buildDiagnostics: null,
  allowIfaceMatch: false,
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
  importFile: document.getElementById("importFile"),
  toast: document.getElementById("toast"),
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

async function applyEditResponse(data) {
  state.document = data.document;
  state.view = data.view;
  state.diagnostics = data.diagnostics || [];
  state.yaml = data.yaml || "";
  state.buildDiagnostics = null;
  els.rootId.value = state.document.rootId || "";
  if (state.document.plugin?.use) {
    els.rootKind.value = state.document.plugin.use;
  }
}

function emitHostEvent(reason, data, operation = "") {
  const evt = {
    reason,
    operation,
    document: data.document,
    yaml: data.yaml,
    diagnostics: data.diagnostics || [],
  };
  if (reason === "build") {
    window.pluginkitManager.onBuild?.(evt);
  } else {
    window.pluginkitManager.onChange?.(evt);
  }
  if (window.parent !== window) {
    window.parent.postMessage({ type: "pluginkit:document", ...evt }, "*");
  }
}

async function edit(op) {
  const data = await api("/api/edit", {
    method: "POST",
    body: JSON.stringify({ document: state.document, op }),
  });
  await applyEditResponse(data);
  emitHostEvent("edit", data, op.type);
  // setConfig 时保留检查器表单，避免输入过程中重建 DOM 导致光标丢失
  const skipInspector = op.type === "setConfig";
  render({ skipInspector });
  if (skipInspector) {
    refreshInspectorDiagnostics();
  }
}

async function runBuild() {
  const btn = document.getElementById("btnBuild");
  btn.disabled = true;
  btn.textContent = "装配中…";
  try {
    const data = await api("/api/build", {
      method: "POST",
      body: JSON.stringify({ document: state.document }),
    });
    state.buildDiagnostics = data.diagnostics || [];
    render();
    emitHostEvent("build", data);
    const n = errorCount();
    if (n === 0) {
      showToast("试装配成功", "ok");
    } else {
      showToast(`试装配发现 ${n} 个问题`, "warn");
      const first = activeDiagnostics().find((d) => d.severity === "error");
      if (first) {
        state.selectedPath = first.path;
        render();
      }
    }
  } finally {
    btn.disabled = false;
    btn.textContent = "试装配";
  }
}

let toastTimer;
function showToast(message, tone = "info") {
  const el = els.toast;
  el.textContent = message;
  el.className = `toast toast-${tone}`;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el.hidden = true;
  }, 2800);
}

async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (_) {
      // fall through to execCommand
    }
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.left = "-9999px";
  document.body.appendChild(ta);
  ta.select();
  try {
    return document.execCommand("copy");
  } finally {
    document.body.removeChild(ta);
  }
}

function exportFileName() {
  const id = (state.document.rootId || "config").trim();
  return `${id || "config"}.yaml`;
}

function downloadYAML(yaml = state.yaml, filename = exportFileName()) {
  const blob = new Blob([yaml], { type: "text/yaml;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function render(options = {}) {
  renderIssueCount();
  renderTree();
  if (!options.skipInspector) {
    renderInspector();
  }
  els.yamlPreview.textContent = state.yaml;
}

function getInspectorFocusState() {
  const body = els.inspectorBody;
  const active = document.activeElement;
  if (!active || !body.contains(active) || active.tagName !== "INPUT") {
    return null;
  }
  const field = active.dataset.field;
  if (!field) return null;
  return {
    field,
    selectionStart: active.selectionStart,
    selectionEnd: active.selectionEnd,
  };
}

function restoreInspectorFocus(focus) {
  if (!focus) return;
  const input = els.inspectorBody.querySelector(
    `input[data-field="${CSS.escape(focus.field)}"]`
  );
  if (!input) return;
  input.focus();
  if (focus.selectionStart != null && focus.selectionEnd != null) {
    try {
      input.setSelectionRange(focus.selectionStart, focus.selectionEnd);
    } catch (_) {
      // 部分 input 类型不支持 selection
    }
  }
}

function refreshInspectorDiagnostics() {
  const body = els.inspectorBody;
  body.querySelector(".diag-section")?.remove();
  const diags = diagsForPath(state.selectedPath);
  appendDiagnostics(body, diags);
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
  wrap.className = `slot${slot.status === "empty" ? " empty" : ""}${state.selectedPath === slot.path ? " selected" : ""}${hasError(slot.path) ? " has-error" : ""}`;
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
  const focus = getInspectorFocusState();
  body.innerHTML = "";

  if (path === "root" || !path) {
    renderRootOverview(body, diags);
  } else {
    const node = findViewNode(path);
    const slot = findViewSlot(path);

    if (slot && (slot.status === "empty" || slot.list)) {
      renderSlotFillInspector(body, slot, diags);
    } else if (node) {
      renderNodeInspector(body, node, diags);
    } else {
      body.innerHTML = `<p class="empty-hint">选中路径：${escapeHtml(path)}</p>`;
      appendDiagnostics(body, diags);
    }
  }

  restoreInspectorFocus(focus);
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

function returnTypeLabel(kind) {
  return state.catalogByKind[kind]?.returnType || "";
}

function visibleKindCandidates(candidates) {
  if (!candidates?.length) return [];
  if (state.allowIfaceMatch) return candidates;
  return candidates.filter((c) => c.exact);
}

function visibleRefs(refs, candidates) {
  const allowed = new Set(visibleKindCandidates(candidates).map((c) => c.kind));
  return (refs || []).filter((r) => allowed.has(r.kind));
}

function renderSlotFillInspector(body, slot, diags) {
  const append = slot.status === "filled";
  body.innerHTML = `
    <div class="inspector-title">${append ? "添加项" : "填槽"}：${escapeHtml(slot.name)}</div>
    <div class="inspector-sub">${escapeHtml(slot.path)} · 需要 ${escapeHtml(slot.type || "")}</div>
  `;

  const matchToggle = document.createElement("label");
  matchToggle.className = "match-toggle";
  matchToggle.innerHTML = `<input type="checkbox"${state.allowIfaceMatch ? " checked" : ""}> 允许接口匹配`;
  matchToggle.querySelector("input").addEventListener("change", (e) => {
    state.allowIfaceMatch = e.target.checked;
    renderInspector();
  });
  body.appendChild(matchToggle);

  const refs = visibleRefs(slot.refs, slot.kinds);
  if (refs.length) {
    const h = document.createElement("h3");
    h.textContent = "引用已有";
    body.appendChild(h);
    const refList = document.createElement("ul");
    refList.className = "candidate-list";
    for (const ref of refs) {
      const li = document.createElement("li");
      li.className = "candidate-item";
      const returnType = returnTypeLabel(ref.kind);
      li.innerHTML = `<div class="kind">${escapeHtml(ref.id)}</div><div class="meta">${escapeHtml(ref.kind)}${returnType ? ` · ${escapeHtml(returnType)}` : ""} · 引用共享</div>`;
      li.addEventListener("click", () => edit({ type: "attachRef", path: slot.path, refId: ref.id }));
      refList.appendChild(li);
    }
    body.appendChild(refList);
  }

  const candidates = visibleKindCandidates(slot.kinds);
  const h2 = document.createElement("h3");
  h2.textContent = "新建内联";
  body.appendChild(h2);
  if (!candidates.length) {
    const hint = document.createElement("p");
    hint.className = "empty-hint";
    hint.textContent = state.allowIfaceMatch
      ? "没有可用插件"
      : "没有精确匹配的插件，可开启「允许接口匹配」查看更多";
    body.appendChild(hint);
  } else {
    const kinds = document.createElement("ul");
    kinds.className = "candidate-list";
    for (const candidate of candidates) {
      const li = document.createElement("li");
      li.className = `candidate-item${candidate.exact ? "" : " iface-match"}`;
      const matchHint = candidate.exact ? "" : " · 接口匹配";
      li.innerHTML = `<div class="kind">${escapeHtml(candidate.kind)}</div><div class="meta">${escapeHtml(candidate.returnType)}${matchHint}</div>`;
      li.addEventListener("click", () => edit({ type: "attach", path: slot.path, kind: candidate.kind }));
      kinds.appendChild(li);
    }
    body.appendChild(kinds);
  }
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
  const section = document.createElement("div");
  section.className = "diag-section";
  const h = document.createElement("h3");
  h.textContent = "问题";
  section.appendChild(h);
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
  section.appendChild(list);
  body.appendChild(section);
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

document.getElementById("btnBuild").addEventListener("click", () => {
  runBuild().catch((err) => {
    showToast(err.message || "试装配失败", "err");
  });
});

async function importYAMLText(yaml, sourceLabel = "YAML") {
  const ok = await loadYAML(yaml);
  if (ok) {
    els.importDialog.close();
    showToast(`已导入 ${sourceLabel}`, "ok");
  }
  return ok;
}

async function importFromFile(file) {
  if (!file) return false;
  try {
    return await importYAMLText(await file.text(), file.name);
  } catch (err) {
    showToast(err.message, "err");
    return false;
  }
}

document.getElementById("btnExport").addEventListener("click", () => {
  if (!state.yaml?.trim()) {
    showToast("没有可导出的 YAML", "warn");
    return;
  }
  downloadYAML();
  document.querySelector(".yaml-fold")?.setAttribute("open", "");
  showToast(`已下载 ${exportFileName()}`, "ok");
});

document.getElementById("btnImport").addEventListener("click", () => {
  els.yamlInput.value = state.yaml;
  els.importDialog.showModal();
});

document.getElementById("importPickFile").addEventListener("click", () => {
  els.importFile.click();
});

els.importFile.addEventListener("change", async () => {
  const file = els.importFile.files?.[0];
  els.importFile.value = "";
  await importFromFile(file);
});

els.importForm.addEventListener("submit", async (e) => {
  if (e.submitter?.id !== "importConfirm") return;
  try {
    await importYAMLText(els.yamlInput.value);
  } catch (err) {
    showToast(err.message, "err");
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

async function loadYAML(yaml, { confirmReplace = true } = {}) {
  if (confirmReplace && !confirmIfDirty()) {
    return false;
  }
  const data = await api("/api/load", {
    method: "POST",
    body: JSON.stringify({ yaml }),
  });
  await applyEditResponse(data);
  emitHostEvent("load", data, "importYAML");
  state.selectedPath = "root";
  render();
  return true;
}

async function boot() {
  const data = await api("/api/bootstrap");
  state.catalog = data.kinds || [];
  state.catalogByKind = Object.fromEntries(
    state.catalog.map((item) => [item.kind, item])
  );
  fillRootKindOptions();
  if (data.document?.plugin?.use) {
    await applyEditResponse(data);
    state.selectedPath = "root";
    render();
    return;
  }
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

window.pluginkitManager = {
  loadYAML,
  getYAML: () => state.yaml,
  getDocument: () => state.document,
  getDiagnostics: () => activeDiagnostics(),
  onChange: null,
  onBuild: null,
};

boot().catch((err) => {
  els.tree.innerHTML = `<p class="empty-hint">${escapeHtml(err.message)}</p>`;
});
