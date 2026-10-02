package pluginkit

// Defaulter 是插件 Config 类型可选实现的接口。
// build 在 decode 之后、Validate 之前调用 SetDefaults 填充默认值。
// SetDefaults 不返回 error，不接收 ctx，不访问外部系统。
//
// 约定：零值视为未配置。需要区分「显式零值」和「未填」时，
// 插件应使用指针字段（如 *int）。
//
// 注意：bool 字段不要用 SetDefaults 设 true 默认值——用户显式关闭
// （false）与未填同为零值，会被默认值覆盖，导致无法关闭。需要默认
// 开启的开关请使用 *bool，nil 表示未填。
//
// SetDefaults / Validate 必须是纯函数：不访问外部系统、不 panic，
// panic 会沿调用栈传播（Build / ValidatePlan / Describe 均不 recover）。
type Defaulter interface {
	SetDefaults()
}

// Validator 是插件 Config 类型可选实现的接口。
// build 在 SetDefaults 之后、New 之前调用 Validate，
// 返回的 error 会作为 validate 阶段的装配错误响亮报出。
type Validator interface {
	Validate() error
}
