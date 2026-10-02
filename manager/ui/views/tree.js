// views/tree.js —— 树状模式：缩进层级 + 折叠/展开 + 诊断聚合徽标。
// 与列表模式渲染同一份 view 投影，共享 selectedPath；折叠/展开状态是纯 UI 态，
// 按 path 持久化到 localStorage，不改变文档。
// 共享引用（→ id）可就地展开被引实例的子树；refChain 记录祖先引用链，
// 出现循环引用（A → B → A）时停止展开并标注。
import {
  state,
  edit,
  selectPath,
  hasError,
  subtreeHasError,
  escapeHtml,
  sharedTargetPath,
  registerCanvasMode,
  renderCanvas,
} from "../core.js";

const collapsed = new Set(
  JSON.parse(localStorage.getItem("pluginkit:treeCollapsed") || "[]")
);
const expandedRefs = new Set(
  JSON.parse(localStorage.getItem("pluginkit:treeRefExpanded") || "[]")
);

function isCollapsed(path) {
  return collapsed.has(path);
}

function toggleCollapsed(path) {
  if (collapsed.has(path)) {
    collapsed.delete(path);
  } else {
    collapsed.add(path);
  }
  persist("pluginkit:treeCollapsed", collapsed);
}

function toggleRefExpanded(path) {
  if (expandedRefs.has(path)) {
    expandedRefs.delete(path);
  } else {
    expandedRefs.add(path);
  }
  persist("pluginkit:treeRefExpanded", expandedRefs);
}

function persist(key, set) {
  localStorage.setItem(key, JSON.stringify([...set]));
}

// isInside 判断 path 是否位于 ancestor 的子树内（含自身）。
function isInside(path, ancestor) {
  return path === ancestor || path.startsWith(ancestor + ".") || path.startsWith(ancestor + "[");
}

function findShared(refId) {
  return state.view?.shared?.find((s) => s.path === sharedTargetPath(refId)) ?? null;
}

function renderAssembly(body) {
  const label = document.createElement("div");
  label.className = "canvas-mode-label";
  label.textContent = "装配结构";
  body.appendChild(label);
  const tree = document.createElement("div");
  tree.className = "treeview";
  appendNode(tree, state.view.root, 0, [], true);
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
    const tree = document.createElement("div");
    tree.className = "treeview";
    appendSlots(tree, node, 0, [id]);
    body.appendChild(tree);
  } else {
    const hint = document.createElement("p");
    hint.className = "empty-hint";
    hint.textContent = "该实例没有依赖槽位，在右侧检查器编辑 config。";
    body.appendChild(hint);
  }
}

// appendNode 渲染一个插件节点行；未折叠时递归渲染其槽位与子节点。
// refChain 是祖先链上已展开的共享实例 id，用于环检测。
function appendNode(container, node, depth, refChain, isRoot = false) {
  const hasChildren = (node.slots || []).length > 0;
  // 选中路径位于折叠子树内时强制展开，保证「返回」后目标可见。
  let folded = isCollapsed(node.path);
  if (folded && isInside(state.selectedPath, node.path) && state.selectedPath !== node.path) {
    collapsed.delete(node.path);
    persist("pluginkit:treeCollapsed", collapsed);
    folded = false;
  }

  const row = makeRow(node.path, depth, {
    selected: state.selectedPath === node.path,
    error: hasError(node.path),
    kind: "node",
  });

  row.appendChild(makeToggle({
    path: node.path,
    expandable: hasChildren,
    folded,
    onToggle: () => toggleCollapsed(node.path),
  }));

  const main = document.createElement("div");
  main.className = "tv-main";
  main.innerHTML = `
    <span class="tv-kind">${escapeHtml(node.kind || node.role)}</span>
    <span class="tv-path">${escapeHtml(isRoot ? "root" : node.path)}</span>
  `;
  row.appendChild(main);

  // 折叠时聚合子树错误与空槽数，让问题在折叠状态下仍可见。
  if (folded) {
    if (subtreeHasError(node.path)) {
      row.appendChild(makeBadge("!", "err"));
    }
    const empty = countEmptySlots(node);
    if (empty > 0) {
      row.appendChild(makeBadge(`${empty} 空槽`, "warn"));
    }
  }

  // 非 root 节点对应某个槽位项，与列表模式一致提供删除。
  if (!isRoot) {
    row.appendChild(makeRemoveButton(node.path, "删除"));
  }

  row.addEventListener("click", () => {
    state.navFrom = null;
    selectPath(node.path);
  });
  container.appendChild(row);

  if (hasChildren && !folded) {
    appendSlots(container, node, depth, refChain);
  }
}

function appendSlots(container, node, depth, refChain) {
  for (const slot of node.slots || []) {
    appendSlot(container, slot, depth + 1);
    if (slot.status === "empty") continue;
    for (const item of slot.items || []) {
      if (item.role === "ref") {
        appendRef(container, item, depth + 2, refChain);
      } else {
        appendNode(container, item, depth + 2, refChain);
      }
    }
  }
}

