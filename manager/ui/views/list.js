// views/list.js —— 列表模式：嵌套卡片渲染（重构前的默认画布）。
import {
  state,
  edit,
  selectPath,
  hasError,
  escapeHtml,
  sharedTargetPath,
  isRefTargetSelected,
  registerCanvasMode,
} from "../core.js";

function renderAssembly(body) {
  const label = document.createElement("div");
  label.className = "canvas-mode-label";
  label.textContent = "装配结构";
  body.appendChild(label);
  const tree = document.createElement("div");
  tree.className = "tree";
  tree.appendChild(renderNodeCard(state.view.root, true));
  body.appendChild(tree);
}

function renderInstance(body, node) {
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

function enterSharedRef(item) {
  state.navFrom = null;
  selectPath(sharedTargetPath(item.refId));
}

registerCanvasMode("list", {
  label: "列表",
  renderAssembly,
  renderInstance,
});
