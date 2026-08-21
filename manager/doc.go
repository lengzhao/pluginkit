// Package manager 提供 pluginkit 插件配置的 Web 管理 UI 与 HTTP API。
//
// 宿主应用在 main 中 import 自己的插件包（触发 init Register），再启动 manager：
//
//	import "github.com/lengzhao/pluginkit/manager"
//
//	func main() {
//	    manager.Run(manager.Options{Addr: ":8080"})
//	}
//
// manager 读取当前进程注册表，不支持运行时加载未编译插件。
package manager
