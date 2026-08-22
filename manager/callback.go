package manager

import (
	"context"
	"log/slog"
)

// ChangeReason 描述文档变更来源。
type ChangeReason string

const (
	// ChangeEdit 表示用户通过 /api/edit 修改了文档。
	ChangeEdit ChangeReason = "edit"
	// ChangeLoad 表示通过 /api/load 或 importYAML 整表替换。
	ChangeLoad ChangeReason = "load"
	// ChangeBuild 表示用户触发了试装配。
	ChangeBuild ChangeReason = "build"
)

// DocumentEvent 是工作台状态变更事件，供宿主 OnChange / OnBuild 回调。
type DocumentEvent struct {
	Reason      ChangeReason `json:"reason"`
	Operation   string       `json:"operation,omitempty"`
	Document    Document     `json:"document"`
	YAML        string       `json:"yaml"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (s *server) notifyChange(ctx context.Context, reason ChangeReason, op string, resp editResponse) {
	evt := DocumentEvent{
		Reason:      reason,
		Operation:   op,
		Document:    resp.Document,
		YAML:        resp.YAML,
		Diagnostics: resp.Diagnostics,
	}
	var fn func(context.Context, DocumentEvent) error
	switch reason {
	case ChangeBuild:
		fn = s.onBuild
	default:
		fn = s.onChange
	}
	if fn == nil {
		return
	}
	if err := fn(ctx, evt); err != nil {
		slog.Warn("manager callback failed", "reason", reason, "operation", op, "err", err)
	}
}
