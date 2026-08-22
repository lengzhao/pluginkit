const state = {
  catalog: [],
  catalogByKind: {},
  document: { rootId: "", plugin: { use: "" }, shared: {} },
  view: { root: null, shared: [] },
  diagnostics: [],
  yaml: "",
  selectedPath: "root",
  navFrom: null,
  navCollapsed: localStorage.getItem("pluginkit:navCollapsed") === "1",
  buildDiagnostics: null,
  allowIfaceMatch: false,
};

const els = {
  layout: document.getElementById("layout"),
  rootId: document.getElementById("rootId"),
  rootKind: document.getElementById("rootKind"),
  issueCount: document.getElementById("issueCount"),
  navigator: document.getElementById("navigator"),
  navBody: document.getElementById("navBody"),
  navCollapse: document.getElementById("navCollapse"),
  navExpand: document.getElementById("navExpand"),
  canvasBody: document.getElementById("canvasBody"),
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
        state.navFrom = null;
        selectPath(first.path);
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
  applyNavCollapsed();
  renderIssueCount();
  renderNavigator();
  renderCanvas();
  if (!options.skipInspector) {
    renderInspector();
  }
  els.yamlPreview.textContent = state.yaml;
}

function applyNavCollapsed() {
  els.layout.classList.toggle("nav-collapsed", state.navCollapsed);
  els.navExpand.hidden = !state.navCollapsed;
}

function setNavCollapsed(collapsed) {
  state.navCollapsed = collapsed;
  localStorage.setItem("pluginkit:navCollapsed", collapsed ? "1" : "0");
  applyNavCollapsed();
}

function resolveSharedDefinitionPath(path) {
  if (!path?.startsWith("shared.")) return null;
  const shared = state.view?.shared || [];
  const exact = shared.find((s) => s.path === path);
  if (exact) return exact.path;
  let best = null;
  for (const s of shared) {
    if (path.startsWith(s.path + ".") || path.startsWith(s.path + "[")) {
      if (!best || s.path.length > best.length) best = s.path;
    }
  }
  return best;
}

function navInstancePath(path) {
  if (!path || path === "root" || path.startsWith("root.")) return "root";
  return resolveSharedDefinitionPath(path) || "root";
}

function sharedTargetPath(refId) {
  return `shared.${refId}`;
}

function isRefTargetSelected(item) {
  const target = sharedTargetPath(item.refId);
  return state.selectedPath === item.path || state.selectedPath === target;
}

function subtreeHasError(path) {
  return activeDiagnostics().some(
    (d) =>
      d.severity === "error" &&
      (d.path === path || d.path.startsWith(path + ".") || d.path.startsWith(path + "["))
  );
}

function renderNavigator() {
  const body = els.navBody;
  body.innerHTML = "";
  if (!state.view?.root) {
    body.innerHTML = `<p class="empty-hint">正在加载…</p>`;
    return;
  }

  const activeNav = navInstancePath(state.selectedPath);

  const assembly = document.createElement("div");
  assembly.className = "nav-group";
  assembly.innerHTML = `<div class="nav-group-title">装配</div>`;
  assembly.appendChild(
    renderNavItem({
      path: "root",
      id: state.document.rootId || "root",
      kind: state.view.root.kind || state.document.plugin?.use || "",
      selected: activeNav === "root",
    })
  );
  body.appendChild(assembly);

  const shared = state.view.shared || [];
  if (shared.length) {
    const group = document.createElement("div");
    group.className = "nav-group";
    group.innerHTML = `<div class="nav-group-title">共享实例 ${shared.length}</div>`;
    for (const node of shared) {
      const id = node.path.slice("shared.".length);
      group.appendChild(
        renderNavItem({
          path: node.path,
          id,
          kind: node.kind || "",
          refCount: node.refCount ?? 0,
          selected: activeNav === node.path,
        })
      );
    }
    body.appendChild(group);
  }
}

function renderNavItem({ path, id, kind, refCount = 0, selected }) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = `nav-item${selected ? " selected" : ""}${subtreeHasError(path) ? " has-error" : ""}`;
  btn.innerHTML = `
    <div class="nav-item-main">
      <div class="nav-item-id">${escapeHtml(id)}</div>
      <div class="nav-item-kind">${escapeHtml(kind)}</div>
    </div>
    <div class="nav-item-badges"></div>
  `;
  const badges = btn.querySelector(".nav-item-badges");
  if (path.startsWith("shared.") && refCount > 0) {
    const badge = document.createElement("span");
    badge.className = "nav-badge";
    badge.textContent = `×${refCount}`;
    badges.appendChild(badge);
  }
  if (subtreeHasError(path)) {
    const badge = document.createElement("span");
    badge.className = "nav-badge err";
    badge.textContent = "!";
    badges.appendChild(badge);
  }
  btn.addEventListener("click", () => {
    if (path === "root") {
      selectPath(path);
      return;
    }
    const from = state.selectedPath;
    selectPath(path, { navFrom: from && from !== path ? from : null });
  });
  return btn;
}