function appendSlot(container, slot, depth) {
  const row = makeRow(slot.path, depth, {
    selected: state.selectedPath === slot.path,
    error: hasError(slot.path),
    kind: `slot${slot.status === "empty" ? " empty" : ""}`,
  });

  row.appendChild(makeToggle({ expandable: false }));

  const main = document.createElement("div");
  main.className = "tv-main";
  const meta = `${slot.list ? "[]" : ""}${slot.type || ""}${slot.optional ? " · optional" : ""}`;
  const status = slot.status === "empty" ? `<span class="tv-empty">未配置</span>` : "";
  main.innerHTML = `
    <span class="tv-slot-name">${escapeHtml(slot.name)}</span>
    <span class="tv-path">${escapeHtml(meta)}</span>
    ${status}
  `;
  row.appendChild(main);

  row.addEventListener("click", () => {
    state.navFrom = null;
    selectPath(slot.path);
  });
  container.appendChild(row);
}

// appendRef 渲染共享引用行。引用可就地展开被引实例的子树（行内路径用
// 被引实例的真实 path，即 shared.<id>.*，选中/检查器/诊断直接生效）；
// 引用链成环时禁止展开并标注「循环引用」。
function appendRef(container, item, depth, refChain) {
  const target = sharedTargetPath(item.refId);
  const shared = findShared(item.refId);
  const cyclic = refChain.includes(item.refId);
  // 选中路径位于被引实例内时自动展开，保证返回/跳转后目标可见。
  let expanded = expandedRefs.has(item.path);
  if (!expanded && !cyclic && shared && isInside(state.selectedPath, target)) {
    expandedRefs.add(item.path);
    persist("pluginkit:treeRefExpanded", expandedRefs);
    expanded = true;
  }

  const row = makeRow(item.path, depth, {
    selected: state.selectedPath === item.path || state.selectedPath === target,
    error: hasError(item.path),
    kind: "ref",
  });
  row.dataset.refTarget = target;

  row.appendChild(makeToggle({
    path: item.path,
    expandable: !!shared && !cyclic && (shared.slots || []).length > 0,
    folded: !expanded,
    onToggle: () => toggleRefExpanded(item.path),
  }));

  const main = document.createElement("div");
  main.className = "tv-main";
  main.innerHTML = `
    <span class="tv-ref">→ ${escapeHtml(item.refId)}</span>
    <span class="tv-path">${escapeHtml(item.kind || "?")}</span>
  `;
  row.appendChild(main);

  if (cyclic) {
    row.appendChild(makeBadge("循环引用", "err"));
  } else if (!shared) {
    row.appendChild(makeBadge("未找到定义", "err"));
  }

  // × 解除该位置的引用（不删除共享定义），与列表模式语义一致。
  row.appendChild(makeRemoveButton(item.path, "解除引用"));

  row.addEventListener("click", () => {
    state.navFrom = null;
    selectPath(target);
  });
  container.appendChild(row);

  if (expanded && shared) {
    appendSlots(container, shared, depth, [...refChain, item.refId]);
  }
}

function makeRow(path, depth, { selected, error, kind }) {
  const row = document.createElement("div");
  row.className = `tv-row tv-${kind.split(" ")[0]}${selected ? " selected" : ""}${error ? " has-error" : ""}${kind.includes("empty") ? " empty" : ""}`;
  row.dataset.path = path;
  row.style.paddingLeft = `${depth * 20 + 6}px`;
  return row;
}

function makeToggle({ path, expandable, folded, onToggle }) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "tv-toggle";
  if (!expandable) {
    btn.textContent = "·";
    btn.disabled = true;
    btn.tabIndex = -1;
    return btn;
  }
  btn.textContent = folded ? "▸" : "▾";
  btn.title = folded ? "展开" : "折叠";
  btn.addEventListener("click", (e) => {
    e.stopPropagation();
    onToggle(path);
    // 折叠/展开是纯 UI 态，只重绘画布，不触发完整 render。
    renderCanvas();
  });
  return btn;
}

// makeRemoveButton 生成行尾 ×，悬停行时可见（CSS 控制）。
function makeRemoveButton(path, title) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "tv-remove";
  btn.title = title;
  btn.setAttribute("aria-label", title);
  btn.textContent = "×";
  btn.addEventListener("click", (e) => {
    e.stopPropagation();
    edit({ type: "remove", path });
  });
  return btn;
}

function makeBadge(text, tone) {
  const badge = document.createElement("span");
  badge.className = `tv-badge ${tone}`;
  badge.textContent = text;
  return badge;
}

function countEmptySlots(node) {
  let n = 0;
  const visit = (nd) => {
    for (const slot of nd.slots || []) {
      if (slot.status === "empty") n += 1;
      for (const item of slot.items || []) {
        if (item.role !== "ref") visit(item);
      }
    }
  };
  visit(node);
  return n;
}

registerCanvasMode("tree", {
  label: "树状",
  renderAssembly,
  renderInstance,
});
