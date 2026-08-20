package build

import "fmt"

// Stage 表示装配失败发生的阶段。
type Stage string

const (
	StageResolve   Stage = "resolve"
	StageDecode    Stage = "decode"
	StageDeps      Stage = "deps"
	StageConstruct Stage = "construct"
	StageTypeCheck Stage = "typecheck"
)

// Error 是装配错误，包含字段名、插件 use、实例 id 与阶段。
type Error struct {
	Field string
	Use   string
	ID    string
	Stage Stage
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	msg := "unknown error"
	if e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("pluginkit %s: field=%q use=%q id=%q: %s", e.Stage, e.Field, e.Use, e.ID, msg)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func assembleErr(field, use, id string, stage Stage, err error) *Error {
	return &Error{Field: field, Use: use, ID: id, Stage: stage, Err: err}
}
