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
// 工作台通过 POST /api/edit 应用编辑命令（填槽、改 config、提取共享、导入 YAML），
// 返回 document + view + diagnostics + yaml。POST /api/build 在 structure/plan 通过后
// 可选执行宿主 ValidateBuild 做试装配。
//
// manager 读取当前进程注册表，不支持运行时加载未编译插件。
package manager