function renderCanvas() {
  const body = els.canvasBody;
  body.innerHTML = "";
  if (!state.view?.root) {
    body.innerHTML = `<p class="empty-hint">正在加载…</p>`;
    return;
  }

  const canvasTarget = navInstancePath(state.selectedPath);
  if (canvasTarget === "root") {
    renderAssemblyCanvas(body);
    return;
  }

  const node = findSharedViewNode(canvasTarget);
  if (node) renderInstanceCanvas(body, node);
  else renderAssemblyCanvas(body);
}

function findSharedViewNode(path) {
  return state.view?.shared?.find((s) => s.path === path) ?? null;
}

function renderAssemblyCanvas(body) {
  const label = document.createElement("div");
  label.className = "canvas-mode-label";
  label.textContent = "装配结构";
  body.appendChild(label);
  const tree = document.createElement("div");
  tree.className = "tree";
  tree.appendChild(renderNodeCard(state.view.root, true));
  body.appendChild(tree);
}

function renderInstanceCanvas(body, node) {
  const label = document.createElement("div");
  label.className = "canvas-mode-label";
  label.textContent = "实例";
  body.appendChild(label);

  const id = node.path.slice("shared.".length);
  const header = document.createElement("div");
  header.className = "instance-header";
  header.innerHTML = `
    <div class="instance-title">${escapeHtml(id)}</div>
    <div class="instance-meta">${escapeHtml(node.kind || "")} · 引用 ${node.refCount ?? 0} 处</div>
  `;
  body.appendChild(header);

  if (node.slots?.length) {
    const slots = document.createElement("div");
    slots.className = "slots";
    for (const slot of node.slots) {
      slots.appendChild(renderSlot(slot));
    }
    body.appendChild(slots);
  } else {
    const hint = document.createElement("p");
    hint.className = "empty-hint";
    hint.textContent = "该实例没有依赖槽位，在右侧检查器编辑 config。";
    body.appendChild(hint);
  }
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
    state.navFrom = null;
    selectPath(node.path);
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
      state.navFrom = null;
      selectPath(slot.path);
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
      state.navFrom = null;
      selectPath(slot.path);
    });
    items.appendChild(add);
  }
  wrap.appendChild(items);
  return wrap;
}

function renderItem(item, slot) {
  const row = document.createElement("div");
  row.className = "slot-item";
  row.dataset.path = item.path;

  const main = document.createElement("div");
  main.className = "slot-item-main";

  if (item.role === "ref") {
    const chip = document.createElement("div");
    chip.className = `ref-chip${isRefTargetSelected(item) ? " selected" : ""}`;
    chip.dataset.path = item.path;
    chip.dataset.refTarget = sharedTargetPath(item.refId);
    chip.textContent = `→ ${item.refId} (${item.kind || "?"})`;
    chip.addEventListener("click", (e) => {
      e.stopPropagation();
      enterSharedRef(item);
    });
    main.append(chip, createItemRemoveButton(item));
    row.appendChild(main);
    return row;
  }

  const link = document.createElement("div");
  link.className = `inline-link${state.selectedPath === item.path ? " selected" : ""}`;
  link.dataset.path = item.path;
  link.innerHTML = `<div class="node-kind">${escapeHtml(item.kind)}</div><div class="node-path">${escapeHtml(item.path)}</div>`;
  link.addEventListener("click", (e) => {
    e.stopPropagation();
    state.navFrom = null;
    selectPath(item.path);
  });
  main.append(link, createItemRemoveButton(item));
  row.appendChild(main);

  if (item.slots?.length) {
    const nested = document.createElement("div");
    nested.className = "slots";
    nested.style.marginTop = "8px";
    for (const s of item.slots) {
      nested.appendChild(renderSlot(s));
    }
    row.appendChild(nested);
  }
  return row;
}

function createItemRemoveButton(item) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "slot-item-remove";
  btn.title = item.role === "ref" ? "解除引用" : "删除";
  btn.setAttribute("aria-label", btn.title);
  btn.textContent = "×";
  btn.addEventListener("click", (e) => {
    e.stopPropagation();
    edit({ type: "remove", path: item.path });
  });
  return btn;
}

