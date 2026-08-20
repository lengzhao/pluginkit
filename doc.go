// Package pluginkit 只登记插件类型：kind 与构造函数。
//
// init() 里应只调用 Register，不读取配置、不构造实例。
// 按 kind 查看配置字段和依赖扩展点见 Describe；PluginDescription.Template 可导出可填写配置骨架。
// 配置识别见 [github.com/lengzhao/pluginkit/config]，
// 启动期实例化见 [github.com/lengzhao/pluginkit/build]。
package pluginkit
