// Package build 在启动期按配置构造插件实例图。
//
// Build 按 root id 构造一个类型化根实例；BuildInto 按目标 struct 填充多个扩展点。
// 包内会先编译装配计划，再按依赖顺序执行构造。
// Collect 和 CollectInstances 可从 Result 中按类型筛选已构造实例。
// 运行期不应再调用本包；使用方只使用已经构造好的实例。
package build