function collectRefSites(refId) {
  const sites = [];
  const visit = (node) => {
    if (!node) return;
    if (node.role === "ref" && node.refId === refId) sites.push(node.path);
    for (const slot of node.slots || []) {
      for (const item of slot.items || []) visit(item);
    }
  };
  visit(state.view?.root);
  for (const s of state.view?.shared || []) visit(s);
  return sites;
}

function selectPath(path, { navFrom = null } = {}) {
  state.selectedPath = path;
  state.navFrom = navFrom;
  render();
  scrollToSelected(path);
}

function enterSharedRef(item) {
  state.navFrom = null;
  selectPath(sharedTargetPath(item.refId));
}

function scrollToSelected(path) {
  requestAnimationFrame(() => {
    const el = els.canvasBody.querySelector(`[data-path="${CSS.escape(path)}"]`);
    el?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  });
}

function prependInspectorNav(body, path) {
  if (!state.navFrom || state.navFrom === path) return;

  const nav = document.createElement("div");
  nav.className = "inspector-nav";
  const back = document.createElement("button");
  back.type = "button";
  back.className = "nav-back";
  back.textContent = `← 返回 ${shortPathLabel(state.navFrom)}`;
  back.addEventListener("click", () => {
    const from = state.navFrom;
    state.navFrom = null;
    selectPath(from);
  });
  nav.appendChild(back);
  body.insertBefore(nav, body.firstChild);
}

function shortPathLabel(path) {
  if (path === "root") return "Root";
  if (path.startsWith("shared.")) {
    const rest = path.slice("shared.".length);
    const dot = rest.indexOf(".");
    return dot === -1 ? `共享 ${rest}` : `共享 ${rest.slice(0, dot)} · ${rest.slice(dot + 1)}`;
  }
  const deps = path.indexOf(".deps.");
  if (deps === -1) return path;
  return path.slice(deps + ".deps.".length);
}

function appendRefSites(body, refId) {
  const sites = collectRefSites(refId);
  if (!sites.length) return;
  const h = document.createElement("h3");
  h.textContent = "被引用于";
  body.appendChild(h);
  const list = document.createElement("ul");
  list.className = "candidate-list";
  for (const site of sites) {
    const li = document.createElement("li");
    li.className = "candidate-item";
    li.innerHTML = `<div class="kind">${escapeHtml(shortPathLabel(site))}</div><div class="meta">${escapeHtml(site)}</div>`;
    li.addEventListener("click", () => {
      state.navFrom = null;
      selectPath(site);
    });
    list.appendChild(li);
  }
  body.appendChild(list);
}

function renderInspector() {
  const path = state.selectedPath;
  const diags = diagsForPath(path);
  const body = els.inspectorBody;
  const focus = getInspectorFocusState();
  body.innerHTML = "";

  if (path === "root" || !path) {
    const node = state.view?.root;
    if (node) {
      renderNodeInspector(body, node, diags);
    } else {
      body.innerHTML = `<p class="empty-hint">无文档</p>`;
      appendDiagnostics(body, diags);
    }
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
    prependInspectorNav(body, path);
  }

  restoreInspectorFocus(focus);
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
      selectPath(`shared.${node.refId}`, { navFrom: node.path });
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

  if (node.role === "shared") {
    appendRefSites(body, node.path.slice("shared.".length).split(".")[0]);
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
      state.navFrom = null;
      selectPath(d.path);
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
  state.navFrom = null;
  state.selectedPath = "root";
  render();
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
  state.navFrom = null;
  state.selectedPath = "root";
  render();
});

els.issueCount.addEventListener("click", () => {
  const first = activeDiagnostics().find((d) => d.severity === "error");
  if (first) {
    state.navFrom = null;
    selectPath(first.path);
  }
});

els.navCollapse.addEventListener("click", () => setNavCollapsed(true));
els.navExpand.addEventListener("click", () => setNavCollapsed(false));

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
  state.navFrom = null;
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
    state.navFrom = null;
    state.selectedPath = "root";
    render();
    return;
  }
  const kind = state.catalog.some((k) => k.kind === "agent")
    ? "agent"
    : state.catalog[0]?.kind;
  if (kind) {
    await edit({ type: "newRoot", kind });
    state.navFrom = null;
    state.selectedPath = "root";
    render();
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
  els.canvasBody.innerHTML = `<p class="empty-hint">${escapeHtml(err.message)}</p>`;
});
