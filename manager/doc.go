// Package manager 提供 pluginkit 插件配置的工作台 Web UI 与 HTTP API。
//
// 宿主应用在 main 中 import 自己的插件包（触发 init Register），再启动 manager：
//
//	import "github.com/lengzhao/pluginkit/manager"
//
//	func main() {
//	    manager.Run(manager.Options{Addr: ":8080"})
//	}
//
// 工作台通过 GET /api/bootstrap 获取 catalog 与可选 InitialYAML；
// POST /api/load 或 POST /api/edit（importYAML）加载 YAML；
// POST /api/edit 应用其他编辑命令，返回 document + view + diagnostics + yaml。
// Options.OnChange / OnBuild 在 edit、load、build 成功后回调 DocumentEvent。
// 可选执行宿主 ValidateBuild 做试装配。
package manager
