package config

// PluginUse 描述如何选用一个已注册的插件类型。
type PluginUse struct {
	ID     string
	Use    string
	Config map[string]any
	Deps   map[string]any
}

// Field 是 plugins 下一个配置键的规范化结果。
type Field struct {
	Name string
	List bool
	Uses []PluginUse
}
