// inspector.js —— 右栏检查器：config 表单、填槽候选、引用操作、诊断。
// 通过 registerInspectorRenderer 挂到 core 的 render 循环，三种画布模式共用。
import {
  state,
  els,
  edit,
  selectPath,
  diagsForPath,
  escapeHtml,
  shortPathLabel,
  collectRefSites,
  findViewNode,
  findViewSlot,
  registerInspectorRenderer,
} from "./core.js";

function getInspectorFocusState() {
  const body = els.inspectorBody;
  const active = document.activeElement;
  if (!active || !body.contains(active) || (active.tagName !== "INPUT" && active.tagName !== "TEXTAREA")) {
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
    `[data-field="${CSS.escape(focus.field)}"]`
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

// fieldControl 按字段 kind 渲染对应控件：bool → checkbox，number → 数字框，
// string → 文本框，其余（slice/map/struct 等）→ JSON textarea 兜底。
function fieldControl(field) {
  const kind = field.kind || "json";
  let input;
  if (kind === "bool") {
    input = document.createElement("input");
    input.type = "checkbox";
    input.checked = field.value === true;
  } else if (kind === "number") {
    input = document.createElement("input");
    input.type = "number";
    input.step = "any";
    input.value = field.value ?? "";
  } else if (kind === "string") {
    input = document.createElement("input");
    input.type = "text";
    input.value = field.value ?? "";
  } else {
    input = document.createElement("textarea");
    input.rows = 2;
    input.value = field.value === undefined || field.value === null ? "" : JSON.stringify(field.value);
  }
  input.dataset.field = field.name;
  input.dataset.kind = kind;
  return input;
}

// readFieldValue 把控件内容还原为 JSON 类型。
// skip 表示留空（不提交该字段，由插件 SetDefaults 兜底）；ok=false 表示输入非法。
function readFieldValue(el) {
  switch (el.dataset.kind) {
    case "bool":
      return { ok: true, value: el.checked };
    case "number": {
      if (el.value.trim() === "") return { ok: true, skip: true };
      const n = Number(el.value);
      return Number.isNaN(n) ? { ok: false } : { ok: true, value: n };
    }
    case "json": {
      const text = el.value.trim();
      if (text === "") return { ok: true, skip: true };
      try {
        return { ok: true, value: JSON.parse(text) };
      } catch {
        return { ok: false };
      }
    }
    default:
      return { ok: true, value: el.value };
  }
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
      const caption = document.createElement("span");
      caption.className = "field-caption";
      caption.textContent = `${field.name} (${field.type})${field.optional ? " · 可选" : ""}`;
      const input = fieldControl(field);
      let timer;
      input.addEventListener("input", () => {
        input.classList.remove("invalid");
        clearTimeout(timer);
        timer = setTimeout(() => {
          const cfg = {};
          let valid = true;
          for (const el of form.querySelectorAll("[data-field]")) {
            const r = readFieldValue(el);
            if (!r.ok) {
              el.classList.add("invalid");
              valid = false;
              continue;
            }
            if (!r.skip) cfg[el.dataset.field] = r.value;
          }
          if (!valid) return;
          edit({ type: "setConfig", path: node.path, config: cfg });
        }, 300);
      });
      label.append(caption, input);
      if (field.default !== undefined && field.default !== null && field.default !== "") {
        const hint = document.createElement("span");
        hint.className = "field-default";
        hint.textContent = `默认：${typeof field.default === "object" ? JSON.stringify(field.default) : field.default}`;
        label.appendChild(hint);
      }
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

registerInspectorRenderer(renderInspector, refreshInspectorDiagnostics);
